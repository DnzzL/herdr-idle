---
id: TASK-2
title: Daemon does not run after install until herdr restarts
status: Done
assignee: []
created_date: 2026-10-27 17:16
updated_date: '2026-10-29T14:40:00Z'
labels:
  - needs-triage
dependencies: []
type: bug
ordinal: 11000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Dogfood finding (2026-10-27). herdr runs [[startup]] hooks only at server start (run_plugin_startup_hooks); linking or enabling the plugin starts nothing, and herdr has no plugin.enabled/linked event. A fresh install puts nothing asleep until the next herdr restart, silently. Dogfooding had to start the daemon by hand.

Open question: how to start it without a restart. Candidates: idle-now/preview/config actions spawn the daemon when none holds the daemon lock; README tells the user to restart herdr. Caveat: a daemon launched from an event or action command may count against herdr's in-flight plugin command limit while it lives.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 After herdr plugin link or enable, sweeps run without a herdr restart, or the user is told plainly that a restart is needed
- [x] #2 At most one daemon per herdr session still holds (daemon lock)
<!-- AC:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Every plugin command (new pane.focused hook, close hooks, actions) runs ensure-daemon: if no daemon holds the lock, it starts one in its own session with no pipe to herdr (herdr counts a command in flight until its output closes). The daemon logs to daemon.log itself. Verified in a disposable herdr session: link, first focus, daemon running, hook succeeded; repeated ensure keeps one daemon. ADR-0002.
<!-- SECTION:FINAL_SUMMARY:END -->
