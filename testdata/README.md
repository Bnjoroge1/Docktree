# testdata

Fixtures used by Docktree's tests. Organized by intent.

## Layout

```
testdata/
├── docker-compose.yml      # Single small Compose file used by the e2e harness
│                           # (tests/e2e_test.go copies it into a fake repo).
│
├── compose-variants/       # Hand-crafted minimal fixtures, one per Compose
│                           # feature or syntax variant. Each file exercises
│                           # exactly one thing (long-syntax ports, build map,
│                           # depends_on list, named volume, profiles, …).
│                           # Used by internal/compose unit tests and by the
│                           # integration test in tests/compose_config_test.go.
│
└── corpus/                 # Vendored real-world Compose files from popular
                            # open-source projects. Used by corpus-level tests
                            # to catch drift between Docktree's parser/override
                            # generator and what real Compose files look like
                            # in the wild. See corpus/README.md.
```

## Adding a new fixture

- **Synthetic variant** (you want to exercise one specific Compose feature):
  add a numbered file under `compose-variants/`.
- **Real-world file** (you saw a real project break Docktree): vendor it under
  `corpus/<project>/compose.yml` and add a `SOURCE` file. See
  `corpus/README.md` for the format.
