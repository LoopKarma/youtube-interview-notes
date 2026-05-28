// Command youtube-helper downloads a YouTube video's audio, transcribes it with
// Whisper and renders a structured markdown summary or lecture conspect.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jessevdk/go-flags"
	"github.com/joho/godotenv"

	"github.com/molov/youtube-helper/internal/app"
	"github.com/molov/youtube-helper/internal/downloader"
	"github.com/molov/youtube-helper/internal/openai"
	"github.com/molov/youtube-helper/internal/runner"
	"github.com/molov/youtube-helper/internal/summarizer"
	"github.com/molov/youtube-helper/internal/transcriber"
)

// options mirrors the CLI flags one-to-one (see the README options table).
type options struct {
	Output    string `short:"o" long:"output" default:"." description:"Output directory for the markdown file."`
	Model     string `short:"m" long:"model" default:"gpt-5-mini" description:"OpenAI model for summarization."`
	Mode      string `long:"mode" default:"summary" choice:"summary" choice:"lecture" description:"Output style: summary (Q&A digest) or lecture (detailed conspect)."`
	MaxChunks int    `long:"max-chunks" default:"0" description:"Max number of 10-min audio chunks to transcribe (0 = no cap)."`
	Args      struct {
		URL string `positional-arg-name:"URL" required:"yes"`
	} `positional-args:"yes"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var opts options
	if _, err := flags.Parse(&opts); err != nil {
		// go-flags already printed help/usage; exit cleanly on --help.
		if flags.WroteHelp(err) {
			return nil
		}
		return err
	}

	_ = godotenv.Load()
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is not set (see .env.example)")
	}

	cacheDir, err := cacheDir()
	if err != nil {
		return err
	}

	client := openai.New(apiKey)
	application := &app.App{
		Downloader:  downloader.New(cacheDir, runner.Exec),
		Transcriber: transcriber.New(client, cacheDir, runner.Exec),
		Summarizer:  summarizerFunc{client: client},
		Out:         os.Stdout,
	}

	_, err = application.Run(context.Background(), app.Options{
		URL:       opts.Args.URL,
		Output:    opts.Output,
		Model:     opts.Model,
		Mode:      opts.Mode,
		MaxChunks: opts.MaxChunks,
	})
	return err
}

// summarizerFunc adapts a summarizer.ChatClient into the app.Summarizer
// interface by binding it to the package-level Summarize function.
type summarizerFunc struct {
	client summarizer.ChatClient
}

func (s summarizerFunc) Summarize(ctx context.Context, transcript, model, mode string) (string, error) {
	return summarizer.Summarize(ctx, s.client, transcript, model, mode)
}

// cacheDir returns ~/.cache/youtube-helper, matching the Python tool's layout.
func cacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".cache", "youtube-helper"), nil
}
