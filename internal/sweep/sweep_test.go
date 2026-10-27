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
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DnzzL/herdr-idle/internal/config"
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
	mu     sync.Mutex // sweeps may run side by side
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
	// typed is everything sent to each pane's terminal, in order; a line
	// run with Enter (a command, not a prefill) lands only here.
	typed map[string][]string
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
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.screens[paneID], f.readErr
}

// herdrLag is how long after an agent exits herdr's process detector
// notices; it wipes plugin reports made before then. The snapshot stops
// listing the agent sooner, so it cannot tell when that has happened.
const herdrLag = 500 * time.Millisecond

func (f *fakeHerdr) Snapshot(context.Context) ([]herdr.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.exitAt[r.PaneID]; ok && !f.herdrNoticedExit(r.PaneID) {
		return nil // accepted, then wiped when the detector catches up
	}
	f.events = append(f.events, "report_agent "+r.PaneID)
	f.reports = append(f.reports, r)
	return nil
}

func (f *fakeHerdr) ReportMetadata(_ context.Context, r herdr.MetadataReport) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "report_metadata "+r.PaneID)
	f.meta = append(f.meta, r)
	return nil
}

func (f *fakeHerdr) SendText(_ context.Context, paneID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.exitAt[paneID]; ok && !f.agentGone(paneID) {
		f.events = append(f.events, "send_text while the agent still runs")
		return nil
	}
	f.typed[paneID] = append(f.typed[paneID], text)
	if strings.HasSuffix(text, "\r") {
		return nil
	}
	f.events = append(f.events, "send_text "+paneID+" "+text)
	return nil
}

// fakeSignaler stands in for the OS: SIGTERM to a pane's foreground group
// ends the agent 2s later, handing the terminal back to the shell.
type fakeSignaler struct {
	h *fakeHerdr
	// onTerminate, if set, runs as the signal is sent.
	onTerminate func()
}

func (s *fakeSignaler) Children(_ context.Context, pid int) ([]herdr.Process, error) {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	return s.h.children[pid], nil
}

