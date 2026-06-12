// Package app wires the download → transcribe → summarize → write pipeline
// together. Every external step is an interface so the whole flow can be driven
// by fakes in tests.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/molov/youtube-helper/internal/downloader"
	"github.com/molov/youtube-helper/internal/transcriber"
)

// Downloader fetches audio for a URL.
type Downloader interface {
	Download(ctx context.Context, url string) (downloader.VideoInfo, error)
}

// Transcriber turns an audio file into timestamped segments.
type Transcriber interface {
	Transcribe(ctx context.Context, audioPath, videoID string, maxChunks int) ([]transcriber.Segment, error)
}

// Summarizer renders a transcript into structured markdown for the given mode.
type Summarizer interface {
	Summarize(ctx context.Context, transcript, model, mode string) (string, error)
}

// Options holds everything the pipeline needs for one run; it maps 1:1 to the
// CLI flags.
type Options struct {
	URL       string
	Output    string
	Model     string
	Mode      string
	MaxChunks int
}

// App runs the pipeline against injected dependencies. Out receives progress
// messages (use os.Stdout in production, a buffer in tests).
type App struct {
	Downloader  Downloader
	Transcriber Transcriber
	Summarizer  Summarizer
	Out         io.Writer
}

var unsafeChars = regexp.MustCompile(`[<>:"/\\|?*]`)
var whitespace = regexp.MustCompile(`\s+`)

// SanitizeFilename turns a video title into a filesystem-safe name: it strips
// reserved characters, collapses whitespace to underscores and caps the length
// at 200 runes.
func SanitizeFilename(name string) string {
	name = unsafeChars.ReplaceAllString(name, "")
	name = whitespace.ReplaceAllString(strings.TrimSpace(name), "_")
	r := []rune(name)
	if len(r) > 200 {
		r = r[:200]
	}
	return string(r)
}

// Run executes the full pipeline and returns the path of the written markdown
// file. The output filename gains a ".lecture" infix in lecture mode, matching
// the Python tool.
func (a *App) Run(ctx context.Context, opts Options) (string, error) {
	a.echo("Downloading audio from: %s", opts.URL)
	video, err := a.Downloader.Download(ctx, opts.URL)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	a.echo("Downloaded: %s (%s)", video.Title, downloader.FormatDuration(video.Duration))

	a.echo("Transcribing audio with Whisper...")
	segments, err := a.Transcriber.Transcribe(ctx, video.AudioPath, video.VideoID, opts.MaxChunks)
	if err != nil {
		return "", fmt.Errorf("transcribe: %w", err)
	}
	a.echo("Transcription complete: %d segments", len(segments))

	transcript := transcriber.SegmentsToText(segments)
	a.echo("Generating %s with %s...", opts.Mode, opts.Model)
	body, err := a.Summarizer.Summarize(ctx, transcript, opts.Model, opts.Mode)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	markdown := fmt.Sprintf("# %s\n**URL:** %s\n**Duration:** %s\n\n%s\n",
		video.Title, video.URL, downloader.FormatDuration(video.Duration), body)

	if err := os.MkdirAll(opts.Output, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	suffix := ""
	if opts.Mode != "summary" {
		suffix = "." + opts.Mode
	}
	outPath := filepath.Join(opts.Output, SanitizeFilename(video.Title)+suffix+".md")
	if err := os.WriteFile(outPath, []byte(markdown), 0o644); err != nil {
		return "", fmt.Errorf("write output: %w", err)
	}
	a.echo("Output saved to: %s", outPath)
	return outPath, nil
}

// echo writes a progress line to Out when one is configured.
func (a *App) echo(format string, args ...any) {
	if a.Out == nil {
		return
	}
	fmt.Fprintf(a.Out, format+"\n", args...)
}
