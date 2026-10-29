---
id: TASK-8
title: Enter on an asleep pane whose folder was deleted explains what happened
status: Done
assignee: []
created_date: '2026-10-29T14:33:00Z'
updated_date: '2026-10-29T14:56:00Z'
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
- [x] #1 Enter in a pane whose folder no longer exists prints that the folder is gone and the command to resume from another folder; no agent starts
- [x] #2 Enter with the folder present resumes exactly as before
- [x] #3 Tested with a deleted folder in a disposable herdr session
<!-- AC:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Chosen with the maintainer: option C (daemon checks folders) over a longer prefill. Each sweep, an asleep pane whose cwd no longer exists and whose shell is at its prompt gets Ctrl+U (clears the prefill) and an echo naming the folder and the resume command without cd; once per shell (a rearmed shell is told again). Sweeper.DirExists seam (os.Stat in main). Test through the Sweeper; Ctrl+U clearing a prefill verified in a disposable herdr session (zsh). Misses a folder deleted after the last sweep: Enter then fails on cd as before.
<!-- SECTION:FINAL_SUMMARY:END -->
