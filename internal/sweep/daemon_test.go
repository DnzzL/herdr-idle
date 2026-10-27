package sweep_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DnzzL/herdr-idle/internal/config"
	"github.com/DnzzL/herdr-idle/internal/herdr"
	"github.com/DnzzL/herdr-idle/internal/sweep"
)

// fakePlugin is herdr's plugin registry as the daemon sees it.
type fakePlugin struct {
	mu      sync.Mutex
	enabled bool
	err     error
	asked   []string
}

func (p *fakePlugin) PluginEnabled(_ context.Context, id string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, id)
	return p.enabled, p.err
}

func (p *fakePlugin) set(enabled bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled, p.err = enabled, err
}

// daemon runs d in the background until the test ends or it returns;
// the returned channel yields its result.
func startDaemon(t *testing.T, d sweep.Daemon) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Go(func() { done <- d.Run(ctx) })
	t.Cleanup(func() { cancel(); wg.Wait() })
	return done
}

// newDaemon returns a daemon over s whose config sets interval_minutes.
func newDaemon(t *testing.T, s sweep.Sweeper, intervalMinutes int) (sweep.Daemon, *fakePlugin) {
	t.Helper()
	writeConfig(t, s, fmt.Sprintf("interval_minutes = %d\n", intervalMinutes))
	p := &fakePlugin{enabled: true}
	return sweep.Daemon{Sweeper: s, Plugin: p}, p
}

func sigterms(h *fakeHerdr) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, e := range h.events {
		if strings.HasPrefix(e, "SIGTERM") {
			n++
		}
	}
	return n
}

func returned(done <-chan error) (error, bool) {
	select {
	case err := <-done:
		return err, true
	default:
		return nil, false
	}
}

func TestDaemonSweepsAtStartThenEveryInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		d, _ := newDaemon(t, s, 7)
		startDaemon(t, d)

		// Clock starts at the first sweep (0m); sweeps at 7m, 14m … 63m:
		// the first one past the 60m quiet period is at 63m.
		time.Sleep(62 * time.Minute)
		synctest.Wait()
		if n := sigterms(h); n != 0 {
			t.Fatalf("asleep before the 63m sweep: %q", h.events)
		}
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if n := sigterms(h); n != 1 {
			t.Fatalf("not asleep after the 63m sweep: %q", h.events)
		}
	})
}

func TestDaemonExitsOncePluginIsDisabledRemovedOrHerdrGone(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		err     error
	}{
		{"disabled or removed", false, nil},
		{"herdr server gone", true, fmt.Errorf("dial: %w", herdr.ErrUnreachable)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a, p := claudePane("claude")
				h, s := newWorld(t, a, p)
				d, plugin := newDaemon(t, s, 10)
				done := startDaemon(t, d)

				time.Sleep(5 * time.Minute)
				plugin.set(tt.enabled, tt.err)
				time.Sleep(time.Minute)
				synctest.Wait()
				err, ok := returned(done)
				if !ok {
					t.Fatal("daemon still running a minute later")
				}
				if err != nil {
					t.Fatalf("daemon returned %v, want nil", err)
				}
				if plugin.asked[0] != "dnzzl.idle" {
					t.Errorf("asked about plugin %q", plugin.asked[0])
				}
				time.Sleep(2 * time.Hour)
				if n := sigterms(h); n != 0 {
					t.Fatalf("swept after exiting: %q", h.events)
				}
			})
		})
	}
}

func TestDaemonKeepsRunningThroughAFailedPluginCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		d, plugin := newDaemon(t, s, 10)
		done := startDaemon(t, d)

		plugin.set(false, errors.New("herdr plugin.list: timeout"))
		time.Sleep(5 * time.Minute)
		plugin.set(true, nil)
		time.Sleep(56 * time.Minute)
		synctest.Wait()
		if _, ok := returned(done); ok {
			t.Fatal("daemon exited on a failed check")
		}
		if n := sigterms(h); n != 1 {
			t.Fatalf("sweeps stopped: %q", h.events)
		}
	})
}

func TestSecondDaemonForTheSameSessionExitsWithoutSweeping(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		first, _ := newDaemon(t, s, 10)
		startDaemon(t, first)
		synctest.Wait()

		// The second daemon gets a herdr of its own: anything it does shows.
		h2 := &fakeHerdr{agents: h.agents, procs: map[string]herdr.ProcessInfo{pane: p}, exitAt: map[string]time.Time{}}
		s2 := s
		s2.Herdr, s2.Signaler = h2, &fakeSignaler{h: h2}
		s2.Config = &config.File{Path: filepath.Join(t.TempDir(), "config.toml")}
		second, _ := newDaemon(t, s2, 1)
		if err := second.Run(t.Context()); !errors.Is(err, sweep.ErrAlreadyRunning) {
			t.Fatalf("second daemon returned %v, want ErrAlreadyRunning", err)
		}
		time.Sleep(2 * time.Hour)
		synctest.Wait()
		if len(h2.events) != 0 {
			t.Fatalf("second daemon acted: %q", h2.events)
		}
	})
}

func TestEachSessionGetsItsOwnDaemon(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h1, s1 := newWorld(t, a, p)
		h2, s2 := newWorld(t, a, p) // another session: its own state file
		d1, _ := newDaemon(t, s1, 10)
		d2, _ := newDaemon(t, s2, 10)
		done1 := startDaemon(t, d1)
		done2 := startDaemon(t, d2)

		time.Sleep(61 * time.Minute)
		synctest.Wait()
		for i, done := range []<-chan error{done1, done2} {
			if err, ok := returned(done); ok {
				t.Fatalf("daemon %d returned %v", i+1, err)
			}
		}
		if sigterms(h1) != 1 || sigterms(h2) != 1 {
			t.Fatalf("both sessions should sweep: %q / %q", h1.events, h2.events)
		}
	})
}

func TestDaemonRestartResetsQuietClocks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		first, _ := newDaemon(t, s, 10)
		ctx, stop := context.WithCancel(t.Context())
		var wg sync.WaitGroup
		wg.Go(func() {
			if err := first.Run(ctx); err != nil {
				t.Errorf("first daemon: %v", err)
			}
		})
		time.Sleep(50 * time.Minute)
		stop()
		wg.Wait()

		// The agent may have worked unseen while no daemon ran: the new
		// daemon gives every pane a full quiet period from its start.
		second, _ := newDaemon(t, s, 10)
		startDaemon(t, second)
		time.Sleep(59 * time.Minute)
		synctest.Wait()
		if n := sigterms(h); n != 0 {
			t.Fatalf("asleep on a clock from before the restart: %q", h.events)
		}
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if n := sigterms(h); n != 1 {
			t.Fatalf("not asleep after a full quiet period: %q", h.events)
		}
	})
}

func TestDaemonRearmsAsleepPanesAtStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, p := claudePane("claude")
		h, s := newWorld(t, a, p)
		putAsleep(t, s)
		h.restart(pane)
		h.events = nil

		d, _ := newDaemon(t, s, 10)
		startDaemon(t, d)
		time.Sleep(5 * time.Second)
		synctest.Wait()
		h.mu.Lock()
		defer h.mu.Unlock()
		want := "send_text " + pane + " claude --resume " + sessionID
		if len(h.events) != 3 || h.events[2] != want {
			t.Fatalf("not rearmed at start: %q", h.events)
		}
	})
}
