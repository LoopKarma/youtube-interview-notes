// Package openai is a minimal HTTP client for the two OpenAI endpoints this
// app needs: audio transcription (Whisper) and chat completions. It is kept
// deliberately small and its BaseURL is configurable so the whole client can be
// exercised against an httptest server with no network access.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/molov/youtube-helper/internal/transcriber"
)

// DefaultBaseURL is the public OpenAI API root.
const DefaultBaseURL = "https://api.openai.com/v1"

// Client talks to the OpenAI REST API.
type Client struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

// New returns a Client for the given API key using the default base URL and
// http.Client. Override BaseURL/HTTP on the returned value for tests.
func New(apiKey string) *Client {
	return &Client{
		APIKey:  apiKey,
		BaseURL: DefaultBaseURL,
		HTTP:    http.DefaultClient,
	}
}

// baseURL returns the configured base URL or the default.
func (c *Client) baseURL() string {
	if c.BaseURL == "" {
		return DefaultBaseURL
	}
	return c.BaseURL
}

// httpClient returns the configured http.Client or the default.
func (c *Client) httpClient() *http.Client {
	if c.HTTP == nil {
		return http.DefaultClient
	}
	return c.HTTP
}

// transcriptionResponse mirrors Whisper's verbose_json payload.
type transcriptionResponse struct {
	Segments []transcriber.Segment `json:"segments"`
}

// TranscribeFile uploads path to the transcription endpoint and returns the
// recognized segments. It satisfies transcriber.AudioTranscriber.
func (c *Client) TranscribeFile(ctx context.Context, path string) ([]transcriber.Segment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open audio chunk: %w", err)
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("build upload: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, fmt.Errorf("copy audio: %w", err)
	}
	for k, v := range map[string]string{
		"model":                   "whisper-1",
		"response_format":         "verbose_json",
		"timestamp_granularities": "segment",
	} {
		if err := mw.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("write field %s: %w", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close upload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/audio/transcriptions", &body)
	if err != nil {
		return nil, fmt.Errorf("build transcription request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	var out transcriptionResponse
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return out.Segments, nil
}

// chatRequest is the chat completions request payload.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the subset of the chat completions response we read.
type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Chat sends a system+user message pair and returns the assistant reply. It
// satisfies summarizer.ChatClient.
func (c *Client) Chat(ctx context.Context, model, system, user string, temperature float64) (string, error) {
	payload, err := json.Marshal(chatRequest{
		Model:       model,
		Temperature: temperature,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build chat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	var out chatResponse
	if err := c.do(req, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("chat completion returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}

// do executes req, checks the status code and decodes a JSON body into v.
func (c *Client) do(req *http.Request, v any) error {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