func (s *fakeSignaler) Terminate(pgid int) error {
	if s.onTerminate != nil {
		s.onTerminate()
	}
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
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
		typed:        map[string][]string{},
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
		Config:     &config.File{Path: filepath.Join(t.TempDir(), "config.toml")},
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
	time.Sleep(61 * time.Minute)
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
		{"prompt dropped, flag values kept",
			[]string{"claude", "--model", "opus", "fix the login bug", "--permission-mode", "plan"},
			"claude --resume " + sessionID + " --model opus --permission-mode plan"},
		{"optional value taken unless an option follows",
			[]string{"claude", "-d", "hooks", "--debug", "--verbose", "go on"},
			"claude --resume " + sessionID + " -d hooks --debug --verbose"},
		{"variadic values kept until the next option",
			[]string{"claude", "--add-dir", "../a", "../b", "--model", "opus"},
			"claude --resume " + sessionID + " --add-dir ../a ../b --model opus"},
		{"everything after -- dropped",
			[]string{"claude", "--model", "opus", "--", "--not-a-flag prompt"},
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

// TestEveryResumableKindIsPrefilledWithItsRecipe holds one row per kind herdr
// 0.9.5 resumes: an argv with flags, a stale resume token and a prompt gives
// herdr's planner command (src/agent_resume.rs) plus the flags. The option
// lists behind each row, and their sources, are in recipes.go.
func TestEveryResumableKindIsPrefilledWithItsRecipe(t *testing.T) {
	const (
		id   = "s-new"
		path = "/home/me/.pi/sessions/s-new.jsonl"
		cur  = "/home/me/.local/share/cursor-agent/versions/2026.10.1/"
	)
	tests := []struct {
		kind, refKind, ref string
		argv               []string
		want               string
	}{
		{"claude", "id", id, []string{"claude", "--model", "opus", "--resume", "old", "fix it"},
			"claude --resume s-new --model opus"},
		{"codex", "id", id,
			[]string{"codex", "--sandbox", "workspace-write", "resume", "--last", "-c", "model=o3", "go on"},
			"codex resume s-new --sandbox workspace-write -c model=o3"},
		{"copilot", "id", id,
			[]string{"copilot", "--model", "gpt-5", "--resume=old", "-i", "fix it", "--allow-tool", "shell"},
			"copilot --resume=s-new --model gpt-5 --allow-tool shell"},
		{"devin", "id", id,
			[]string{"devin", "--permission-mode", "accept-edits", "-r", "old", "--", "fix it"},
			"devin --resume s-new --permission-mode accept-edits"},
		{"droid", "id", id, []string{"droid", "--auto", "high", "--resume", "old", "review app.tsx"},
			"droid --resume s-new --auto high"},
		{"kimi", "id", id, []string{"kimi", "-m", "k2", "--session", "old", "-p", "fix it", "--yolo"},
			"kimi --session s-new -m k2 --yolo"},
		{"mastracode", "id", id,
			[]string{"node", "/usr/lib/node_modules/mastracode/dist/cli.js", "--mode", "plan", "resume", "old"},
			"mastracode --thread s-new --mode plan"},
		{"pi", "path", path,
			[]string{"pi", "--model", "sonnet", "--session", "/old.jsonl", "@notes.md", "fix it"},
			"pi --session " + path + " --model sonnet"},
		{"omp", "path", path, []string{"omp", "--thinking", "high", "-r", "old", "fix it"},
			"omp --resume=" + path + " --thinking high"},
		{"hermes", "id", id, []string{"hermes", "chat", "-m", "hermes-4", "--resume", "old", "-q", "hi"},
			"hermes --resume s-new -m hermes-4"},
		{"opencode", "id", id,
			[]string{"opencode", "-m", "anthropic/sonnet", "-s", "old", "--prompt", "fix it", "."},
			"opencode --session s-new -m anthropic/sonnet"},
		{"qodercli", "id", id, []string{"qodercli", "--permission-mode", "plan", "-r", "old", "fix it"},
			"qodercli --resume s-new --permission-mode plan"},
		{"qwen", "id", id, []string{"qwen", "--approval-mode", "yolo", "--resume", "old", "fix it"},
			"qwen --resume s-new --approval-mode yolo"},
		{"kilo", "id", id, []string{"kilo", "-m", "x/y", "--continue", "--fork", "--prompt", "fix"},
			"kilo --session s-new -m x/y"},
		{"cursor", "id", id,
			[]string{cur + "node", cur + "index.js", "--model", "gpt-5", "--resume", "old", "fix it"},
			"cursor-agent --resume s-new --model gpt-5"},
		{"agy", "id", id, []string{"agy", "--mode", "plan", "-c", "--conversation", "old", "fix"},
			"agy --conversation s-new --mode plan"},
		{"grok", "id", id, []string{"grok", "-m", "grok-4", "--continue", "--fork-session", "fix"},
			"grok --resume s-new -m grok-4"},
		{"letta", "id", id,
			[]string{"letta", "-m", "sonnet", "--agent", "agent-1", "--conversation", "old", "hi"},
			"letta --conversation s-new -m sonnet"},
		{"letta", "id", "default:agent-9", []string{"letta", "--new", "-a", "agent-1"},
			"letta --conversation default --agent agent-9"},
		{"pi", "id", "0b1c", []string{"pi", "-c"}, "pi --session 0b1c"},
	}
	for _, tt := range tests {
		t.Run(tt.kind+" "+tt.want, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a, p := claudePane(tt.argv...)
				a.Agent = tt.kind
				a.Session = &herdr.AgentSession{
					Source: "herdr:" + tt.kind, Agent: tt.kind, Kind: tt.refKind, Value: tt.ref,
				}
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
		{"path ref for a kind resumed by id", func(a *herdr.Agent) { a.Session.Kind = "path" },
			2 * time.Hour, "no session ref"},
		{"letta default conversation without an agent",
			func(a *herdr.Agent) {
				a.Agent, a.Session.Agent, a.Session.Value = "letta", "letta", "default:"
			},
			2 * time.Hour, `session ref "default:" cannot be resumed`},
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

// writeConfig replaces the config file the sweeper reads.
func writeConfig(t *testing.T, s sweep.Sweeper, toml string) {
	t.Helper()
	if err := os.WriteFile(s.Config.Path, []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestExcludedPaneOrSessionIsNeverPutAsleep(t *testing.T) {
	for _, id := range []string{pane, sessionID} {
		t.Run(id, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a, p := claudePane("claude")
				h, s := newWorld(t, a, p)
				writeConfig(t, s, `exclude = ["w9:p9", "`+id+`"]`)
				run(t, s)
				time.Sleep(3 * time.Hour)
				out, err := s.Run(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if len(h.events) != 0 || out[0].Reason != "excluded" {
					t.Fatalf("events %q, outcome %+v", h.events, out)
				}
			})
		})
	}
}

func TestDryRunSaysWhatItWouldDoAndDoesNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude", "--model", "opus")
		h, s := newWorld(t, a, p)
		writeConfig(t, s, "dry_run = true\n")
		run(t, s)
		time.Sleep(61 * time.Minute)
		out, err := s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.events) != 0 {
			t.Fatalf("dry run acted: %q", h.events)
		}
		want := "would be put asleep, resume with: claude --resume " + sessionID + " --model opus"
		if out[0].Asleep || out[0].Reason != want {
			t.Fatalf("outcome %+v, want reason %q", out[0], want)
		}

		// Turning dry_run off acts on the next sweep: the clock kept running.
		writeConfig(t, s, "dry_run = false\n")
		run(t, s)
		if len(h.events) != 4 {
			t.Fatalf("events after dry_run off: %q", h.events)
		}
	})
}

func TestConfigChangeAppliesOnTheNextSweep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		writeConfig(t, s, "# partial file: the rest stays default\nidle_minutes = 90 # longer\n")
		run(t, s)
		time.Sleep(61 * time.Minute)
		run(t, s)
		if len(h.events) != 0 {
			t.Fatalf("asleep after 61m with idle_minutes = 90: %q", h.events)
		}

		writeConfig(t, s, "idle_minutes = 30\n")
		run(t, s)
		if len(h.events) != 4 {
			t.Fatalf("not asleep after 61m with idle_minutes = 30: %q", h.events)
		}
	})
}

