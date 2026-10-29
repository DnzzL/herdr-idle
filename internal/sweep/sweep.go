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
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/DnzzL/herdr-idle/internal/config"
	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// Herdr is the part of herdr's API a sweep uses.
type Herdr interface {
	Snapshot(ctx context.Context) ([]herdr.Agent, error)
	ProcessInfo(ctx context.Context, paneID herdr.PaneID) (herdr.ProcessInfo, error)
	// ReadRecent returns the pane's recent screen as plain text.
	ReadRecent(ctx context.Context, paneID herdr.PaneID) (string, error)
	ReportAgent(ctx context.Context, r herdr.AgentReport) error
	ReportMetadata(ctx context.Context, r herdr.MetadataReport) error
	SendText(ctx context.Context, paneID herdr.PaneID, text string) error
	// Panes returns every pane in the session, agent or not.
	Panes(ctx context.Context) ([]herdr.PaneID, error)
}

// Signaler stops processes.
type Signaler interface {
	// Terminate sends SIGTERM to every process in the process group.
	Terminate(pgid int) error
}

// Processes reads the OS process table. herdr only lists the foreground
// process group; an agent runs its tool commands in groups of their own.
type Processes interface {
	// List returns every live process, zombies left out.
	List(ctx context.Context) ([]OSProcess, error)
}

// Sessions reads what agents record about their own sessions.
type Sessions interface {
	// ClaudeEntrypoint returns how a Claude Code session was started: "cli"
	// interactive, "sdk-cli" for claude -p, "" when the file does not say.
	// It fails with fs.ErrNotExist when no session file has that id.
	ClaudeEntrypoint(ctx context.Context, sessionID string) (string, error)
}

// OSProcess is one row of the OS process table.
type OSProcess struct {
	PID, PPID int
	Argv      []string
}

// Sweeper holds what a sweep needs. The zero value is not usable: Run needs
// every field set, ForgetClosed only Herdr and StatePath.
type Sweeper struct {
	Herdr     Herdr
	Signaler  Signaler
	Processes Processes
	Sessions  Sessions
	// DirExists reports whether a folder still exists.
	DirExists func(dir string) bool
	// StatePath is the JSON file holding each pane's quiet clock and the
	// panes put asleep. A missing file starts every clock now.
	StatePath string
	// CaptureDir receives one file per pane put asleep: its recent screen,
	// saved before the agent is stopped.
	CaptureDir string
	// Config is read again at the start of every sweep. An invalid file
	// keeps the last valid config this File loaded; with none, the sweep
	// does nothing.
	Config *config.File
}

// Outcome is what a sweep decided for one agent pane.
type Outcome struct {
	PaneID herdr.PaneID
	Agent  herdr.Kind
	Asleep bool   // put asleep by this sweep
	Reason string // why it was or was not put asleep
}

// PluginID is the herdr plugin id herdr-idle runs and reports under; it
// must match herdr-plugin.toml.
const PluginID = "dnzzl.idle"

const (
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
	// settleGrace bounds the herdr calls that follow a pane's wait.
	settleGrace = 15 * time.Second
	// settleLockWait bounds the wait for another sweep's settle: it may be
	// waiting out several agents, one exitTimeout each.
	settleLockWait = 10 * time.Minute
)

// termIgnored is the reason given when the agent outlived SIGTERM. It is
// never sent SIGKILL: whatever it is doing is left to finish.
var termIgnored = fmt.Sprintf("agent still running %s after SIGTERM, left alone", exitTimeout)

// Run sweeps the session once. It returns one outcome per agent pane, and
// one per asleep pane it rearmed. A failure on one pane does not stop the
// others; their errors are joined.
//
// Once an agent has been sent SIGTERM, ctx ending no longer stops the sweep
// from putting its pane asleep; a pane left half done (the process died, a
// herdr call failed, the agent exited late) is finished by a later sweep.
// Sweeps of the same state file, and ForgetClosed, wait for each other only
// while the state file is read and written, never while an agent exits.
func (s Sweeper) Run(ctx context.Context) ([]Outcome, error) {
	cfg, ok, cfgErr := s.Config.Load()
	if !ok {
		return nil, cfgErr
	}
	outcomes, err := s.stopQuiet(ctx, cfg, false)
	if outcomes == nil && err != nil {
		return nil, err
	}
	settled, serr := s.settle(ctx)
	told, terr := s.tellGoneDirs(ctx)
	serr = errors.Join(serr, terr)
	for _, o := range append(settled, told...) {
		i := slices.IndexFunc(outcomes, func(p Outcome) bool { return p.PaneID == o.PaneID })
		if i < 0 {
			outcomes = append(outcomes, o)
			continue
		}
		outcomes[i].Asleep, outcomes[i].Reason = o.Asleep, o.Reason
	}
	return outcomes, errors.Join(cfgErr, err, serr)
}

