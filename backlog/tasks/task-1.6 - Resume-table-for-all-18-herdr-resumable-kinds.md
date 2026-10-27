---
id: TASK-1.6
title: Resume table for all 18 herdr-resumable kinds
status: Done
assignee:
  - '@claude'
created_date: '2026-10-27 09:06'
updated_date: '2026-10-27 09:44'
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
- [x] #1 One sweep table row per kind asserts the exact prefilled argv from an original argv carrying flags and a stale resume token
- [x] #2 Recipes match herdr 0.9.5 planner output for each kind
- [x] #3 Unsupported kinds and missing session refs get no action and a reason
- [x] #4 Positional prompt arguments are dropped from the resume command for every kind; value-taking flags keep their values
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
DECISION (user): resume command drops positional arguments (CLI prompts) so they are not replayed on wake. Requires, per kind, the flags that take a value; an unknown value-taking flag may lose its value (resume still works). Applies to claude too (fix the existing claude recipe).

Merged. Resume builder knows per-option arity (none/one/optional/many); drops positionals and everything after --. Recipes per kind in internal/sweep with source comments. Flag-list confidence: high for claude/droid/opencode/pi/codex/omp/hermes/qwen/kilo/grok/letta; medium for kimi/copilot/devin/cursor/qodercli/mastracode; medium-low agy.

Fragile: mastracode herdr recipe (--thread) may only work headless; kept herdr form per AC2. opencode/kilo drop [project] positional (rely on pane cwd). Combined short flags not split. hermes python launch and symlinked commands → skipped as unrecognised.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All 18 herdr-resumable kinds put asleep with herdr 0.9.5 resume tokens, original flags kept, prompts dropped. Real resume per kind deferred to TASK-1.9.
<!-- SECTION:FINAL_SUMMARY:END -->
