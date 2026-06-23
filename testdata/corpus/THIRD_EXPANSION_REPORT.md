# Third corpus expansion report

Date: 2026-06-23

## Scope

Added 20 more diverse upstream Compose files. Vendored `compose.yml` files were kept as upstream content; only fixture files (`.env`, env_file placeholders, bind-mount placeholder files) were added beside them.

New projects: akeneo, calcom, changedetection, chroma, clickhouse-postgres, erpnext, gitlab, langfuse, linkwarden, matrix-synapse, moodle, open-webui, owncloud, photoprism, prestashop, qdrant-demo, searxng, spring-postgres, umami, weaviate.

## Method

- Built Docktree to `/tmp/docktree-corpus`.
- Ran `go test ./... -count=1`.
- Ran `testdata/corpus/validate.py --docktree-bin /tmp/docktree-corpus` over all 60 corpus projects.
- Ran `testdata/corpus/validate.py --docktree-bin /tmp/docktree-corpus --start` over all 60 corpus projects.
- Start probes used Docktree clear+override files, `--wait`, `--no-build`, and `--pull never`; failures from missing local images are expected host-state results, not config/parser failures.

## Summary

- Total corpus projects now: 60
- Third-batch projects added: 20
- Full corpus Compose config passed: 60/60
- Full corpus Docktree dry-run passed: 60/60
- Full corpus Docktree override config passed: 60/60
- Third-batch start probes started: 0/20
- Third-batch start cleanup passed: 20/20

No new Docktree code issues came up. Differences were covered by fixture values only.

## Third-batch project results

| Project | Services | Ports | Compose config | Docktree dry-run | Override config | Started? | Start reason | Cleanup |
|---|---:|---:|---|---|---|---|---|---|
| akeneo | 9 | 6 | ok | ok | ok | not-started | missing local image: google/cloud-sdk:375.0.0-emulators | ok |
| calcom | 4 | 3 | ok | ok | ok | not-started | missing local image: postgres:latest, redis:latest | ok |
| changedetection | 1 | 1 | ok | ok | ok | not-started | missing local image: ghcr.io/dgtlmoon/changedetection.io:latest | ok |
| chroma | 1 | 1 | ok | ok | ok | not-started | missing local image: docktree/docktree-test-61f1a4/server:latest | ok |
| clickhouse-postgres | 2 | 3 | ok | ok | ok | not-started | missing local image: postgres:latest | ok |
| erpnext | 10 | 1 | ok | ok | ok | not-started | missing local image: frappe/erpnext:v16.23.1 | ok |
| gitlab | 2 | 2 | ok | ok | ok | not-started | missing local image: kkimurak/sameersbn-postgresql:17 | ok |
| langfuse | 5 | 7 | ok | ok | ok | not-started | missing local image: cgr.dev/chainguard/minio:latest, redis:7 | ok |
| linkwarden | 3 | 1 | ok | ok | ok | not-started | missing local image: postgres:16-alpine | ok |
| matrix-synapse | 1 | 1 | ok | ok | ok | not-started | missing local image: postgres:12-alpine | ok |
| moodle | 2 | 2 | ok | ok | ok | not-started | missing local image: bitnami/mariadb:latest | ok |
| open-webui | 2 | 1 | ok | ok | ok | not-started | missing local image: ollama/ollama:latest | ok |
| owncloud | 2 | 2 | ok | ok | ok | not-started | missing local image: mariadb:latest | ok |
| photoprism | 6 | 8 | ok | ok | ok | not-started | missing local image: photoprism/traefik:latest | ok |
| prestashop | 1 | 0 | ok | ok | ok | not-started | missing local image: ubuntu:latest | ok |
| qdrant-demo | 1 | 1 | ok | ok | ok | not-started | missing local image: docktree/docktree-test-61f1a4/web:latest | ok |
| searxng | 4 | 3 | ok | ok | ok | not-started | missing local image: dalf/morty:latest | ok |
| spring-postgres | 1 | 1 | ok | ok | ok | not-started | missing local image: docktree/docktree-test-61f1a4/backend:latest, postgres:latest | ok |
| umami | 1 | 1 | ok | ok | ok | not-started | missing local image: ghcr.io/umami-software/umami:latest | ok |
| weaviate | 21 | 22 | ok | ok | ok | not-started | missing local image: semitechnologies/reranker-transformers:cross-encoder-ms-marco-MiniLM-L-6-v2, ollama/ollama:latest | ok |

## Differences found in the third batch

- PhotoPrism interpolates bind host IP fields; fixture values must be valid IP addresses (`127.0.0.1`), not arbitrary strings.
- Spring/Postgres references a bind-mounted secret file; `db/password.txt` was added as a fixture so start probes reach the image-availability check instead of failing on a missing host file.
- Chroma, qdrant-demo, nginx-golang, and spring-postgres include build services. Docktree dry-run generates build image names correctly; start probes with `--no-build --pull never` fail because those generated images do not exist locally.
- Local AI/search apps added coverage for GPU/LLM/vector-search style stacks: open-webui, langfuse, weaviate, chroma, qdrant-demo, searxng.
- Commerce/CMS/collaboration coverage expanded with calcom, umami, changedetection, linkwarden, gitlab, owncloud, erpnext, moodle, prestashop, akeneo, matrix-synapse.

## Issues fixed during this expansion

- Added valid fixture env values for PhotoPrism host-IP interpolation.
- Added the Spring/Postgres bind-mounted password fixture.
- Replaced an invalid Bitwarden Handlebars template candidate with a real Compose file (`spring-postgres`) so corpus files remain directly parseable Compose YAML.
- No vendored upstream `compose.yml` files were edited after download.
