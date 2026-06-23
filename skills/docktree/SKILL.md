---
name: docktree
description: Drop-in Docker Compose runner that gives each git worktree its own isolated project (unique ports, container names, volumes) and optional shared per-database services. Use when the user mentions "docktree", wants to run Compose services per worktree without port/name collisions, asks to bring a worktree's services up or down, needs the allocated ports for a worktree, wants to clean stale worktree containers/volumes, sync setup files across worktrees, or coordinate the repo-scoped shared platform tier. Triggers include "docktree up/down/status/ports/clean/sync/platform", "spin up this worktree", "what ports is this worktree on", "isolated docker per branch", "shared db per worktree".
allowed-tools: Bash(docktree:*)
---
# docktree

Docktree wraps `docker compose` so every git worktree gets its own isolated

project (unique ports, container names, volumes). The CLI is the source of

truth — this skill only covers what an agent needs to call it correctly.

Install: `brew install Bnjoroge1/tap/docktree` (or see the repo `README.md`).

## Always start here

```bash
docktree help              # full command list
docktree <cmd> --help      # per-command flags, authoritative
```

`--help` ships with the binary, so it never drifts from what's installed.

## Use `--json` for every machine-read

`--json` is a global flag that goes **before** the subcommand:

```bash
docktree --json <cmd> [args]
```

Two categories — know which you're calling before parsing:

| Category | Commands | `--json` |
|---|---|---|
| Native (one JSON object on stdout) | `status`, `ports [--all]`, `volumes [--all]`, `clean [--dry-run]`, `sync`, `create`, `prepare`, `up`, `down`, `stop`, `platform <sub>`, `help`, `version`, `<cmd> --help` | ✅ |
| Docker Compose passthrough (raw docker output) | `build`, `config`, `cp`, `docker`, `exec`, `images`, `kill`, `logs`, `ls`, `pause`, `port`, `pull`, `push`, `restart`, `rm`, `run`, `start`, `top`, `unpause`, `wait`, `watch` | ❌ — pass docker's own flags (e.g. `docktree ls --format json`, `docktree ps --format json`); per-command `--help` is also text-only |

### Help / version JSON shape

```bash
docktree --json help                  # HelpDoc for root, lists every subcommand
docktree --json <cmd> --help          # HelpDoc for one native command
docktree --json version               # {"name":"docktree","version":"..."}
```

`HelpDoc` fields: `command`, `synopsis`, `usage[]`, `options[]` (each
`{flags[], value?, description}`), `arguments[]`, `subcommands[]`, `examples[]`,
`notes[]`, `global_flags[]` (root only).

### stderr stays human-readable

Under `--json`, docktree writes exactly one JSON object to **stdout** for
native commands — including `up`, `down`, `platform up/down`. Docker's progress
UI goes to **stderr**, not stdout. Don't merge them:

```bash
docktree --json up   2>/dev/null | jq .   # clean
docktree --json up   2>&1         | jq .   # BREAKS — docker progress polluted stdout
```

If you want to log docker's progress too, redirect stderr to a file, not stdout.


## Errors

Under `--json` (or any non-TTY stdout) errors are always:

```json
{"error": "<code>", "message": "<text>", "details": <optional>}
```

Exit codes: `0` ok, `1` general, `2` usage, `3` config, `4` docker,

`5` noop, `6` conflict.

## Common workflows

- **Start / stop a worktree**: `docktree --json up 2>/dev/null`,
  `docktree --json down 2>/dev/null`.
- **Is anything running?** `docktree --json status` — `{"stopped":true}` or

  full instance payload.
- **Allocated ports for this worktree**: `docktree --json ports`. Across all

  worktrees: `docktree --json ports --all` (useful to discover sibling

  worktrees' URLs without `cd`ing into them).
- **Before assuming containers exist**: always `status` first — `up` is the

  only command that creates them.
- **Cleanup**: `docktree --json clean --dry-run` first, then

  `docktree --json clean --yes` (add `--volumes` to drop volumes too).
- **Propagate setup files** (`.env`, etc.) to every worktree:

  `docktree --json sync`.
- **Shared platform tier** (when `docktree.yml` defines `shared.services`):

  `docktree --json platform status | up | down`.

## Out of scope

- Authoring `docktree.yml` — read the project's own `docktree.yml` and

  `README.md`; 
- Debugging Compose files — use `docktree config` (passthrough) to see the

  fully-merged compose docktree will run.

