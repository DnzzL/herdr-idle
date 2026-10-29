package sweep

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// paneState is what the plugin remembers about one pane between sweeps.
type paneState struct {
	// Quiet clock: herdr has no timestamp for a state change, so the sweep
	// notes when it first saw the current seq and status.
	LastChangeSeq uint64       `json:"last_change_seq"`
	LastStatus    herdr.Status `json:"last_status"`
	LastChangeAt  time.Time    `json:"last_change_at"`

	// Saved before the agent is sent SIGTERM, at StoppedAt: what resumes it.
	Kind       herdr.Kind `json:"kind,omitempty"`
	SessionRef string     `json:"session_ref,omitempty"`
	ResumeArgv []string   `json:"resume_argv,omitempty"`
	Cwd        string     `json:"cwd,omitempty"`
	StoppedAt  time.Time  `json:"stopped_at,omitzero"`
	// TermIgnored is set when the agent outlived SIGTERM; it is not
	// signalled again until its state changes.
	TermIgnored bool `json:"term_ignored,omitempty"`
	// AsleepAt is set once the agent is gone and the pane reported asleep.
	AsleepAt time.Time `json:"asleep_at,omitzero"`
	// PrefilledShell is the pid of the shell last prefilled: a herdr
	// restart brings a new shell, which needs the prefill again.
	PrefilledShell int `json:"prefilled_shell,omitempty"`
	// CapturePath is the file holding the pane's screen from before it was
	// last put asleep; it goes when the pane is closed.
	CapturePath string `json:"capture_path,omitempty"`
	// DirGone is set once the pane was told its folder no longer exists.
	DirGone bool `json:"dir_gone,omitempty"`
}

// stopping reports whether the pane's agent was sent SIGTERM and the pane
// is not asleep yet: it is put asleep once the agent is gone.
func (ps paneState) stopping() bool {
	return !ps.StoppedAt.IsZero() && ps.AsleepAt.IsZero()
}

// prefill is the command line typed at the pane's prompt: back to the
// agent's directory, then its resume command.
func (ps paneState) prefill() string {
	cmd := shellJoin(ps.ResumeArgv)
	if ps.Cwd == "" {
		return cmd
	}
	return "cd " + shellQuote(ps.Cwd) + " && " + cmd
}

// state maps pane id to that pane's state.
type state map[herdr.PaneID]paneState

// observe returns the state for this snapshot. Asleep and stopping panes
// are kept, even when herdr no longer lists them. Every other agent pane
// keeps its clock while its seq and status hold, and starts it at now when
// they change or when the pane is new. An asleep pane listed under a real
// kind again was woken: herdr has dropped the stand-in report, so it is
// awake again with a clock starting now. A stopping pane listed under its
// kind still runs its agent: it stays stopping until the agent exits, unless
// the agent ignored SIGTERM and has changed state since. Other panes no
// longer in the snapshot are forgotten.
func observe(prev state, agents []herdr.Agent, now time.Time) state {
	next := state{}
	for id, ps := range prev {
		if !ps.StoppedAt.IsZero() || !ps.AsleepAt.IsZero() {
			next[id] = ps
		}
	}
	for _, a := range agents {
		ps, seen := prev[a.PaneID]
		asleep := !ps.AsleepAt.IsZero()
		if a.Agent == standInKind && (asleep || ps.stopping()) {
			continue // ours
		}
		changed := !seen || ps.LastChangeSeq != a.StateChangeSeq || ps.LastStatus != a.Status
		if asleep || changed && (!ps.stopping() || ps.TermIgnored) {
			// The capture outlives the wake, so closing the pane still
			// removes it; the next sleep overwrites it.
			ps = paneState{LastChangeSeq: a.StateChangeSeq, LastStatus: a.Status, LastChangeAt: now, CapturePath: ps.CapturePath}
		}
		next[a.PaneID] = ps
	}
	return next
}

func loadState(path string) (state, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s state
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}
	if s == nil {
		s = state{}
	}
	return s, nil
}

// saveState writes the file atomically, so a crash mid-write never loses the
// record of which panes are asleep.
func saveState(path string, s state) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	tmp := path + ".tmp"
	if err := writeSynced(tmp, b); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// writeSynced writes b to path and flushes it to disk, so a rename over the
// old file never leaves an empty one after a crash.
func writeSynced(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
