# Issue tracker: Backlog.md

Issues live in the `backlog/` folder and are managed exclusively through the `backlog` CLI (v1.53.0). Never edit `backlog/tasks/*.md`, drafts, docs, or decisions directly — the CLI keeps metadata, relationships, and history consistent.

## Operations

- **List**: `backlog search "" --plain --limit 20`
- **Read**: `backlog task <id> --plain`
- **Create**: `backlog task create "Title" -s "To Do" --ac "<criterion>"`
- **Change status**: `backlog task edit <id> -s "In Progress"` / `-s "Done"`
- **Acceptance criteria / notes**: `backlog task edit <id> --ac "..." --append-notes "..."` (repeat `--append-notes` per line)
- **Blocking**: `--dep task-1,task-2`; never start a blocked task
- **Archive**: `backlog task archive <id>`
- **Labels**: `backlog task edit <id> --add-label ready-for-agent`

## When a skill says "publish to the issue tracker"

Create a task with `backlog task create`. Put the spec or question in the description or via `--append-notes`, and list the concrete done conditions as `--ac` entries.

## When a skill says "fetch the relevant ticket"

Run `backlog task <id> --plain`. The user normally passes the task ID directly.

## Project conventions

- Statuses: `To Do`, `In Progress`, `Done` (from `backlog/config.yml`).
- Triage roles are **labels**, not statuses — see `triage-labels.md`.
- Task prefix `task`, IDs unpadded (`task-1`, `task-2`).
- Filesystem-only mode: no git integration, no auto-commit, no branch awareness. Committing is a separate manual step.
- Labels are configured in `backlog/config.yml`; `backlog config set labels` refuses, so edit the list there directly.
