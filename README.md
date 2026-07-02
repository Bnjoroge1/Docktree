# Docktree — drop-in Docker Compose for git worktrees

Running Docker Compose across multiple git worktrees collides on three things: project name, host ports, and container names. Docktree gives every worktree its own isolated Compose project — unique ports (auto-allocated), unique container names, unique volumes — by generating override files on top of your existing `docker-compose.yml`. Zero config to start, but you can customize it as you want.

It exists because agents work better when they can spin up their own isolated stack to test end-to-end, without stepping on a sibling worktree or your own running services.

Docktree is agent-native: every command speaks `--json` (see [For AI agents](#for-ai-agents)).

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
docktree proxy        # reverse proxy routing by hostname to worktree ports
docktree tunnel start # expose worktree externally via Cloudflare Tunnel or ngrok
docktree clean        # remove stale resources from missing worktrees (--dry-run first)
docktree sync         # propagate setup.copy files to every worktree
docktree platform up  # start the repo-scoped shared-services tier (when configured)
```

Docker Compose passthroughs (`build`, `config`, `logs`, `exec`, `run`, `ls`,
…) work too, with this worktree's project context pre-filled. Run
`docktree help` or `docktree <cmd> --help` for the authoritative reference.

Global flag: `--json` (before the subcommand) emits machine-readable JSON for
every native command, including `help` and `version`.

## For AI agents

Docktree ships agent skills that teach coding agents (Claude Code, Codex,
Cursor, OpenCode, and 60+ others) how to drive the CLI: which commands honor
`--json`, the error envelope, and the stderr/stdout split. A second skill
walks the agent through `docktree init` to generate `docktree.yml`.

Install via [`npx skills`](https://github.com/vercel-labs/skills):

```bash
npx skills add Bnjoroge1/Docktree              # current project
npx skills add Bnjoroge1/Docktree -g           # globally
npx skills add Bnjoroge1/Docktree --list       # preview without installing
```

See [`skills/`](./skills/) for the skill source.

## Configuration

Docktree works without configuration. To customize, create `docktree.yml` (or
run `docktree init` to generate one from your compose files):

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

proxy:
  port: 8320       # reverse proxy listen port
  host: 127.0.0.1  # reverse proxy listen host
  tld: localhost   # top-level domain for instance routing

tunnel:
  provider: cloudflare  # "cloudflare" or "ngrok"
  port: 0               # 0 = auto-detect HTTP port (80/8080)
```

### Shared databases and secret wrappers

With `shared.services` and `tenancy: per_database`, Docktree rewrites database URLs that are visible as Compose environment variables. If your app builds `DATABASE_URL` inside a runtime shell command (Infisical, Doppler, Vault, etc.), Docktree cannot safely rewrite it — prefer reading a Docktree-provided `DATABASE_URL` from the environment, have the wrapper respect an existing one, or fall back to isolated per-worktree database containers.

## Windows

## Windows

On Windows, use it through WSL2 with Docker Desktop WSL integration enabled.
Use through WSL2 with Docker Desktop's WSL integration enabled.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). `main` is protected; merges are gated
on code-owner review.

## License

MIT
