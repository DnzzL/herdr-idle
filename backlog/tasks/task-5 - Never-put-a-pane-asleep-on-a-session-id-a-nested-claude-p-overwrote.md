---
id: TASK-5
title: Never put a pane asleep on a session id a nested claude -p overwrote
status: To Do
assignee: []
created_date: '2026-10-28 11:00'
labels:
  - ready-for-agent
dependencies: []
ordinal: 14000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
herdr-idle builds the resume command from the session id herdr holds for the pane. Any `claude -p` run inside an agent pane (a script, a hook, a manual test) fires the herdr claude hook, and herdr replaces the pane's session id with the one-shot session. Seen live on 2026-10-28: a test `claude -p` in pane w8Q:p1 replaced its id. Putting that pane asleep would prefill `claude --resume <one-shot id>`: Enter resumes the wrong conversation, and the real one is hard to find. Claude records how a session was started (`entrypoint` field in its session file: `cli` for interactive, other values for `-p`/SDK). No asleep pane was affected as of 2026-10-28 (all 42 ids were `cli`). Reading the claude session file couples the plugin to a private file format; keep the check to claude.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A claude pane whose session was started non-interactively is left awake, with a preview reason naming the problem
- [ ] #2 A claude pane whose session file cannot be found or read is left awake, with a preview reason
- [ ] #3 An interactive claude session is still put asleep as before
- [ ] #4 Other kinds are unaffected
- [ ] #5 Tests through the Sweeper cover the three claude cases; README lists the new condition
<!-- AC:END -->
