---
id: TASK-1
title: >-
  Spec: herdr-idle — put idle agent panes asleep to free RAM, keep panes in
  place
status: To Do
assignee: []
created_date: '2026-10-27 09:00'
updated_date: '2026-10-27 09:17'
labels:
  - ready-for-agent
dependencies: []
references:
  - 'https://github.com/herdrdev/herdr'
  - 'https://github.com/DnzzL/herdr-automations'
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## Problem Statement

I keep many coding-agent panes open in herdr. Most sit waiting for days on a PR review or a reply, yet each one keeps 1–2 GB of RAM (the agent plus its MCP servers). Closing them frees the memory, but then the pane, its place in the layout and its spot in the sidebar agent list are gone, and finding the right session again means digging through each harness's resume picker. herdr itself only resumes agents on a server restart, never on demand. Nothing works on macOS and Linux alike, for every harness herdr knows how to resume.

## Solution

`herdr-idle`, a minimal herdr plugin written in Go, for macOS and Linux. A background daemon checks agent panes every few minutes. A pane quiet for long enough is put **asleep**:
- the daemon saves the agent's recent screen to a file;
- it stops the agent process, which frees its RAM;
- the pane stays where it is, back at its shell, with real scrollback;
- the pane stays in the sidebar agent list, tagged `asleep`;
- the shell input line is prefilled with the exact command that resumes the same session, with the original flags.

Pressing Enter wakes the agent; Ctrl+C clears the line. Asleep panes stay asleep across herdr restarts instead of all waking at once.

## User Stories

1. As a herdr user, I want idle agent panes put asleep automatically, so that my RAM is freed without manual work.
2. As a herdr user, I want an asleep pane to stay in its tab and layout position, so that my workspace looks the same.
3. As a herdr user, I want an asleep pane to stay in the sidebar agent list, so that I can still see and reach every agent.
4. As a herdr user, I want asleep panes tagged `asleep` (not `idle`), so that I don't confuse them with herdr's own idle status.
5. As a herdr user, I want the resume command prefilled in the asleep pane's shell, so that one Enter brings the agent back.
6. As a herdr user, I want the agent to resume the same session, so that the conversation continues where I left it.
7. As a herdr user, I want the agent to resume with the flags I started it with (e.g. permission mode, model), so that it behaves as before and doesn't block on permissions.
8. As a herdr user, I want the agent to resume in the same working directory, so that it sees the same project.
9. As a herdr user, I want to keep the real terminal scrollback of an asleep pane, so that I can scroll back through what the agent printed.
10. As a herdr user, I want the agent's recent screen saved to a file before it is stopped, so that an unsent draft or a full-screen TUI's last view is never lost.
11. As a herdr user, I want the asleep pane to print where that capture file is, so that I can find it.
12. As a herdr user, I want Ctrl+C to clear the prefilled command, so that I can use the pane as a plain shell.
13. As a Claude Code user, I want Claude panes supported, so that my main fleet sleeps.
14. As a user of any harness herdr can resume (claude, codex, copilot, devin, droid, kimi, mastracode, pi, omp, hermes, opencode, qodercli, qwen, kilo, cursor, agy, grok, letta), I want my panes supported, so that every agent I use can sleep.
15. As a user of a harness herdr cannot resume, or one whose herdr integration is not installed, I want those panes left alone, so that I never lose a session that can't come back.
16. As a herdr user, I want a pane I'm focused on never put asleep, so that the agent doesn't vanish while I look at it.
17. As a herdr user, I want a working or blocked agent never put asleep, so that running work is never cut.
18. As a herdr user, I want an agent with a child process other than its MCP or language servers (shell, dev server, test run) never put asleep, so that its background work survives.
19. As a herdr user, I want the quiet period configurable (default 60 minutes), so that I can tune it.
20. As a herdr user, I want the check interval configurable (default 10 minutes), so that I can tune it.
21. As a herdr user, I want to exclude specific panes or sessions, so that agents running loops or cron jobs keep running.
22. As a herdr user, I want a dry-run mode, so that I can see what would sleep before trusting it.
23. As a herdr user, I want config changes picked up on the next check, without a restart, so that tuning is quick.
24. As a herdr user, I want a missing config file to mean defaults, so that the plugin works right after install.
25. As a herdr user, I want a `preview` action listing every agent pane and why it would or would not sleep, so that I understand its decisions.
26. As a herdr user, I want an `idle-now` action, so that I can run a check without waiting.
27. As a herdr user, I want a `config` action opening the config in my editor, so that I don't hunt for its path.
28. As a herdr user, I want asleep panes to stay asleep after a herdr restart, so that a restart doesn't re-spawn every agent at once and spike RAM.
29. As a herdr user, I want asleep panes restored after a restart (still listed, still prefilled), so that nothing changes from my side.
30. As a herdr user, I want a woken agent to drop its `asleep` tag and count as a normal agent again, so that it can sleep later.
31. As a herdr user, I want a resumed agent to get a full quiet period before sleeping again, so that it doesn't sleep right after I wake it.
32. As a herdr user, I want an agent that doesn't stop within 30 seconds left alone (no force kill), so that a slow shutdown can't corrupt its session.
33. As a herdr user, I want a daemon restart to reset all quiet timers, so that it never puts an agent asleep too early.
34. As a herdr user, I want one daemon per herdr session, so that two sessions don't fight over the same panes.
35. As a herdr user, I want the daemon to exit when the plugin is disabled or removed, so that nothing lingers.
36. As a herdr user, I want closed panes' state and capture files cleaned up, so that the state directory doesn't grow forever.
37. As a herdr user, I want installation to fetch a prebuilt, checksum-verified binary for my OS/arch, so that I don't need Go installed.
38. As a herdr user without a matching prebuilt binary, I want installation to fall back to building from source, so that it still works.
39. As a herdr user, I want each release to carry a changelog written for users, so that I know what changed.
40. As a herdr user, I want a log of every decision and action, so that I can debug why a pane slept or didn't.

