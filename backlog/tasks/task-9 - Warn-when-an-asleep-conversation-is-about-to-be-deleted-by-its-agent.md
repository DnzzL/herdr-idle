---
id: TASK-9
title: Warn when an asleep conversation is about to be deleted by its agent
status: To Do
assignee: []
created_date: '2026-10-29T14:33:00Z'
labels:
  - needs-triage
dependencies: []
ordinal: 18000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Claude Code deletes conversation files older than cleanupPeriodDays (30 days by default). A pane asleep longer than that keeps a resume command for a conversation that no longer exists. v0.1.0 only warns in the README; this ticket is the in-pane warning. Each kind has its own retention rule.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 An asleep claude pane close to the retention limit says so in the pane
- [ ] #2 Kinds whose retention is unknown are documented as such
<!-- AC:END -->
