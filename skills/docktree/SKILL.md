---
name: docktree
description: Drop-in Docker Compose runner that gives each git worktree its own isolated project (unique ports, container names, volumes), optional shared per-database services, a reverse proxy for local hostname routing, and external tunnels. Use when the user mentions "docktree", wants to run Compose services per worktree without port/name collisions, asks to bring a worktree's services up or down, needs the allocated ports for a worktree, wants to clean stale worktree containers/volumes, sync setup files across worktrees, coordinate the repo-scoped shared platform tier, or expose a worktree via proxy or tunnel. Triggers include "docktree up/down/status/ports/clean/sync/platform/proxy/tunnel", "spin up this worktree", "what ports is this worktree on", "isolated docker per branch", "shared db per worktree", "reverse proxy worktree", "tunnel worktree externally".
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
| Native (one clean JSON object on stdout) | `status`, `ports [--all]`, `volumes [--all]`, `clean [--dry-run]`, `sync`, `create`, `prepare`, `up`, `down`, `stop`, `platform <sub>`, `help`, `version`, `<cmd> --help` *(all native commands including proxy/tunnel)* | ✅ |
| Daemon / long-running (emits one startup JSON immediately) | `proxy` (routes by hostname; writes one startup `ProxyResult` JSON to stdout, then redirects server logs to stderr. Blocks until Ctrl+C, returns no second render on shutdown) | ✅ |
| Docker Compose passthrough (raw docker output) | `build`, `config`, `cp`, `docker`, `exec`, `images`, `kill`, `logs`, `ls`, `pause`, `port`, `pull`, `push`, `restart`, `rm`, `run`, `start`, `top`, `unpause`, `wait`, `watch` | ❌ — pass docker's own flags (e.g. `docktree ls --format json`, `docktree ps --format json`); per-command `--help` is also text-only |

### Help / version JSON shape

```bash
docktree --json help                  # HelpDoc for root, lists every subcommand
docktree --json <cmd> --help          # HelpDoc for any native command (including proxy/tunnel)
docktree --json version               # {"name":"docktree","version":"0.5.0"}
```

`HelpDoc` fields: `command`, `synopsis`, `usage[]`, `options[]` (each
`{flags[], value?, description}`), `arguments[]`, `subcommands[]`, `examples[]`,
`notes[]`, `global_flags[]` (root only).

### stderr stays human-readable

Under `--json`, docktree writes exactly one JSON object to **stdout** for
native commands — including `up`, `down`, `platform up/down`, `proxy`, and `tunnel`.
All progress and server logs go to **stderr**, not stdout. Don't merge them:

```bash
docktree --json up   2>/dev/null | jq .   # clean
docktree --json up   2>&1         | jq .   # BREAKS — docker progress polluted stdout
```

If you want to log progress too, redirect stderr to a file, not stdout.

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
- **Docker network pool pressure**: if `up` reports exhausted Docker address
  pools, rerun `docktree --json up --prune-networks 2>/dev/null` (or manually
  `docker network prune --force`) after checking the user is comfortable pruning
  unused Docker networks. `up` now cleans partial resources after this failure.
- **Propagate setup files** (`.env`, etc.) to every worktree:
  `docktree --json sync`.
- **First-time hint**: when `docktree up` starts services in a worktree that
  has no `docktree.yml` and the compose file contains shareable services
  (postgres, redis, minio, etc.), the human-readable output includes a
  one-line tip. In `--json` mode the same info is in the `hint` field of
  the `UpResult`. Use this to decide whether to help the user set up
  `shared.services` in `docktree.yml`.
- **Shared platform tier** (when `docktree.yml` defines `shared.services`):
  `docktree --json platform status | up | down`.
- **Reverse proxy for worktree access**: `docktree proxy --port 8320`
  (or `docktree proxy` with defaults). Routes `http://<instance>.localhost`
  to each worktree's allocated ports. In `--json` mode, emits one startup
  `ProxyResult` JSON immediately on stdout, and writes proxy server logs to
  stderr. Run in a background terminal; tear down with Ctrl+C.
- **Expose a worktree externally**: `docktree tunnel start`
  (defaults to cloudflared, uses the first HTTP port). In `--json` mode,
  returns `TunnelStartResult` cleanly. Specify a service:
  `docktree tunnel start --service ui`. Stop with `docktree tunnel stop`
  (returns `TunnelStoppedResult` under `--json`). List all active tunnels:
  `docktree tunnel list` (returns `TunnelListResult`). JSON status:
  `docktree --json tunnel status` (returns `TunnelStatusResult`, including
  status `"none"` when no tunnel exists).

## Out of scope

- Authoring `docktree.yml` — read the project's own `docktree.yml` and
  `README.md`; see the [`docktree-init`](../docktree-init/SKILL.md) skill.
- Debugging Compose files — use `docktree config` (passthrough) to see the
  fully-merged compose docktree will run.
