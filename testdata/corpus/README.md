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

## 2026-06-23 third expansion report

Added 20 more upstream Compose files: akeneo, calcom, changedetection, chroma, clickhouse-postgres, erpnext, gitlab, langfuse, linkwarden, matrix-synapse, moodle, open-webui, owncloud, photoprism, prestashop, qdrant-demo, searxng, spring-postgres, umami, weaviate.

The full 60-project corpus passes Docker Compose config, Docktree dry-run, and Docker Compose config with Docktree clear+override files layered in. Start probes were attempted for all 60 projects with `--pull never`; none started because required images were not present locally. See `THIRD_EXPANSION_REPORT.md` for the third-batch per-project table and differences.

## 2026-06-23 expansion report

Added 20 more diverse upstream Compose files: airflow, elastic, elk, flask-redis, graylog, hasura, hydra, jitsi, kafka, kratos, localstack, mattermost, nextcloud-postgres, nginx-golang, opensearch, prometheus-grafana, redash, temporal, traefik, wordpress.

The full 40-project corpus passes Docker Compose config, Docktree dry-run, and Docker Compose config with Docktree clear+override files layered in. Start probes were attempted for all 40 projects with `--pull never`; none started because required images were not present locally. See `EXPANSION_REPORT.md` for the second-batch per-project table and differences.

## 2026-06-22 validation report

Imported 20 upstream Compose files and validated them with `go test ./internal/compose -count=1` plus `testdata/corpus/validate.py`.
All 20 corpus projects pass Docker Compose config, Docktree dry-run, and Docker Compose config with Docktree's generated clear+override files layered back in.

Start probes were attempted for all 20 projects with Docktree clear+override files, `--wait`, and `--pull never`; none started because required images were not present locally. See `REPORT.md` for the per-project table and reasons.

Issues found and fixed:

- Docktree loaded process environment variables but did not load `.env` from the Compose file directory before interpolation. Real-world Compose files such as authentik, immich, sentry, and supabase depend on `.env` for required variables and typed fields. `LoadFull` now merges the Compose directory `.env` before OS environment overrides.
- compose-go's consistency check rejected real Compose files accepted by Docker Compose when services declared both `network_mode` and `networks`; Docktree now skips that check during load.
- Docktree's generated override attached isolated networks to `network_mode` services; it now omits that override for those services so the layered Compose config remains valid.
