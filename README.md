# herdr-idle

**Puts idle agent panes asleep to free their memory.** The pane stays where it
is, still listed in the sidebar, with the command that resumes the same session
typed on its input line. Press Enter to wake it.

A [herdr](https://herdr.dev) plugin for macOS and Linux.

## Why

An agent pane waiting days for a PR review or a reply still holds its agent and
its MCP servers in memory, often 1–2 GB each. Closing it frees the memory but
loses its place in the layout, its spot in the agent list, and the easy way back
to the right session. herdr only resumes agents when its server restarts.

## Install

```bash
herdr plugin install DnzzL/herdr-idle
```

The install fetches a prebuilt, checksum-verified binary for macOS or Linux
(arm64/amd64), so Go is not needed. When there is no binary for your platform,
the checksum doesn't match, or the binary doesn't run, it builds from source
instead, which needs [Go](https://go.dev/dl/).

Once installed, a background check runs every 10 minutes in each herdr session.
With no config file, the defaults below apply.

## What an asleep pane looks like

A pane is put asleep when all of these hold:

- herdr shows its agent `idle` or `done`, with no change for `idle_minutes`;
- you are not focused on the pane;
- the agent runs nothing besides its MCP or language servers (no shell, dev
  server or test run). A server is recognised by its program or package name:
  a word `mcp`, `lsp` or `langserver`, `language-server`,
  `@modelcontextprotocol/…`, or `gopls`, `rust-analyzer`, `clangd`, `tsserver`,
  `jdtls`;
- its kind is in the [supported table](#supported-kinds) and herdr knows its
  session id;
- it is not in `exclude`.

Then the plugin:

1. saves the agent's recent screen to a file in the plugin state directory;
2. stops the agent and its MCP servers with `SIGTERM`. An agent still running
   30 seconds later is left alone, never killed, and not stopped again until
   its state changes; if it exits later, the pane is put asleep at the next
   check;
3. once the shell is back at its prompt, prints the saved screen and where it
   is (a `cat` and an `echo` it runs), so the pane still shows the end of the
   conversation; then types `cd` to the agent's directory and the
   resume command on the input line, without pressing Enter. It types them
   once per shell: only a new shell, after a herdr restart, gets them again.

The resume command continues the same session, in the same directory, with the
flags you started the agent with (permission mode, model…). Enter wakes the
agent; Ctrl+C clears the line and leaves a plain shell. The pane's scrollback
is still there.

In the sidebar, the pane keeps the agent's name and shows `asleep` instead of
`idle`. It is reported with state `unknown`, so herdr's priority-sorted agent
list (`agent_panel_sort = "priority"`) ranks it last, below idle agents. herdr cannot list a pane whose agent has exited under that agent's own
kind, so the plugin reports it under the stand-in kind `maki`: `herdr agent
list` and the socket API show asleep panes as agent `maki`, from source
`dnzzl.idle`. Once you wake the agent, the pane is listed under its real kind
again, and gets a full quiet period before it can sleep again.

Checks run in a background process that herdr starts with its server, one per
herdr session. It stops when the plugin is disabled or removed, or herdr stops.

After a herdr restart, asleep panes stay asleep: within half a minute they come
back listed as `asleep` and prefilled, and no agent starts on its own. The
scrollback is gone after a restart; the capture file is then the only record of
the last screen, and it is printed again.

To make asleep panes stand out, style `state_text` in
`~/.config/herdr/config.toml`:

```toml
[ui.sidebar.agents]
rows = [
  ["state_icon", "agent", { token = "state_text", rules = [{ equals = "asleep", dim = true }] }],
  ["workspace", "tab"],
]
```

## Config

`config.toml` in the plugin config directory
(`herdr plugin config-dir dnzzl.idle`), or run the `config` action. A missing
file or key means the default. Changes apply on the next check, without a
restart. An invalid file is reported with every problem; checks keep the last
valid config, or do nothing when there was none.

```toml
idle_minutes = 60       # quiet time before a pane is put asleep
interval_minutes = 10   # time between two checks
exclude = []            # pane ids or agent session ids never put asleep
dry_run = false         # true: log what would happen, do nothing
```

Use `exclude` for agents that run loops or scheduled jobs inside their session:
the plugin cannot see that work and would otherwise put them asleep.

## Actions

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

## Supported kinds

The plugin carries its own resume recipe for each kind herdr can resume, ported
from herdr 0.9.5. A kind stays "unverified" until someone has put a real pane
asleep and resumed the same session with it, which also confirms the harness
saves its session when stopped. Its herdr integration must be installed, or
herdr does not know the session id and the pane is left alone.

| Kind | Status |
|---|---|
| `claude` | unverified |
| `codex` | unverified |
| `copilot` | unverified |
| `devin` | unverified |
| `droid` | unverified |
| `kimi` | unverified |
| `mastracode` | unverified |
| `pi` | unverified |
| `omp` | unverified |
| `hermes` | unverified |
| `opencode` | unverified |
| `qodercli` | unverified |
| `qwen` | unverified |
| `kilo` | unverified |
| `cursor` | unverified |
| `agy` | unverified |
| `grok` | unverified |
| `letta` | unverified |

Kinds herdr cannot resume (`amp`, `gemini`, `cline`, `kiro`, `maki`, `muse`)
are never put asleep. When herdr adds or changes a kind, this table follows.

## What it does to your machine

- Sends `SIGTERM` to the foreground process group of qualifying agent panes.
  Never `SIGKILL`.
- Types into those panes' shells: one `cat` of the screen capture and `echo` of
  its path, run with Enter, then the resume command, without Enter.
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
- When the background process outlives a quick herdr restart, its log is lost
  from then on: herdr no longer reads it.

## Development

```bash
go test -race -shuffle=on ./...
```

Pushing a `v*` tag that matches `version` in `herdr-plugin.toml` publishes a
release with the four binaries, `checksums.txt`, and that version's
[CHANGELOG](CHANGELOG.md) section as notes.
