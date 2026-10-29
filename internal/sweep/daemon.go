package sweep

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DnzzL/herdr-idle/internal/config"
	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// Plugin is the part of herdr's plugin registry the daemon watches.
type Plugin interface {
	// PluginEnabled reports whether the plugin is installed and enabled. It
	// fails with herdr.ErrUnreachable once herdr's server is gone.
	PluginEnabled(ctx context.Context, id string) (bool, error)
}

// ErrAlreadyRunning means another daemon serves this herdr session.
var ErrAlreadyRunning = errors.New("a daemon already runs for this herdr session")

// checkEvery is how often the daemon checks that the plugin is still enabled
// and herdr still up, between sweeps.
const checkEvery = 30 * time.Second

// Daemon sweeps a herdr session on a schedule: herdr has no timer event.
type Daemon struct {
	// Sweeper does each sweep; its Config also sets the interval between
	// two sweeps, read again after each one.
	Sweeper Sweeper
	Plugin  Plugin
	// Log receives each sweep's outcomes and error, and failed plugin
	// checks. Nil drops them.
	Log func([]Outcome, error)
	// Upgraded, if set, reports whether the daemon's binary was replaced
	// since it last asked; the daemon then calls Reexec. Reexec replaces
	// the process and returns only on failure, when the daemon carries on
	// with the old build until the binary changes again.
	Upgraded func() bool
	Reexec   func() error
	// KeepClocks is set by a re-executed daemon: it never stopped watching,
	// so the quiet clocks hold.
	KeepClocks bool
}

// Run serves the session until ctx ends, the plugin is disabled or removed,
// or herdr's server is gone; it then returns nil. One daemon runs per state
// file, so per herdr session: a second one returns ErrAlreadyRunning at once,
// without sweeping.
//
// On start every quiet clock restarts, unless KeepClocks, since the agents
// may have worked unseen while no daemon ran, and the first sweep runs at once: it rearms
// the panes put asleep before a herdr restart. Between sweeps, every check
// also rearms, for a restart this daemon outlived.
func (d Daemon) Run(ctx context.Context) error {
	unlock, err := lock(ctx, d.Sweeper.StatePath+".daemon.lock", false)
	if errors.Is(err, errLocked) {
		return ErrAlreadyRunning
	}
	if err != nil {
		return err
	}
	defer unlock()
	if !d.KeepClocks {
		if err := resetClocks(ctx, d.Sweeper.StatePath); err != nil {
			return err
		}
	}
	log := d.Log
	if log == nil {
		log = func([]Outcome, error) {}
	}

	next := time.Now()
	for {
		if !time.Now().Before(next) {
			outcomes, err := d.Sweeper.Run(ctx)
			if ctx.Err() != nil {
				return nil
			}
			log(outcomes, err)
			next = time.Now().Add(d.interval())
		}
		if err := sleep(ctx, min(checkEvery, time.Until(next))); err != nil {
			return nil
		}
		if d.Upgraded != nil && d.Upgraded() {
			// The lock goes with the old image: its descriptor is
			// close-on-exec.
			log(nil, fmt.Errorf("binary replaced, re-executing: %w", d.Reexec()))
		}
		enabled, err := d.Plugin.PluginEnabled(ctx, PluginID)
		switch {
		case ctx.Err() != nil, errors.Is(err, herdr.ErrUnreachable):
			return nil
		case err != nil:
			log(nil, err) // herdr is there but did not answer: try again later
		case !enabled:
			return nil
		case time.Now().Before(next):
			// A herdr restart this daemon outlived (it never saw herdr
			// down) left the new server's daemon locked out: rearm now
			// rather than at the next sweep.
			outcomes, err := d.Sweeper.settle(ctx)
			if ctx.Err() != nil {
				return nil
			}
			if len(outcomes) > 0 || err != nil {
				log(outcomes, err)
			}
		}
	}
}

// EnsureDaemon calls start unless a daemon already serves the herdr
// session whose state file is statePath. Two callers racing may both
// start one: the daemon lock lets only one of them run.
func EnsureDaemon(statePath string, start func() error) error {
	unlock, err := lock(context.Background(), statePath+".daemon.lock", false)
	if errors.Is(err, errLocked) {
		return nil
	}
	if err != nil {
		return err
	}
	unlock()
	return start()
}

// interval is the configured time between sweeps. The sweep has already
// reported a config problem; the last valid config, or the default, holds.
func (d Daemon) interval() time.Duration {
	cfg, ok, _ := d.Sweeper.Config.Load()
	if !ok {
		cfg = config.Default()
	}
	return cfg.Interval
}

// resetClocks forgets every quiet clock, keeping only the asleep panes: the
// next sweep starts each clock afresh.
func resetClocks(ctx context.Context, statePath string) error {
	unlock, err := lockState(ctx, statePath)
	if err != nil {
		return err
	}
	defer unlock()
	st, err := loadState(statePath)
	if err != nil {
		return err
	}
	for id, ps := range st {
		if ps.AsleepAt.IsZero() {
			delete(st, id)
		}
	}
	return saveState(statePath, st)
}
