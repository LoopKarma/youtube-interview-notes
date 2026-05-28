#!/bin/bash
# Background worker for the youtube-helper Alfred workflow.
# Runs the heavy pipeline detached, then opens the result and notifies.
# Invoked as: MODE=summary|lecture summarize.sh <youtube-url>
set -uo pipefail

# --- PATH: GUI apps get a minimal PATH; add Homebrew, pyenv shims and go/bin
#     so yt-dlp, ffmpeg/ffprobe and the binary resolve. (#1 cause of
#     "works in terminal, fails from Alfred".)
export PATH="/opt/homebrew/bin:$HOME/.pyenv/shims:$HOME/go/bin:/usr/bin:/bin:/usr/sbin:/sbin"

BIN="$HOME/go/bin/youtube-helper"
OUT_DIR="$HOME/Documents/YouTube Summaries"
MODE="${MODE:-summary}"
URL="${1:-}"

notify() { # subtitle, message
  /usr/bin/osascript -e "display notification \"$2\" with title \"youtube-helper\" subtitle \"$1\"" >/dev/null 2>&1
}

[ -n "$URL" ] || { notify "Error" "no URL"; exit 1; }
[ -x "$BIN" ] || { notify "Error" "binary missing: run 'go install ./cmd/youtube-helper'"; exit 1; }

# --- API key from Keychain (encrypted, per-process — no plaintext plist,
#     no global launchctl env).
OPENAI_API_KEY=$(/usr/bin/security find-generic-password -a "$USER" -s OPENAI_API_KEY -w 2>/dev/null)
export OPENAI_API_KEY
[ -n "$OPENAI_API_KEY" ] || { notify "Error" "OPENAI_API_KEY not in Keychain"; exit 1; }

# --- Dedup: one job per video id. Second trigger of same URL is a no-op.
VID=$(printf '%s' "$URL" | grep -oE '[a-zA-Z0-9_-]{11}' | head -1)
LOCK="/tmp/yt-helper-${VID:-unknown}.lock"
if ! mkdir "$LOCK" 2>/dev/null; then
  notify "Already running" "${VID:-this video}"
  exit 0
fi
trap 'rmdir "$LOCK" 2>/dev/null' EXIT

mkdir -p "$OUT_DIR"

# --- Run. Capture output so we can find the saved path and surface errors.
LOG=$("$BIN" --output "$OUT_DIR" --mode "$MODE" "$URL" 2>&1)
STATUS=$?

if [ "$STATUS" -ne 0 ]; then
  notify "Failed" "$(printf '%s' "$LOG" | tail -1)"
  exit "$STATUS"
fi

FILE=$(printf '%s\n' "$LOG" | sed -n 's/^Output saved to: //p' | tail -1)
if [ -n "$FILE" ] && [ -f "$FILE" ]; then
  open "$FILE"                       # last mile: pop the result open
  notify "Done · opened" "$(basename "$FILE")"
else
  open "$OUT_DIR"
  notify "Done" "saved to $OUT_DIR"
fi
