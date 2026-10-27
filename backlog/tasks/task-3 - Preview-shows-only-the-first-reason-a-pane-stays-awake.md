---
id: TASK-3
title: Preview shows only the first reason a pane stays awake
status: To Do
assignee: []
created_date: '2026-10-27 17:16'
labels:
  - ready-for-agent
dependencies: []
type: bug
ordinal: 12000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Dogfood finding (2026-10-27). preview (and the daemon log) prints one reason per pane: the first rule that blocks it. In the first hour every pane reads "quiet for 0s, needs 1h0m0s", hiding other blockers (busy child process, no session ref, excluded) that would still keep it awake once quiet. The user cannot tell which panes will actually sleep.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 preview lists every rule that keeps a pane awake, not only the first
- [ ] #2 A pane blocked only by the quiet period is distinguishable from one also blocked by another rule
- [ ] #3 Tested through the sweep seam (Preview)
<!-- AC:END -->
