# Docktree

Run Docker Compose services across multiple git worktrees without port conflicts.

Each worktree gets its own isolated project with unique ports, container names, and volumes — all managed through generated Compose overrides.

## Install

```bash
# From source
go install github.com/bnjoroge/docktree/cmd/docktree@latest

# Or build locally
git clone https://github.com/bnjoroge/docktree.git
cd docktree
go build -o docktree ./cmd/docktree
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

Each worktree runs independently — no port conflicts, no name collisions.

## Configuration

Docktree works without configuration. If you need to customize behavior, create `docktree.yml`:

```yaml
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

## Commands

| Command | Description |
|---------|-------------|
| `docktree up` | Start services in current worktree |
| `docktree down` | Stop services |
| `docktree status` | Show managed services |
| `docktree ports` | Show allocated ports |
| `docktree clean` | Remove stale resources |
| `docktree --version` | Print version |

## Flags

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `up -f <file>` | Use specific compose file |
| `clean --dry-run` | Preview what would be removed |
| `clean --yes` | Skip confirmation prompt |
| `clean --volumes` | Also remove volumes |

## License

MIT