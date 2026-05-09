# youtube-helper

CLI tool. Download YouTube audio, transcribe with Whisper, summarize with GPT into structured markdown.

## Requirements

- Python ≥ 3.12
- OpenAI API key

`ffmpeg` + `ffprobe` bundled via `static-ffmpeg` (auto-fetched on first run, cached in venv).

## Install

```bash
# from repo root
python3.12 -m venv .venv
source .venv/bin/activate
pip install -e .
```

## Configure

Copy `.env.example` → `.env`, set key:

```
OPENAI_API_KEY=sk-...
```

`.env` auto-loaded at runtime via `python-dotenv`.

## Run

```bash
youtube-helper <YOUTUBE_URL>
```

Writes `<Sanitized_Video_Title>.md` to current dir.

### Options

| Flag | Default | Purpose |
|------|---------|---------|
| `-o, --output DIR` | `.` | Output dir for markdown |
| `-m, --model NAME` | `gpt-5-mini` | OpenAI model |
| `--mode {summary,lecture}` | `summary` | Output style (see below) |
| `--max-chunks N` | none | Cap 10-min audio chunks (cost/test control) |

### Modes

- `summary` — Q&A digest per section + research topics. Output: `<title>.md`
- `lecture` — detailed conspect: parts, key concepts, ordered notes, takeaways. Faithful to source. Output: `<title>.lecture.md`

Both modes share the audio + transcript cache — switching mode on a re-run only re-prompts GPT, no re-download/re-transcribe.

### Examples

```bash
# basic summary
youtube-helper "https://youtu.be/dQw4w9WgXcQ"

# lecture conspect (uses cached audio/transcript if previously run)
youtube-helper "https://youtu.be/..." --mode lecture

# custom dir + cheap test (1 chunk = first 10 min)
youtube-helper "https://youtu.be/..." -o ./out --max-chunks 1

# different model
youtube-helper "https://youtu.be/..." -m gpt-5
```

## Pipeline

1. **Download** — `yt-dlp` grabs `bestaudio`, ffmpeg (via `static-ffmpeg`) encodes to 64kbps mp3
2. **Split** — files > 24MB chunked into 10-min segments (Whisper 25MB limit)
3. **Transcribe** — Whisper `whisper-1`, verbose JSON, segment timestamps
4. **Summarize** — GPT produces sectioned markdown: timecodes, Q&A, research topics
5. **Write** — `<title>.md` with header (title, URL, duration) + summary body

## Cache

`~/.cache/youtube-helper/`
- `audio/<video_id>.mp3` — downloaded audio (skip re-download)
- `transcripts/<video_id>.json` — Whisper segments (skip re-transcription)

Re-running same URL only re-summarizes (cheap).

## Layout

```
src/youtube_helper/
├── cli.py          # Click entrypoint, orchestration
├── downloader.py   # yt-dlp + audio cache
├── transcriber.py  # ffmpeg chunking + Whisper
└── summarizer.py   # GPT prompt + structured markdown
```
