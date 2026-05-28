package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newClient points a Client at a test server.
func newClient(srv *httptest.Server) *Client {
	c := New("test-key")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()
	return c
}

func TestTranscribeFile(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "chunk.mp3")
	require.NoError(t, os.WriteFile(audio, []byte("audio-bytes"), 0o644))

	var gotAuth, gotModel, gotFormat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/audio/transcriptions", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, r.ParseMultipartForm(1<<20))
		gotModel = r.FormValue("model")
		gotFormat = r.FormValue("response_format")
		_, _, err := r.FormFile("file")
		require.NoError(t, err)
		w.Write([]byte(`{"segments":[{"start":0,"end":1.5,"text":"hello"}]}`))
	}))
	defer srv.Close()

	segs, err := newClient(srv).TranscribeFile(context.Background(), audio)
	require.NoError(t, err)

	require.Len(t, segs, 1)
	assert.Equal(t, "hello", segs[0].Text)
	assert.Equal(t, 1.5, segs[0].End)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, "whisper-1", gotModel)
	assert.Equal(t, "verbose_json", gotFormat)
}

func TestChat(t *testing.T) {
	var gotAuth string
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"# result"}}]}`))
	}))
	defer srv.Close()

	out, err := newClient(srv).Chat(context.Background(), "gpt-5-mini", "sys", "usr", 0.3)
	require.NoError(t, err)

	assert.Equal(t, "# result", out)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, "gpt-5-mini", gotBody.Model)
	assert.Equal(t, 0.3, gotBody.Temperature)
	require.Len(t, gotBody.Messages, 2)
	assert.Equal(t, "system", gotBody.Messages[0].Role)
	assert.Equal(t, "sys", gotBody.Messages[0].Content)
	assert.Equal(t, "user", gotBody.Messages[1].Role)
	assert.Equal(t, "usr", gotBody.Messages[1].Content)
}

func TestChat_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	_, err := newClient(srv).Chat(context.Background(), "m", "s", "u", 0.3)
	require.Error(t, err)
}

func TestDo_NonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()
	_, err := newClient(srv).Chat(context.Background(), "m", "s", "u", 0.3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestTranscribeFile_MissingFile(t *testing.T) {
	_, err := New("k").TranscribeFile(context.Background(), "/no/such/file.mp3")
	require.Error(t, err)
}
