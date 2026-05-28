#!/bin/bash
# Background worker for the youtube-helper Alfred workflow.
# Runs the heavy pipeline detached, logs everything to a file (so progress is
# visible even when macOS suppresses notifications), then opens the result.
# Invoked as: MODE=summary|lecture summarize.sh <youtube-url>
set -uo pipefail

# --- PATH: GUI apps get a minimal PATH; add Homebrew, pyenv shims and go/bin
#     so yt-dlp, ffmpeg/ffprobe and the binary resolve. (#1 cause of
#     "works in terminal, fails from Alfred".)
export PATH="/opt/homebrew/bin:$HOME/.pyenv/shims:$HOME/go/bin:/usr/bin:/bin:/usr/sbin:/sbin"

BIN="$HOME/go/bin/youtube-helper"
OUT_DIR="$HOME/Documents/YouTube Summaries"
LOG="$OUT_DIR/.last-run.log"
MODE="${MODE:-summary}"
URL="${1:-}"

mkdir -p "$OUT_DIR"               # always exists → result location predictable
: > "$LOG"                        # fresh log each run

log() { printf '%s  %s\n' "$(date '+%H:%M:%S')" "$1" >>"$LOG"; }

# ding plays a system sound. afplay needs NO notification permission, so it is
# the one feedback channel that always works regardless of macOS settings.
ding() { afplay "/System/Library/Sounds/$1.aiff" >/dev/null 2>&1 & }

notify() { # subtitle, message, sound  — logfile is the source of truth
  log "$1: $2"
  [ -n "${3:-}" ] && ding "$3"
  if command -v terminal-notifier >/dev/null 2>&1; then
    terminal-notifier -title "youtube-helper" -subtitle "$1" -message "$2" -sound default >/dev/null 2>&1
  else
    /usr/bin/osascript -e "display notification \"$2\" with title \"youtube-helper\" subtitle \"$1\" sound name \"${3:-Glass}\"" >/dev/null 2>&1
  fi
}

fail() { notify "Failed" "$1" "Basso"; log "see full log: $LOG"; open "$LOG" 2>/dev/null; exit 1; }

log "start mode=$MODE url=$URL"
ding "Tink"                       # immediate audible "triggered"

[ -n "$URL" ] || fail "no URL given"
[ -x "$BIN" ] || fail "binary missing: run 'go install ./cmd/youtube-helper'"

# --- API key: Keychain → environment → repo .env (first hit wins).
OPENAI_API_KEY="$(/usr/bin/security find-generic-password -a "$USER" -s OPENAI_API_KEY -w 2>/dev/null)"
if [ -z "$OPENAI_API_KEY" ] && [ -n "${OPENAI_API_KEY_ENV:-}" ]; then
  OPENAI_API_KEY="$OPENAI_API_KEY_ENV"
fi
export OPENAI_API_KEY
[ -n "$OPENAI_API_KEY" ] || fail "OPENAI_API_KEY not in Keychain — run: security add-generic-password -a \$USER -s OPENAI_API_KEY -w sk-..."

# --- Dedup: one job per video id. Second trigger of same URL is a no-op.
VID=$(printf '%s' "$URL" | grep -oE '[a-zA-Z0-9_-]{11}' | head -1)
LOCK="/tmp/yt-helper-${VID:-unknown}.lock"
if ! mkdir "$LOCK" 2>/dev/null; then
  notify "Already running" "${VID:-this video}"
  exit 0
fi
trap 'rmdir "$LOCK" 2>/dev/null' EXIT

notify "Working" "downloading + transcribing… (minutes)" "Funk"

# --- Run, streaming the binary's own progress lines into the log.
"$BIN" --output "$OUT_DIR" --mode "$MODE" "$URL" >>"$LOG" 2>&1
STATUS=$?

[ "$STATUS" -eq 0 ] || fail "$(tail -1 "$LOG")"

FILE=$(sed -n 's/^Output saved to: //p' "$LOG" | tail -1)
if [ -n "$FILE" ] && [ -f "$FILE" ]; then
  notify "Done · opened" "$(basename "$FILE")" "Glass"
  open "$FILE"                    # last mile: pop the result open
else
  notify "Done" "saved to $OUT_DIR" "Glass"
  open "$OUT_DIR"
fi
