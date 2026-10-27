// Package sweep runs one pass over a herdr session's agent panes and puts the
// quiet ones asleep: the agent is stopped with SIGTERM, the pane stays at its
// shell, is reported to herdr as asleep, and its prompt is prefilled with the
// command that resumes the same session.
package sweep

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// Herdr is the part of herdr's API a sweep uses.
type Herdr interface {
	Snapshot(ctx context.Context) ([]herdr.Agent, error)
	ProcessInfo(ctx context.Context, paneID string) (herdr.ProcessInfo, error)
	// ReadRecent returns the pane's recent screen as plain text.
	ReadRecent(ctx context.Context, paneID string) (string, error)
	ReportAgent(ctx context.Context, r herdr.AgentReport) error
	ReportMetadata(ctx context.Context, r herdr.MetadataReport) error
	SendText(ctx context.Context, paneID, text string) error
}

// Signaler stops processes.
type Signaler interface {
	// Terminate sends SIGTERM to every process in the process group.
	Terminate(pgid int) error
}

// Processes reads the OS process table. herdr only lists the foreground
// process group; an agent runs its tool commands in groups of their own.
type Processes interface {
	// Children returns the direct children of pid, whatever their group.
	Children(ctx context.Context, pid int) ([]herdr.Process, error)
}

// Sweeper holds what a sweep needs. The zero value is not usable: every field
// must be set.
type Sweeper struct {
	Herdr     Herdr
	Signaler  Signaler
	Processes Processes
	// StatePath is the JSON file holding each pane's quiet clock and the
	// panes put asleep. A missing file starts every clock now.
	StatePath string
	// CaptureDir receives one file per pane put asleep: its recent screen,
	// saved before the agent is stopped.
	CaptureDir string
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

// termIgnored is putAsleep's reason when the agent outlived SIGTERM. It is
// never sent SIGKILL: whatever it is doing is left to finish.
var termIgnored = fmt.Sprintf("agent still running %s after SIGTERM, left alone", exitTimeout)

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
			if reason == termIgnored {
				ps := next[a.PaneID]
				ps.TermIgnored = true
				next[a.PaneID] = ps
			}
			o.Reason = reason
		}
		outcomes = append(outcomes, o)
	}
	if err := saveState(s.StatePath, next); err != nil {
		errs = append(errs, err)
	}
	return outcomes, errors.Join(errs...)
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
	children, err := s.Processes.Children(ctx, agentProc.PID)
	if err != nil {
		return nil, "", err
	}
	if p, busy := busyChild(agentProc.PID, append(info.Foreground, children...)); busy {
		return nil, "running " + strings.Join(p.Argv, " "), nil
	}
	argv, ok := resumeArgv(a.Agent, agentProc.Argv, a.Session.Value)
	if !ok {
		return nil, fmt.Sprintf("agent command line not recognised: %q", agentProc.Argv), nil
	}

	// Nothing on screen may be lost: no capture, no SIGTERM.
	capture, err := s.capture(ctx, a.PaneID)
	if err != nil {
		return nil, "", err
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
		return nil, termIgnored, nil
	}

	// The agent is stopped: from here on the pane must end up prefilled
	// whatever herdr does with the report, or the session is hard to find.
	listed, err := s.reportAsleep(ctx, a.PaneID, deadline)
	if err != nil {
		return nil, "", err
	}
	if err := s.Herdr.ReportMetadata(ctx, herdr.MetadataReport{
		PaneID:          a.PaneID,
		Source:          source,
		AppliesToSource: source,
		DisplayAgent:    a.Agent,
		StateLabels:     map[string]string{"idle": "asleep"},
	}); err != nil {
		return nil, "", err
	}
	// Run with Enter before the prefill, so the prefill stays editable at
	// the next prompt. The leading space keeps it out of shell history
	// where the shell honours that.
	notice := " " + shellJoin([]string{"echo", "herdr-idle: screen saved to " + capture}) + "\r"
	if err := s.Herdr.SendText(ctx, a.PaneID, notice); err != nil {
		return nil, "", err
	}
	if err := s.Herdr.SendText(ctx, a.PaneID, shellJoin(argv)); err != nil {
		return nil, "", err
	}
	if !listed {
		return argv, "put asleep, but herdr did not keep it in the agent list", nil
	}
	return argv, "put asleep", nil
}

// capture saves the pane's recent screen to its file in CaptureDir and
// returns the file's path.
func (s Sweeper) capture(ctx context.Context, paneID string) (string, error) {
	text, err := s.Herdr.ReadRecent(ctx, paneID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.CaptureDir, 0o700); err != nil {
		return "", fmt.Errorf("save screen: %w", err)
	}
	// Pane ids look like w1:p2; keep file names portable.
	path := filepath.Join(s.CaptureDir, strings.ReplaceAll(paneID, ":", "-")+".txt")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", fmt.Errorf("save screen: %w", err)
	}
	return path, nil
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
