package sweep

import (
	"fmt"
	"slices"
	"time"

	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// whyAwake returns why the pane must stay awake, or "" when every rule lets
// it be put asleep.
func whyAwake(a herdr.Agent, ps paneState, now time.Time, idleAfter time.Duration) string {
	if !ps.AsleepAt.IsZero() {
		return "asleep"
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
	if quiet := now.Sub(ps.LastChangeAt); quiet < idleAfter {
		return fmt.Sprintf("quiet for %s, needs %s", quiet.Truncate(time.Minute), idleAfter)
	}
	return ""
}
