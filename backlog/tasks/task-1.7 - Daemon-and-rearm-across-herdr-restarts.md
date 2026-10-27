---
id: TASK-1.7
title: Daemon and rearm across herdr restarts
status: Done
assignee:
  - '@claude'
created_date: '2026-10-27 09:07'
updated_date: '2026-10-27 10:00'
labels:
  - ready-for-agent
dependencies:
  - TASK-1.3
parent_task_id: TASK-1
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
herdr has no timer event, so something must run sweeps on a schedule; and a herdr restart turns asleep panes into plain shells, which must not all wake at once (RAM spike). See TASK-1.

## What to build

A `[[startup]]` hook starts one daemon per herdr session (lock keyed on the session socket); it sweeps every `interval_minutes` and exits when the plugin is disabled or removed. On startup it rearms panes it holds as asleep: once each shell reaches its prompt, it re-reports `asleep` and re-prefills the resume command, without resuming any agent. A daemon restart resets all quiet clocks to now.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Starting a second daemon for the same herdr session exits without sweeping; different sessions each get one
- [x] #2 Sweeps run every `interval_minutes`; the daemon exits once the plugin is disabled or removed
- [x] #3 After a herdr restart, asleep panes are listed as `asleep` again with the prefill, and no agent is started
- [x] #4 Covered by tests through the sweep/rearm seam with the fake clock; no bare goroutines
- [x] #5 Concurrent writers of the state file (daemon sweep, pane-closed hook, idle-now) cannot resurrect or lose entries
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Per spike: after herdr restart all plugin reports/labels/prefill are gone and nothing resumes; rearm re-sends report_agent(maki)+report_metadata+prefill once the shell owns the foreground.

Merged. Seam: sweep.Daemon{Sweeper, Plugin, Log}.Run(ctx) error (ErrAlreadyRunning), Plugin.PluginEnabled; Run rearms asleep panes herdr no longer lists; file lock around state read-modify-write; [[startup]] daemon. dry_run still rearms (restores only).

Real check on disposable session: one daemon per session, exits ~25s after disable/unlink, rearm after server restart without starting the agent. Fix: daemon survives server stop → SIGPIPE ignored, rearm every 30s check. Gaps: unrestored panes kept in state forever; log lost after outliving a restart.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Startup daemon per herdr session sweeps every interval, exits when the plugin is disabled, rearms asleep panes after restart; state file guarded by a lock. Verified via synctest seam tests and a disposable herdr session.
<!-- SECTION:FINAL_SUMMARY:END -->
