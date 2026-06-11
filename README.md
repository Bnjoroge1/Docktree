# Docktree

This project exists to solve a very straightforward problem. If you are working with Docker Compose services across multiple git worktrees and you want them to run simultaneously, you will inevitably run into 3 different types of conflicts: the docker compose project name, port conflicts with the different instances of the same service(e.g two postgres db containers both wanting to use port 5432), and service name conflicts. 

Why would I want to run multiple instances at once? To be honest, there wasnt much of a reason before agents became mainstream. Before building it, i was basically either a) just running one instance at a time, but this limited the agents ability to really work autonomously, b) setting the compose project name and ports or letting docker assign a random port for each worktree. Works mostly well until you are running more than even 2 worktrees, now you gotta remember the ports for each worktree, c) have each worktree run its in own docker daemon inside another container. Could tottally work but that ends up being a DinD setup which requires your containers to run as privileged containers which in my opinion is unnecessary overhead. Not to mention the other resource overheads involved with could also try running  Git worktrees (unfortunately) seems like what the industry has settled on as a way to paralellize agent output. This is basically a layer over that, but specifically to address docker compose uniqueness requirements. 
The main use case I've been using for is to have the agent work autonomously on specific tasks, and more crucially enable them to fully test the end to end flow. Each worktree gets its own services, and its own database. You can isolate the database containers per worktree, or give each worktree its own database in a shared container. Currently supported for postgresql, mysql, and mongodb with no application-config changes. You can then hook up each agent with something like playwright or (Bombadil)[https://github.com/antithesishq/bombadil] or agent-browser and have the agents prepare screenshots or a report of their work. 

Docktree was designed to be agent-native from the start so agents could spin up their own docktrees, or drive multiple instances(this becomes very useful if they want to actually close the loop with end to end testing. )
2. 

Each worktree gets its own isolated project with unique ports, container names, and volumes. They're all managed through generated Compose overrides.

## Install

```bash
# One-line install (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/Bnjoroge/docktree/main/install.sh | sh

# Or via Homebrew
brew tap bnjoroge/tap
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


## License

MIT