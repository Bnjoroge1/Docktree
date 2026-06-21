# Docktree vs Docker Compose Compatibility

## CLI Commands

| Compose command | Docktree | Notes |
|---|---|---|
| `up` | `docktree up` | Full lifecycle: discovers/parses compose files, allocates ports, generates overrides, runs `docker compose up -d` |
| `down` | `docktree down` | Stops compose project; optionally drops tenant DBs and Docker volumes |
| `down -v` | `docktree down -v` | Drops tenant databases + Docker volumes |
| `down -a` | `docktree down -a` | Applies to all worktree instances in the repo |
| `logs` | `docktree logs` | Passthrough to `docker compose logs` |
| `exec` | `docktree exec` | Passthrough to `docker compose exec` |
| `run` | `docktree run` | Passthrough to `docker compose run --rm` |
| `stop` | `docktree stop` | `docker compose stop` |
| `status` | `docktree status` | Runs `docker compose ps` through the current project |
| `ports` | `docktree ports` | Shows allocated host ports from the port registry |
| `build` | **Missing** | No way to build images without starting |
| `pull` | **Missing** | Can't pre-pull images |
| `push` | **Missing** | Can't push built images |
| `restart` | **Missing** | Only stop/start |
| `rm` | **Missing** | No explicit remove-stopped |
| `start` | **Missing** | Only up/down |
| `pause` / `unpause` | **Missing** | No pause support |
| `top` | **Missing** | No process listing |
| `config` | **Missing** | No compose config validation/printing |
| `cp` | **Missing** | No file copy to/from containers |
| `create` | **Missing** | Only up (up = create + start) |
| `events` | **Missing** | No event stream |
| `images` | **Missing** | No image listing |
| `kill` | **Missing** | No kill |
| `ls` | **Missing** | No compose project listing |
| `wait` | **Missing** | No wait for container exit |
| `watch` | **Missing** | No dev mode hot-reload |

## Compose File Feature Preservation

Docktree has two distinct code paths for compose file handling:

### Legacy Path (no shared services)

When `shared.services` is not configured, Docktree passes the **original compose files** directly to `docker compose`, plus a thin override file that only touches `container_name`, `image`, `ports`, `labels`, and `volumes`. Docker Compose reads the originals, so all features work natively.

Docktree's internal `ComposeProject` model (`internal/compose/compose.go`) only captures 8 fields per service — used solely for port allocation math and override generation. It does **not** need to be richer because it is not the thing Docker Compose sees.

### Shared-Services Path (full synthesis)

When `shared.services` is configured, Docktree replaces the original files with a single generated worktree compose file. This file must be self-contained, so everything is serialized via compose-go's `MarshalYAML()` which preserves all fields. This is also why `$` escaping matters — the generated file gets re-parsed by Docker Compose.

## Compose File Features

### Features Preserved in Both Paths

- All service fields from compose files (container control, resource limits, healthchecks, deploy config, secrets, configs, etc.)
- `x-` extension fields
- `profiles`
- `extends`
- Build configuration beyond `context` (args, dockerfile, target, secrets, platforms, cache, etc.)
- `depends_on` with conditions
- `healthcheck`
- `deploy` (replicas, resources, restart policy, etc.)
- `restart` policy
- `logging` config
- `env_file`
- `secrets` / `configs`
- Network aliases, ipv4/ipv6, priority
- Volumes bind propagation, tmpfs options
- MacAddress, privileged, cap_add/drop, sysctls, dns, extra_hosts, tmpfs, devices
- `Command` / `Entrypoint`

### Features Modified by Docktree (shared-services path)

| Feature | Modification |
|---|---|
| `ports` on platform services | Stripped (platform services are reached over Docker DNS, not from host) |
| `depends_on` on platform services | Stripped (platform services are standalone) |
| `depends_on` on worktree services | Edges to platform services are pruned (they don't exist in the worktree project) |
| `networks` on worktree services | Platform external network is added |
| `environment` on worktree services | `$` is escaped to `$$` to prevent host env expansion |
| `command` / `entrypoint` | `$` is escaped to `$$` |
| Database URLs in `environment` | Rewritten to point to per-worktree tenant databases |

### Features Lost in Reduced Model (override generation only)

These fields are not captured in Docktree's `ComposeProject` struct, but are still present in the original compose files passed to Docker Compose. They are only lost for port allocation and override generation decisions:

| Feature | Impact |
|---|---|
| `build` args, dockerfile, target, secrets, platforms | Build config beyond `context` not visible |
| `depends_on` conditions (healthy/completed) | Only service names are tracked |
| `healthcheck` | Not visible for port readiness decisions |
| `deploy` (replicas, resources, restart policy) | Not visible |
| `restart` policy | Not visible |
| `logging` config | Not visible |
| `env_file` | Not visible |
| `profiles` | Not visible |
| `secrets` / `configs` | Passed through in worktree synthesis but not in legacy path |
| `network` aliases, ipv4/ipv6, priority | Lost in reduced model |
| `volumes` bind propagation, tmpfs options | Flattened to strings |
| MacAddress, privileged, cap_add/drop, sysctls, dns, extra_hosts, tmpfs, devices | Completely unmapped from reduced model |

### Top-Level Compose Constructs

| Feature | Status |
|---|---|
| `services` | Fully supported |
| `networks` | Fully supported (external, name) |
| `volumes` | Fully supported (external, name) |
| `secrets` | Passed through in worktree synthesis |
| `configs` | Passed through in worktree synthesis |
| `name` | Not handled (Docktree generates its own project name) |
| `x-` extensions | Preserved by compose-go |
| `include` | Not handled |

### .env File Handling

Docktree checks `.env` at startup for 3 warning conditions:

1. `COMPOSE_PROJECT_NAME` is set — warns Docktree will pass `-p` so its generated name wins
2. `COMPOSE_FILE` is set — warns Docktree will use it
3. Any key matching `(^|_)(PORT|PUBLISHED)` with a numeric value — warns that port remapping may need app updates
