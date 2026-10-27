// Package sweep runs one pass over a herdr session's agent panes and puts the
// quiet ones asleep: the agent is stopped with SIGTERM, the pane stays at its
// shell, is reported to herdr as asleep, and its prompt is prefilled with the
// command that resumes the same session.
package sweep

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// Herdr is the part of herdr's API a sweep uses.
type Herdr interface {
	Snapshot(ctx context.Context) ([]herdr.Agent, error)
	ProcessInfo(ctx context.Context, paneID string) (herdr.ProcessInfo, error)
	ReportAgent(ctx context.Context, r herdr.AgentReport) error
	ReportMetadata(ctx context.Context, r herdr.MetadataReport) error
	SendText(ctx context.Context, paneID, text string) error
}

// Signaler stops processes.
type Signaler interface {
	// Terminate sends SIGTERM to every process in the process group.
	Terminate(pgid int) error
}

// Sweeper holds what a sweep needs. The zero value is not usable: Run needs
// every field set, PaneClosed only StatePath.
type Sweeper struct {
	Herdr    Herdr
	Signaler Signaler
	// StatePath is the JSON file holding each pane's quiet clock and the
	// panes put asleep. A missing file starts every clock now.
	StatePath string
	// IdleAfter is how long a pane must stay quiet before it is put asleep.
	IdleAfter time.Duration
}

// Outcome is what a sweep decided for one agent pane.
type Outcome struct {
	PaneID string
	Agent  string
	Asleep bool   // put asleep by this sweep
	Reason string // why it was or was not put asleep
}

const (
	// source is the plugin id herdr-idle reports under.
	source = "dnzzl.idle"
	// standInKind is the kind an asleep pane is reported as. herdr drops a
	// report of the real kind once that agent has exited, and cannot resume
	// this one, so a herdr restart never wakes the pane.
	standInKind = "maki"
	// exitTimeout bounds the wait for an agent to exit after SIGTERM.
	exitTimeout = 30 * time.Second
	pollEvery   = 250 * time.Millisecond
	// reportSettle is how long a report is given to survive before it is
	// checked: herdr's process detector (300ms tick) wipes plugin reports
	// when it notices the agent exited, which can be after the shell is back.
	reportSettle = time.Second
)

// Run sweeps the session once. It returns one outcome per agent pane. A
// failure on one pane does not stop the others; their errors are joined.
func (s Sweeper) Run(ctx context.Context) ([]Outcome, error) {
	agents, err := s.Herdr.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	prev, err := loadState(s.StatePath)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	next := observe(prev, agents, now)

	var outcomes []Outcome
	var errs []error
	for _, a := range agents {
		o := Outcome{PaneID: a.PaneID, Agent: a.Agent}
		o.Reason = whyAwake(a, next[a.PaneID], now, s.IdleAfter)
		if o.Reason == "" {
			argv, reason, err := s.putAsleep(ctx, a)
			if err != nil {
				errs = append(errs, fmt.Errorf("pane %s: %w", a.PaneID, err))
				reason = "error: " + err.Error()
			}
			if argv != nil {
				ps := next[a.PaneID]
				ps.Kind, ps.SessionRef, ps.ResumeArgv, ps.AsleepAt = a.Agent, a.Session.Value, argv, time.Now()
				next[a.PaneID] = ps
				o.Asleep = true
			}
			o.Reason = reason
		}
		outcomes = append(outcomes, o)
	}
	// After a herdr restart our reports are gone: asleep panes are plain
	// shells herdr no longer lists. Report and prefill them again.
	listed := map[string]bool{}
	for _, a := range agents {
		listed[a.PaneID] = true
	}
	for _, id := range slices.Sorted(maps.Keys(next)) {
		ps := next[id]
		if ps.AsleepAt.IsZero() || listed[id] {
			continue
		}
		reason, err := s.rearm(ctx, id, ps)
		if err != nil {
			errs = append(errs, fmt.Errorf("pane %s: %w", id, err))
			reason = "error: " + err.Error()
		}
		outcomes = append(outcomes, Outcome{PaneID: id, Agent: ps.Kind, Reason: reason})
	}
	if err := saveState(s.StatePath, next); err != nil {
		errs = append(errs, err)
	}
	return outcomes, errors.Join(errs...)
}

// PaneClosed forgets a pane herdr has closed: its state entry and its capture
// file are removed. An unknown pane is not an error.
func (s Sweeper) PaneClosed(ctx context.Context, paneID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	st, err := loadState(s.StatePath)
	if err != nil {
		return err
	}
	ps, ok := st[paneID]
	if !ok {
		return nil
	}
	if ps.CapturePath != "" {
		if err := os.Remove(ps.CapturePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove capture: %w", err)
		}
	}
	delete(st, paneID)
	return saveState(s.StatePath, st)
}

