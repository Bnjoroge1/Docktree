#!/usr/bin/env bash
# teardown-demo.sh — stop docktree instances and remove demo worktrees/repo.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DEMO_DIR="$REPO_ROOT/demo"
DEMO_REPO="$DEMO_DIR/demo-repo"
WT_DIR="$DEMO_DIR/worktrees"

DOCKTREE="${DOCKTREE:-$REPO_ROOT/docktree}"

echo "▸ Stopping docktree instances (best-effort)…"
for wt in main feature; do
  if [ -d "$WT_DIR/$wt" ]; then
    (cd "$WT_DIR/$wt" && "$DOCKTREE" down 2>/dev/null) || true
  fi
done

echo "▸ Cleaning orphaned docktree resources (best-effort)…"
"$DOCKTREE" clean --yes 2>/dev/null || true

echo "▸ Removing demo worktrees…"
for wt in main feature; do
  if [ -d "$WT_DIR/$wt" ]; then
    git -C "$DEMO_REPO" worktree remove --force "$WT_DIR/$wt" 2>/dev/null || true
    rm -rf "$WT_DIR/$wt"
  fi
done
[ -d "$DEMO_REPO" ] && git -C "$DEMO_REPO" worktree prune 2>/dev/null || true

echo "▸ Removing demo repo…"
rm -rf "$DEMO_REPO"

echo "✓ Demo teardown complete."
