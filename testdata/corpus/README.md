# corpus

Real-world Compose files vendored from popular open-source projects. The point
is to catch drift between Docktree and Compose-in-the-wild: every time someone
files a bug like "Docktree breaks on project X," we vendor X's compose file
here and add it to the corpus test.

## Layout

```
corpus/
└── <project>/
    ├── compose.yml      # vendored as-is from the upstream repo
    └── SOURCE           # metadata: url, license, status
```

The `SOURCE` file is plain key/value:

```
url: <raw url the file was fetched from>
license: <upstream project's license>
status: parses | known-gap: <short reason>
```

`status: parses` means Docktree's parser handles the file today.
`status: known-gap: …` means it does not — the file documents a real bug we
haven't fixed yet. Corpus tests should skip these (and fail loudly if a
known-gap file *starts* parsing — that means we fixed it and the status needs
to flip).

## Current inventory

| Project        | Services (~) | Status | What it exercises |
|----------------|--------------|--------|-------------------|
| authentik      | many         | parses | required `.env` interpolation inside environment and published ports |
| chatwoot       | ~8           | parses | `version: '3'`, redis/postgres/sidekiq |
| directus       | ~6           | parses | multi-DB matrix via profiles + healthchecks |
| firefly-iii    | ~3           | parses | env_file references |
| immich         | ~5           | parses | GPU device requests, env interpolation in volumes |
| invoiceninja   | ~5           | parses | obsolete `version:` field, env directory |
| listmonk       | ~3           | parses | long-syntax volumes (`type: bind`) under `service.volumes` |
| mailcow        | ~25          | parses | env interpolation in published ports, huge service graph |
| mastodon       | ~5           | parses | `env_file: .env.production`, deploy.resources |
| minio          | 4            | parses | 4-node distributed cluster, same image repeated |
| n8n            | 2            | parses | postgres + n8n with healthcheck-gated `depends_on` |
| netbox         | ~6           | parses | env_file fan-out, init container pattern |
| nextcloud-aio  | 1 (mastercontainer) | parses | named compose project, single privileged container |
| outline        | 2            | parses | minimal dev-style postgres/redis exposing 5432/6379 |
| paperless-ngx  | ~4           | parses | env_file at top of every service |
| penpot         | ~6           | parses | `x-` extension fields, anchors |
| plausible      | 3            | parses | clickhouse + postgres + plausible |
| sentry         | ~30          | parses | long-syntax volumes, massive service graph |
| supabase       | ~15          | parses | env interpolation in published ports, kong/auth/postgres |
| uptime-kuma    | 1            | parses | minimal single-service baseline |

## 2026-06-22 validation report

Imported 20 upstream Compose files and ran `go test ./internal/compose -count=1`.
All 20 corpus projects parse through Docktree.

Issue found and fixed: Docktree loaded process environment variables but did not load `.env` from the Compose file directory before interpolation. Real-world Compose files such as authentik, immich, sentry, and supabase depend on `.env` for required variables and typed fields. `LoadFull` now merges the Compose directory `.env` before OS environment overrides, matching Docker Compose precedence for these files.
