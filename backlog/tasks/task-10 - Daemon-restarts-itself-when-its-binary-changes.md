---
id: TASK-10
title: Daemon restarts itself when its binary changes
status: To Do
assignee: []
created_date: '2026-10-29T14:33:00Z'
updated_date: '2026-10-29T14:34:00Z'
labels:
  - ready-for-agent
dependencies:
  - TASK-2
ordinal: 19000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
After `herdr plugin update` (or a local rebuild), the running daemon keeps the old binary until herdr restarts, so a fix does not reach the user. herdr-automations re-executes its daemon when the binary changes. Probably shares a fix with TASK-2 (no daemon after install).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Replacing the binary makes the running daemon continue on the new one within one interval, without losing state
- [ ] #2 No second daemon runs at any point
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
A daemon start resets every quiet clock on purpose (TestDaemonRestartResetsQuietClocks: the agent may have worked unseen while no daemon ran). A self-restart on binary change keeps running throughout, so it should keep the clocks; decide and test that.
<!-- SECTION:NOTES:END -->
