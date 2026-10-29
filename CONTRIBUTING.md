# Contributing

The scope is deliberately narrow: **stop an idle agent, keep its pane, make
Enter bring the same conversation back.** Scheduling, notifications, and
anything that starts agents on its own belong to other plugins. A PR that
grows the plugin past that line is likely to be declined on scope rather than
on quality: open an issue first and we'll work out whether it fits.

## What is most wanted

- **Agent kinds tested in the wild.** Only `claude`, `pi` and `opencode` are
  verified. Put a pane of another kind asleep, press Enter, and file an
  [agent kind report](../../issues/new/choose), whatever happened. A report
  is worth more than code.
- **Processes that wrongly keep a pane awake** (or wrongly let it sleep). The
  preview action says why each pane stays awake; paste its line in a bug.
- **Shells other than zsh.** The plugin types into your shell; reports from
  bash and fish are welcome.

## Working on it

```bash
go build -o bin/herdr-idle . && go test -race ./...
go vet ./... && gofmt -l .          # both must be clean; CI enforces them
herdr plugin link .                 # run your checkout as the installed plugin
```

`herdr plugin link` replaces any GitHub install of the plugin, and the daemon
re-executes itself when the binary changes, so a rebuild is enough to test.
Try changes in a throwaway herdr session (`herdr --session test server`), not
the one holding your real agents.

Config lives in `herdr plugin config-dir dnzzl.idle`; state, saved screens and
`daemon.log` in `~/.local/state/herdr/plugins/dnzzl.idle`. Both survive
uninstalls; delete them by hand for a clean slate.

## House style

- **Never lose a conversation.** Everything needed to resume is on disk before
  an agent is signalled; when in doubt, the pane stays awake and says why.
- **Never start an agent on the user's behalf.** The plugin types the resume
  command; only the user presses Enter.
- **Comments explain why, not what.**
- Tests go through the `Sweeper` and `Daemon` seams with the fakes in
  `internal/sweep`, not through private helpers.
- Every user-visible change gets a `CHANGELOG.md` entry describing what it
  means for someone using the plugin, not what was refactored.
