---
id: TASK-1.6
title: Resume table for all 18 herdr-resumable kinds
status: To Do
assignee: []
created_date: '2026-10-27 09:06'
labels:
  - ready-for-agent
dependencies:
  - TASK-1.2
parent_task_id: TASK-1
ordinal: 7000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Every harness herdr can resume should be able to sleep; herdr has no on-demand resume API, so the plugin carries its own port of herdr 0.9.5 per-kind recipes (agent resume planner in herdr source). See TASK-1.

## What to build

Resume support for claude, codex, copilot, devin, droid, kimi, mastracode, pi, omp, hermes, opencode, qodercli, qwen, kilo, cursor, agy, grok, letta: the prefilled argv keeps the original flags, strips that kind existing resume tokens (flag, `--flag=value` and subcommand forms such as `codex resume <id>`), and appends the recipe tokens with the session ref (id or path, as herdr reports it). Kinds without a recipe, or panes without a session ref, are skipped with a reason.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 One sweep table row per kind asserts the exact prefilled argv from an original argv carrying flags and a stale resume token
- [ ] #2 Recipes match herdr 0.9.5 planner output for each kind
- [ ] #3 Unsupported kinds and missing session refs get no action and a reason
<!-- AC:END -->
