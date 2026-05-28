# Alfred workflow — youtube-helper

Copy a YouTube URL, type `yt`, get the markdown summary opened for you. Heavy work runs in the background; you get a "started" ping and a "done" ping.

## What it does (and which productivity traps it dodges)

| Concern | Handling |
|---------|----------|
| GUI PATH can't find `yt-dlp`/`ffmpeg`/binary | worker exports Homebrew + pyenv-shims + `go/bin` PATH |
| API key security | pulled from **Keychain** at runtime — no plaintext file, no global `launchctl` env |
| Long job blocks Alfred | worker is **detached** (`nohup … &`); Alfred returns instantly |
| "Is it running?" | immediate `Summarizing…` notification |
| Double trigger → 2 jobs | per-video-id **lockfile** in `/tmp` |
| Fake "saved" on failure | branches on **exit code**; real error text in notification |
| Hunt for the output | auto-`open`s the `.md` when done |
| Wrong clipboard content | validates URL is `youtube.com`/`youtu.be`, else rejects |

## Setup (one time)

```bash
# 1. store the OpenAI key in Keychain
security add-generic-password -a "$USER" -s OPENAI_API_KEY -w "sk-YOUR-KEY"

# 2. install the binary to ~/go/bin
go install ./cmd/youtube-helper

# 3. install summarize.sh where the workflow expects it
#    (it runs from the workflow dir after import — nothing extra needed,
#     but keep the file executable)
chmod +x alfred/youtube-helper/summarize.sh
```

## Install the workflow

Either double-click the packaged file:

```bash
make -C alfred pack      # builds alfred/youtube-helper.alfredworkflow
open alfred/youtube-helper.alfredworkflow
```

…or in Alfred: **Workflows → import** the `.alfredworkflow`.

## Use

| Input | Result |
|-------|--------|
| `yt https://youtu.be/…` | summary mode |
| `yt lecture https://youtu.be/…` | lecture conspect |
| copy URL, then just `yt` | uses clipboard |

Output → `~/Documents/YouTube Summaries/`, opened automatically.

## Progress & troubleshooting

macOS often silences `osascript` notifications (the posting app lacks notification permission), so the **logfile is the source of truth**:

```bash
tail -f "$HOME/Documents/YouTube Summaries/.last-run.log"
```

Every run truncates and rewrites it: start line, the binary's own progress, and a final `Done`/`Failed`. On failure the log is opened automatically.

To get banner notifications reliably, install `terminal-notifier` (`brew install terminal-notifier`) — the worker uses it when present, else falls back to `osascript`.

Common failures (all logged):
- `OPENAI_API_KEY not in Keychain` → run the setup step 1 command.
- `binary missing` → `go install ./cmd/youtube-helper`.
- nothing happens at all → the worker isn't being reached; run it directly to isolate: `MODE=summary ./alfred/youtube-helper/summarize.sh "<url>"`.

## Optional extras (add in Alfred UI, no plist edits)

- **Hotkey**: drag a *Hotkey* trigger onto the canvas → connect to the Run Script → bind e.g. `⌥⌘Y`.
- **Universal Action**: add a *Universal Action* trigger so you can select a URL anywhere → connect to the same Run Script.

## Notes

- First run downloads + transcribes (minutes for long videos). Re-running the same URL is fast — audio + transcript are cached under `~/.cache/youtube-helper/`.
- Change output dir or default mode at the top of `summarize.sh`.
