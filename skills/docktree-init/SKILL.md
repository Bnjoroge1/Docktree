---
name: docktree-init
description: Generate a docktree.yml for a repo that uses Docker Compose. Scans compose files, detects shareable services (postgres, redis, minio, etc.), and walks the user through configuration decisions. Use when the user asks to "set up docktree", "configure docktree for this repo", "generate docktree.yml", "onboard this repo to docktree", or when docktree up prints a tip about shareable services.
allowed-tools: Bash(docktree:*)
---
# docktree-init

Generates a `docktree.yml` by scanning the repo's compose files and walking the user through the decisions the CLI can't make on its own. 

## Procedure

### 1. Scan

```bash
docktree init --json --dry-run
```

This only detects compose files, and shareable services,

```jsonc
{
  "written": "- (stdout)",
  "todos": [
    { "path": "shared.services.db.tenancy", "question": "...", "kind": "enum", "options": ["full_share", "per_database"] },
    { "path": "shared.services.db.url_envs", "question": "...", "kind": "string_list", "options": ["DATABASE_URL"] }
  ],
  "warnings": [
    { "path": "shared.services.db", "message": "... secrets wrapper ..." }
  ]
}
```

### 2. Present findings

Show the user what was detected:

- Which services look shareable (kind, image).
- Which consumer env vars were found (url_envs).
- Any warnings (secrets wrappers, etc.).

If `todos` is empty, there's nothing to decide — run `docktree init` to write

the file and you're done.

### 3. Ask one question per TODO

For each item in `todos`, ask the user a single targeted question:

- `**kind: "enum"**` — present the `options` list and ask which one.

  Default to the first option if the user says "skip" or "default".
- `**kind: "string_list"**` — show the detected values and ask the user to

  confirm, add, or remove entries. Default to the `options` list as-is.

Ask questions in order. Don't batch — one question at a time keeps it clear.

### 4. Write the config

Collect all answers into a JSON object keyed by `todo.path`:

```json
{
  "shared.services.db.tenancy": "per_database",
  "shared.services.db.url_envs": ["DATABASE_URL", "DB_DSN"],
  "shared.services.redis.tenancy": "full_share",
  "shared.services.redis.url_envs": ["REDIS_URL"]
}
```

Pipe it to:

```bash
echo '<answers-json>' | docktree init --apply
```

This builds the config from the canonical model (no string manipulation), validates it against `config.ValidateShared`, and writes `docktree.yml`.

### 5. Validate

```bash
docktree --json up --validate 2>/dev/null | jq .
```

This runs structural validation: checks for missing build contexts, invalid port ranges, and port allocation conflicts — without starting any containers. Returns a `ValidateResult` with `valid: true` or `valid: false` plus an `errors` array.

If this errors or `valid` is false, the generated config is invalid — show the error and offer to fix. Otherwise, tell the user the config is ready and they can run `docktree up`.

## What this skill does NOT do

- Edit `docktree.yml` directly. The CLI is the only writer.
- Guess tenancy or url_envs. Every decision goes through the user.

Handle repos with no compose files. If `docktree init --json --dry-run` returns an error about missing compose files, tell the user.