// Preview says what Run would decide for every agent pane, as a dry run
// does, and changes nothing: not even the quiet clocks.
func (s Sweeper) Preview(ctx context.Context) ([]Outcome, error) {
	cfg, ok, cfgErr := s.Config.Load()
	if !ok {
		return nil, cfgErr
	}
	outcomes, err := s.stopQuiet(ctx, cfg, true)
	return outcomes, errors.Join(cfgErr, err)
}

// stopQuiet updates the quiet clocks and sends SIGTERM to every agent pane
// that may be put asleep, under the state lock. Each pane's resume command
// is saved before its agent is signalled. With preview it only says what it
// would do.
func (s Sweeper) stopQuiet(ctx context.Context, cfg config.Config, preview bool) ([]Outcome, error) {
	if !preview {
		unlock, err := lockState(ctx, s.StatePath)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}
	agents, err := s.Herdr.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	prev, err := loadState(s.StatePath)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	st := observe(prev, agents, now)

	var outcomes []Outcome
	var errs []error
	for _, a := range agents {
		o := Outcome{PaneID: a.PaneID, Agent: a.Agent}
		o.Reason = whyAwake(a, st[a.PaneID], now, cfg)
		if o.Reason == "" {
			reason, err := s.stop(ctx, a, st, preview || cfg.DryRun)
			if err != nil {
				errs = append(errs, fmt.Errorf("pane %s: %w", a.PaneID, err))
				reason = "error: " + err.Error()
			}
			o.Reason = reason
		}
		outcomes = append(outcomes, o)
	}
	if !preview {
		errs = append(errs, saveState(s.StatePath, st))
	}
	return outcomes, errors.Join(errs...)
}

// stop sends SIGTERM to the pane's agent once everything that resumes it is
// saved in st and on disk. It returns why it left the pane alone, or that
// the pane is being put asleep. A dry run stops before the first action.
func (s Sweeper) stop(ctx context.Context, a herdr.Agent, st state, dryRun bool) (string, error) {
	// The session ref vanishes from herdr once the agent exits: read
	// everything needed to resume before stopping it.
	info, err := s.Herdr.ProcessInfo(ctx, a.PaneID)
	if err != nil {
		return "", err
	}
	agentProc, ok := groupLeader(info)
	if !ok {
		return "no agent process in the foreground", nil
	}
	table, err := s.Processes.List(ctx)
	if err != nil {
		return "", err
	}
	if p, busy := busyChild(agentProc.PID, withChildren(info.Foreground, agentProc.PID, table)); busy {
		if len(p.Argv) == 0 {
			return fmt.Sprintf("running pid %d, command line unknown", p.PID), nil
		}
		return "running " + strings.Join(p.Argv, " "), nil
	}
	argv, ok := resumeArgv(a.Agent, agentProc.Argv, a.Session.Value)
	if !ok {
		return fmt.Sprintf("agent command line not recognised: %q", agentProc.Argv), nil
	}
	if reason, err := s.notInteractive(ctx, a); reason != "" || err != nil {
		return reason, err
	}
	before := st[a.PaneID]
	ps := before
	ps.Kind, ps.SessionRef, ps.ResumeArgv, ps.Cwd = a.Agent, a.Session.Value, argv, agentProc.Cwd
	if dryRun {
		return "would be put asleep, resume with: " + ps.prefill(), nil
	}

	// Nothing on screen may be lost: no capture, no SIGTERM.
	ps.CapturePath, err = s.capture(ctx, a.PaneID)
	if err != nil {
		return "", err
	}
	// Nor the resume command: it is on disk before the agent can exit.
	ps.StoppedAt, ps.TermIgnored = time.Now(), false
	st[a.PaneID] = ps
	if err := saveState(s.StatePath, st); err != nil {
		st[a.PaneID] = before
		return "", err
	}
	if err := s.Signaler.Terminate(info.ForegroundGroupID); err != nil {
		st[a.PaneID] = before
		return "", err
	}
	return "being put asleep", nil
}

