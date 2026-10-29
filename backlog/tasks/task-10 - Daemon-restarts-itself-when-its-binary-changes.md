---
id: TASK-10
title: Daemon restarts itself when its binary changes
status: Done
assignee: []
created_date: '2026-10-29T14:33:00Z'
updated_date: '2026-10-29T14:40:00Z'
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
- [x] #1 Replacing the binary makes the running daemon continue on the new one within one interval, without losing state
- [x] #2 No second daemon runs at any point
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
A daemon start resets every quiet clock on purpose (TestDaemonRestartResetsQuietClocks: the agent may have worked unseen while no daemon ran). A self-restart on binary change keeps running throughout, so it should keep the clocks; decide and test that.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Daemon checks its binary size+mtime every 30s and re-executes itself (same pid, lock released by close-on-exec), keeping quiet clocks via HERDR_IDLE_REEXEC. A failed re-exec is logged once and the old build carries on. Tests: re-exec within a check, clocks kept; manual: rebuild in a disposable session re-executed within 35s, one daemon throughout.
<!-- SECTION:FINAL_SUMMARY:END -->
