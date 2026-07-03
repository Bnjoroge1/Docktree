# drop-in docker compose for git worktrees

![Docktree Demo](demo/docktree-demo.gif)

If you wanna run multiple Docker Compose across multiple git worktrees collides, especially for your agents, you will inevitably face collisions on project name, host ports, and container names. Docktree gives every worktree its own isolated Compose project, unique ports (auto-allocated), container names,  and volumes by generating override files on top of your existing `docker-compose.yml`. Unlike other solutions where you either have to run the dock er compose projects in docker(DiND is imo unnecessary) or youy have to rewrite your docker compose yaml files, Docktree doesnt touch your docker compose files. You can run it with zero config to start, but you can customize it as you want.

Docktree works great for use cases such as:

1. **Parallel AI Agents.**
  1. **** Running parallel ai agents independently. Each agent gets its own docker compose stack(database etc), they can do some work and send you a link of their completed work for review.
2. **Review Multiple PRs Locally**

  Check out several pull requests as worktrees and run them side by side. Compare UI, API behavior, and database changes without constantly tearing stacks down.

Docktree is agent-native: every command speaks `--json` (see [For AI agents](#for-ai-agents)).

## Install

```bash
# macOS / Linux/ Wsl2
curl -fsSL docktree.dev/install.sh | sh

# Homebrew
brew tap Bnjoroge1/tap
brew trust Bnjoroge1/tap   # Homebrew 4.4+ requires trusting unsigned custom taps
brew install docktree

# From source
go install github.com/bnjoroge/docktree/cmd/docktree@latest
```

Pin a version or relocate the binary via environment variables:

```bash
VERSION=v0.1.0     curl -fsSL https://docktree.dev/install.sh | sh
INSTALL_DIR=~/.local/bin curl -fsSL https://docktree.dev/install.sh | sh
```

## How it works

1. You have a Compose project in a git repo.
2. Create a worktree: `git worktree add ../feature-x feature-x` or use an existing one.
3. In the worktree, run `docktree up`.



See: docktree.dev/docs for full docs and reference

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
docktree proxy        # reverse proxy routing by hostname to worktree ports
docktree tunnel start # expose current worktree externally via Cloudflare Tunnel
```

If Docker has many stale bridge networks, `docktree up --prune-networks`
removes unused Docker networks before starting. Docktree also detects Docker
address-pool exhaustion during `up`, cleans partial resources, and suggests
rerunning with `--prune-networks` (or `docker network prune --force`).

Docker Compose passthroughs (`build`, `config`, `logs`, `exec`, `run`, `ls`,…) work too, with this worktree's project context pre-filled. Run `docktree help` or `docktree <cmd> --help` for the authoritative reference.

Global flag: `--json` (before the subcommand) emits machine-readable JSON for every native command, including `help` and `version`.

## For AI agents

Docktree ships agent skills that teach coding agents (Claude Code, Codex,

Cursor, OpenCode, and 60+ others) how to drive the CLI: which commands honor

`--json`, the error envelope, and the stderr/stdout split. A second skill

walks the agent through `docktree init` to generate `docktree.yml`.

Install via [`npx skills`](https://github.com/vercel-labs/skills):

```bash
npx skills add Bnjoroge1/docktree              # current project
npx skills add Bnjoroge1/docktree -g           # globally
npx skills add Bnjoroge1/docktree --list       # preview without installing
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
```

### Shared databases and secret wrappers

With `shared.services` and `tenancy: per_database`, Docktree rewrites database URLs that are visible as Compose environment variables. If your app builds `DATABASE_URL` inside a runtime shell command (Infisical, Doppler, Vault, etc.), Docktree cannot safely rewrite it — prefer reading a Docktree-provided `DATABASE_URL` from the environment, have the wrapper respect an existing one, or fall back to isolated per-worktree database containers.

## Windows

Use through WSL2 with Docker Desktop's WSL integration enabled.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). 

## License

MIT