## Implementation Decisions

- **Platform via herdr, not the OS.** All process facts (agent argv, cwd, foreground process group, child processes) come from herdr's `pane.process_info`. No `/proc`, `ps` or `lsof` → the same code runs on macOS and Linux.
- **Talks to herdr over its JSON socket API.** Calls used: `session.snapshot`, `pane.process_info`, `agent.read` (recent source), `pane.report_agent`, `pane.release_agent`, `pane.send_text`. The sweep declares the small interface it needs; a concrete socket client implements it.
- **Resume table owned by the plugin.** herdr has no on-demand resume API. It only resumes at server restart, with argv that drops the original flags. The plugin ports herdr's per-kind recipes for the 18 kinds whose integration reports a session ref. Each recipe: kind + herdr session ref (id or path) → resume tokens.
- **Original flags kept.** Resume argv = original argv from `pane.process_info`, minus that kind's existing resume tokens (e.g. `--resume <id>`, `codex resume <id>`, `--resume=<id>`), plus the recipe's resume tokens. Subcommand-style kinds (codex) need care when stripping.
- **Unsupported → never asleep.** No recipe, or no session ref in the snapshot → the pane is skipped, and `preview` says why.
- **Asleep criteria (generic, every harness):**
  - herdr status `idle` or `done`
  - pane not focused
  - no state change for `idle_minutes`
  - foreground children limited to MCP/language servers
  - session ref present and kind supported
  - not in `exclude`
  - No harness-internal pending-work check in v1 (Claude `/loop`, cron, background subagents); `exclude` covers those.
