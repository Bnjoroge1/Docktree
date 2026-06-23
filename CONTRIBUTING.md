# Contributing to Docktree

Thanks for taking the time to contribute! Docktree is a small, focused Go CLI
that makes Docker Compose play nicely across many git worktrees. This guide
explains how to get a dev environment running, the conventions we follow, and
how changes land in `main`.

## Table of contents

- [Code of conduct](#code-of-conduct)
- [Ways to contribute](#ways-to-contribute)
- [Development setup](#development-setup)
- [Project layout](#project-layout)
- [Building and running](#building-and-running)
- [Testing](#testing)
- [Linting and formatting](#linting-and-formatting)
- [Commit and pull request guidelines](#commit-and-pull-request-guidelines)
- [Branch protection and merge policy](#branch-protection-and-merge-policy)
- [Reporting bugs and requesting features](#reporting-bugs-and-requesting-features)

## Code of conduct

Be respectful, assume good intent, and keep discussions technical. Harassment
of any kind is not welcome. Maintainers may remove comments, commits, or
contributors that don't meet this bar.

## Ways to contribute

- **Report a bug** or **request a feature** by opening an issue.
- **Improve the docs** — typos, clearer wording, and better examples are all
  valuable.
- **Send a pull request** for a fix or feature. For anything non-trivial,
  please open an issue first so we can agree on the approach before you invest
  time in code.

## Development setup

Prerequisites:

- **Go** — the version pinned in [`go.mod`](go.mod) (currently Go 1.25).
- **Docker** with the Compose plugin (`docker compose`) for end-to-end testing.
- **[`just`](https://github.com/casey/just)** (optional, but the task runner
  used throughout this guide).

Clone and fetch dependencies:

```bash
git clone https://github.com/Bnjoroge1/Docktree.git
cd Docktree
go mod download
```

## Project layout

```
cmd/docktree/    CLI entry point
internal/        Library code (compose handling, port allocation, CLI, ...)
tests/           Integration tests
testdata/        Fixtures used by tests
Formula/         Homebrew formula
```

## Building and running

```bash
just build          # build the ./docktree binary
just run -- status  # run the CLI without installing (args after --)
just install        # build and `go install` into your GOPATH/bin
```

Without `just`:

```bash
go build ./cmd/docktree
go run ./cmd/docktree status
```

## Testing

Run the unit tests while iterating, and the full race-enabled suite before
opening a PR — this mirrors what CI runs:

```bash
just test       # unit tests under ./internal/...
just test-all   # every package
go test -race -count=1 ./...   # exactly what CI runs
```

Guidelines:

- Add tests for new behavior and for any bug you fix (a failing test that the
  fix makes pass is ideal).
- Test observable behavior and edge cases, not internal plumbing.
- Use the fixtures in `testdata/` rather than reaching for live Docker where a
  fixture will do. **Do not add mocks** for things a real fixture can cover.

## Linting and formatting

CI runs `go vet` and [`golangci-lint`](https://golangci-lint.run/) (config in
[`.golangci.yml`](.golangci.yml)). Keep the tree clean:

```bash
just fmt    # gofmt -w .
just lint   # go vet ./...
golangci-lint run   # full linter suite (matches CI)
```

Also make sure modules stay tidy — CI fails if they aren't:

```bash
go mod tidy
git diff --exit-code go.mod go.sum
```

## Commit and pull request guidelines

- Write clear, imperative commit subjects ("Add port range override", not
  "added stuff"). Keep the subject under ~72 characters and explain the *why*
  in the body when it isn't obvious.
- Keep each PR focused on a single concern; split unrelated changes.
- Before pushing, make sure the following all pass locally:
  - `go build ./...`
  - `go vet ./...`
  - `go test -race -count=1 ./...`
  - `golangci-lint run`
- In the PR description, explain what changed and why, and link any related
  issue (e.g. `Closes #123`).
- CI (build, vet, test, lint) must be green before a PR is eligible to merge.

## Branch protection and merge policy

`main` is a **protected branch**:

- All changes land through pull requests — direct pushes to `main` are
  rejected.
- Every PR requires an approving review from the code owner
  (**@Bnjoroge1**, see [`.github/CODEOWNERS`](.github/CODEOWNERS)).
- Required CI checks (build/test and lint) must pass.
- Stale approvals are dismissed when new commits are pushed, conversations
  must be resolved, and history is kept linear (no merge commits).

In practice this means **only the repository owner (@Bnjoroge1) merges to
`main`**. Contributors should expect their PRs to be reviewed and merged by the
maintainer rather than merging themselves.

## Reporting bugs and requesting features

Open an issue and include, where relevant:

- What you expected to happen and what actually happened.
- Steps to reproduce (a minimal `docker-compose.yml` and the exact `docktree`
  command help a lot).
- Your OS, Docker/Compose version, and `docktree --version`.
- Relevant logs or the output of the command run with `--json`.

Thanks again for contributing! 🐳
