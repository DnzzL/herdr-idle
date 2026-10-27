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
	LastChangeSeq uint64    `json:"last_change_seq"`
	LastStatus    string    `json:"last_status"`
	LastChangeAt  time.Time `json:"last_change_at"`
	// TermIgnored is set when the agent outlived SIGTERM; it is not
	// signalled again until its state changes.
	TermIgnored bool `json:"term_ignored,omitempty"`

	// Set once the pane is asleep.
	Kind       string    `json:"kind,omitempty"`
	SessionRef string    `json:"session_ref,omitempty"`
	ResumeArgv []string  `json:"resume_argv,omitempty"`
	AsleepAt   time.Time `json:"asleep_at,omitzero"`
	// CapturePath is the file holding the pane's screen from before it was
	// put asleep; it goes when the pane is closed.
	CapturePath string `json:"capture_path,omitempty"`
}

// state maps pane id to that pane's state.
type state map[string]paneState

// observe returns the state for this snapshot: asleep panes are kept as they
// are, even when herdr no longer lists them; every other agent pane keeps its
// clock while its seq and status hold, and starts it at now when they change
// or when the pane is new. An asleep pane listed under a real kind again was
// woken: herdr has dropped the stand-in report, so it is awake again with a
// clock starting now. Other panes no longer in the snapshot are forgotten.
func observe(prev state, agents []herdr.Agent, now time.Time) state {
	next := state{}
	for id, ps := range prev {
		if !ps.AsleepAt.IsZero() {
			next[id] = ps
		}
	}
	for _, a := range agents {
		ps, seen := prev[a.PaneID]
		woken := !ps.AsleepAt.IsZero()
		if woken && a.Agent == standInKind {
			continue // still asleep
		}
		if woken || !seen || ps.LastChangeSeq != a.StateChangeSeq || ps.LastStatus != a.Status {
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
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}
