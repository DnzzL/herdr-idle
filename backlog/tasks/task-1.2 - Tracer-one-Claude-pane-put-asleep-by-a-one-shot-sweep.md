---
id: TASK-1.2
title: 'Tracer: one Claude pane put asleep by a one-shot sweep'
status: Done
assignee:
  - '@claude'
created_date: '2026-10-27 09:06'
updated_date: '2026-10-27 13:39'
labels:
  - ready-for-agent
dependencies:
  - TASK-1.1
parent_task_id: TASK-1
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
First end-to-end path through every layer, for Claude only, run on demand (no daemon yet). See TASK-1 for the full spec.

## What to build

Running `herdr-idle sweep` once puts every qualifying Claude pane asleep: its foreground process group gets SIGTERM, the pane is reported to herdr with state text `asleep` (no `resume_argv`), and the shell line is prefilled with the original argv minus any existing `--resume <id>`, plus `--resume <session id>`. Minimal rules only: herdr status idle or done, pane not focused, quiet for at least the default idle period, session ref present. The quiet clock is tracked by the plugin from `state_change_seq` changes in its state file. The plugin installs locally with `herdr plugin link` and a local `go build`.

Before code: record the ADR (SIGTERM + plugin-owned resume table, rejected: relying on herdr restart-only resume). Establish the single test seam: a sweep run against a hand-written fake herdr client, a fake process signaler and a fake clock (`testing/synctest`). Follow the DnzzL Effect-style Go guide "Always" tier.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 ADR recorded before implementation, naming the rejected alternative
- [x] #2 A qualifying Claude pane gets SIGTERM on its foreground process group, an `asleep` report, and the resume command prefilled without Enter
- [x] #3 Prefilled argv keeps the original flags and replaces any existing resume token with the current session id
- [x] #4 Focused, working/blocked, not-yet-quiet and no-session-ref panes get no action (sweep table tests)
- [x] #5 Quiet clock starts when a state change is first observed; a fresh state file starts every clock at now
- [x] #6 Tests run through the sweep seam only and pass with `go test -race -shuffle=on`; vet/staticcheck clean
- [x] #7 Manual check on a disposable herdr session: a real Claude pane goes asleep and Enter resumes the same session
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Per TASK-1 decision: report under stand-in kind maki + report_metadata (display_agent=real kind, state label idle=asleep, applies_to_source dnzzl.idle). Read session ref before SIGTERM; prefill only once foreground group == shell pid. A pane the plugin reported (agent maki, source dnzzl.idle) must never be treated as a sleep candidate itself.

Merged into feat/herdr-idle. Seam: internal/sweep Sweeper.Run(ctx) ([]Outcome, error) over Herdr + Signaler interfaces; internal/herdr socket client; main run(ctx,args,getenv,stdout) with sweep|version, requires HERDR_SOCKET_PATH.

Manual on disposable session: fake claude pane put asleep (maki/display claude/idle=asleep, prefill with flags kept). Real claude same-session resume NOT verified → AC7 open, covered by TASK-1.9.

Fix: herdr process check wipes the plugin report ~300ms after exit; sweep re-sends report_agent until it sticks 1s, within 30s. Fragile on herdr timing.

State file is per herdr session: state-<sha256(socket)[:6]>.json (pane ids are per-session).

Open: positional prompt on argv (claude "fix X") is kept → replayed on wake. Pending user decision.

Positional prompt decision moved to TASK-1.6 (drop positionals).

2026-10-27 dogfood on live session (canary config, other panes excluded): 3 real claude panes w1Q:p1/p3/p6 put asleep at 14:38 (maki/display claude/asleep, screens saved); user pressed Enter → same conversation resumed. AC7 verified.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Claude tracer: SIGTERM on foreground group, maki stand-in report with asleep label, resume prefilled without Enter. Verified by tests and on 3 real claude panes in live dogfood (Enter resumes same session). Risk: depends on herdr 0.9.5 report timing.
<!-- SECTION:FINAL_SUMMARY:END -->