// settle finishes putting asleep every pane whose agent was sent SIGTERM,
// and rearms every asleep pane herdr no longer lists (after a herdr restart
// our reports are gone). Each is reported asleep once its shell owns the
// terminal, and prefilled unless that shell already was. It never starts an
// agent. It returns one outcome per pane it did something for.
//
// Settles run one at a time, outside the state lock. A settle carries on
// after ctx ends for the panes already signalled, and then skips the rearms.
func (s Sweeper) settle(ctx context.Context) ([]Outcome, error) {
	rearm := ctx.Err() == nil
	ctx = context.WithoutCancel(ctx)
	lockCtx, cancel := context.WithTimeout(ctx, settleLockWait)
	unlock, err := lock(lockCtx, s.StatePath+".settle.lock", true)
	cancel()
	if err != nil {
		return nil, err
	}
	defer unlock()

	// Writers replace the file whole: it reads consistently without the
	// lock.
	st, err := loadState(s.StatePath)
	if err != nil {
		return nil, err
	}
	todo := state{}
	listed := map[herdr.PaneID]bool{}
	for id, ps := range st {
		if ps.stopping() {
			todo[id] = ps
		}
	}
	if rearm && len(st) > len(todo) {
		agents, err := s.Herdr.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range agents {
			listed[a.PaneID] = true
		}
		for id, ps := range st {
			if !ps.AsleepAt.IsZero() && !listed[id] {
				todo[id] = ps
			}
		}
	}
	if len(todo) == 0 {
		return nil, nil
	}

	var outcomes []Outcome
	var errs []error
	for _, id := range slices.Sorted(maps.Keys(todo)) {
		ps := todo[id]
		// A pane stopped by an earlier sweep gets one check.
		deadline := time.Now().Add(exitTimeout)
		if ps.stopping() {
			deadline = time.Now().Add(max(time.Until(ps.StoppedAt.Add(exitTimeout)), 0))
		}
		pctx, cancel := context.WithTimeout(ctx, time.Until(deadline)+settleGrace)
		next, o, err := s.settlePane(pctx, id, ps, deadline)
		cancel()
		todo[id] = next
		if err != nil {
			errs = append(errs, fmt.Errorf("pane %s: %w", id, err))
			o = Outcome{PaneID: id, Agent: ps.Kind, Reason: "error: " + err.Error()}
		}
		if o.Reason != "" {
			outcomes = append(outcomes, o)
		}
	}

	// Only panes still known are written back: one closed meanwhile stays
	// forgotten.
	unlockState, err := lockState(ctx, s.StatePath)
	if err != nil {
		return outcomes, errors.Join(append(errs, err)...)
	}
	defer unlockState()
	st, err = loadState(s.StatePath)
	if err != nil {
		return outcomes, errors.Join(append(errs, err)...)
	}
	for id, ps := range todo {
		if _, ok := st[id]; ok {
			st[id] = ps
		}
	}
	return outcomes, errors.Join(append(errs, saveState(s.StatePath, st))...)
}

