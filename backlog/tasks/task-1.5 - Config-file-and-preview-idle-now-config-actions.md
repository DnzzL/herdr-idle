---
id: TASK-1.5
title: 'Config file and preview, idle-now, config actions'
status: In Progress
assignee:
  - '@claude'
created_date: '2026-10-27 09:06'
updated_date: '2026-10-27 09:46'
labels:
  - ready-for-agent
dependencies:
  - TASK-1.2
parent_task_id: TASK-1
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Users need to tune the quiet period, keep loop/cron agents running, try it safely, and understand each decision. See TASK-1.

## What to build

A `config.toml` in the plugin config dir, re-read on every sweep, missing means defaults: `idle_minutes` (60), `interval_minutes` (10), `exclude` (pane or session ids, default empty), `dry_run` (false). Every agent pane gets a reason for sleeping or not. Plugin actions: `preview` (lists every agent pane and its reason, performs nothing), `idle-now` (runs a sweep now), `config` (opens the config in the user editor, creating it with commented defaults).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Excluded panes or sessions are never put asleep
- [x] #2 `dry_run` logs what would happen and performs no SIGTERM, report or prefill
- [x] #3 A config change applies on the next sweep without restart; a missing or partial file uses defaults; an invalid file reports every problem and keeps the previous config
- [ ] #4 `preview` shows each agent pane with its reason; `idle-now` and `config` actions work from herdr
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Merged. Seam: Sweeper.Config *config.File (reloaded each sweep), Sweeper.Preview(ctx) dry, internal/config (Parse joins errors with line numbers, File.Load keeps last valid). CLI: preview [--hold], config. Actions open overlay panes via herdr plugin pane open.

Decision: invalid config with no previous valid one → sweep does nothing (not defaults), so a typo cannot turn dry_run into real SIGTERMs.

AC4 only verified on the CLI: actions/overlays not run in a real herdr (sandbox refused XDG_CONFIG_HOME). TOML subset: flat keys only, anything else reported as a problem.
<!-- SECTION:NOTES:END -->
