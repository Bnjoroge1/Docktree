#!/usr/bin/env bash
# render-demo.sh — assemble final demo video from VHS tapes and browser recording.
#
# Requires: vhs (optional), ffmpeg (optional). Prints install instructions if missing.
set -euo pipefail

# Force color output in VHS terminal recordings even if the parent environment disables it
export CLICOLOR_FORCE=1
unset NO_COLOR

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DEMO_DIR="$REPO_ROOT/demo"
OUT_DIR="$DEMO_DIR/out"
TAPE_DIR="$DEMO_DIR/recordings"

mkdir -p "$OUT_DIR"

# ── Helper: check for a tool ──
have() { command -v "$1" >/dev/null 2>&1; }

# ── Step 1: Render VHS tapes to MP4 ──
if have vhs; then
  echo "▸ Rendering VHS tapes…"
  for tape in "$TAPE_DIR"/*.tape; do
    [ -f "$tape" ] || continue
    name="$(basename "$tape" .tape)"
    # Skip if MP4 already exists.
    if [ -f "$OUT_DIR/$name.mp4" ]; then
      echo "  → $name (already rendered, skipping)"
      continue
    fi
    echo "  → $name"
    vhs "$tape" 2>/dev/null || echo "    (vhs failed for $name, skipping)"
  done
else
  echo "⚠ vhs not found — skipping terminal GIF/MP4 rendering."
  echo "  Install: brew install vhs  (macOS)  or  https://github.com/charmbracelet/vhs"
fi

# ── Step 2: Assemble with ffmpeg ──
if ! have ffmpeg; then
  echo ""
  echo "⚠ ffmpeg not found — skipping video assembly."
  echo "  Install: brew install ffmpeg  (macOS)  or  https://ffmpeg.org/download.html"
  echo ""
  echo "Individual clips are in $OUT_DIR/"
  exit 0
fi

echo ""
echo "▸ Assembling final video with ffmpeg…"

# Collect rendered terminal clips in order.
TERM_CLIPS=()
for name in 01-problem 02-docktree-up-main 03-docktree-up-feature 04-two-worktrees 05-docker-proxy 06-docker-tunnel docktree-demo; do
  for ext in mp4 gif; do
    f="$OUT_DIR/$name.$ext"
    if [ -f "$f" ]; then
      TERM_CLIPS+=("$f")
      break
    fi
  done
done

# Concatenate terminal clips into a single video.
if [ ${#TERM_CLIPS[@]} -gt 0 ]; then
  CONCAT_LIST="$OUT_DIR/concat-list.txt"
  : > "$CONCAT_LIST"
  for clip in "${TERM_CLIPS[@]}"; do
    echo "file '$clip'" >> "$CONCAT_LIST"
  done

  # Re-encode each clip to a common format, then concat.
  NORMALIZED=()
  for clip in "${TERM_CLIPS[@]}"; do
    norm="$OUT_DIR/.norm-$(basename "$clip" .mp4).mp4"
    norm="${norm%.gif}.mp4"
    ffmpeg -y -i "$clip" -c:v libx264 -pix_fmt yuv420p -r 30 \
      -vf "scale=1200:600:force_original_aspect_ratio=decrease,pad=1200:600:(ow-iw)/2:(oh-ih)/2:black" \
      "$norm" 2>/dev/null || true
    [ -f "$norm" ] && NORMALIZED+=("$norm")
  done

  if [ ${#NORMALIZED[@]} -gt 0 ]; then
    NORM_LIST="$OUT_DIR/.norm-list.txt"
    : > "$NORM_LIST"
    for clip in "${NORMALIZED[@]}"; do
      echo "file '$clip'" >> "$NORM_LIST"
    done
    ffmpeg -y -f concat -safe 0 -i "$NORM_LIST" -c copy "$OUT_DIR/terminal-demo.mp4" 2>/dev/null || true
    rm -f "$OUT_DIR/.norm-"*.mp4 "$NORM_LIST"
    echo "  → terminal-demo.mp4"
  fi
  rm -f "$CONCAT_LIST"
fi

# Convert browser recording if present.
BROWSER_MP4=""
if [ -f "$OUT_DIR/browser-demo.webm" ]; then
  ffmpeg -y -i "$OUT_DIR/browser-demo.webm" -c:v libx264 -pix_fmt yuv420p -r 30 \
    "$OUT_DIR/browser-demo.mp4" 2>/dev/null || true
  BROWSER_MP4="$OUT_DIR/browser-demo.mp4"
  echo "  → browser-demo.mp4"
fi

# Side-by-side composition: terminal left, browser right.
if [ -f "$OUT_DIR/terminal-demo.mp4" ] && [ -n "$BROWSER_MP4" ]; then
  echo "▸ Creating side-by-side composition…"
  ffmpeg -y \
    -i "$OUT_DIR/terminal-demo.mp4" \
    -i "$BROWSER_MP4" \
    -filter_complex "
      [0:v]scale=960:600:force_original_aspect_ratio=decrease,pad=960:600:(ow-iw)/2:(oh-ih)/2:black[t];
      [1:v]scale=960:600:force_original_aspect_ratio=decrease,pad=960:600:(ow-iw)/2:(oh-ih)/2:black[b];
      [t][b]hstack=inputs=2[v]
    " -map "[v]" -c:v libx264 -pix_fmt yuv420p -r 30 \
    "$OUT_DIR/docktree-demo.mp4" 2>/dev/null && echo "  → docktree-demo.mp4" || {
      # Fallback: sequential concatenation if side-by-side fails.
      echo "  (side-by-side failed, trying sequential concat…)"
      SEQ_LIST="$OUT_DIR/.seq-list.txt"
      : > "$SEQ_LIST"
      echo "file '$OUT_DIR/terminal-demo.mp4'" >> "$SEQ_LIST"
      echo "file '$BROWSER_MP4'" >> "$SEQ_LIST"
      ffmpeg -y -f concat -safe 0 -i "$SEQ_LIST" -c copy "$OUT_DIR/docktree-demo.mp4" 2>/dev/null || true
      rm -f "$SEQ_LIST"
      [ -f "$OUT_DIR/docktree-demo.mp4" ] && echo "  → docktree-demo.mp4"
    }
fi

echo ""
echo "✓ Render complete. Output in $OUT_DIR/"
ls -1 "$OUT_DIR"/*.mp4 2>/dev/null || echo "  (no mp4 files generated)"
