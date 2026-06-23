# Docktree - drop-in docker compose for managing your worktrees

If you work with Docker Compose and want to launch multiple instances(whether these are in the form of clones/worktrees/jj workspaces), you will inevitably run into 3 different types of conflicts: the docker compose project name, port conflicts and service name conflicts. 

Why would you want to run multiple instances at once? Maybe you want to hot-fix something that happened in prod without stashing your ongoing changes or it cant quite fit in a branch. To be honest, there wasn't much of a reason before agents became mainstream. Before building this project, i was basically either just running one instance at a time, but this limited the agents ability to really work autonomously,  or setting the compose project name and ports or letting docker assign a random port for each worktree. Works mostly well until you are running more than even 2 worktrees, now you gotta remember the ports for each worktree, or you could have each worktree run its in own docker daemon inside another container. Could totally work but that ends up being a DinD setup which requires your containers to run as privileged containers, among a host(pun most intended) of other performance issues, which in my opinion is unnecessary overhead. 

This project is basically a layer over git worktrees(for now, adding support JJ workspaces is in the roadmap), but specifically to address docker compose uniqueness constraints mentioned above. This means it could easily work or complement your existing way of managing worktrees, whether using worktrunk, something homegrown, desktop orchestrators or terminal orchestrators. It's a drop-in replacement for docker compose that works out of the box with zero config(you can further customize it if you want), by overriding your docker compose files. 

The main use case I've been using for is to have the agent work autonomously on specific tasks, and more crucially enable them to fully test the end to end flow. Each worktree gets its own isolated services(the default), or for either of {mysql, postgres, mongodb} you can give each worktree its own database in a shared container with no application changes. You can then hook up each agent with Playwright or Agent-browser or (Bombadil)[[https://github.com/antithesishq/bombadil]]([https://github.com/antithesishq/bombadil]](https://github.com/antithesishq/bombadil])) and have the agents prepare screenshots or a report of their work. 

Docktree was designed to be agent-native from the start so agents could manage the lifecycles of the docktree instances themselves so they can bring up their own docktrees, or drive multiple instances(this becomes very useful if they want to actually close the loop with end to end testing.). 

2. 

Each worktree gets its own isolated project with unique ports, container names, and volumes. They're all managed through generated Compose overrides.

## Install

```bash
# One-line install (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/Bnjoroge1/docktree/main/install.sh | sh

# Or via Homebrew
brew tap Bnjoroge1/tap
brew trust Bnjoroge1/tap  # Homebrew 4.4+ requires trusting unsigned custom taps
brew install docktree

# Or from source
go install github.com/bnjoroge/docktree/cmd/docktree@latest
```

### Install options

```bash
# Install a specific version
VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/bnjoroge/docktree/main/install.sh | sh

# Install to a custom directory
INSTALL_DIR=~/.local/bin curl -fsSL https://raw.githubusercontent.com/bnjoroge/docktree/main/install.sh | sh
```

## Usage

```bash
# Start services in the current worktree
docktree up

# Stop services
docktree down

# Show running status
docktree status

# Show allocated ports
docktree ports

# Clean stale resources
docktree clean
```

## How it works

1. You have a Docker Compose project with services
2. Create a git worktree: `git worktree add ../feature-branch feature-branch`
3. Run `docktree up` in the worktree
4. Docktree:
  - Detects your compose files
  - Allocates unique ports from the range 41000-49999
  - Generates an override file with isolated container names and ports
  - Starts the project with `docker compose up -d`

Each worktree runs independently without any port conflicts, or name collisions.

## Configuration

Docktree works without configuration. If you need to customize behavior, create `docktree.yml`:

```yaml
setup:
  copy:
    - .env
  symlink:
    - node_modules

compose:
  files:
    - docker-compose.yml

ports:
  bind_host: "127.0.0.1"
  range: "41000-49999"

volumes:
  share:
    - cache-data  # Share this volume across worktrees
```

### Note on shared databases and secret wrappers

When using `shared.services` with `tenancy: per_database`, Docktree can only rewrite or inject database URLs that are visible as Compose environment variables. If your app constructs `DATABASE_URL` inside a runtime shell command, for example through Infisical or another secrets wrapper, Docktree cannot safely rewrite that inline command.

Prefer letting the app read a Docktree-provided `DATABASE_URL` from the environment, or make the shell command respect an existing `DATABASE_URL` before constructing a fallback value. If the command must always build the database URL from secrets at runtime, use isolated per-worktree database containers instead of shared `per_database` tenancy.

## Windows

On Windows, use it through WSL2 with Docker Desktop WSL integration enabled.

## Commands


| Command              | Description                        |
| -------------------- | ---------------------------------- |
| `docktree up`        | Start services in current worktree |
| `docktree down`      | Stop services                      |
| `docktree status`    | Show managed services              |
| `docktree ports`     | Show allocated ports               |
| `docktree clean`     | Remove stale resources             |
| `docktree --version` | Print version                      |


## Flags


| Flag              | Description                   |
| ----------------- | ----------------------------- |
| `--json`          | Output as JSON                |
| `up -f <file>`    | Use specific compose file     |
| `clean --dry-run` | Preview what would be removed |
| `clean --yes`     | Skip confirmation prompt      |
| `clean --volumes` | Also remove volumes           |


## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for how to set
up a dev environment, run the tests, and open a pull request. Note that `main`
is protected and merges are gated on code-owner review.

## License

MIT