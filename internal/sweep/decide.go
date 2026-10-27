package sweep

import (
	"fmt"
	"slices"
	"time"

	"github.com/DnzzL/herdr-idle/internal/config"
	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// whyAwake returns why the pane must stay awake, or "" when every rule lets
// it be put asleep.
func whyAwake(a herdr.Agent, ps paneState, now time.Time, cfg config.Config) string {
	if !ps.AsleepAt.IsZero() {
		return "asleep"
	}
	if cfg.Excludes(a.PaneID) || (a.Session != nil && cfg.Excludes(a.Session.Value)) {
		return "excluded"
	}
	if ps.stopping() && ps.TermIgnored {
		return "ignored SIGTERM, left alone"
	}
	if ps.stopping() {
		return "being put asleep"
	}
	if a.Focused {
		return "focused"
	}
	if a.Status != "idle" && a.Status != "done" {
		return "agent is " + a.Status
	}
	r, ok := recipes[a.Agent]
	if !ok {
		return "no resume recipe for " + a.Agent
	}
	if a.Session == nil || a.Session.Agent != a.Agent || !slices.Contains(r.refKinds, a.Session.Kind) || a.Session.Value == "" {
		return "no session ref"
	}
	if r.resume(a.Session.Value) == nil {
		return fmt.Sprintf("session ref %q cannot be resumed", a.Session.Value)
	}
	if quiet := now.Sub(ps.LastChangeAt); quiet < cfg.IdleAfter {
		return fmt.Sprintf("quiet for %s, needs %s", quiet.Truncate(time.Minute), cfg.IdleAfter)
	}
	return ""
}
