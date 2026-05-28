package transcriber

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatTimecode(t *testing.T) {
	cases := []struct {
		secs float64
		want string
	}{
		{0, "00:00"},
		{9.4, "00:09"},
		{222, "03:42"},
		{3600, "01:00:00"},
		{3909.9, "01:05:09"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, FormatTimecode(tc.secs))
		})
	}
}

func TestSegmentsToText(t *testing.T) {
	segs := []Segment{
		{Start: 0, End: 2, Text: "hello"},
		{Start: 65, End: 70, Text: "world"},
	}
	assert.Equal(t, "[00:00] hello\n[01:05] world", SegmentsToText(segs))
	assert.Equal(t, "", SegmentsToText(nil))
}

// fakeTranscriber returns canned segments and records each file it sees.
type fakeTranscriber struct {
	segs  []Segment
	files []string
	err   error
}

func (f *fakeTranscriber) TranscribeFile(_ context.Context, path string) ([]Segment, error) {
	f.files = append(f.files, path)
	if f.err != nil {
		return nil, f.err
	}
	return f.segs, nil
}

// fakeRunner simulates ffprobe (returns duration) and ffmpeg (creates a chunk).
type fakeRunner struct {
	duration string
	calls    [][]string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	switch name {
	case "ffprobe":
		return []byte(f.duration + "\n"), nil
	case "ffmpeg":
		out := args[len(args)-1] // chunk path is the final argument
		if err := os.WriteFile(out, []byte("chunk"), 0o644); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return nil, nil
}

func writeAudio(t *testing.T, dir string, size int) string {
	t.Helper()
	p := filepath.Join(dir, "audio.mp3")
	require.NoError(t, os.WriteFile(p, make([]byte, size), 0o644))
	return p
}

func TestTranscribe_SingleChunk(t *testing.T) {
	dir := t.TempDir()
	audio := writeAudio(t, dir, 100) // under the limit → one chunk, offset 0

	ft := &fakeTranscriber{segs: []Segment{{Start: 1, End: 2, Text: "  spaced  "}}}
	svc := New(ft, dir, &fakeRunner{})

	segs, err := svc.Transcribe(context.Background(), audio, "vid1", 0)
	require.NoError(t, err)

	require.Len(t, segs, 1)
	assert.Equal(t, "spaced", segs[0].Text, "text should be trimmed")
	assert.Equal(t, []string{audio}, ft.files, "small file transcribed directly, no chunking")

	// Cache file written.
	_, err = os.Stat(svc.cachePath("vid1"))
	require.NoError(t, err)
}

func TestTranscribe_CacheHit(t *testing.T) {
	dir := t.TempDir()
	audio := writeAudio(t, dir, 100)

	// Pre-seed the cache; the client must not be called.
	svc := New(&fakeTranscriber{err: assert.AnError}, dir, &fakeRunner{})
	cached := []Segment{{Start: 5, End: 6, Text: "cached"}}
	require.NoError(t, svc.saveCache("vid1", cached))

	segs, err := svc.Transcribe(context.Background(), audio, "vid1", 0)
	require.NoError(t, err)
	assert.Equal(t, cached, segs)
}

func TestTranscribe_Chunking_OffsetsApplied(t *testing.T) {
	dir := t.TempDir()
	audio := writeAudio(t, dir, 1000)

	fr := &fakeRunner{duration: "1500"} // 1500s @ 600s chunks → 3 chunks
	ft := &fakeTranscriber{segs: []Segment{{Start: 10, End: 20, Text: "x"}}}
	svc := New(ft, dir, fr)
	svc.maxFileSize = 10 // force chunking

	segs, err := svc.Transcribe(context.Background(), audio, "vid2", 0)
	require.NoError(t, err)

	require.Len(t, segs, 3, "three chunks, one segment each")
	// Offsets: 0, 600, 1200 added to the segment start of 10.
	assert.Equal(t, 10.0, segs[0].Start)
	assert.Equal(t, 610.0, segs[1].Start)
	assert.Equal(t, 1210.0, segs[2].Start)

	// ffprobe called once + ffmpeg called per chunk.
	var ffmpeg int
	for _, c := range fr.calls {
		if c[0] == "ffmpeg" {
			ffmpeg++
		}
	}
	assert.Equal(t, 3, ffmpeg)
}

func TestTranscribe_MaxChunks(t *testing.T) {
	dir := t.TempDir()
	audio := writeAudio(t, dir, 1000)

	fr := &fakeRunner{duration: "1500"} // would be 3 chunks
	ft := &fakeTranscriber{segs: []Segment{{Start: 0, End: 1, Text: "x"}}}
	svc := New(ft, dir, fr)
	svc.maxFileSize = 10

	segs, err := svc.Transcribe(context.Background(), audio, "vid3", 1) // cap at 1
	require.NoError(t, err)

	assert.Len(t, segs, 1)
	assert.Len(t, ft.files, 1, "only one chunk transcribed due to max-chunks=1")
}

func TestTranscribe_ClientError(t *testing.T) {
	dir := t.TempDir()
	audio := writeAudio(t, dir, 100)
	svc := New(&fakeTranscriber{err: assert.AnError}, dir, &fakeRunner{})
	_, err := svc.Transcribe(context.Background(), audio, "vid4", 0)
	require.Error(t, err)
}

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	svc := New(nil, dir, nil)
	want := []Segment{{Start: 1.5, End: 2.5, Text: "hi"}}
	require.NoError(t, svc.saveCache("v", want))

	// On-disk format is the same JSON shape the Python tool wrote.
	raw, err := os.ReadFile(svc.cachePath("v"))
	require.NoError(t, err)
	var got []Segment
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, want, got)
}

// guard: probeDuration parses ffprobe output with surrounding whitespace.
func TestProbeDuration(t *testing.T) {
	fr := &fakeRunner{duration: "  42.5  "}
	svc := New(nil, t.TempDir(), fr)
	d, err := svc.probeDuration(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, 42.5, d)
}
