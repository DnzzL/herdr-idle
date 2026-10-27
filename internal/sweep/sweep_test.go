package sweep_test

import (
	"context"
	"errors"
	"fmt"
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
	agents  []herdr.Agent
	procs   map[string]herdr.ProcessInfo
	exitAt  map[string]time.Time // pane → when its terminated agent is gone
	events  []string
	reports []herdr.AgentReport
	meta    []herdr.MetadataReport
	// children is the OS side: each pid's child processes, whatever their
	// process group.
	children map[int][]herdr.Process
	screens  map[string]string // pane → its recent screen text
	readErr  error
	// ignoreTERM lists process groups that survive SIGTERM.
	ignoreTERM map[int]bool
	// captureDir is where the sweep saves screens; onDiskAtTerm is what it
	// held each time a SIGTERM went out (path → content).
	captureDir   string
	onDiskAtTerm map[string]string
}

func (f *fakeHerdr) ReadRecent(_ context.Context, paneID string) (string, error) {
	return f.screens[paneID], f.readErr
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
		if !f.agentGone(a.PaneID) {
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

func (s *fakeSignaler) Children(_ context.Context, pid int) ([]herdr.Process, error) {
	return s.h.children[pid], nil
}

func (s *fakeSignaler) Terminate(pgid int) error {
	s.h.events = append(s.h.events, fmt.Sprintf("SIGTERM %d", pgid))
	files, _ := filepath.Glob(filepath.Join(s.h.captureDir, "*"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		s.h.onDiskAtTerm[f] = string(b)
	}
	for id, p := range s.h.procs {
		if p.ForegroundGroupID == pgid && !s.h.ignoreTERM[pgid] {
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
		agents:       []herdr.Agent{a},
		procs:        map[string]herdr.ProcessInfo{a.PaneID: p},
		exitAt:       map[string]time.Time{},
		children:     map[int][]herdr.Process{},
		screens:      map[string]string{},
		captureDir:   filepath.Join(t.TempDir(), "captures"),
		onDiskAtTerm: map[string]string{},
	}
	sys := &fakeSignaler{h: h}
	s := sweep.Sweeper{
		Herdr:      h,
		Signaler:   sys,
		Processes:  sys,
		StatePath:  filepath.Join(t.TempDir(), "state.json"),
		CaptureDir: h.captureDir,
		IdleAfter:  60 * time.Minute,
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
			"notice",
			"send_text " + pane + " claude --resume " + sessionID + " --model opus",
		}
		got := slices.Clone(h.events)
		if len(got) > 3 && strings.HasPrefix(got[3], "send_text "+pane+"  echo ") {
			got[3] = "notice" // its text is checked by the capture test
		}
		if !slices.Equal(got, want) {
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
		if strings.ContainsAny(h.events[4], "\r\n") {
			t.Errorf("prefill pressed Enter: %q", h.events[4])
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
				if len(h.events) != 5 || h.events[4] != want {
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
		if len(h.events) != 5 {
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
		if len(h.events) != 5 {
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
		if len(h.events) != 5 || out[0].Reason != "asleep" {
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

func TestPaneWithRunningWorkIsLeftAlone(t *testing.T) {
	tests := []struct {
		name  string
		child []string
	}{
		{"dev server", []string{"npm", "run", "dev"}},
		{"shell", []string{"/bin/zsh", "-c", "go test ./..."}},
		{"server under a path named mcp", []string{"node", "/Users/me/mcp-tools/server.js"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" in the foreground group", func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a, p := claudePane("claude")
				p.Foreground = append(p.Foreground, herdr.Process{PID: agentPGID + 9, Argv: tt.child})
				h, s := newWorld(t, a, p)
				assertLeftAlone(t, h, s, "running "+strings.Join(tt.child, " "))
			})
		})
		// The agent's tool commands run in their own process group: herdr
		// does not list them, the OS does.
		t.Run(tt.name+" in its own group", func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a, p := claudePane("claude")
				h, s := newWorld(t, a, p)
				h.children[agentPGID] = []herdr.Process{
					{PID: agentPGID + 1, Argv: []string{"bun", "/x/backlog", "mcp", "start"}},
					{PID: 9000, Argv: tt.child},
				}
				assertLeftAlone(t, h, s, "running "+strings.Join(tt.child, " "))
			})
		})
	}
}

// assertLeftAlone sweeps once the pane has been quiet long enough and fails
// unless it got no action, for reason.
func assertLeftAlone(t *testing.T, h *fakeHerdr, s sweep.Sweeper, reason string) {
	t.Helper()
	run(t, s)
	time.Sleep(2 * time.Hour)
	out, err := s.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 0 {
		t.Fatalf("acted: %q", h.events)
	}
	if len(out) != 1 || out[0].Asleep || out[0].Reason != reason {
		t.Fatalf("outcome = %+v, want reason %q", out, reason)
	}
}

func TestMCPAndLanguageServersDoNotKeepAPaneAwake(t *testing.T) {
	servers := [][]string{
		{"/Users/me/.local/bin/codebase-memory-mcp"},
		{"npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		{"uvx", "mcp-server-fetch"},
		{"/usr/local/bin/typescript-language-server", "--stdio"},
		{"pyright-langserver", "--stdio"},
		{"gopls"},
		{"rust-analyzer"},
	}
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		var procs []herdr.Process
		for i, argv := range servers {
			procs = append(procs, herdr.Process{PID: agentPGID + 10 + i, Argv: argv})
		}
		p.Foreground = append(p.Foreground, procs...)
		h, s := newWorld(t, a, p)
		h.children[agentPGID] = procs
		putAsleep(t, s)
		if len(h.events) == 0 || h.events[0] != "SIGTERM 4242" {
			t.Fatalf("not put asleep: %q", h.events)
		}
	})
}

func TestScreenIsSavedBeforeSIGTERMAndItsPathShownBeforeThePrefill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude", "--model", "opus")
		h, s := newWorld(t, a, p)
		b, q := a, p // a second pane: captures must not overwrite each other
		b.PaneID, q.ForegroundGroupID = "w1:p2", 5000
		q.Foreground = []herdr.Process{{PID: 5000, Argv: []string{"claude"}}}
		h.agents, h.procs[b.PaneID] = append(h.agents, b), q
		h.screens[a.PaneID] = "> draft I never sent\n"
		h.screens[b.PaneID] = "other pane\n"

		putAsleep(t, s)

		var shown []string
		for _, e := range h.events {
			if strings.HasPrefix(e, "send_text "+a.PaneID+" ") {
				shown = append(shown, strings.TrimPrefix(e, "send_text "+a.PaneID+" "))
			}
		}
		if len(shown) != 2 || shown[1] != "claude --resume "+sessionID+" --model opus" {
			t.Fatalf("pane %s got %q, want a notice then the prefill", a.PaneID, shown)
		}
		notice, ok := strings.CutPrefix(shown[0], " echo 'herdr-idle: screen saved to ")
		path, ok2 := strings.CutSuffix(notice, "'\r")
		if !ok || !ok2 {
			t.Fatalf("notice = %q, want an echo of the capture path, run with Enter", shown[0])
		}
		if got, ok := h.onDiskAtTerm[path]; !ok || got != h.screens[a.PaneID] {
			t.Fatalf("at SIGTERM, %s held %q (exists %v); captures: %q", path, got, ok, h.onDiskAtTerm)
		}
		if filepath.Dir(path) != s.CaptureDir {
			t.Errorf("capture %s is outside %s", path, s.CaptureDir)
		}
		var other int
		for f, c := range h.onDiskAtTerm {
			if f != path && c == h.screens[b.PaneID] {
				other++
			}
		}
		if other != 1 {
			t.Errorf("the second pane has no capture of its own: %q", h.onDiskAtTerm)
		}
	})
}

func TestNoSIGTERMWhenTheScreenCannotBeSaved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		h.readErr = errors.New("agent.read failed")
		run(t, s)
		time.Sleep(2 * time.Hour)
		out, err := s.Run(t.Context())
		if !errors.Is(err, h.readErr) {
			t.Errorf("err = %v, want the read error", err)
		}
		if len(h.events) != 0 || out[0].Asleep {
			t.Fatalf("acted without a capture: %q", h.events)
		}
	})
}

func TestAgentAliveAfterSIGTERMIsLeftAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		h.ignoreTERM = map[int]bool{agentPGID: true}
		run(t, s)
		time.Sleep(2 * time.Hour)

		out, err := s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		// Only Terminate exists on the signaler: no SIGKILL can be sent.
		if !slices.Equal(h.events, []string{"SIGTERM 4242"}) {
			t.Fatalf("events = %q, want one SIGTERM and nothing else", h.events)
		}
		want := "agent still running 30s after SIGTERM, left alone"
		if out[0].Asleep || out[0].Reason != want {
			t.Fatalf("outcome = %+v, want reason %q", out[0], want)
		}

		time.Sleep(time.Hour)
		out, err = s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.events) != 1 {
			t.Fatalf("signalled again: %q", h.events)
		}
		if out[0].Reason != "ignored SIGTERM, left alone" {
			t.Fatalf("outcome = %+v", out[0])
		}
	})
}
