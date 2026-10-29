---
id: TASK-7
title: Reveal herdr-idle as an open-source herdr plugin
status: To Do
assignee: []
created_date: 2026-10-29 14:02
updated_date: '2026-10-29T14:33:00Z'
labels:
  - ready-for-human
dependencies:
  - TASK-1.8
  - TASK-1.9
  - TASK-2
  - TASK-4
  - TASK-5
  - TASK-8
  - TASK-10
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Publish herdr-idle to herdr users so they get their RAM back, and collect verification reports for the kinds still unverified. Audience: existing herdr users only (the plugin cannot be sold without selling herdr). Success at 30 days: about 10 stars, about 3 kinds verified by outside users, 0 bugs that lose a conversation.

Decisions (grilling, 2026-10-29):
- Two stages. Stage 1: public repo with the `herdr-plugin` topic, which lists it on herdr.dev/plugins automatically (index refresh 30 min, no review): a quiet launch. Stage 2, about 7 days later and only if no conversation-losing bug showed up: posts.
- Channels (drafts only, the maintainer posts): replies in herdr discussions #631 and #1829 (existing hibernation requests), an awesome-herdr PR, herdr Discord. Not HN, Reddit, X, nor agent memory-leak issues.
- Message: "Your idle agents give back their RAM. The pane stays, the conversation resumes with Enter." One measured figure per pane (GB per idle claude pane), not the maintainer total.
- The Enter claim must hold: verification matrix (plain wake, after herdr restart, after typing/Ctrl+C, deleted folder, nested claude -p, long sleep) for claude, pi, opencode, recorded in TASK-1.9.
- Feedback through GitHub issues only, two templates (kind verification report, bug); CONTRIBUTING modelled on herdr-automations (narrow scope, "most wanted: kinds tested in the wild", local run with herdr plugin link, house style). Discussions off.
- First run: keep dry_run false; preview action first in the README install section; README says plainly agents get stopped. Default idle_minutes 120.
- License MIT. First version v0.1.0.
- README warns that Claude deletes conversations older than 30 days by default (TASK-9 for the in-pane warning, after launch).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 MIT LICENSE, CONTRIBUTING and two issue templates (kind verification report, bug) are in the repo
- [ ] #2 README works as landing page: problem, one measured RAM figure per idle claude pane, one-line install, preview action first, plain statement that agents get stopped, 30-day deletion warning
- [ ] #3 v0.1.0 released and the repo public with the herdr-plugin topic, after the maintainer OK
- [ ] #4 Drafts ready for discussions #631 and #1829, the awesome-herdr PR and Discord, reviewed by the maintainer
- [ ] #5 Stage 2 date set 7 days after stage 1, and a 30-day check date
<!-- AC:END -->
