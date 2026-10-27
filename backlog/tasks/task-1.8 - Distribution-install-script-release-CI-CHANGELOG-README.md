---
id: TASK-1.8
title: 'Distribution: install script, release CI, CHANGELOG, README'
status: To Do
assignee: []
created_date: '2026-10-27 09:07'
labels:
  - ready-for-agent
dependencies:
  - TASK-1.2
parent_task_id: TASK-1
ordinal: 9000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Users should install with `herdr plugin install DnzzL/herdr-idle` without Go, and see what changed per release. Mirrors DnzzL/herdr-automations. The repo must not mention any third-party prior-art project. See TASK-1.

## What to build

A `[[build]]` install script fetches the prebuilt binary for darwin/linux × amd64/arm64 matching the manifest version, verifies it against the release checksums and that it runs, and otherwise falls back to `go build`. A version tag triggers CI that builds the four binaries plus checksums and publishes the release with notes from that version CHANGELOG section. A README covers install, config, actions, sidebar colour rows and the supported-kinds table, each kind marked "unverified" until manually checked.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Install on macOS arm64 with no Go installed yields a working binary from the release
- [ ] #2 Checksum mismatch, missing asset or non-running binary falls back to `go build`
- [ ] #3 Pushing a tag publishes a release whose notes are that version CHANGELOG section
- [ ] #4 README documents install, config keys, actions and supported kinds; no mention of third-party prior art
<!-- AC:END -->