- **Quiet clock.** The snapshot has `state_change_seq` but no timestamp. The daemon records when it last saw each pane's seq/status change, in its state file. A daemon restart resets clocks to now.
- **Putting a pane asleep**, in order:
  1. capture recent screen to a per-pane file in the plugin state directory;
  2. `SIGTERM` the agent's foreground process group (also stops its MCP servers);
  3. wait up to 30s; still alive → leave the pane, log it, no `SIGKILL`;
  4. once the shell is back at its prompt, `pane.report_agent` with state text `asleep` and **no** `resume_argv`;
  5. `pane.send_text` the resume command without Enter, plus a line pointing at the capture file.
- **No `resume_argv` handed to herdr.** On a herdr restart, herdr would otherwise resume every asleep agent at once.
- **Rearm on startup.** The daemon re-reports and re-prefills panes it holds as asleep, once each shell reaches its prompt.
- **Wake detection.** When an asleep pane hosts a live agent again, the next sweep releases the plugin's report and drops the pane's asleep state. The quiet clock restarts from that moment.
- **State file.** One JSON file in the plugin state directory, one entry per pane:
  ```
  pane_id → { kind, session_ref, resume_argv, last_change_seq, last_change_at, asleep_at, capture_path }
  ```
  `pane.closed` removes the entry and its capture file.
- **Config.** `config.toml` in the plugin's config dir, re-read every sweep, missing → defaults. Keys:
  - `idle_minutes` = 60
  - `interval_minutes` = 10
  - `exclude` = [] (pane or session ids)
  - `dry_run` = false
- **Daemon lifecycle.** A `[[startup]]` hook starts one daemon per herdr session (lock file keyed on the session socket). The daemon exits when the plugin is disabled or removed. herdr has no timer event, so the daemon keeps the clock.
- **CLI surface.** Subcommands: `daemon`, `sweep`, `preview`, `config`, `rearm`, `version`. Plugin actions: `idle-now`, `preview`, `config`. Plugin id `dnzzl.idle`, binary `herdr-idle`, repo `DnzzL/herdr-idle`.
- **Distribution.** A `[[build]]` install script downloads the prebuilt binary for darwin/linux × amd64/arm64 and verifies `checksums.txt`. It falls back to `go build` when no asset fits, the checksum mismatches, or the binary won't run. A version tag triggers CI, which builds the binaries and publishes the release with notes from that version's CHANGELOG section.
- **Go style.** Stdlib only (+ `golang.org/x/sync` if needed). The DnzzL Effect-style Go guide's "Always" tier applies: `ctx` first and `error` last on I/O, pure decision logic, consumer-declared interfaces, `run(ctx, getenv, …)` in `main`, no bare goroutines, sentinel/typed errors.
- **Repo hygiene.** The repo must not mention any third-party prior-art project.
- **ADR to record before building:** SIGTERM + plugin-owned resume table, vs. relying on herdr's restart-only resume.

## Testing Decisions

- **One test seam: the sweep.** Tests drive a single sweep against:
  - a hand-written fake herdr client (scripted snapshot and process info; records reports, releases, sent text);
  - a fake process signaler;
  - a fake clock via `testing/synctest`.
- **Outputs asserted, never internals:** which process groups got `SIGTERM`; the `asleep` reports; the exact prefilled resume argv; capture files written; `preview` reasons; state after wake and after rearm.
- **Resume table as sweep table rows.** One row per supported kind: original argv (with flags and an existing resume token) → expected argv sent. Unsupported kinds and missing session refs → no signal, no report.
- **Decision rules as sweep table rows.** One row per rule (focused, working, blocked, child process, excluded, not quiet long enough, dry run) → asserts no action.
- **Lifecycle scenarios.** Asleep → woken → clock restarts. Agent alive after 30s → left alone. Daemon restart → clocks reset. Rearm restores report + prefill without resuming. Pane closed → state and capture removed.
- **Good test:** behaviour seen from herdr's and the OS's side, nothing about function boundaries. Each test proven able to fail by breaking the line it guards. Run with `-race -shuffle=on`.
- **Prior art:** none in this repo yet (greenfield). DnzzL/herdr-automations is the reference for the fake-herdr-client style, install script and release workflow.
- **Manual verification (not automated):**
  - First spike: a shell pane with only a plugin `pane.report_agent` stays in the sidebar agent list.
  - A kind becomes "supported" in the README only after one real asleep → Enter → same-session resume on it, confirming it flushes its session on `SIGTERM`.

