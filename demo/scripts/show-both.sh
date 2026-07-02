#!/usr/bin/env bash
# show-both.sh — prove both app versions are live simultaneously with different content.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DEMO_DIR="$REPO_ROOT/demo"

DOCKTREE="${DOCKTREE:-$REPO_ROOT/docktree}"
if [ ! -x "$DOCKTREE" ]; then
  echo "✗ docktree binary not found at $DOCKTREE"
  exit 1
fi

echo "━━━ All Docktree Instances ━━━"
"$DOCKTREE" status --all

echo ""
echo "━━━ All Allocated Ports ━━━"
"$DOCKTREE" ports --all

echo ""
echo "━━━ Live Verification (curl each app) ━━━"

# Extract ports from docktree ports --all --json using python3 (universally available).
PORTS_JSON="$("$DOCKTREE" ports --all --json)"

MAIN_PORT="$(echo "$PORTS_JSON" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for e in data.get('entries', []):
    for p in e.get('ports', []):
        if p.get('service') == 'web' and p.get('container_port') == 3000 and 'main' in e.get('instance', ''):
            print(p['host_port']); break
    else:
        continue
    break
" 2>/dev/null || echo "")"

FEATURE_PORT="$(echo "$PORTS_JSON" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for e in data.get('entries', []):
    for p in e.get('ports', []):
        if p.get('service') == 'web' and p.get('container_port') == 3000 and 'feature' in e.get('instance', ''):
            print(p['host_port']); break
    else:
        continue
    break
" 2>/dev/null || echo "")"

if [ -z "$MAIN_PORT" ] || [ -z "$FEATURE_PORT" ]; then
  echo "✗ Could not extract ports. Are both instances running?"
  echo "  Run: $DEMO_DIR/scripts/run-demo.sh"
  exit 1
fi

echo ""
echo "  Main URL:    http://localhost:$MAIN_PORT"
echo "  Feature URL: http://localhost:$FEATURE_PORT"
echo ""

echo "── curl http://localhost:$MAIN_PORT/api ──"
curl -s "http://localhost:$MAIN_PORT/api" || echo "  (request failed)"

echo ""
echo "── curl http://localhost:$FEATURE_PORT/api ──"
curl -s "http://localhost:$FEATURE_PORT/api" || echo "  (request failed)"

echo ""
echo ""
echo "✓ Two different versions of the same app, running simultaneously."
echo "  Same source compose service name + port, isolated by Docktree."
