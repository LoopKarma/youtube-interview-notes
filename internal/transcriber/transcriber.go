// Package transcriber turns an audio file into timestamped text segments using
// an OpenAI Whisper backend. Large files are split into time-bounded chunks to
// stay under Whisper's upload limit, and results are cached per video id.
package transcriber

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/molov/youtube-helper/internal/runner"
)

const (
	// MaxFileSize keeps uploads under Whisper's 25MB limit.
	MaxFileSize = 24 * 1024 * 1024
	// ChunkDuration is the length, in seconds, of each split chunk.
	ChunkDuration = 600
)

// Segment is a single timestamped span of transcribed speech. JSON tags match
// the on-disk cache format and the Whisper verbose_json response.
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// AudioTranscriber transcribes a single audio file into segments. The OpenAI
// client implements this; tests supply a fake.
type AudioTranscriber interface {
	TranscribeFile(ctx context.Context, path string) ([]Segment, error)
}

// FormatTimecode renders seconds as HH:MM:SS, dropping hours when zero.
func FormatTimecode(seconds float64) string {
	total := int(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// SegmentsToText formats segments into a newline-separated, timecode-prefixed
// transcript suitable for feeding to the summarizer.
func SegmentsToText(segs []Segment) string {
	lines := make([]string, 0, len(segs))
	for _, seg := range segs {
		lines = append(lines, fmt.Sprintf("[%s] %s", FormatTimecode(seg.Start), seg.Text))
	}
	return strings.Join(lines, "\n")
}

// Service performs transcription with caching and chunking. Construct with New.
type Service struct {
	client   AudioTranscriber
	cacheDir string
	runner   runner.Runner

	// maxFileSize and chunkDuration are configurable to make the chunking path
	// exercisable in tests; New seeds them with the package defaults.
	maxFileSize   int64
	chunkDuration int
}

// New returns a Service that transcribes via client, caches under
// cacheDir/transcripts and uses r to invoke ffprobe/ffmpeg.
func New(client AudioTranscriber, cacheDir string, r runner.Runner) *Service {
	return &Service{
		client:        client,
		cacheDir:      cacheDir,
		runner:        r,
		maxFileSize:   MaxFileSize,
		chunkDuration: ChunkDuration,
	}
}

// chunk pairs a chunk file path with its time offset (seconds) in the original.
type chunk struct {
	path   string
	offset float64
}

// cachePath is the JSON cache file for a given video id.
func (s *Service) cachePath(videoID string) string {
	return filepath.Join(s.cacheDir, "transcripts", videoID+".json")
}

// Transcribe returns the segments for audioPath, using the cache when present.
// maxChunks > 0 caps how many chunks are transcribed (cost/test control); a
// value <= 0 means no cap.
func (s *Service) Transcribe(ctx context.Context, audioPath, videoID string, maxChunks int) ([]Segment, error) {
	if cached, err := s.loadCache(videoID); err != nil {
		return nil, err
	} else if cached != nil {
		return cached, nil
	}

	chunks, cleanup, err := s.split(ctx, audioPath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if maxChunks > 0 && maxChunks < len(chunks) {
		chunks = chunks[:maxChunks]
	}

	var all []Segment
	for _, c := range chunks {
		segs, err := s.client.TranscribeFile(ctx, c.path)
		if err != nil {
			return nil, fmt.Errorf("transcribe chunk at %.0fs: %w", c.offset, err)
		}
		for _, seg := range segs {
			all = append(all, Segment{
				Start: seg.Start + c.offset,
				End:   seg.End + c.offset,
				Text:  strings.TrimSpace(seg.Text),
			})
		}
	}

	if err := s.saveCache(videoID, all); err != nil {
		return nil, err
	}
	return all, nil
}

// loadCache reads cached segments for videoID, returning nil (not an error)
// when no cache exists.
func (s *Service) loadCache(videoID string) ([]Segment, error) {
	data, err := os.ReadFile(s.cachePath(videoID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read transcript cache: %w", err)
	}
	var segs []Segment
	if err := json.Unmarshal(data, &segs); err != nil {
		return nil, fmt.Errorf("parse transcript cache: %w", err)
	}
	return segs, nil
}

// saveCache writes segments for videoID to the cache directory.
func (s *Service) saveCache(videoID string, segs []Segment) error {
	path := s.cachePath(videoID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create transcript cache dir: %w", err)
	}
	data, err := json.Marshal(segs)
	if err != nil {
		return fmt.Errorf("encode transcript cache: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write transcript cache: %w", err)
	}
	return nil
}

// split returns the chunks to transcribe. A file at or under the size limit is
// returned as a single chunk; larger files are sliced with ffmpeg into a temp
// directory. The returned cleanup removes any temporary chunks.
func (s *Service) split(ctx context.Context, audioPath string) ([]chunk, func(), error) {
	noop := func() {}

	fi, err := os.Stat(audioPath)
	if err != nil {
		return nil, noop, fmt.Errorf("stat audio: %w", err)
	}
	if fi.Size() <= s.maxFileSize {
		return []chunk{{path: audioPath, offset: 0}}, noop, nil
	}

	total, err := s.probeDuration(ctx, audioPath)
	if err != nil {
		return nil, noop, err
	}

	tmpDir, err := os.MkdirTemp("", "yt-helper-chunks-")
	if err != nil {
		return nil, noop, fmt.Errorf("create chunk temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	var chunks []chunk
	for offset := 0.0; offset < total; offset += float64(s.chunkDuration) {
		out := filepath.Join(tmpDir, fmt.Sprintf("chunk_%d.mp3", int(offset)))
		_, err := s.runner.Run(ctx, "ffmpeg", "-y", "-i", audioPath,
			"-ss", strconv.FormatFloat(offset, 'f', -1, 64),
			"-t", strconv.Itoa(s.chunkDuration),
			"-acodec", "libmp3lame", "-ab", "64k",
			"-v", "quiet", out,
		)
		if err != nil {
			cleanup()
			return nil, noop, fmt.Errorf("split chunk at %.0fs: %w", offset, err)
		}
		chunks = append(chunks, chunk{path: out, offset: offset})
	}
	return chunks, cleanup, nil
}

// probeDuration returns the total duration of audioPath in seconds via ffprobe.
func (s *Service) probeDuration(ctx context.Context, audioPath string) (float64, error) {
	out, err := s.runner.Run(ctx, "ffprobe", "-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", audioPath,
	)
	if err != nil {
		return 0, fmt.Errorf("ffprobe duration: %w", err)
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe duration %q: %w", string(out), err)
	}
	return d, nil
}
