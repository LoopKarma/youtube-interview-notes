import re
from pathlib import Path

import click
import static_ffmpeg
from dotenv import load_dotenv
from openai import OpenAI

from youtube_helper.downloader import download_audio, format_duration
from youtube_helper.summarizer import summarize_transcript
from youtube_helper.transcriber import segments_to_text, transcribe_audio

static_ffmpeg.add_paths()


def sanitize_filename(name: str) -> str:
    """Turn a video title into a safe filename."""
    name = re.sub(r'[<>:"/\\|?*]', "", name)
    name = re.sub(r"\s+", "_", name.strip())
    return name[:200]


@click.command()
@click.argument("url")
@click.option(
    "--output",
    "-o",
    type=click.Path(file_okay=False),
    default=".",
    help="Output directory for the markdown file.",
)
@click.option("--model", "-m", default="gpt-4o", help="OpenAI model for summarization.")
@click.option(
    "--mode",
    type=click.Choice(["summary", "lecture"]),
    default="summary",
    help="Output style: 'summary' (Q&A digest) or 'lecture' (detailed conspect).",
)
@click.option(
    "--max-chunks",
    type=int,
    default=None,
    help="Max number of 10-min audio chunks to transcribe (for testing/cost control).",
)
def main(url: str, output: str, model: str, mode: str, max_chunks: int | None) -> None:
    """Transcribe a YouTube video and generate a structured markdown output."""
    load_dotenv()
    client = OpenAI()

    # 1. Download audio
    click.echo(f"Downloading audio from: {url}")
    video = download_audio(url)
    click.echo(f"Downloaded: {video.title} ({format_duration(video.duration)})")

    # 2. Transcribe
    click.echo("Transcribing audio with Whisper...")
    segments = transcribe_audio(video.audio_path, client, video.video_id, max_chunks=max_chunks)
    click.echo(f"Transcription complete: {len(segments)} segments")

    # 3. Summarize / conspect
    transcript_text = segments_to_text(segments)
    click.echo(f"Generating {mode} with {model}...")
    body = summarize_transcript(transcript_text, client, model=model, mode=mode)

    # 4. Build markdown
    header = (
        f"# {video.title}\n"
        f"**URL:** {video.url}\n"
        f"**Duration:** {format_duration(video.duration)}\n"
    )
    markdown = f"{header}\n{body}\n"

    # 5. Write output
    out_dir = Path(output)
    out_dir.mkdir(parents=True, exist_ok=True)
    suffix = "" if mode == "summary" else f".{mode}"
    filename = f"{sanitize_filename(video.title)}{suffix}.md"
    out_path = out_dir / filename
    out_path.write_text(markdown, encoding="utf-8")

    click.echo(f"Output saved to: {out_path}")