// putAsleep stops the pane's agent and leaves its shell prefilled with the
// resume command. It returns the prefilled argv once the pane is asleep, or
// nil and the reason it left the pane alone.
func (s Sweeper) putAsleep(ctx context.Context, a herdr.Agent) ([]string, string, error) {
	// The session ref vanishes from herdr once the agent exits: read
	// everything needed to resume before stopping it.
	info, err := s.Herdr.ProcessInfo(ctx, a.PaneID)
	if err != nil {
		return nil, "", err
	}
	agentProc, ok := groupLeader(info)
	if !ok {
		return nil, "no agent process in the foreground", nil
	}
	argv, ok := resumeArgv(a.Agent, agentProc.Argv, a.Session.Value)
	if !ok {
		return nil, fmt.Sprintf("agent command line not recognised: %q", agentProc.Argv), nil
	}

	if err := s.Signaler.Terminate(info.ForegroundGroupID); err != nil {
		return nil, "", err
	}
	deadline := time.Now().Add(exitTimeout)
	gone, err := s.waitForShell(ctx, a.PaneID, deadline)
	if err != nil {
		return nil, "", err
	}
	if !gone {
		return nil, fmt.Sprintf("agent still running %s after SIGTERM, left alone", exitTimeout), nil
	}

	// The agent is stopped: from here on the pane must end up prefilled
	// whatever herdr does with the report, or the session is hard to find.
	listed, err := s.markAsleep(ctx, a.PaneID, a.Agent, argv, deadline)
	if err != nil {
		return nil, "", err
	}
	if !listed {
		return argv, "put asleep, but herdr did not keep it in the agent list", nil
	}
	return argv, "put asleep", nil
}

// rearm marks an asleep pane herdr no longer lists, once its shell owns the
// terminal. It never starts the agent: the resume command is only typed.
func (s Sweeper) rearm(ctx context.Context, paneID string, ps paneState) (string, error) {
	deadline := time.Now().Add(exitTimeout)
	free, err := s.waitForShell(ctx, paneID, deadline)
	if herr, ok := errors.AsType[*herdr.Error](err); ok && herr.Code == "pane_not_found" {
		return "pane not found, rearm on a later sweep", nil
	}
	if err != nil {
		return "", err
	}
	if !free {
		return "shell busy, rearm on a later sweep", nil
	}
	listed, err := s.markAsleep(ctx, paneID, ps.Kind, ps.ResumeArgv, deadline)
	if err != nil {
		return "", err
	}
	if !listed {
		return "rearmed, but herdr did not keep it in the agent list", nil
	}
	return "rearmed", nil
}

// markAsleep reports the pane as an asleep agent of kind and prefills its
// shell with argv. It reports whether herdr kept the pane listed.
func (s Sweeper) markAsleep(ctx context.Context, paneID, kind string, argv []string, deadline time.Time) (bool, error) {
	listed, err := s.reportAsleep(ctx, paneID, deadline)
	if err != nil {
		return false, err
	}
	if err := s.Herdr.ReportMetadata(ctx, herdr.MetadataReport{
		PaneID:          paneID,
		Source:          source,
		AppliesToSource: source,
		DisplayAgent:    kind,
		StateLabels:     map[string]string{"idle": "asleep"},
	}); err != nil {
		return false, err
	}
	if err := s.Herdr.SendText(ctx, paneID, shellJoin(argv)); err != nil {
		return false, err
	}
	return listed, nil
}

// waitForShell polls until the pane's shell owns the terminal again, so the
// prefill lands at its prompt instead of in the exiting agent. It reports
// false if that has not happened by deadline.
func (s Sweeper) waitForShell(ctx context.Context, paneID string, deadline time.Time) (bool, error) {
	for {
		info, err := s.Herdr.ProcessInfo(ctx, paneID)
		if err != nil {
			return false, err
		}
		if info.ShellPID != 0 && info.ForegroundGroupID == info.ShellPID {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		if err := sleep(ctx, pollEvery); err != nil {
			return false, err
		}
	}
}

// reportAsleep reports the pane under the stand-in kind until herdr still
// lists it reportSettle later, or deadline passes.
func (s Sweeper) reportAsleep(ctx context.Context, paneID string, deadline time.Time) (bool, error) {
	for {
		if err := s.Herdr.ReportAgent(ctx, herdr.AgentReport{
			PaneID: paneID, Source: source, Agent: standInKind, State: "idle",
		}); err != nil {
			return false, err
		}
		if err := sleep(ctx, reportSettle); err != nil {
			return false, err
		}
		agents, err := s.Herdr.Snapshot(ctx)
		if err != nil {
			return false, err
		}
		for _, b := range agents {
			if b.PaneID == paneID && b.Agent == standInKind {
				return true, nil
			}
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-t.C:
		return nil
	}
}

// groupLeader is the process the shell launched: the leader of the
// foreground group. Its siblings are the agent's own children (MCP servers).
func groupLeader(info herdr.ProcessInfo) (herdr.Process, bool) {
	if info.ForegroundGroupID == 0 || info.ForegroundGroupID == info.ShellPID {
		return herdr.Process{}, false
	}
	for _, p := range info.Foreground {
		if p.PID == info.ForegroundGroupID {
			return p, true
		}
	}
	return herdr.Process{}, false
}
