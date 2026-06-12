package summarizer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeChat records the last call and returns canned output.
type fakeChat struct {
	model, system, user string
	reply               string
	err                 error
}

func (f *fakeChat) Chat(_ context.Context, model, system, user string) (string, error) {
	f.model, f.system, f.user = model, system, user
	return f.reply, f.err
}

func TestSummarize_ModeSelectsPrompt(t *testing.T) {
	cases := []struct {
		mode       string
		wantPrompt string
	}{
		{"summary", SummaryPrompt},
		{"lecture", LecturePrompt},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			fc := &fakeChat{reply: "ok"}
			out, err := Summarize(context.Background(), fc, "[00:00] hi", "gpt-5-mini", tc.mode)
			require.NoError(t, err)

			assert.Equal(t, "ok", out)
			assert.Equal(t, tc.wantPrompt, fc.system, "system prompt must match the mode")
			assert.Equal(t, "gpt-5-mini", fc.model)
			assert.Contains(t, fc.user, "[00:00] hi", "transcript passed in user message")
		})
	}
}

func TestSummarize_ModelForwarded(t *testing.T) {
	fc := &fakeChat{reply: "ok"}
	_, err := Summarize(context.Background(), fc, "t", "gpt-5", "summary")
	require.NoError(t, err)
	assert.Equal(t, "gpt-5", fc.model)
}

func TestSummarize_UnknownMode(t *testing.T) {
	fc := &fakeChat{reply: "ok"}
	_, err := Summarize(context.Background(), fc, "t", "gpt-5-mini", "bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestSummarize_ClientError(t *testing.T) {
	fc := &fakeChat{err: assert.AnError}
	_, err := Summarize(context.Background(), fc, "t", "gpt-5-mini", "summary")
	require.Error(t, err)
}

func TestPromptsCoverBothModes(t *testing.T) {
	assert.Len(t, Prompts, 2)
	assert.Contains(t, Prompts, "summary")
	assert.Contains(t, Prompts, "lecture")
}