func TestInvalidConfigKeepsThePreviousOneAndReportsEveryProblem(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		writeConfig(t, s, "dry_run = true\n")
		run(t, s)

		writeConfig(t, s, `idle_minutes = "soon"
dry_run = true
dryrun = false
exclude = "w1:p1"
interval_minutes = 0
`)
		time.Sleep(61 * time.Minute)
		_, err := s.Run(t.Context())
		if len(h.events) != 0 {
			t.Fatalf("invalid file dropped the previous dry_run: %q", h.events)
		}
		if err == nil {
			t.Fatal("invalid config not reported")
		}
		for _, want := range []string{
			"line 1: idle_minutes", "line 3: dryrun: unknown key",
			"line 4: exclude", "line 5: interval_minutes",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error lacks %q:\n%v", want, err)
			}
		}
		if strings.Contains(err.Error(), "line 2") {
			t.Errorf("valid line reported: %v", err)
		}
	})
}

func TestInvalidConfigWithNoPreviousOneDoesNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		run(t, s)
		time.Sleep(61 * time.Minute)
		// A fresh process (one-shot sweep, new daemon) has no previous
		// config: falling back to defaults could turn a dry run real.
		s.Config = &config.File{Path: s.Config.Path}
		writeConfig(t, s, "dry_run = ture\n")
		out, err := s.Run(t.Context())
		if err == nil || len(out) != 0 || len(h.events) != 0 {
			t.Fatalf("err %v, outcomes %+v, events %q", err, out, h.events)
		}
	})
}

func TestTemplateConfigMeansDefaults(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		writeConfig(t, s, config.Template)
		putAsleep(t, s)
		if len(h.events) != 4 {
			t.Fatalf("events: %q", h.events)
		}
	})
}

func TestExcludeArrayCanSpanLines(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		writeConfig(t, s, `exclude = [
  "w9:p9", # a loop agent
  'w1:p1',
]
`)
		putAsleep(t, s)
		if len(h.events) != 0 {
			t.Fatalf("excluded pane acted on: %q", h.events)
		}
	})
}

