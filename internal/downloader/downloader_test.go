package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/molov/youtube-helper/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		name string
		secs int
		want string
	}{
		{"zero", 0, "00:00"},
		{"seconds only", 9, "00:09"},
		{"minutes", 222, "03:42"},
		{"exactly one hour", 3600, "01:00:00"},
		{"hours minutes seconds", 3909, "01:05:09"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, FormatDuration(tc.secs))
		})
	}
}

func TestExtractVideoID(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"watch query", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"short link", "https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"embed path", "https://www.youtube.com/embed/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"with extra params", "https://youtu.be/dQw4w9WgXcQ?t=42", "dQw4w9WgXcQ"},
		{"no id", "https://example.com/video", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ExtractVideoID(tc.url))
		})
	}
}

// fakeRunner records calls and delegates to fn.
type fakeRunner struct {
	calls [][]string
	fn    func(name string, args []string) ([]byte, error)
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.fn(name, args)
}

const sampleJSON = `{"id":"dQw4w9WgXcQ","title":"Never Gonna Give You Up","duration":212}`

func TestDownload_CacheMiss(t *testing.T) {
	cache := t.TempDir()
	fr := &fakeRunner{fn: func(name string, args []string) ([]byte, error) {
		return []byte(sampleJSON), nil
	}}
	svc := New(cache, fr)

	info, err := svc.Download(context.Background(), "https://youtu.be/dQw4w9WgXcQ")
	require.NoError(t, err)

	assert.Equal(t, "Never Gonna Give You Up", info.Title)
	assert.Equal(t, 212, info.Duration)
	assert.Equal(t, "dQw4w9WgXcQ", info.VideoID)
	assert.Equal(t, filepath.Join(cache, "audio", "dQw4w9WgXcQ.mp3"), info.AudioPath)

	require.Len(t, fr.calls, 1)
	joined := strings.Join(fr.calls[0], " ")
	assert.Contains(t, joined, "yt-dlp")
	assert.Contains(t, joined, "--extract-audio")
	assert.Contains(t, joined, "mp3")
	assert.NotContains(t, joined, "--skip-download")
}

func TestDownload_CacheHit(t *testing.T) {
	cache := t.TempDir()
	audioDir := filepath.Join(cache, "audio")
	require.NoError(t, os.MkdirAll(audioDir, 0o755))
	cached := filepath.Join(audioDir, "dQw4w9WgXcQ.mp3")
	require.NoError(t, os.WriteFile(cached, []byte("fake-audio"), 0o644))

	fr := &fakeRunner{fn: func(name string, args []string) ([]byte, error) {
		return []byte(sampleJSON), nil
	}}
	svc := New(cache, fr)

	info, err := svc.Download(context.Background(), "https://youtu.be/dQw4w9WgXcQ")
	require.NoError(t, err)

	assert.Equal(t, cached, info.AudioPath)
	assert.Equal(t, "Never Gonna Give You Up", info.Title)

	// Cache hit must only fetch metadata, never re-download.
	require.Len(t, fr.calls, 1)
	joined := strings.Join(fr.calls[0], " ")
	assert.Contains(t, joined, "--skip-download")
	assert.NotContains(t, joined, "--extract-audio")
}

func TestDownload_RunnerError(t *testing.T) {
	fr := &fakeRunner{fn: func(name string, args []string) ([]byte, error) {
		return nil, assert.AnError
	}}
	svc := New(t.TempDir(), fr)
	_, err := svc.Download(context.Background(), "https://youtu.be/dQw4w9WgXcQ")
	require.Error(t, err)
}

func TestParseInfo_DefaultTitle(t *testing.T) {
	info, err := parseInfo([]byte(`{"id":"abc","duration":10}`))
	require.NoError(t, err)
	assert.Equal(t, "Unknown", info.Title)
}

var _ runner.Runner = (*fakeRunner)(nil)
