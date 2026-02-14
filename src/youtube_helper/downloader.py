import re
from dataclasses import dataclass
from pathlib import Path

import yt_dlp

CACHE_DIR = Path.home() / ".cache" / "youtube-helper"


@dataclass
class VideoInfo:
    title: str
    url: str
    duration: int  # seconds
    audio_path: Path
    video_id: str


def format_duration(seconds: int) -> str:
    h, rem = divmod(seconds, 3600)
    m, s = divmod(rem, 60)
    if h:
        return f"{h:02d}:{m:02d}:{s:02d}"
    return f"{m:02d}:{s:02d}"


def _extract_video_id(url: str) -> str | None:
    """Try to extract video ID from URL for cache key."""
    match = re.search(r"(?:v=|/)([a-zA-Z0-9_-]{11})", url)
    return match.group(1) if match else None


def download_audio(url: str) -> VideoInfo:
    """Download audio from a YouTube video. Uses cache if available."""
    audio_dir = CACHE_DIR / "audio"
    audio_dir.mkdir(parents=True, exist_ok=True)

    # Check cache first
    video_id = _extract_video_id(url)
    if video_id:
        cached = audio_dir / f"{video_id}.mp3"
        if cached.exists():
            # Still need metadata — extract without downloading
            with yt_dlp.YoutubeDL({"quiet": True, "no_warnings": True}) as ydl:
                info = ydl.extract_info(url, download=False)
            return VideoInfo(
                title=info.get("title", "Unknown"),
                url=url,
                duration=info.get("duration", 0),
                audio_path=cached,
                video_id=video_id,
            )

    ydl_opts = {
        "format": "bestaudio/best",
        "outtmpl": str(audio_dir / "%(id)s.%(ext)s"),
        "quiet": True,
        "no_warnings": True,
        "postprocessors": [
            {
                "key": "FFmpegExtractAudio",
                "preferredcodec": "mp3",
                "preferredquality": "64",
            }
        ],
    }

    with yt_dlp.YoutubeDL(ydl_opts) as ydl:
        info = ydl.extract_info(url, download=True)
        vid = info["id"]
        filename = audio_dir / f"{vid}.mp3"

    return VideoInfo(
        title=info.get("title", "Unknown"),
        url=url,
        duration=info.get("duration", 0),
        audio_path=Path(filename),
        video_id=vid,
    )
