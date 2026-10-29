---
id: TASK-6
title: Panes with a title-rewriting MCP server never sleep
status: Done
assignee: []
created_date: '2026-10-29 14:01'
updated_date: '2026-10-29 14:01'
labels:
  - ready-for-agent
dependencies: []
ordinal: 15000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
chrome-devtools-mcp rewrites its process title, so herdr lists it in the foreground group with no command line. herdr-idle counted any unnamed process as running work: panes with that server loaded stayed awake forever, logged as `running ` with nothing after it (w1Q:p6, w70:p1Z on 2026-10-29).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A foreground process herdr lists without a command line is identified from the OS process table
- [x] #2 A process unnamed in both keeps the pane awake with a readable reason
<!-- AC:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Missing command lines are filled from the ps table (Processes seam now lists the whole table). Unnamed processes still block, with reason "running pid N, command line unknown". Verified: two Sweeper tests, full suite with -race; read-only live preview with idle_minutes=1 shows w1Q:p6 and w70:p1Z would now sleep.
<!-- SECTION:FINAL_SUMMARY:END -->
