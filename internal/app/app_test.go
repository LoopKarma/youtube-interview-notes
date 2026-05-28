package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/molov/youtube-helper/internal/downloader"
	"github.com/molov/youtube-helper/internal/transcriber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"reserved chars stripped", `a<b>c:d"e/f\g|h?i*j`, "abcdefghij"},
		{"whitespace to underscore", "hello   world", "hello_world"},
		{"trim then collapse", "  spaced  out  ", "spaced_out"},
		{"plain", "Normal Title", "Normal_Title"},
		{"length cap", strings.Repeat("x", 250), strings.Repeat("x", 200)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, SanitizeFilename(tc.in))
		})
	}
}

// fakeDownloader / fakeTranscriber / fakeSummarizer let us drive the pipeline
// without touching the network or external tools.
type fakeDownloader struct {
	info downloader.VideoInfo
	err  error
}

func (f *fakeDownloader) Download(_ context.Context, url string) (downloader.VideoInfo, error) {
	f.info.URL = url
	return f.info, f.err
}

type fakeTranscriber struct {
	segs         []transcriber.Segment
	gotAudioPath string
	gotVideoID   string
	gotMaxChunks int
	err          error
}

func (f *fakeTranscriber) Transcribe(_ context.Context, audioPath, videoID string, maxChunks int) ([]transcriber.Segment, error) {
	f.gotAudioPath, f.gotVideoID, f.gotMaxChunks = audioPath, videoID, maxChunks
	return f.segs, f.err
}

type fakeSummarizer struct {
	gotTranscript, gotModel, gotMode string
	reply                            string
	err                              error
}

func (f *fakeSummarizer) Summarize(_ context.Context, transcript, model, mode string) (string, error) {
	f.gotTranscript, f.gotModel, f.gotMode = transcript, model, mode
	return f.reply, f.err
}

// newApp builds an App with default-happy fakes and returns the parts tests
// inspect.
func newApp() (*App, *fakeDownloader, *fakeTranscriber, *fakeSummarizer, *bytes.Buffer) {
	fd := &fakeDownloader{info: downloader.VideoInfo{
		Title:     "My Talk: Part 1",
		Duration:  3661, // 01:01:01
		AudioPath: "/cache/audio/vid.mp3",
		VideoID:   "vid12345678",
	}}
	ft := &fakeTranscriber{segs: []transcriber.Segment{{Start: 0, End: 2, Text: "hi"}}}
	fs := &fakeSummarizer{reply: "## body"}
	buf := &bytes.Buffer{}
	return &App{Downloader: fd, Transcriber: ft, Summarizer: fs, Out: buf}, fd, ft, fs, buf
}

func TestRun_SummaryMode(t *testing.T) {
	a, _, ft, fs, buf := newApp()
	out := t.TempDir()

	path, err := a.Run(context.Background(), Options{
		URL: "https://youtu.be/x", Output: out, Model: "gpt-5-mini", Mode: "summary",
	})
	require.NoError(t, err)

	// summary mode → no infix.
	assert.Equal(t, filepath.Join(out, "My_Talk_Part_1.md"), path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	got := string(content)
	assert.Contains(t, got, "# My Talk: Part 1")
	assert.Contains(t, got, "**URL:** https://youtu.be/x")
	assert.Contains(t, got, "**Duration:** 01:01:01")
	assert.Contains(t, got, "## body")

	// transcript built from segments was forwarded to the summarizer.
	assert.Equal(t, "[00:00] hi", fs.gotTranscript)
	assert.Equal(t, "summary", fs.gotMode)
	assert.Equal(t, "/cache/audio/vid.mp3", ft.gotAudioPath)
	assert.Equal(t, "vid12345678", ft.gotVideoID)

	assert.Contains(t, buf.String(), "Downloading audio")
	assert.Contains(t, buf.String(), "Output saved to:")
}

func TestRun_LectureMode(t *testing.T) {
	a, _, _, fs, _ := newApp()
	out := t.TempDir()

	path, err := a.Run(context.Background(), Options{
		URL: "u", Output: out, Model: "gpt-5-mini", Mode: "lecture",
	})
	require.NoError(t, err)

	// lecture mode → ".lecture" infix before .md.
	assert.Equal(t, filepath.Join(out, "My_Talk_Part_1.lecture.md"), path)
	assert.Equal(t, "lecture", fs.gotMode)
}

func TestRun_OutputDirCreatedNested(t *testing.T) {
	a, _, _, _, _ := newApp()
	out := filepath.Join(t.TempDir(), "deep", "nested")

	path, err := a.Run(context.Background(), Options{
		URL: "u", Output: out, Model: "m", Mode: "summary",
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(path, out))
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestRun_ModelParamForwarded(t *testing.T) {
	a, _, _, fs, _ := newApp()
	_, err := a.Run(context.Background(), Options{
		URL: "u", Output: t.TempDir(), Model: "gpt-5", Mode: "summary",
	})
	require.NoError(t, err)
	assert.Equal(t, "gpt-5", fs.gotModel)
}

func TestRun_MaxChunksParamForwarded(t *testing.T) {
	a, _, ft, _, _ := newApp()
	_, err := a.Run(context.Background(), Options{
		URL: "u", Output: t.TempDir(), Model: "m", Mode: "summary", MaxChunks: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, ft.gotMaxChunks)
}

func TestRun_StageErrorsPropagate(t *testing.T) {
	t.Run("download", func(t *testing.T) {
		a, fd, _, _, _ := newApp()
		fd.err = assert.AnError
		_, err := a.Run(context.Background(), Options{URL: "u", Output: t.TempDir(), Mode: "summary"})
		require.ErrorContains(t, err, "download")
	})
	t.Run("transcribe", func(t *testing.T) {
		a, _, ft, _, _ := newApp()
		ft.err = assert.AnError
		_, err := a.Run(context.Background(), Options{URL: "u", Output: t.TempDir(), Mode: "summary"})
		require.ErrorContains(t, err, "transcribe")
	})
	t.Run("summarize", func(t *testing.T) {
		a, _, _, fs, _ := newApp()
		fs.err = assert.AnError
		_, err := a.Run(context.Background(), Options{URL: "u", Output: t.TempDir(), Mode: "summary"})
		require.ErrorContains(t, err, "summarize")
	})
}
