---
id: TASK-7
title: Reveal herdr-idle as an open-source herdr plugin
status: To Do
assignee: []
created_date: 2026-10-29 14:02
updated_date: '2026-10-29T15:29:00Z'
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
- [x] #1 MIT LICENSE, CONTRIBUTING and two issue templates (kind verification report, bug) are in the repo
- [x] #2 README works as landing page: problem, one measured RAM figure per idle claude pane, one-line install, preview action first, plain statement that agents get stopped, 30-day deletion warning
- [ ] #3 v0.1.0 released and the repo public with the herdr-plugin topic, after the maintainer OK
- [ ] #4 Drafts ready for discussions #631 and #1829, the awesome-herdr PR and Discord, reviewed by the maintainer
- [ ] #5 Stage 2 date set 7 days after stage 1, and a 30-day check date
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## Post drafts (stage 2, maintainer posts; nothing published)

### Reply in herdr discussions #631 and #1829
I built this as a plugin: https://github.com/DnzzL/herdr-idle

An agent pane quiet for two hours (configurable), that you are not looking at, gets its agent stopped with SIGTERM. The pane stays where it is and stays in the sidebar, tagged `asleep`; the resume command for the same session is typed on its input line. Press Enter and the conversation comes back, same folder, same flags. A herdr restart does not wake asleep agents.

Here an idle Claude Code pane with three MCP servers holds 540–770 MB; with ~40 asleep that is ~25 GB back.

Verified with claude, pi and opencode; 15 other kinds use herdr's resume recipes but are untested. If you run one of them, a kind report (issue template) helps a lot.

`herdr plugin install DnzzL/herdr-idle`, then the `preview` action shows what it would put asleep, changing nothing.

### awesome-herdr PR (Plugins section)
- [herdr-idle](https://github.com/DnzzL/herdr-idle) — Stops idle agents to free their RAM; the pane stays, the resume command is prefilled, Enter brings the conversation back.

### Discord
herdr-idle: a plugin that gives back the RAM of agents you left idle. After two quiet hours it stops the agent (SIGTERM), keeps the pane and its spot in the sidebar, and types the resume command; Enter resumes the same conversation. ~0.5–0.8 GB per idle Claude Code pane here. `herdr plugin install DnzzL/herdr-idle`, try the preview action first. Reports for kinds other than claude/pi/opencode welcome: https://github.com/DnzzL/herdr-idle

Correction to the drafts after the TASK-1.9 matrix: replace "Verified with claude, pi and opencode; 15 other kinds" with "Verified with claude and opencode; pi resumes its session, 15 other kinds use herdr's resume recipes but are untested."
<!-- SECTION:NOTES:END -->
