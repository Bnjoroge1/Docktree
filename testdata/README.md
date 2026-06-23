# testdata

Fixtures used by Docktree's tests. Organized by intent.

## Layout

```
testdata/
├── docker-compose.yml      # Single small Compose file used by the e2e harness
│                           # (tests/e2e_test.go copies it into a fake repo).
│
├── compose-*.yml           # Hand-crafted fixtures, each exercising one or more
│                           # Compose features (ports, build, profiles, volumes,
│                           # healthchecks, networks, shared services, etc.).
│                           # Used by internal/compose unit tests.
│
├── api/                    # Tiny FastAPI service used by docker-compose.yml
│   ├── Dockerfile
│   ├── Dockerfile.dev      # Alternate Dockerfile used by compose-with-build.yml
│   └── app.py
├── ui/                     # Tiny nginx static service
│   ├── Dockerfile
│   └── index.html
├── web/                    # Tiny nginx static service (alternate name)
│   ├── Dockerfile
│   └── index.html
└── worker/                 # Tiny long-running worker for compose-with-build.yml
    ├── Dockerfile
    └── app.py
```

## Adding a new fixture

Add a `compose-*.yml` file in the testdata root to exercise specific Compose
features or syntax variants.
