#!/usr/bin/env bash
# setup-demo.sh — create a self-contained demo git repo with two worktrees.
#
# The demo repo is separate from the Docktree repo so docktree doesn't
# inherit shared-services config from the parent repo. This keeps the demo
# a clean single-service app with no platform tier.
#
# Idempotent: removes existing demo worktrees/repo before recreating.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DEMO_DIR="$REPO_ROOT/demo"
DEMO_REPO="$DEMO_DIR/demo-repo"
WT_DIR="$DEMO_DIR/worktrees"

# Locate the docktree binary.
DOCKTREE="${DOCKTREE:-$REPO_ROOT/docktree}"
if [ ! -x "$DOCKTREE" ]; then
  echo "▸ Building docktree…"
  (cd "$REPO_ROOT" && just build)
  DOCKTREE="$REPO_ROOT/docktree"
fi
export DOCKTREE

echo "▸ Cleaning up any previous demo state…"
for wt in main feature; do
  if [ -d "$WT_DIR/$wt" ]; then
    git -C "$DEMO_REPO" worktree remove --force "$WT_DIR/$wt" 2>/dev/null || true
    rm -rf "$WT_DIR/$wt"
  fi
done
rm -rf "$DEMO_REPO"
mkdir -p "$WT_DIR"

echo "▸ Creating self-contained demo git repo…"
mkdir -p "$DEMO_REPO/demo/sample-app"
cp -R "$DEMO_DIR/sample-app/." "$DEMO_REPO/demo/sample-app/"

# Demo-specific docktree.yml — no shared services, points to demo compose file.
cat > "$DEMO_REPO/docktree.yml" <<'DTCFG'
compose:
  files:
    - demo/sample-app/docker-compose.yml
ports:
  mode: dynamic
  bind_host: 127.0.0.1
  range: 41000-49999
transforms:
  container_name: strip
  built_image: rewrite
  docker_socket: warn
state:
  directory: .docktree
DTCFG

git init -q "$DEMO_REPO"
git -C "$DEMO_REPO" config user.name "Docktree Demo"
git -C "$DEMO_REPO" config user.email "demo@docktree.local"
git -C "$DEMO_REPO" checkout -b main 2>/dev/null || true
git -C "$DEMO_REPO" add -A
git -C "$DEMO_REPO" commit -q -m "demo: initial sample app" --no-verify

echo "▸ Creating worktree A (demo-main)…"
git -C "$DEMO_REPO" worktree add -B demo-main "$WT_DIR/main" HEAD

echo "▸ Creating worktree B (demo-feature)…"
git -C "$DEMO_REPO" worktree add -B demo-feature "$WT_DIR/feature" HEAD

# --- Write version-specific config into each worktree ---

cat > "$WT_DIR/main/demo/sample-app/version.json" <<'EOF'
{
  "version_label": "Version A: Main",
  "branch_label": "demo-main",
  "checkout_label": "Main checkout",
  "bg_color": "#0f172a",
  "accent_color": "#3b82f6"
}
EOF

cat > "$WT_DIR/feature/demo/sample-app/version.json" <<'EOF'
{
  "version_label": "Version B: Feature",
  "branch_label": "demo-feature",
  "checkout_label": "Feature checkout",
  "bg_color": "#0f1f1a",
  "accent_color": "#10b981"
}
EOF

# Commit the version differences so each branch is self-describing.
git -C "$WT_DIR/main" add demo/sample-app/version.json
git -C "$WT_DIR/main" commit -q -m "demo: set Main version config" --no-verify

git -C "$WT_DIR/feature" add demo/sample-app/version.json
git -C "$WT_DIR/feature" commit -q -m "demo: set Feature version config" --no-verify

echo ""
echo "✓ Demo worktrees ready:"
echo "    Repo:   $DEMO_REPO"
echo "    A:      $WT_DIR/main  (branch demo-main, blue accent)"
echo "    B:      $WT_DIR/feature  (branch demo-feature, emerald accent)"
echo ""
echo "Next: run $DEMO_DIR/scripts/run-demo.sh"
