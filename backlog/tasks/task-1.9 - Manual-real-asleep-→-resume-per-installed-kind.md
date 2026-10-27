---
id: TASK-1.9
title: 'Manual: real asleep → resume per installed kind'
status: To Do
assignee: []
created_date: '2026-10-27 09:07'
labels:
  - ready-for-human
dependencies:
  - TASK-1.6
  - TASK-1.7
parent_task_id: TASK-1
ordinal: 10000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Whether each harness flushes its session on SIGTERM and resumes cleanly cannot be proven by unit tests; a kind is only advertised as supported after a real cycle. See TASK-1.

## What to build

On the maintainer machine, for each installed kind with a herdr integration (currently claude, pi, droid, opencode): start an agent with non-default flags, chat, let it go asleep (or `idle-now` with a low `idle_minutes`), restart herdr once, then press Enter and confirm the same session and flags come back. Mark each passing kind "verified" in the README.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 claude, pi, droid and opencode each complete asleep → herdr restart → Enter with the same session and original flags
- [ ] #2 README supported-kinds table marks each passing kind verified; failures are filed as bugs
<!-- AC:END -->
