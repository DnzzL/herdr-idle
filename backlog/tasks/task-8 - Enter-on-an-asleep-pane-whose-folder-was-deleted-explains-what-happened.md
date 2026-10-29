---
id: TASK-8
title: Enter on an asleep pane whose folder was deleted explains what happened
status: To Do
assignee: []
created_date: '2026-10-29T14:33:00Z'
labels:
  - ready-for-agent
dependencies: []
ordinal: 17000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The resume command is `cd <dir> && <agent> --resume <id>`. When the folder is gone (a removed worktree, seen live on 2026-10-29 for three panes), `cd` fails with a bare shell error and nothing starts; the user does not know the conversation is still reachable. Resuming elsewhere is rejected: the agent could edit the wrong repo.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Enter in a pane whose folder no longer exists prints that the folder is gone and the command to resume from another folder; no agent starts
- [ ] #2 Enter with the folder present resumes exactly as before
- [ ] #3 Tested with a deleted folder in a disposable herdr session
<!-- AC:END -->
