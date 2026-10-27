---
id: TASK-1.4
title: 'Safety rules: child processes, screen capture, no force kill'
status: In Progress
assignee:
  - '@claude'
created_date: '2026-10-27 09:06'
updated_date: '2026-10-27 09:35'
labels:
  - ready-for-agent
dependencies:
  - TASK-1.2
parent_task_id: TASK-1
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Stopping an agent must never cut running work or lose what is on screen (an unsent draft, a full-screen TUI view). See TASK-1.

## What to build

A pane whose agent has a child process other than its MCP or language servers (shell, dev server, test run) is skipped. Before SIGTERM, the recent screen is saved to a per-pane capture file in the plugin state directory, and the asleep pane prints that path. If the agent is still alive 30 seconds after SIGTERM, the pane is left alone and logged; no SIGKILL is ever sent.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A pane with a non-MCP/LSP child process gets no SIGTERM
- [ ] #2 A capture file exists before SIGTERM is sent, and its path is shown in the asleep pane
- [ ] #3 An agent alive 30s after SIGTERM is left alone: no report, no prefill, no SIGKILL, logged
- [ ] #4 Covered by sweep-seam tests with the fake clock
<!-- AC:END -->
