package sweep_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DnzzL/herdr-idle/internal/herdr"
	"github.com/DnzzL/herdr-idle/internal/sweep"
)

const (
	pane      = "w1:p1"
	shellPID  = 100
	agentPGID = 4242
	sessionID = "be00b9d0-1111-2222-3333-444455556666"
)

// fakeHerdr scripts one session: its agent panes and each pane's processes.
// Every call that changes something lands in events, in order, alongside the
// signals sent by fakeSignaler.
type fakeHerdr struct {
	agents []herdr.Agent
	procs  map[string]herdr.ProcessInfo
	exitAt map[string]time.Time // pane → when its terminated agent is gone
	// shellOnly panes run no agent at all: herdr lists them only while a
	// plugin report holds them.
	shellOnly map[string]bool
	// shellFreeAt is when a pane's shell takes the foreground back.
	shellFreeAt map[string]time.Time
	events      []string
	reports     []herdr.AgentReport
	meta        []herdr.MetadataReport
}

// herdrLag is how long after an agent exits herdr's process detector
// notices; it wipes plugin reports made before then. The snapshot stops
// listing the agent sooner, so it cannot tell when that has happened.
const herdrLag = 500 * time.Millisecond

func (f *fakeHerdr) Snapshot(context.Context) ([]herdr.Agent, error) {
	var out []herdr.Agent
	for _, a := range f.agents {
		if r, ok := f.lastReport(a.PaneID); ok {
			out = append(out, herdr.Agent{PaneID: a.PaneID, Agent: r.Agent, Status: r.State})
			continue
		}
		if !f.agentGone(a.PaneID) && !f.shellOnly[a.PaneID] {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeHerdr) lastReport(paneID string) (herdr.AgentReport, bool) {
	for _, r := range slices.Backward(f.reports) {
		if r.PaneID == paneID {
			return r, true
		}
	}
	return herdr.AgentReport{}, false
}

func (f *fakeHerdr) herdrNoticedExit(paneID string) bool {
	at, ok := f.exitAt[paneID]
	return ok && !time.Now().Before(at.Add(herdrLag))
}

func (f *fakeHerdr) ProcessInfo(_ context.Context, paneID string) (herdr.ProcessInfo, error) {
	p, ok := f.procs[paneID]
	if !ok {
		return herdr.ProcessInfo{}, fmt.Errorf("no pane %s", paneID)
	}
	if f.agentGone(paneID) {
		return herdr.ProcessInfo{ShellPID: p.ShellPID, ForegroundGroupID: p.ShellPID}, nil
	}
	if at, ok := f.shellFreeAt[paneID]; ok && !time.Now().Before(at) {
		p.ForegroundGroupID, p.Foreground = p.ShellPID, nil
	}
	return p, nil
}

func (f *fakeHerdr) agentGone(paneID string) bool {
	at, ok := f.exitAt[paneID]
	return ok && !time.Now().Before(at)
}

func (f *fakeHerdr) ReportAgent(_ context.Context, r herdr.AgentReport) error {
	if _, ok := f.exitAt[r.PaneID]; ok && !f.herdrNoticedExit(r.PaneID) {
		return nil // accepted, then wiped when the detector catches up
	}
	f.events = append(f.events, "report_agent "+r.PaneID)
	f.reports = append(f.reports, r)
	return nil
}

func (f *fakeHerdr) ReportMetadata(_ context.Context, r herdr.MetadataReport) error {
	f.events = append(f.events, "report_metadata "+r.PaneID)
	f.meta = append(f.meta, r)
	return nil
}

func (f *fakeHerdr) SendText(_ context.Context, paneID, text string) error {
	if _, ok := f.exitAt[paneID]; ok && !f.agentGone(paneID) {
		f.events = append(f.events, "send_text while the agent still runs")
		return nil
	}
	f.events = append(f.events, "send_text "+paneID+" "+text)
	return nil
}

// fakeSignaler stands in for the OS: SIGTERM to a pane's foreground group
// ends the agent 2s later, handing the terminal back to the shell.
type fakeSignaler struct{ h *fakeHerdr }

func (s *fakeSignaler) Terminate(pgid int) error {
	s.h.events = append(s.h.events, fmt.Sprintf("SIGTERM %d", pgid))
	for id, p := range s.h.procs {
		if p.ForegroundGroupID == pgid {
			s.h.exitAt[id] = time.Now().Add(2 * time.Second)
		}
	}
	return nil
}

func claudePane(argv ...string) (herdr.Agent, herdr.ProcessInfo) {
	a := herdr.Agent{
		PaneID:         pane,
		Agent:          "claude",
		Status:         "idle",
		StateChangeSeq: 7,
		Session: &herdr.AgentSession{
			Source: "herdr:claude", Agent: "claude", Kind: "id", Value: sessionID,
		},
	}
	p := herdr.ProcessInfo{
		ShellPID:          shellPID,
		ForegroundGroupID: agentPGID,
		Foreground: []herdr.Process{
			{PID: agentPGID + 1, Argv: []string{"bun", "/x/backlog", "mcp", "start"}},
			{PID: agentPGID, Argv: argv},
		},
	}
	return a, p
}

func newWorld(t *testing.T, a herdr.Agent, p herdr.ProcessInfo) (*fakeHerdr, sweep.Sweeper) {
	t.Helper()
	h := &fakeHerdr{
		agents: []herdr.Agent{a},
		procs:  map[string]herdr.ProcessInfo{a.PaneID: p},
		exitAt: map[string]time.Time{},
	}
	s := sweep.Sweeper{
		Herdr:     h,
		Signaler:  &fakeSignaler{h: h},
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		IdleAfter: 60 * time.Minute,
	}
	return h, s
}

func run(t *testing.T, s sweep.Sweeper) {
	t.Helper()
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
}

func TestQuietClaudePaneIsPutAsleep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude", "--model", "opus")
		h, s := newWorld(t, a, p)

		run(t, s)
		if len(h.events) != 0 {
			t.Fatalf("first sweep on a fresh state file acted: %q", h.events)
		}

		time.Sleep(61 * time.Minute)
		run(t, s)

		want := []string{
			"SIGTERM 4242",
			"report_agent " + pane,
			"report_metadata " + pane,
			"send_text " + pane + " claude --resume " + sessionID + " --model opus",
		}
		if !slices.Equal(h.events, want) {
			t.Fatalf("events:\n got %q\nwant %q", h.events, want)
		}
		wantReport := herdr.AgentReport{PaneID: pane, Source: "dnzzl.idle", Agent: "maki", State: "idle"}
		if h.reports[0] != wantReport {
			t.Errorf("report_agent = %+v, want %+v", h.reports[0], wantReport)
		}
		m := h.meta[0]
		if m.Source != "dnzzl.idle" || m.AppliesToSource != "dnzzl.idle" ||
			m.DisplayAgent != "claude" || m.StateLabels["idle"] != "asleep" {
			t.Errorf("report_metadata = %+v", m)
		}
		if strings.ContainsAny(h.events[3], "\r\n") {
			t.Errorf("prefill pressed Enter: %q", h.events[3])
		}
	})
}

