#!/usr/bin/env bash
# run-demo.sh — start both app versions with docktree, show status and ports.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DEMO_DIR="$REPO_ROOT/demo"
WT_DIR="$DEMO_DIR/worktrees"

DOCKTREE="${DOCKTREE:-$REPO_ROOT/docktree}"
if [ ! -x "$DOCKTREE" ]; then
  echo "✗ docktree binary not found at $DOCKTREE"
  echo "  Run: (cd $REPO_ROOT && just build)"
  exit 1
fi

if [ ! -d "$WT_DIR/main" ] || [ ! -d "$WT_DIR/feature" ]; then
  echo "✗ Demo worktrees not found. Run: $DEMO_DIR/scripts/setup-demo.sh"
  exit 1
fi

echo "━━━ Starting Worktree A (demo-main) ━━━"
cd "$WT_DIR/main"
"$DOCKTREE" up --build

echo ""
echo "━━━ Starting Worktree B (demo-feature) ━━━"
cd "$WT_DIR/feature"
"$DOCKTREE" up --build

echo ""
echo "━━━ Status (all instances) ━━━"
"$DOCKTREE" status --all

echo ""
echo "━━━ Ports (all instances) ━━━"
"$DOCKTREE" ports --all

echo ""
echo "✓ Both versions are running. Run $DEMO_DIR/scripts/show-both.sh to verify."
