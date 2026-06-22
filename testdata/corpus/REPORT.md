# Corpus validation report

Date: 2026-06-22

## Method

- Built Docktree from the current tree to `/tmp/docktree-corpus`.
- For each corpus project, ran `docker compose config` against the vendored compose file and fixture environment.
- Ran `docktree --json up --dry-run -f <compose.yml>`.
- Fed Docktree dry-run `clear_preview` and `override_preview` back into `docker compose config`.
- Ran a bounded start probe with Docktree clear+override files using `docker compose up -d --wait --wait-timeout 30 --no-build --pull never`.
- Ran `docker compose down -v --remove-orphans` after every start probe.

## Summary

- Projects tested: 20
- Compose config passed: 20/20
- Docktree dry-run passed: 20/20
- Docktree override config passed: 20/20
- Start probes started: 0/20
- Start probe cleanup passed: 20/20

No Docktree config or override failures remain. Start probes did not start because this host does not have the required images locally and the probe intentionally used `--pull never`.

## Per-project results

| Project | Services | Ports | Isolated volumes | Compose config | Docktree dry-run | Override config | Started? | Start reason | Cleanup |
|---|---:|---:|---:|---|---|---|---|---|---|
| authentik | 3 | 2 | 0 | ok | ok | ok | not-started | missing local image: postgres:16-alpine | ok |
| chatwoot | 4 | 2 | 0 | ok | ok | ok | not-started | missing local image: chatwoot/chatwoot:latest, redis:alpine | ok |
| directus | 12 | 14 | 0 | ok | ok | ok | not-started | missing local image: mcr.microsoft.com/mssql/server:2019-latest | ok |
| firefly-iii | 2 | 1 | 0 | ok | ok | ok | not-started | missing local image: mariadb:lts | ok |
| immich | 3 | 1 | 0 | ok | ok | ok | not-started | missing local image: valkey/valkey:9@sha256:4963247afc4cd33c7d3b2d2816b9f7f8eeebab148d29056c2ca4d7cbc966f2d9 | ok |
| invoiceninja | 2 | 1 | 0 | ok | ok | ok | not-started | missing local image: mysql:8 | ok |
| listmonk | 1 | 1 | 0 | ok | ok | ok | not-started | missing local image: postgres:17-alpine | ok |
| mailcow | 18 | 12 | 0 | ok | ok | ok | not-started | missing local image: ghcr.io/mailcow/unbound:1.25.1-1, ghcr.io/mailcow/olefy:1.15 | ok |
| mastodon | 3 | 2 | 0 | ok | ok | ok | not-started | missing local image: postgres:14-alpine | ok |
| minio | 5 | 2 | 0 | ok | ok | ok | not-started | missing local image: quay.io/minio/minio:RELEASE.2025-09-06T17-38-46Z | ok |
| n8n | 3 | 1 | 0 | ok | ok | ok | not-started | missing local image: postgres:16 | ok |
| netbox | 4 | 0 | 0 | ok | ok | ok | not-started | missing local image: valkey/valkey:9.0-alpine, postgres:18-alpine | ok |
| nextcloud-aio | 1 | 3 | 0 | ok | ok | ok | not-started | missing local image: ghcr.io/nextcloud-releases/all-in-one:latest | ok |
| outline | 1 | 1 | 0 | ok | ok | ok | not-started | missing local image: postgres:latest, redis:latest | ok |
| paperless-ngx | 2 | 1 | 0 | ok | ok | ok | not-started | missing local image: redis:8 | ok |
| penpot | 7 | 2 | 0 | ok | ok | ok | not-started | missing local image: penpotapp/mcp:x | ok |
| plausible | 3 | 1 | 0 | ok | ok | ok | not-started | missing local image: postgres:16-alpine | ok |
| sentry | 30 | 1 | 6 | ok | ok | ok | not-started | missing local image: chrislusf/seaweedfs:4.17_large_disk | ok |
| supabase | 10 | 4 | 0 | ok | ok | ok | not-started | missing local image: supabase/studio:2026.06.03-sha-0bca601, darthsim/imgproxy:v3.30.1 | ok |
| uptime-kuma | 1 | 1 | 0 | ok | ok | ok | not-started | missing local image: louislam/uptime-kuma:2 | ok |

## Issues fixed during validation

1. Docktree now loads the Compose-file directory `.env` before applying OS environment overrides, matching Docker Compose interpolation behavior for the corpus fixtures.
2. Docktree now skips compose-go consistency checks that reject Compose files accepted by Docker Compose, notably services declaring both `network_mode` and `networks`.
3. Docktree generated overrides no longer attach an isolated network to services using `network_mode`, because Docker Compose rejects layered files that combine `network_mode` with a `networks` override.
4. The mailcow fixture uses a non-overlapping private IPv4 subnet for local start probes.
