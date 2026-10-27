---
id: TASK-1.3
title: Wake and cleanup of asleep panes
status: Done
assignee:
  - '@claude'
created_date: '2026-10-27 09:06'
updated_date: '2026-10-27 09:40'
labels:
  - ready-for-agent
dependencies:
  - TASK-1.2
parent_task_id: TASK-1
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Once a user presses Enter on an asleep pane, the agent runs again and must become a normal agent the plugin can later put asleep again. Closed panes must not leave state behind. See TASK-1.

## What to build

When a sweep sees a live agent in a pane it holds as asleep, it releases the plugin report and drops the asleep state; the quiet clock restarts from that moment so the agent gets a full quiet period. When herdr emits `pane.closed`, the pane state entry and its capture file are removed.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A woken pane loses its `asleep` report and is not put asleep again before a full quiet period
- [x] #2 Closing a pane removes its state entry and capture file
- [x] #3 Covered by sweep-seam scenario tests (asleep → woken → quiet again → asleep)
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Per spike: herdr clears the maki stand-in report ~4s after the real agent starts. Wake = pane listed again under a real kind (or no longer under our report); plugin then drops its state entry and restarts the quiet clock. Release explicitly only if the report is still held.

Merged. Seam: Sweeper.PaneClosed(ctx, paneID) error; subcommand pane-closed (HERDR_PANE_ID) wired to [[events]] pane.closed. Wake = pane listed under a kind other than maki → asleep state dropped, clock restarts.

No explicit pane.release_agent: relies on herdr auto-clearing the maki report (spike-verified). Absent panes stay asleep for rearm. Not run on real herdr. Two writers (sweep, pane-closed) on the state file without a lock → handed to TASK-1.7.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Woken panes leave asleep state and get a full quiet period; closed panes drop state and capture. Tests through the sweep seam; real herdr check deferred to TASK-1.9.
<!-- SECTION:FINAL_SUMMARY:END -->