// putAsleep runs a sweep on a fresh state file, waits out the quiet period
// and sweeps again: the shortest path to a pane put asleep.
func putAsleep(t *testing.T, s sweep.Sweeper) {
	t.Helper()
	run(t, s)
	time.Sleep(s.IdleAfter + time.Minute)
	run(t, s)
}

func TestPrefillKeepsFlagsAndResumesTheCurrentSession(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"bare", []string{"claude"}, "claude --resume " + sessionID},
		{"flags kept",
			[]string{"claude", "--model", "opus", "--permission-mode", "acceptEdits"},
			"claude --resume " + sessionID + " --model opus --permission-mode acceptEdits"},
		{"old resume id replaced",
			[]string{"claude", "--resume", "old-id", "--dangerously-skip-permissions"},
			"claude --resume " + sessionID + " --dangerously-skip-permissions"},
		{"old resume=id replaced",
			[]string{"claude", "--model=opus", "--resume=old-id"},
			"claude --resume " + sessionID + " --model=opus"},
		{"short -r replaced", []string{"claude", "-r", "old-id"}, "claude --resume " + sessionID},
		{"resume picker without id", []string{"claude", "--resume", "--model", "opus"},
			"claude --resume " + sessionID + " --model opus"},
		{"continue dropped", []string{"claude", "-c", "--continue"}, "claude --resume " + sessionID},
		{"session-id and fork dropped",
			[]string{"claude", "--session-id", "old-id", "--fork-session"},
			"claude --resume " + sessionID},
		{"command by path", []string{"/Users/me/.local/bin/claude", "--model", "opus"},
			"claude --resume " + sessionID + " --model opus"},
		{"script run by node", []string{"node", "/opt/homebrew/bin/claude", "--model", "opus"},
			"claude --resume " + sessionID + " --model opus"},
		{"spaces and quotes quoted",
			[]string{"claude", "--append-system-prompt", "don't; rm -rf ~"},
			"claude --resume " + sessionID + ` --append-system-prompt 'don'\''t; rm -rf ~'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a, p := claudePane(tt.argv...)
				h, s := newWorld(t, a, p)
				putAsleep(t, s)
				want := "send_text " + pane + " " + tt.want
				if len(h.events) != 4 || h.events[3] != want {
					t.Fatalf("events:\n got %q\nwant last %q", h.events, want)
				}
			})
		})
	}
}

func TestPanesThatMustStayAwakeGetNoAction(t *testing.T) {
	tests := []struct {
		name   string
		change func(*herdr.Agent)
		quiet  time.Duration
		reason string
	}{
		{"focused", func(a *herdr.Agent) { a.Focused = true }, 2 * time.Hour, "focused"},
		{"working", func(a *herdr.Agent) { a.Status = "working" }, 2 * time.Hour, "agent is working"},
		{"blocked", func(a *herdr.Agent) { a.Status = "blocked" }, 2 * time.Hour, "agent is blocked"},
		{"not quiet long enough", func(*herdr.Agent) {}, 59 * time.Minute, "quiet for 59m0s, needs 1h0m0s"},
		{"no session ref", func(a *herdr.Agent) { a.Session = nil }, 2 * time.Hour, "no session ref"},
		{"empty session ref", func(a *herdr.Agent) { a.Session.Value = "" }, 2 * time.Hour, "no session ref"},
		{"no resume recipe", func(a *herdr.Agent) { a.Agent = "gemini"; a.Session.Agent = "gemini" },
			2 * time.Hour, "no resume recipe for gemini"},
		{"a pane herdr-idle put asleep", func(a *herdr.Agent) { a.Agent = "maki"; a.Session = nil },
			2 * time.Hour, "no resume recipe for maki"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a, p := claudePane("claude")
				tt.change(&a)
				h, s := newWorld(t, a, p)
				run(t, s)
				time.Sleep(tt.quiet)
				out, err := s.Run(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if len(h.events) != 0 {
					t.Fatalf("acted: %q", h.events)
				}
				if len(out) != 1 || out[0].Asleep || out[0].Reason != tt.reason {
					t.Fatalf("outcome = %+v, want reason %q", out, tt.reason)
				}
			})
		})
	}
}

func TestDonePaneIsPutAsleep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		a.Status = "done"
		h, s := newWorld(t, a, p)
		putAsleep(t, s)
		if len(h.events) != 4 {
			t.Fatalf("events: %q", h.events)
		}
	})
}

func TestQuietClockRestartsOnEveryStateChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		run(t, s) // clock starts now: herdr gives no time for past changes

		time.Sleep(40 * time.Minute)
		h.agents[0].StateChangeSeq++ // the agent worked and went idle again
		run(t, s)

		time.Sleep(59 * time.Minute) // 99m since the first sweep, 59m quiet
		run(t, s)
		if len(h.events) != 0 {
			t.Fatalf("asleep before a full quiet period since the change: %q", h.events)
		}

		time.Sleep(2 * time.Minute)
		run(t, s)
		if len(h.events) != 4 {
			t.Fatalf("not asleep after a full quiet period: %q", h.events)
		}
	})
}

func TestAsleepPaneIsNotPutAsleepAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		putAsleep(t, s)
		// herdr now shows the stand-in report: it must not count as a new
		// pane to put asleep.
		time.Sleep(3 * time.Hour)
		out, err := s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.events) != 4 || out[0].Reason != "asleep" {
			t.Fatalf("events %q, outcome %+v", h.events, out)
		}
	})
}

func TestUnrecognisedCommandLineIsLeftAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("node", "/somewhere/cli.js")
		h, s := newWorld(t, a, p)
		putAsleep(t, s)
		if len(h.events) != 0 {
			t.Fatalf("acted on an argv that does not run claude: %q", h.events)
		}
	})
}

// wake is the user pressing Enter on the asleep pane: the agent runs again
// under its own kind, and herdr drops the stand-in report a few seconds
// later.
func (f *fakeHerdr) wake(paneID string) {
	delete(f.exitAt, paneID)
	f.reports = slices.DeleteFunc(f.reports, func(r herdr.AgentReport) bool { return r.PaneID == paneID })
}

func TestWokenPaneSleepsAgainOnlyAfterAFullQuietPeriod(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		putAsleep(t, s)
		if len(h.events) != 4 {
			t.Fatalf("not asleep: %q", h.events)
		}

		time.Sleep(3 * time.Hour)
		h.wake(pane) // same seq: nothing promises herdr bumps it
		h.events = nil
		run(t, s) // notices the wake: the clock starts now

		time.Sleep(59 * time.Minute)
		out, err := s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.events) != 0 {
			t.Fatalf("put asleep again before a full quiet period: %q", h.events)
		}
		if out[0].Reason != "quiet for 59m0s, needs 1h0m0s" {
			t.Fatalf("outcome %+v", out[0])
		}

		time.Sleep(2 * time.Minute)
		run(t, s)
		if len(h.events) != 4 || h.events[0] != "SIGTERM 4242" {
			t.Fatalf("not asleep again after a full quiet period: %q", h.events)
		}
	})
}

func TestClosedPaneLeavesNoStateOrCaptureBehind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		_, s := newWorld(t, a, p)
		putAsleep(t, s)
		capture := recordCapture(t, s, pane)

		if err := s.PaneClosed(t.Context(), pane); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(capture); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("capture file still there: %v", err)
		}
		if st := readState(t, s); st[pane] != nil {
			t.Errorf("state entry survived: %v", st[pane])
		}
	})
}

func readState(t *testing.T, s sweep.Sweeper) map[string]map[string]any {
	t.Helper()
	b, err := os.ReadFile(s.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]map[string]any
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestClosingAnotherPaneLeavesAsleepPaneAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		putAsleep(t, s)
		capture := recordCapture(t, s, pane)
		if err := s.PaneClosed(t.Context(), "w9:p9"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(capture); err != nil {
			t.Errorf("capture of a pane still open removed: %v", err)
		}
		time.Sleep(3 * time.Hour)
		out, err := s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.events) != 4 || out[0].Reason != "asleep" {
			t.Fatalf("another pane's close touched this one: %q %+v", h.events, out)
		}
	})
}

// recordCapture writes a capture file for the pane and points its state
// entry at it, as putting a pane asleep does once screens are captured.
func recordCapture(t *testing.T, s sweep.Sweeper, paneID string) string {
	t.Helper()
	capture := filepath.Join(filepath.Dir(s.StatePath), "capture.txt")
	if err := os.WriteFile(capture, []byte("screen"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := readState(t, s)
	st[paneID]["capture_path"] = capture
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.StatePath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return capture
}

// restart is a herdr server restart: every plugin report and label is gone,
// the pane comes back as a fresh shell (new pid) and nothing is resumed.
func (f *fakeHerdr) restart(paneID string) {
	f.reports = slices.DeleteFunc(f.reports, func(r herdr.AgentReport) bool { return r.PaneID == paneID })
	f.meta = nil
	f.procs[paneID] = herdr.ProcessInfo{ShellPID: shellPID + 1, ForegroundGroupID: shellPID + 1}
	delete(f.exitAt, paneID)
	f.shellOnly = map[string]bool{paneID: true}
}

func TestRearmAfterHerdrRestartRelistsAndPrefillsWithoutStartingTheAgent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude", "--model", "opus")
		h, s := newWorld(t, a, p)
		putAsleep(t, s)

		h.restart(pane)
		h.events = nil
		out, err := s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		want := []string{
			"report_agent " + pane,
			"report_metadata " + pane,
			"send_text " + pane + " claude --resume " + sessionID + " --model opus",
		}
		if !slices.Equal(h.events, want) {
			t.Fatalf("events:\n got %q\nwant %q", h.events, want)
		}
		m := h.meta[0]
		if m.DisplayAgent != "claude" || m.StateLabels["idle"] != "asleep" || m.AppliesToSource != "dnzzl.idle" {
			t.Errorf("report_metadata = %+v", m)
		}
		if r, _ := h.lastReport(pane); r.Agent != "maki" || r.Source != "dnzzl.idle" || r.State != "idle" {
			t.Errorf("report_agent = %+v", r)
		}
		if len(out) != 1 || out[0].PaneID != pane || out[0].Agent != "claude" || out[0].Reason != "rearmed" {
			t.Errorf("outcomes = %+v", out)
		}

		// Listed again: later sweeps leave it be, and it still counts as asleep.
		h.events = nil
		time.Sleep(3 * time.Hour)
		out, err = s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.events) != 0 || len(out) != 1 || out[0].Reason != "asleep" {
			t.Fatalf("rearmed twice or lost: %q %+v", h.events, out)
		}
	})
}

func TestRearmWaitsForTheShellPrompt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		putAsleep(t, s)

		h.restart(pane)
		// The restored shell is still starting: its rc file owns the
		// terminal for a few seconds.
		h.procs[pane] = herdr.ProcessInfo{ShellPID: shellPID + 1, ForegroundGroupID: 777}
		h.shellFreeAt = map[string]time.Time{pane: time.Now().Add(3 * time.Second)}
		h.events = nil
		run(t, s)
		want := "send_text " + pane + " claude --resume " + sessionID
		if len(h.events) != 3 || h.events[2] != want {
			t.Fatalf("events %q", h.events)
		}
	})
}

func TestRearmLeavesABusyShellForALaterSweep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		putAsleep(t, s)

		h.restart(pane)
		h.procs[pane] = herdr.ProcessInfo{ShellPID: shellPID + 1, ForegroundGroupID: 777}
		h.events = nil
		out, err := s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.events) != 0 || len(out) != 1 || out[0].Reason != "shell busy, rearm on a later sweep" {
			t.Fatalf("events %q, outcome %+v", h.events, out)
		}

		h.procs[pane] = herdr.ProcessInfo{ShellPID: shellPID + 1, ForegroundGroupID: shellPID + 1}
		run(t, s)
		if len(h.events) != 3 {
			t.Fatalf("not rearmed once the shell was free: %q", h.events)
		}
	})
}
