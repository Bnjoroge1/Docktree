# Docktree — drop-in Docker Compose for git worktrees

Running Docker Compose across multiple git worktrees collides on three things:
project name, host ports, and container names. Docktree gives every worktree
its own isolated Compose project — unique ports (auto-allocated from a fixed
range), unique container names, unique volumes — by generating override files
on top of your existing `docker-compose.yml`. Zero config to start; the
overrides are derived, not copies.

It exists because agents work better when they can spin up their own isolated
stack to test end-to-end, without stepping on a sibling worktree or your own
running services. Docktree is agent-native: every command speaks `--json`
(see [For AI agents](#for-ai-agents)).

## Install

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/Bnjoroge1/docktree/main/install.sh | sh

# Homebrew
brew tap Bnjoroge1/tap
brew trust Bnjoroge1/tap   # Homebrew 4.4+ requires trusting unsigned custom taps
brew install docktree

# From source
go install github.com/bnjoroge/docktree/cmd/docktree@latest
```

Pin a version or relocate the binary via environment variables:

```bash
VERSION=v0.1.0     curl -fsSL https://raw.githubusercontent.com/Bnjoroge1/docktree/main/install.sh | sh
INSTALL_DIR=~/.local/bin curl -fsSL https://raw.githubusercontent.com/Bnjoroge1/docktree/main/install.sh | sh
```

## How it works

1. You have a Compose project in a git repo.
2. Create a worktree: `git worktree add ../feature-x feature-x`.
3. In the worktree, run `docktree up`.

Docktree discovers your compose files, allocates unique ports from `41000–49999`,
writes a generated override with isolated names and ports, and runs
`docker compose up -d`. Worktrees never collide.

## Commands

```bash
docktree up           # start the current worktree's project
docktree down         # stop and remove (add -v to drop volumes too)
docktree stop         # stop without removing
docktree status       # show running services
docktree ports        # show this worktree's allocated host ports (--all for every worktree)
docktree volumes      # show managed volumes (--all for every worktree)
docktree clean        # remove stale resources from missing worktrees (--dry-run first)
docktree sync         # propagate setup.copy files to every worktree
docktree platform up  # start the repo-scoped shared-services tier (when configured)
```

Docker Compose passthroughs (`build`, `config`, `logs`, `exec`, `run`, `ls`,
…) work too, with this worktree's project context pre-filled. Run
`docktree help` or `docktree <cmd> --help` for the authoritative reference.

Global flag: `--json` (before the subcommand) emits machine-readable JSON for
every native command, including `help` and `version`.

## Configuration

Docktree works without configuration. To customize, create `docktree.yml`:

```yaml
compose:
  files:
    - docker-compose.yml

setup:
  copy:
    - .env
  symlink:
    - node_modules

ports:
  bind_host: "127.0.0.1"
  range: "41000-49999"

volumes:
  share:
    - cache-data   # share this volume across worktrees
```

### Note on shared databases and secret wrappers

When using `shared.services` with `tenancy: per_database`, Docktree can only rewrite or inject database URLs that are visible as Compose environment variables. If your app constructs `DATABASE_URL` inside a runtime shell command, for example through Infisical or another secrets wrapper, Docktree cannot safely rewrite that inline command.

Prefer letting the app read a Docktree-provided `DATABASE_URL` from the environment, or make the shell command respect an existing `DATABASE_URL` before constructing a fallback value. If the command must always build the database URL from secrets at runtime, use isolated per-worktree database containers instead of shared `per_database` tenancy.

## For AI agents

Docktree ships an agent skill that teaches coding agents (Claude Code, Codex,
Cursor, OpenCode, and 60+ others) how to drive the CLI: which commands honor
`--json`, the error envelope, and the stderr/stdout split.

Install via [`npx skills`](https://github.com/vercel-labs/skills):

```bash
npx skills add Bnjoroge1/Docktree              # current project
npx skills add Bnjoroge1/Docktree -g           # globally
npx skills add Bnjoroge1/Docktree --list       # preview without installing
```

See [`skills/`](./skills/) for the skill source.

## Windows

Use through WSL2 with Docker Desktop's WSL integration enabled.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). `main` is protected; merges are gated
on code-owner review.

## License

MIT
