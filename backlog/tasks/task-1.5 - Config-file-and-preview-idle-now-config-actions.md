---
id: TASK-1.5
title: 'Config file and preview, idle-now, config actions'
status: To Do
assignee: []
created_date: '2026-10-27 09:06'
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
- [ ] #1 Excluded panes or sessions are never put asleep
- [ ] #2 `dry_run` logs what would happen and performs no SIGTERM, report or prefill
- [ ] #3 A config change applies on the next sweep without restart; a missing or partial file uses defaults; an invalid file reports every problem and keeps the previous config
- [ ] #4 `preview` shows each agent pane with its reason; `idle-now` and `config` actions work from herdr
<!-- AC:END -->