func TestPreviewGivesEveryAgentPaneItsReasonAndChangesNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		focused := a
		focused.PaneID, focused.Focused = "w1:p2", true
		h.agents = append(h.agents, focused)
		run(t, s)
		time.Sleep(61 * time.Minute)

		out, err := s.Preview(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		want := []sweep.Outcome{
			{PaneID: pane, Agent: "claude", Reason: "would be put asleep, resume with: claude --resume " + sessionID},
			{PaneID: "w1:p2", Agent: "claude", Reason: "focused"},
		}
		if !slices.Equal(out, want) || len(h.events) != 0 {
			t.Fatalf("preview:\n got %+v\nwant %+v\nevents %q", out, want, h.events)
		}

		// A preview leaves the quiet clocks alone: on a fresh state file
		// the first real sweep still starts them.
		h2, s2 := newWorld(t, a, p)
		if _, err := s2.Preview(t.Context()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(61 * time.Minute)
		run(t, s2)
		if len(h2.events) != 0 {
			t.Fatalf("preview started the quiet clock: %q", h2.events)
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

		shown := h.typed[a.PaneID]
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

func TestClosingAnAsleepPaneRemovesItsScreenCapture(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		h.screens[pane] = "screen"
		putAsleep(t, s)
		captures, _ := filepath.Glob(filepath.Join(s.CaptureDir, "*"))
		if len(captures) != 1 {
			t.Fatalf("captures = %q, want one", captures)
		}
		if err := s.PaneClosed(t.Context(), pane); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(captures[0]); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("capture still there after the pane closed: %v", err)
		}
	})
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

		asleep := h.typed[pane]
		h.restart(pane)
		h.events, h.typed[pane] = nil, nil
		out, err := s.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		// Scrollback is gone with the restart: the capture file is the only
		// record left, so its path is shown again before the prefill.
		if !slices.Equal(h.typed[pane], asleep) {
			t.Errorf("typed after restart %q, want as when put asleep %q", h.typed[pane], asleep)
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

// addPane adds a second quiet claude pane to the world, its agent in group
// pgid.
func addPane(h *fakeHerdr, id string, pgid int) {
	a, p := claudePane("claude")
	a.PaneID = id
	p.ForegroundGroupID = pgid
	p.Foreground[1].PID = pgid
	h.agents = append(h.agents, a)
	h.procs[id] = p
}

func TestPaneClosedDuringASweepStaysForgotten(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		putAsleep(t, s)

		// A second pane is being put asleep by a sweep; while that sweep
		// waits for the agent to exit, herdr closes the first pane.
		const other = "w1:p2"
		addPane(h, other, 5151)
		run(t, s)
		time.Sleep(61 * time.Minute)
		var wg sync.WaitGroup
		s.Signaler.(*fakeSignaler).onTerminate = func() {
			wg.Go(func() {
				if err := s.PaneClosed(t.Context(), pane); err != nil {
					t.Errorf("pane closed: %v", err)
				}
			})
		}
		run(t, s)
		wg.Wait()

		st := readState(t, s)
		if st[pane] != nil {
			t.Errorf("closed pane resurrected by the sweep: %v", st[pane])
		}
		if st[other]["asleep_at"] == nil {
			t.Errorf("pane put asleep meanwhile lost: %v", st[other])
		}
	})
}

func TestSweepsSideBySidePutAPaneAsleepOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		run(t, s)
		time.Sleep(61 * time.Minute)

		// The daemon's sweep and an idle-now sweep (another process, so
		// its own config file reader) start together.
		var wg sync.WaitGroup
		for range 2 {
			s := s
			s.Config = &config.File{Path: s.Config.Path}
			wg.Go(func() {
				if _, err := s.Run(t.Context()); err != nil {
					t.Errorf("sweep: %v", err)
				}
			})
		}
		wg.Wait()

		var sigterms int
		for _, e := range h.events {
			if strings.HasPrefix(e, "SIGTERM") {
				sigterms++
			}
		}
		if sigterms != 1 {
			t.Errorf("SIGTERM sent %d times: %q", sigterms, h.events)
		}
		if st := readState(t, s); st[pane]["asleep_at"] == nil {
			t.Errorf("asleep pane lost from the state: %v", st[pane])
		}
	})
}
