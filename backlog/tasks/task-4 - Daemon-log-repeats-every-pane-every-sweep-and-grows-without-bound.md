---
id: TASK-4
title: Daemon log repeats every pane every sweep and grows without bound
status: To Do
assignee: []
created_date: '2026-10-27 17:16'
labels:
  - ready-for-agent
dependencies: []
type: bug
ordinal: 13000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Dogfood finding (2026-10-27). daemon.log gets one line per agent pane per sweep (~45 lines / 10 min here, ~6.5k lines/day), plus "pane not found, rearm on a later sweep" every 30 s for an asleep pane that vanished without a close event. Nothing rotates or truncates it. What changed (put asleep, rearmed, woken, errors) is buried in unchanged "quiet for", "excluded", "asleep" lines.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Each sweep logs only panes whose outcome changed, and errors
- [ ] #2 A vanished asleep pane is logged once, not every rearm check
- [ ] #3 The log file stays bounded over weeks of uptime
<!-- AC:END -->
