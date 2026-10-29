# Changelog

What changed for someone using the plugin. Dates are release dates.

## v0.1.0 — unreleased

- First release. Agent panes that stay quiet for two hours are put **asleep**:
  the agent and its MCP servers are stopped to free their memory, and the
  pane stays where it was, back at its shell, with the command that resumes
  the same session, with the same flags, in the same directory, typed on
  the input line.
  Press Enter to wake it; Ctrl+C to clear the line and use a plain shell.
- Asleep panes stay in the sidebar agent list under their real name, tagged
  `asleep`. `herdr agent list` shows them as agent `maki`: that is the
  stand-in herdr needs to keep a stopped agent listed.
- The agent's recent screen is saved to a file before it is stopped; the
  asleep pane prints it, where it is saved, and the resume command.
- A pane is never put asleep while you are focused on it, while its agent is
  working or blocked, while it runs anything besides its MCP or language
  servers, or when it is excluded. Agents herdr cannot resume, or without a
  session id, are left alone. An agent that hasn't stopped 30 seconds after
  being asked is left running, never killed; if it exits later, its pane is
  put asleep at the next check.
- A Claude pane is left awake when its session was not started
  interactively: a `claude -p` run in the pane can make herdr take the
  one-shot session as the pane's, and Enter would resume the wrong
  conversation.
- Asleep panes stay asleep across a herdr restart instead of all waking at
  once, and come back listed and prefilled.
- `config.toml` sets `idle_minutes` (120), `interval_minutes` (10), `exclude`
  and `dry_run`. Changes apply on the next check, without a restart.
- Starts working right after install, from the first pane switch, without a
  herdr restart; picks up a new version on its own after an update. Its log
  is `daemon.log` in the plugin state directory: one line per change and
  per error, never more than about 2 MB.
- Actions: `preview` lists every agent pane and why it would or would not
  sleep; `idle-now` runs a check now; `config` opens the config file.
- Installs a prebuilt, checksum-verified binary on macOS and Linux
  (arm64/amd64), so Go is not needed; anything else builds from source.
- Every herdr kind with a resume recipe is supported but marked unverified
  until checked by hand: see the supported-kinds table in the README.
