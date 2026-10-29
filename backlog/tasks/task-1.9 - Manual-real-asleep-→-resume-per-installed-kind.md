---
id: TASK-1.9
title: 'Manual: real asleep → resume per installed kind'
status: To Do
assignee: []
created_date: 2026-10-27 09:07
updated_date: '2026-10-29T15:29:00Z'
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

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
2026-10-27: claude verified in live dogfood (3 panes slept, Enter resumed same session). Remaining: pi, droid, opencode.

Matrix 2026-10-29, disposable herdr session, real agents, build 811b382. claude: plain wake, after herdr restart (rearmed ~13s, no agent auto-started), Ctrl+C then the "resume with" line, deleted folder: all PASS, same agent_session and the remembered word recalled. opencode: same three PASS (flags kept: -m kept, --prompt dropped); after a wake herdr reports no agent_session (also for a plain `opencode --session X` never put asleep: herdr integration side), so it cannot sleep again. pi: resumes the same .jsonl (no new file, history shown, new turns appended) but no model answers on this machine (auth), replies unverified. claude -p case: a real nested claude -p via the Bash tool did not take over the pane session here; simulated via report_agent_session (session_start_source clear) → pane left awake with the sdk-cli reason. droid: not run. README updated.
<!-- SECTION:NOTES:END -->