// settlePane waits until deadline for the pane's shell to own the terminal,
// then reports it asleep and, unless that shell has it already, prints where
// its screen was saved and prefills the resume command. It returns the
// pane's new state and an outcome, whose Reason is empty when nothing
// changed.
func (s Sweeper) settlePane(ctx context.Context, paneID herdr.PaneID, ps paneState, deadline time.Time) (paneState, Outcome, error) {
	o := Outcome{PaneID: paneID, Agent: ps.Kind}
	shell, err := s.waitForShell(ctx, paneID, deadline)
	if herr, ok := errors.AsType[*herdr.Error](err); ok && herr.Code == "pane_not_found" {
		o.Reason = "pane not found, rearm on a later sweep"
		return ps, o, nil
	}
	if err != nil {
		return ps, o, err
	}
	if shell == 0 {
		switch {
		case !ps.stopping():
			o.Reason = "shell busy, rearm on a later sweep"
		case !ps.TermIgnored:
			ps.TermIgnored = true
			o.Reason = termIgnored
		}
		return ps, o, nil
	}

	// The agent is gone: from here on the pane must end up prefilled
	// whatever herdr does with the report, or the session is hard to find.
	listed, err := s.reportAsleep(ctx, paneID, deadline)
	if err != nil {
		return ps, o, err
	}
	if err := s.Herdr.ReportMetadata(ctx, herdr.MetadataReport{
		PaneID:          paneID,
		Source:          PluginID,
		AppliesToSource: PluginID,
		DisplayAgent:    ps.Kind,
		StateLabels:     map[string]string{string(herdr.StatusUnknown): "asleep"},
	}); err != nil {
		return ps, o, err
	}
	// A shell keeps its prefill until the user acts on it; a new shell (a
	// herdr restart) has lost it, and its scrollback with the notice.
	if ps.PrefilledShell != shell {
		if ps.CapturePath != "" {
			// Show the saved screen, so the pane still tells which
			// conversation it holds, then where it is saved and the
			// resume command, which a keystroke can wipe from the
			// prompt. Run with
			// Enter before the prefill, so the prefill stays editable at
			// the next prompt. The leading space keeps it out
			// of shell history where the shell honours that.
			// command skips aliases and functions: a cat aliased to a
			// pager would swallow the prefill typed next.
			notice := " " + shellJoin([]string{"command", "cat", ps.CapturePath}) + "; " +
				shellJoin([]string{"echo", "herdr-idle: screen saved to " + ps.CapturePath}) + "; " +
				shellJoin([]string{"echo", "herdr-idle: resume with: " + ps.prefill()}) + "\r"
			if err := s.Herdr.SendText(ctx, paneID, notice); err != nil {
				return ps, o, err
			}
		}
		if err := s.Herdr.SendText(ctx, paneID, ps.prefill()); err != nil {
			return ps, o, err
		}
		// A new shell has the cd again: tell it again if the folder is gone.
		ps.PrefilledShell, ps.DirGone = shell, false
	}

	o.Reason = "rearmed"
	if ps.stopping() {
		ps.AsleepAt, ps.TermIgnored = time.Now(), false
		o.Asleep, o.Reason = true, "put asleep"
	}
	if !listed {
		o.Reason += ", but herdr did not keep it in the agent list"
	}
	return ps, o, nil
}

// ForgetClosed forgets every pane herdr no longer has: its state entry and
// its capture file are removed. herdr runs it on pane, tab and workspace
// close, since closing a tab or workspace sends no pane.closed for the panes
// in it. If herdr cannot list its panes, nothing is forgotten. It waits for
// no agent to exit, only for the state file to be free.
func (s Sweeper) ForgetClosed(ctx context.Context) error {
	open, err := s.Herdr.Panes(ctx)
	if err != nil {
		return err
	}
	unlock, err := lockState(ctx, s.StatePath)
	if err != nil {
		return err
	}
	defer unlock()
	st, err := loadState(s.StatePath)
	if err != nil {
		return err
	}
	var errs []error
	for id, ps := range st {
		if slices.Contains(open, id) {
			continue
		}
		if ps.CapturePath != "" {
			if err := os.Remove(ps.CapturePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("remove capture: %w", err))
				continue
			}
		}
		delete(st, id)
	}
	return errors.Join(append(errs, saveState(s.StatePath, st))...)
}

// capture saves the pane's recent screen to its file in CaptureDir and
// returns the file's path.
func (s Sweeper) capture(ctx context.Context, paneID herdr.PaneID) (string, error) {
	text, err := s.Herdr.ReadRecent(ctx, paneID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.CaptureDir, 0o700); err != nil {
		return "", fmt.Errorf("save screen: %w", err)
	}
	path := s.capturePath(paneID)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", fmt.Errorf("save screen: %w", err)
	}
	return path, nil
}

// capturePath is the pane's capture file. Pane ids look like w1:p2; keep
// file names portable.
func (s Sweeper) capturePath(paneID herdr.PaneID) string {
	return filepath.Join(s.CaptureDir, strings.ReplaceAll(string(paneID), ":", "-")+".txt")
}