## Out of Scope

- Freezing agents with `SIGSTOP` or forcing memory reclaim; only stop + resume.
- Harness-internal pending-work detection (Claude `/loop`, cron, background subagents) and reading harness session files.
- Replaying a still view of the conversation inside the pane.
- Harnesses herdr can't resume (amp, gemini, cline, maki, muse, …) and panes without a session ref.
- Force-killing agents that ignore `SIGTERM`.
- Editing the user's herdr config (e.g. sidebar colour rules).
- Windows.
- Contributing an `agent.resume` API to herdr. Filed separately as a feature request; the plugin's table goes away if it lands.

## Further Notes

- Resume recipes mirror herdr 0.9.5's internal planner. When herdr adds or changes a kind, the plugin's table must follow → drift risk, tracked in the README's supported-kinds table.
- Unverified: every one of the 18 harnesses flushing its session on `SIGTERM`. Claude writes its session continuously (high confidence); the others are unknown until each manual check.
- ~85% confident a plugin `report_agent` alone keeps a shell pane in the agent list; the first spike settles it before anything else is built.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 An agent pane matching every asleep criterion has its foreground process group sent SIGTERM, is reported to herdr as `asleep`, and has its resume command prefilled without Enter
- [ ] #2 The prefilled resume argv keeps the original flags and resumes the same session, for each of the 18 kinds herdr can resume
- [ ] #3 Panes that are focused, working/blocked, running non-MCP children, excluded, unsupported, or without a session ref are never put asleep, and `preview` states the reason
- [ ] #4 Recent screen is captured to a file before SIGTERM; an agent still alive after 30s is left alone
- [ ] #5 Asleep panes stay asleep and listed after a herdr restart (no `resume_argv` handed to herdr)
- [ ] #6 A woken agent loses its `asleep` report and its quiet clock restarts
- [ ] #7 Config re-read each sweep with defaults when missing; `dry_run` performs no action
- [ ] #8 Install script fetches a checksum-verified prebuilt binary for darwin/linux × amd64/arm64 and falls back to `go build`
- [ ] #9 Sweep-seam tests pass with `go test -race -shuffle=on`; manual spike confirms report-only shell panes stay in the agent list
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Spike TASK-1.1 findings (herdr 0.9.5, disposable session):

- BLOCKER: after a kind process exits in a pane, a plugin report with the same --agent kind is accepted (ok) but silently dropped until a new agent process of that kind or a server restart (recent_agent_process_exit gate, terminal/state.rs:713-719). Applies to all kinds except full-lifecycle sources (pi/omp/opencode/kilo/kimi/mastracode).

- Custom labels not tied to a kind are cleared in <100ms once the shell is idle (pane.rs:383-410).

- Workaround verified: report a different known kind (maki/pi) + pane.report_metadata --display-agent <real kind> --state-label idle=asleep --applies-to-source dnzzl.idle → listed, survives shell commands, auto-cleared ~4s after the real agent restarts.

- report_agent --state only idle|working|blocked|unknown; the asleep text must come from report_metadata state labels.

- Same-kind report blocks herdr state for a woken agent (shows asleep while working) until released.

- send_text without Enter = editable prefill; must wait for foreground group == shell pid, else stray echo.

- Server restart: plugin reports, labels, prefill and scrollback all gone; nothing resumed; exit gate reset (same-kind report accepted again). Capture file = only record after restart.

- herdr session ref disappears on process exit → read it before SIGTERM.
<!-- SECTION:NOTES:END -->
