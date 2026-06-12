// Package downloader fetches the audio track of a YouTube video via yt-dlp
// and caches the result on disk keyed by the video id, so repeated runs on
// the same URL skip the network entirely.
package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/molov/youtube-helper/internal/runner"
)

// VideoInfo describes a downloaded video and the on-disk path to its audio.
type VideoInfo struct {
	Title     string
	URL       string
	Duration  int // seconds
	AudioPath string
	VideoID   string
}

// FormatDuration renders a number of seconds as HH:MM:SS, dropping the hours
// component when it is zero (e.g. "03:42" or "01:05:09").
func FormatDuration(seconds int) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

var videoIDRe = regexp.MustCompile(`(?:v=|/)([a-zA-Z0-9_-]{11})`)

// ExtractVideoID pulls the 11-character YouTube id out of a URL for use as a
// cache key. It returns an empty string when no id can be found.
func ExtractVideoID(url string) string {
	m := videoIDRe.FindStringSubmatch(url)
	if m == nil {
		return ""
	}
	return m[1]
}

// ytdlpInfo is the subset of yt-dlp's JSON metadata the app consumes.
type ytdlpInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Duration int    `json:"duration"`
}

// Service downloads audio and manages the audio cache. Construct it with New.
type Service struct {
	cacheDir string
	runner   runner.Runner
}

// New returns a Service that caches audio under cacheDir/audio and uses r to
// invoke yt-dlp. Pass runner.Exec for production use.
func New(cacheDir string, r runner.Runner) *Service {
	return &Service{cacheDir: cacheDir, runner: r}
}

// audioDir is the directory holding cached mp3 files.
func (s *Service) audioDir() string {
	return filepath.Join(s.cacheDir, "audio")
}

// Download returns the audio for url, downloading and transcoding to mp3 only
// when it is not already cached. Metadata is always fetched fresh so the title
// and duration are accurate even on a cache hit.
func (s *Service) Download(ctx context.Context, url string) (VideoInfo, error) {
	dir := s.audioDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return VideoInfo{}, fmt.Errorf("create audio cache dir: %w", err)
	}

	if id := ExtractVideoID(url); id != "" {
		cached := filepath.Join(dir, id+".mp3")
		if _, err := os.Stat(cached); err == nil {
			info, err := s.metadata(ctx, url)
			if err != nil {
				return VideoInfo{}, err
			}
			return VideoInfo{
				Title:     info.Title,
				URL:       url,
				Duration:  info.Duration,
				AudioPath: cached,
				VideoID:   id,
			}, nil
		}
	}

	info, err := s.fetch(ctx, url, dir)
	if err != nil {
		return VideoInfo{}, err
	}
	return VideoInfo{
		Title:     info.Title,
		URL:       url,
		Duration:  info.Duration,
		AudioPath: filepath.Join(dir, info.ID+".mp3"),
		VideoID:   info.ID,
	}, nil
}

// metadata fetches video info without downloading the media.
func (s *Service) metadata(ctx context.Context, url string) (ytdlpInfo, error) {
	out, err := s.runner.Run(ctx, "yt-dlp", "--dump-json", "--skip-download", url)
	if err != nil {
		return ytdlpInfo{}, fmt.Errorf("yt-dlp metadata: %w", err)
	}
	return parseInfo(out)
}

// fetch downloads the bestaudio stream, transcodes to 64kbps mp3 into dir and
// returns the parsed metadata printed by yt-dlp.
func (s *Service) fetch(ctx context.Context, url, dir string) (ytdlpInfo, error) {
	out, err := s.runner.Run(ctx, "yt-dlp",
		"--format", "bestaudio/best",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "64K",
		"--output", filepath.Join(dir, "%(id)s.%(ext)s"),
		"--print-json",
		"--no-warnings",
		url,
	)
	if err != nil {
		return ytdlpInfo{}, fmt.Errorf("yt-dlp download: %w", err)
	}
	return parseInfo(out)
}

// parseInfo decodes the first JSON object emitted by yt-dlp, applying the same
// defaults the Python version used for missing fields.
func parseInfo(out []byte) (ytdlpInfo, error) {
	dec := json.NewDecoder(bytes.NewReader(out))
	var info ytdlpInfo
	if err := dec.Decode(&info); err != nil {
		return ytdlpInfo{}, fmt.Errorf("parse yt-dlp json: %w", err)
	}
	if info.Title == "" {
		info.Title = "Unknown"
	}
	return info, nil
}
