import json
import subprocess
import tempfile
from dataclasses import asdict, dataclass
from pathlib import Path

import click
from openai import OpenAI

from youtube_helper.downloader import CACHE_DIR

MAX_FILE_SIZE = 24 * 1024 * 1024  # 24MB to stay under Whisper's 25MB limit
CHUNK_DURATION = 600  # 10 minutes per chunk


@dataclass
class Segment:
    start: float
    end: float
    text: str


def format_timecode(seconds: float) -> str:
    h, rem = divmod(int(seconds), 3600)
    m, s = divmod(rem, 60)
    if h:
        return f"{h:02d}:{m:02d}:{s:02d}"
    return f"{m:02d}:{s:02d}"


def _get_cache_path(video_id: str) -> Path:
    cache_dir = CACHE_DIR / "transcripts"
    cache_dir.mkdir(parents=True, exist_ok=True)
    return cache_dir / f"{video_id}.json"


def _load_cached_segments(video_id: str) -> list[Segment] | None:
    path = _get_cache_path(video_id)
    if not path.exists():
        return None
    data = json.loads(path.read_text())
    return [Segment(**s) for s in data]


def _save_segments_cache(video_id: str, segments: list[Segment]) -> None:
    path = _get_cache_path(video_id)
    path.write_text(json.dumps([asdict(s) for s in segments]))


def _split_audio(audio_path: Path, chunk_duration: int = CHUNK_DURATION) -> list[tuple[Path, float]]:
    """Split audio into chunks, returning list of (chunk_path, time_offset)."""
    result = subprocess.run(
        [
            "ffprobe", "-v", "quiet", "-show_entries", "format=duration",
            "-of", "default=noprint_wrappers=1:nokey=1", str(audio_path),
        ],
        capture_output=True, text=True,
    )
    total_duration = float(result.stdout.strip())

    if audio_path.stat().st_size <= MAX_FILE_SIZE:
        return [(audio_path, 0.0)]

    tmp_dir = tempfile.mkdtemp(prefix="yt-helper-chunks-")
    chunks = []
    offset = 0.0

    while offset < total_duration:
        chunk_path = Path(tmp_dir) / f"chunk_{int(offset)}.mp3"
        subprocess.run(
            [
                "ffmpeg", "-y", "-i", str(audio_path),
                "-ss", str(offset), "-t", str(chunk_duration),
                "-acodec", "libmp3lame", "-ab", "64k",
                "-v", "quiet",
                str(chunk_path),
            ],
            check=True,
        )
        chunks.append((chunk_path, offset))
        offset += chunk_duration

    return chunks


def _transcribe_chunk(chunk_path: Path, client: OpenAI, time_offset: float) -> list[Segment]:
    """Transcribe a single audio chunk and adjust timestamps by offset."""
    with open(chunk_path, "rb") as f:
        response = client.audio.transcriptions.create(
            model="whisper-1",
            file=f,
            response_format="verbose_json",
            timestamp_granularities=["segment"],
        )

    segments = []
    for seg in response.segments:
        start = seg.start if hasattr(seg, "start") else seg["start"]
        end = seg.end if hasattr(seg, "end") else seg["end"]
        text = seg.text if hasattr(seg, "text") else seg["text"]
        segments.append(
            Segment(
                start=start + time_offset,
                end=end + time_offset,
                text=text.strip(),
            )
        )
    return segments


def transcribe_audio(
    audio_path: Path,
    client: OpenAI,
    video_id: str,
    max_chunks: int | None = None,
) -> list[Segment]:
    """Transcribe audio file using OpenAI Whisper API with timestamps.

    Caches results per video_id. Splits large files into chunks.
    """
    cached = _load_cached_segments(video_id)
    if cached:
        click.echo("  Using cached transcription")
        return cached

    chunks = _split_audio(audio_path)
    if max_chunks is not None:
        chunks = chunks[:max_chunks]

    all_segments = []
    for i, (chunk_path, offset) in enumerate(chunks, 1):
        if len(chunks) > 1:
            click.echo(f"  Transcribing chunk {i}/{len(chunks)}...")
        segments = _transcribe_chunk(chunk_path, client, offset)
        all_segments.extend(segments)

        if chunk_path != audio_path:
            chunk_path.unlink(missing_ok=True)

    _save_segments_cache(video_id, all_segments)
    return all_segments


def segments_to_text(segments: list[Segment]) -> str:
    """Format segments into a timestamped transcript string for the summarizer."""
    lines = []
    for seg in segments:
        tc = format_timecode(seg.start)
        lines.append(f"[{tc}] {seg.text}")
    return "\n".join(lines)
