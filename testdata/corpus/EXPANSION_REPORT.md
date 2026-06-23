# Expanded corpus validation report

Date: 2026-06-23

## Scope

Added 20 diverse upstream Compose files to the existing 20-project corpus.

New projects: airflow, elastic, elk, flask-redis, graylog, hasura, hydra, jitsi, kafka, kratos, localstack, mattermost, nextcloud-postgres, nginx-golang, opensearch, prometheus-grafana, redash, temporal, traefik, wordpress.

## Method

- Downloaded upstream Compose files into `testdata/corpus/<project>/compose.yml` with `SOURCE` metadata.
- Added minimal fixture environment files only where needed for Compose interpolation/config validation.
- Built Docktree to `/tmp/docktree-corpus`.
- Ran `go test ./... -count=1`.
- Ran `testdata/corpus/validate.py --docktree-bin /tmp/docktree-corpus` over the full 40-project corpus.
- Ran `testdata/corpus/validate.py --docktree-bin /tmp/docktree-corpus --start` over the full 40-project corpus.

## Summary

- Total corpus projects now: 40
- New projects added: 20
- Full corpus Compose config passed: 40/40
- Full corpus Docktree dry-run passed: 40/40
- Full corpus Docktree override config passed: 40/40
- New-project start probes started: 0/20
- New-project start cleanup passed: 20/20

No Docktree code issues came up in the second 20-project expansion. The differences were fixture-level: upstream files frequently expect `.env` values to be valid paths, booleans, image tags, or container names, not arbitrary strings.

## New project results

| Project | Services | Ports | Compose config | Docktree dry-run | Override config | Started? | Start reason | Cleanup |
|---|---:|---:|---|---|---|---|---|---|
| airflow | 7 | 1 | ok | ok | ok | not-started | missing local image: redis:7.2-bookworm | ok |
| elastic | 5 | 2 | ok | ok | ok | not-started | missing local image: docker.elastic.co/elasticsearch/elasticsearch:latest | ok |
| elk | 3 | 7 | ok | ok | ok | not-started | missing local image: elasticsearch:7.16.1 | ok |
| flask-redis | 1 | 1 | ok | ok | ok | not-started | missing local image: redislabs/redismod:latest | ok |
| graylog | 3 | 13 | ok | ok | ok | not-started | missing local image: mongo:7.0 | ok |
| hasura | 3 | 2 | ok | ok | ok | not-started | missing local image: postgres:15 | ok |
| hydra | 4 | 4 | ok | ok | ok | not-started | missing local image: oryd/hydra-login-consent-node:v26.2.0 | ok |
| jitsi | 4 | 5 | ok | ok | ok | not-started | missing local image: jitsi/prosody:latest | ok |
| kafka | 9 | 8 | ok | ok | ok | not-started | missing local image: confluentinc/cp-zookeeper:7.6.1 | ok |
| kratos | 4 | 4 | ok | ok | ok | not-started | missing local image: oryd/kratos-selfservice-ui-node:v26.2.0 | ok |
| localstack | 1 | 51 | ok | ok | ok | not-started | missing local image: localstack/localstack:latest | ok |
| mattermost | 2 | 0 | ok | ok | ok | not-started | missing local image: mattermost/mattermost-team-edition:latest | ok |
| nextcloud-postgres | 1 | 1 | ok | ok | ok | not-started | missing local image: nextcloud:apache, postgres:alpine | ok |
| nginx-golang | 2 | 1 | ok | ok | ok | not-started | missing local image: docktree/docktree-test-61f1a4/backend:latest | ok |
| opensearch | 3 | 3 | ok | ok | ok | not-started | missing local image: opensearchproject/opensearch-dashboards:latest, opensearchproject/opensearch:latest | ok |
| prometheus-grafana | 5 | 5 | ok | ok | ok | not-started | missing local image: gcr.io/cadvisor/cadvisor:latest, prom/alertmanager:latest | ok |
| redash | 4 | 2 | ok | ok | ok | not-started | missing local image: redis:3.0-alpine | ok |
| temporal | 5 | 2 | ok | ok | ok | not-started | missing local image: elasticsearch:latest, postgres:latest | ok |
| traefik | 2 | 2 | ok | ok | ok | not-started | missing local image: traefik:v3.7, traefik/whoami:latest | ok |
| wordpress | 1 | 1 | ok | ok | ok | not-started | missing local image: wordpress:latest | ok |

## Differences found in the second batch

- Airflow uses `${AIRFLOW_PROJ_DIR}` in bind mounts and `${ENV_FILE_PATH}` for service env files. Fixture values must be relative paths (`./data`, `./data/.env`), otherwise Compose treats `data/dags` as an invalid named volume.
- Jitsi uses `${CONFIG}` as the root of many bind mounts. The fixture must use `./config`; `x/jvb` is parsed as an invalid named volume.
- LocalStack interpolates `container_name`; the fixture must be a valid container name (`localstack-main`).
- Mattermost uses typed booleans and bind-mount roots. `MATTERMOST_CONTAINER_READONLY` must be boolean and data roots must be relative paths.
- LocalStack publishes a wide port range, producing 51 Docktree port assignments. This is the largest published-port spread in the expanded batch.
- nginx-golang includes a build-only backend service. Docktree dry-run correctly generated an image name for the build output, so the start probe fails under `--no-build --pull never` as expected.

## Issues fixed during this expansion

- Added fixture env values for the second batch so Docker Compose and Docktree validate the same resolved model.
- Added `airflow/data/.env` because Airflow references an env file path from `.env`.
- No additional Docktree code changes were required for this second batch after the earlier `.env`, consistency-check, and `network_mode` override fixes.
