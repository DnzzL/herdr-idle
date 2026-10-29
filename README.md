<div align="center">

# herdr-idle

**Your idle agents give back their RAM.<br>The pane stays, the conversation resumes with Enter.**

A [herdr](https://herdr.dev) plugin for macOS and Linux.

[![Release](https://img.shields.io/github/v/release/DnzzL/herdr-idle)](https://github.com/DnzzL/herdr-idle/releases)
[![CI](https://github.com/DnzzL/herdr-idle/actions/workflows/ci.yml/badge.svg)](https://github.com/DnzzL/herdr-idle/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)
[![herdr plugin](https://img.shields.io/badge/herdr-plugin-8a2be2)](https://herdr.dev/plugins)

[Install](#install) · [How it works](#how-it-works) · [Safety](#safety) ·
[Config](#config) · [Supported agents](#supported-agents) · [FAQ](#faq)

</div>

---

An agent pane waiting days for a review still holds its agent and its MCP
servers in memory: **540–770 MB per idle Claude Code pane** with three MCP
servers, measured on macOS. Ten of them hold 5–8 GB and do nothing.

herdr-idle stops those agents after two quiet hours and leaves the pane where
it was. Coming back to an asleep pane looks like this:

```console
  ✻ Baked for 49s · done Monday 5:17 PM
────────────────────────────────────────────────────────────────────────
❯
────────────────────────────────────────────────────────────────────────
herdr-idle: screen saved to ~/.local/state/herdr/plugins/dnzzl.idle/…/w88-p1.txt
herdr-idle: resume with: cd ~/Projects/docs && claude --resume d8382bef-…

~/Projects/docs > cd ~/Projects/docs && claude --resume d8382bef-…█
```

The end of the conversation is still on screen. The command that resumes it is
already typed. Press **Enter** and you're back in the same session, with the
same flags, in the same folder.

| | Memory | Place in layout & sidebar | Way back |
|---|---|---|---|
| Leave the pane open | 540–770 MB, forever | kept | kept |
| Close the pane | freed | lost | find the session id yourself |
| **herdr-idle** | **freed** | **kept, tagged `asleep`** | **Enter** |

## Install

```bash
herdr plugin install DnzzL/herdr-idle
```

That's all. No herdr restart, no config file, no Go toolchain: the install
fetches a prebuilt, checksum-verified binary for macOS or Linux (arm64/amd64).
If there is none for your platform, or it fails its checksum or doesn't run,
it builds from source, which needs [Go](https://go.dev/dl/).

Checks start the first time you switch panes, then run every 10 minutes.

**It stops your agents.** Want to see first what it would do? The preview
lists every agent pane and why it would or would not sleep, and changes
nothing:

```bash
herdr plugin action invoke preview --plugin dnzzl.idle
```

For a trial run that stops nothing, set `dry_run = true` in the
[config](#config).

## How it works

A pane is put asleep when all of these hold:

- herdr shows its agent `idle` or `done`, with no change for `idle_minutes`
  (two hours by default);
- you are not focused on the pane;
- the agent runs nothing besides its MCP or language servers: no shell, dev
  server or test run. A server is recognised by its program or package name:
  a word `mcp`, `lsp` or `langserver`, `language-server`,
  `@modelcontextprotocol/…`, or `gopls`, `rust-analyzer`, `clangd`, `tsserver`,
  `jdtls`;
- its kind is in the [supported table](#supported-agents) and herdr knows its
  session id;
- for `claude`, that session was started interactively. A `claude -p` run
  inside an agent pane makes herdr take the one-shot session as the pane's;
  the plugin checks Claude's session file and leaves the pane awake rather
  than resume the wrong conversation;
- it is not in `exclude`.

Then the plugin:

1. saves the agent's recent screen to a file in the plugin state directory;
2. stops the agent and its MCP servers with `SIGTERM`. An agent still running
   30 seconds later is left alone, never killed, and not stopped again until
   its state changes; if it exits later, the pane is put asleep at the next
   check;
3. once the shell is back at its prompt, prints the saved screen, where it
   is, and the resume command (a `command cat` and two `echo`s it runs), so
   the pane still shows the end of the conversation and the way back even
   if the input line gets cleared; then types `cd` to the agent's directory
   and the resume command on the input line, without pressing Enter. It types
   them once per shell: only a new shell, after a herdr restart, gets them
   again.

Enter wakes the agent; Ctrl+C clears the line and leaves a plain shell. The
pane's scrollback is still there. If the agent's directory is deleted later (a
removed worktree), the next check clears the input line and prints that the
folder is gone, with the command to run from a folder you choose: the agent is
never resumed somewhere else on its own.

In the sidebar, the pane keeps the agent's name and shows `asleep` instead of
`idle`. It is reported with state `unknown`, so herdr's priority-sorted agent
list (`agent_panel_sort = "priority"`) ranks it last, below idle agents. herdr
cannot list a pane whose agent has exited under that agent's own kind, so the
plugin reports it under the stand-in kind `maki`: `herdr agent list` and the
socket API show asleep panes as agent `maki`, from source `dnzzl.idle`. Once
you wake the agent, the pane is listed under its real kind again, and gets a
full quiet period before it can sleep again.

To make asleep panes stand out, style `state_text` in
`~/.config/herdr/config.toml`:

```toml
[ui.sidebar.agents]
rows = [
  ["state_icon", "agent", { token = "state_text", rules = [{ equals = "asleep", dim = true }] }],
  ["workspace", "tab"],
]
```

### Across herdr restarts

Asleep panes stay asleep: within half a minute they come back listed as
`asleep` and prefilled, and no agent starts on its own. The scrollback is gone
after a restart; the capture file is then the only record of the last screen,
and it is printed again.

### The background process

Checks run in a background process, one per herdr session. herdr starts it
with its server; after an install it starts the first time you switch panes or
run an action. It stops when the plugin is disabled or removed, or herdr
stops, and moves to a new binary on its own after an update. It logs to
`~/.local/state/herdr/plugins/dnzzl.idle/daemon.log`: one line each time a
pane's state changes, and every error; past 1 MB it moves to `daemon.log.1`.

## Safety

The one rule: **never lose a conversation.**

- **Nothing is stopped before the way back is on disk.** The screen is saved
  and the resume command known before any signal is sent.
- **`SIGTERM` only, never `SIGKILL`.** An agent that won't stop is left
  running.
- **It never starts an agent for you.** It types the resume command; only you
  press Enter. After a herdr restart, nothing wakes on its own.
- **It never touches the pane you're looking at,** or an agent that is
  working, blocked, or running anything besides its MCP or language servers.
- **When in doubt, the pane stays awake and says why.** The `preview` action
  shows the reason for every pane.
- **No network after install.** The plugin talks to herdr's local socket and
  runs `ps`; it writes only to its own config and state directories.

## Config

`config.toml` in the plugin config directory
(`herdr plugin config-dir dnzzl.idle`), or run the `config` action. A missing
file or key means the default. Changes apply on the next check, without a
restart. An invalid file is reported with every problem; checks keep the last
valid config, or do nothing when there was none.

```toml
idle_minutes = 120      # quiet time before a pane is put asleep
interval_minutes = 10   # time between two checks
exclude = []            # pane ids or agent session ids never put asleep
dry_run = false         # true: log what would happen, do nothing
```

Use `exclude` for agents that run loops or scheduled jobs inside their session:
the plugin cannot see that work and would otherwise put them asleep.

## Actions

From herdr's action menu, or the command line:

```bash
herdr plugin action invoke preview  --plugin dnzzl.idle
herdr plugin action invoke idle-now --plugin dnzzl.idle
herdr plugin action invoke config   --plugin dnzzl.idle
```

| Action | Does |
|---|---|
| `preview` | Lists every agent pane and why it would or would not be put asleep. Changes nothing. |
| `idle-now` | Runs a check now instead of waiting for the next one. |
| `config` | Opens `config.toml` in your editor, creating it with the defaults. |

## Supported agents

The plugin carries its own resume recipe for each kind herdr can resume, ported
from herdr 0.9.5. A kind stays "unverified" until someone has put a real pane
asleep and resumed the same session with it, which also confirms the harness
saves its session when stopped. Its herdr integration must be installed, or
herdr does not know the session id and the pane is left alone.

| Kind | Status |
|---|---|
| `claude` | ✅ verified |
| `opencode` | ✅ verified; see [known gaps](#known-gaps) |
| `pi` | resumes the same session file; replies not checked |
| `codex`, `copilot`, `devin`, `droid`, `kimi`, `mastracode`, `omp`, `hermes`, `qodercli`, `qwen`, `kilo`, `cursor`, `agy`, `grok`, `letta` | unverified |

Kinds herdr cannot resume (`amp`, `gemini`, `cline`, `kiro`, `maki`, `muse`)
are never put asleep. When herdr adds or changes a kind, this table follows.

**Using an unverified kind?** Put one pane asleep, press Enter, and
[tell us what happened](https://github.com/DnzzL/herdr-idle/issues/new/choose).
One report moves a kind to verified.

## FAQ

**Will I lose my conversation?** The agent saves it, as it does when you quit
it yourself; herdr-idle only resumes it. One case to know: agents delete old
conversations on their own. Claude Code removes those untouched for
`cleanupPeriodDays` (30 by default), asleep or not. If you leave panes asleep
for weeks, raise it in `~/.claude/settings.json`.

**What if I'm in the middle of something?** A pane you're focused on, or whose
agent is working, waiting on you, or running a command, is never put asleep.
Its quiet clock restarts on any change.

**Some agents should never sleep.** Add their pane id or session id to
`exclude`. `preview` lists the pane ids.

**Why does `herdr agent list` call my asleep panes `maki`?** herdr can't list
a stopped agent under its own kind; `maki` is the stand-in. The sidebar still
shows the real name. See [How it works](#how-it-works).

**How do I remove it?** `herdr plugin uninstall dnzzl.idle`. Asleep panes keep
their typed resume command; press Enter to wake each one. Config and state
directories survive an uninstall; delete them by hand for a clean slate.

## What it does to your machine

- Sends `SIGTERM` to the foreground process group of qualifying agent panes.
  Never `SIGKILL`.
- Types into those panes' shells: one `command cat` of the screen capture and
  `echo`s of its path and the resume command, run with Enter, then the resume
  command, without Enter.
- Runs `ps` to see what each candidate agent is running.
- Writes one state file per herdr session and one screen capture per asleep
  pane in the plugin state directory. Closing a pane, or the tab or
  workspace holding it, deletes its entry and capture.
- Edits nothing outside its own config and state directories.

## Known gaps

- The `cat`/`echo` line lands in shell history unless the shell skips lines
  starting with a space (zsh `HIST_IGNORE_SPACE`, bash `HISTCONTROL=ignorespace`).
- After a herdr restart, an asleep pane whose shell is busy (a command typed
  before it was restored as asleep) is restored once the shell is free.
- An asleep pane herdr does not bring back after a restart keeps its state
  entry and capture file until the next pane, tab or workspace is closed.
- An `opencode` pane woken from asleep gets no session id from herdr's
  opencode integration (nor does any `opencode --session …` run), so it is
  not put asleep again until opencode is started fresh in it.
- Agents delete old conversations on their own: Claude Code removes those
  untouched for `cleanupPeriodDays` (30 by default). A pane asleep longer than
  that keeps a resume command for a conversation that is gone.

## Contributing

Reports of agent kinds tested in the wild are the most wanted contribution,
worth more than code: see [CONTRIBUTING.md](CONTRIBUTING.md). Bugs and ideas go
to [issues](https://github.com/DnzzL/herdr-idle/issues/new/choose).

```bash
go test -race -shuffle=on ./...
```

Pushing a `v*` tag that matches `version` in `herdr-plugin.toml` publishes a
release with the four binaries, `checksums.txt`, and that version's
[CHANGELOG](CHANGELOG.md) section as notes.

## License

[MIT](LICENSE)
