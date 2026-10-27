---
id: TASK-1.3
title: Wake and cleanup of asleep panes
status: To Do
assignee: []
created_date: '2026-10-27 09:06'
updated_date: '2026-10-27 09:20'
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
- [ ] #1 A woken pane loses its `asleep` report and is not put asleep again before a full quiet period
- [ ] #2 Closing a pane removes its state entry and capture file
- [ ] #3 Covered by sweep-seam scenario tests (asleep → woken → quiet again → asleep)
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Per spike: herdr clears the maki stand-in report ~4s after the real agent starts. Wake = pane listed again under a real kind (or no longer under our report); plugin then drops its state entry and restarts the quiet clock. Release explicitly only if the report is still held.
<!-- SECTION:NOTES:END -->
