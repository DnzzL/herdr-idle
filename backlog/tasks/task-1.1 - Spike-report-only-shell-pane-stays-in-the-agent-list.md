---
id: TASK-1.1
title: 'Spike: report-only shell pane stays in the agent list'
status: To Do
assignee: []
created_date: '2026-10-27 09:06'
labels:
  - ready-for-agent
dependencies: []
parent_task_id: TASK-1
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The whole design keeps an asleep pane visible by reporting it to herdr from the plugin after the agent process has exited, leaving a plain shell in the pane. This is assumed (~85%) but unverified on herdr 0.9.5. If it fails, the asleep design must be revised before anything is built.

## What to build

On a real herdr session (use a disposable named session, never the user's main one), take a plain shell pane, report it with `pane.report_agent` from a plugin-style source, agent kind of an existing harness and state text `asleep`, and observe whether it appears in the agent list and sidebar, whether it survives the shell running commands, and what `pane.release_agent` does. Also confirm `pane.send_text` without Enter prefills the shell line. Record findings as notes on TASK-1.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Findings on TASK-1 state whether a report-only shell pane appears in `herdr agent list` / snapshot agents and the sidebar
- [ ] #2 Findings state whether the report survives commands run in the shell and what `pane.release_agent` restores
- [ ] #3 Findings state whether `pane.send_text` without Enter leaves an editable prefilled line
- [ ] #4 If any assumption fails, TASK-1 notes propose the design change; no production code written
<!-- AC:END -->