// waitForShell polls until the pane's shell owns the terminal again, so the
// prefill lands at its prompt instead of in the exiting agent, and returns
// the shell's pid. It returns 0 if that has not happened by deadline.
func (s Sweeper) waitForShell(ctx context.Context, paneID herdr.PaneID, deadline time.Time) (int, error) {
	for {
		info, err := s.Herdr.ProcessInfo(ctx, paneID)
		if err != nil {
			return 0, err
		}
		if info.ShellPID != 0 && info.ForegroundGroupID == info.ShellPID {
			return info.ShellPID, nil
		}
		if !time.Now().Before(deadline) {
			return 0, nil
		}
		if err := sleep(ctx, pollEvery); err != nil {
			return 0, err
		}
	}
}

// reportAsleep reports the pane under the stand-in kind until herdr still
// lists it reportSettle later, or deadline passes. State unknown ranks
// last in herdr's priority-sorted agent list: a sleeping agent needs no
// attention.
func (s Sweeper) reportAsleep(ctx context.Context, paneID herdr.PaneID, deadline time.Time) (bool, error) {
	for {
		if err := s.Herdr.ReportAgent(ctx, herdr.AgentReport{
			PaneID: paneID, Source: PluginID, Agent: standInKind, State: herdr.StatusUnknown,
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

// notInteractive says why the pane's Claude session may not be the pane's
// conversation: a claude -p run in the pane makes herdr take its one-shot
// session as the pane's. Other kinds are not checked.
func (s Sweeper) notInteractive(ctx context.Context, a herdr.Agent) (string, error) {
	if a.Agent != "claude" {
		return "", nil
	}
	entry, err := s.Sessions.ClaudeEntrypoint(ctx, a.Session.Value)
	if errors.Is(err, fs.ErrNotExist) {
		return "no Claude session file for " + a.Session.Value, nil
	}
	if err != nil {
		return "", err
	}
	if entry == "cli" {
		return "", nil
	}
	if entry == "" {
		entry = "unknown"
	}
	return fmt.Sprintf("session %s was not started interactively (%s): a claude -p run in this pane may have replaced the pane's session", a.Session.Value, entry), nil
}

// tellGoneDirs tells each asleep pane whose folder was deleted (a removed
// worktree) that its prefill can no longer work, once, and clears that
// prefill: resuming in another folder could let the agent edit the wrong
// repository, so the user picks the folder. A pane whose shell is busy is
// told at a later sweep.
func (s Sweeper) tellGoneDirs(ctx context.Context) ([]Outcome, error) {
	unlock, err := lockState(ctx, s.StatePath)
	if err != nil {
		return nil, err
	}
	defer unlock()
	st, err := loadState(s.StatePath)
	if err != nil {
		return nil, err
	}
	var outcomes []Outcome
	var errs []error
	for _, id := range slices.Sorted(maps.Keys(st)) {
		ps := st[id]
		if ps.AsleepAt.IsZero() || ps.DirGone || ps.Cwd == "" || s.DirExists(ps.Cwd) {
			continue
		}
		info, err := s.Herdr.ProcessInfo(ctx, id)
		if herr, ok := errors.AsType[*herdr.Error](err); ok && herr.Code == "pane_not_found" {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("pane %s: %w", id, err))
			continue
		}
		if info.ForegroundGroupID != info.ShellPID {
			continue
		}
		msg := "herdr-idle: " + ps.Cwd + " no longer exists. To resume this conversation from another folder, cd there and run: " + shellJoin(ps.ResumeArgv)
		// Ctrl+U clears the prefill first, or Enter would run it with the
		// message.
		if err := s.Herdr.SendText(ctx, id, "\x15 "+shellJoin([]string{"echo", msg})+"\r"); err != nil {
			errs = append(errs, fmt.Errorf("pane %s: %w", id, err))
			continue
		}
		ps.DirGone = true
		st[id] = ps
		outcomes = append(outcomes, Outcome{PaneID: id, Agent: ps.Kind, Reason: "asleep, folder " + ps.Cwd + " gone: told the pane"})
	}
	if len(outcomes) > 0 {
		errs = append(errs, saveState(s.StatePath, st))
	}
	return outcomes, errors.Join(errs...)
}
