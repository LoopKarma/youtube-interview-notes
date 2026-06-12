// Package summarizer turns a timestamped transcript into structured markdown by
// prompting a chat model. Two output styles are supported: a Q&A "summary" and
// a detailed "lecture" conspect.
package summarizer

import (
	"context"
	"fmt"
)

// SummaryPrompt drives the Q&A digest output style.
const SummaryPrompt = `You are a video content analyst. Given a timestamped transcript, produce a structured markdown summary following this EXACT format. Do not include a top-level heading — start directly with sections.

For each logical section of the video:

---

## Section N: [Descriptive Section Title]
**Timecodes:** MM:SS - MM:SS

### Questions & Answers
**Q: [Key question addressed in this section]**
A: [Detailed answer summary]

**Q: [Another question if applicable]**
A: [Detailed answer summary]

### Key Topics for Research
- Topic 1
- Topic 2
- Topic 3

Rules:
- Use the timecodes from the transcript to determine section boundaries
- Each section should cover a coherent topic or theme
- Questions should capture what the speaker is explaining or discussing
- Answers should be detailed but concise summaries of what was said
- Topics should be specific enough to be useful search terms
- Use the exact timecode format from the transcript (MM:SS or HH:MM:SS)
`

// LecturePrompt drives the detailed conspect output style.
const LecturePrompt = `You are an expert note-taker. Given a timestamped transcript of a lecture, produce a detailed conspect (structured lecture notes) in markdown. Do not include a top-level heading — start directly with sections. Preserve technical accuracy: definitions, formulas, code, names, and numbers must be faithful to the source.
Catch abbreviaions and specific industrie related terms that would be great to explain.

For each logical part of the lecture:

---

## Part N: [Topic Title]
**Timecodes:** MM:SS - MM:SS

### Overview
One short paragraph stating what this part teaches.

### Key Concepts
- **[Term]** — definition / explanation
- **[Term]** — definition / explanation

### Notes
Detailed bullet-point notes following the lecturer's flow. Capture:
- Claims, arguments, derivations (in order)
- Step-by-step reasoning when present
- Examples and analogies (label them: *Example:* ...)
- Code, formulas, or commands — use fenced code blocks or inline code
- Caveats, exceptions, "common mistakes" the lecturer warns about

### Questions Asked
List every question raised during this part — by the audience, an interviewer, or the lecturer themselves (rhetorical or self-posed). Reproduce the FULL question as asked, not a paraphrase. If the question was answered in the video, include the answer; otherwise mark it unanswered. Omit this subsection entirely if no questions were asked in this part.

- **Q ([MM:SS]):** [Full question, verbatim or as close as the transcript allows]
  **A:** [Answer given in the video, faithful to what was said]
- **Q ([MM:SS]):** [Full question]
  **A:** *Not answered in the video.*

### Takeaways
- 2-5 bullets the listener must remember from this part

Rules:
- Use timecodes from the transcript to determine part boundaries
- Each part = one coherent subject; do not over-fragment
- Prefer faithful reproduction over compression — this is a conspect, not a summary
- Keep terminology exactly as the lecturer uses it
- Use the exact timecode format from the transcript (MM:SS or HH:MM:SS)
- For "Questions Asked": capture the actual question wording, include who asked if clear from context (e.g. "audience member", "interviewer", "lecturer rhetorically"), and never invent questions that were not asked
`

// Prompts maps each supported mode to its system prompt.
var Prompts = map[string]string{
	"summary": SummaryPrompt,
	"lecture": LecturePrompt,
}

// ChatClient sends a system+user prompt pair to a chat model and returns the
// assistant's reply. The OpenAI client implements this; tests supply a fake.
type ChatClient interface {
	Chat(ctx context.Context, model, system, user string) (string, error)
}

// Summarize sends the transcript to the model and returns structured markdown
// for the given mode. It errors on an unknown mode rather than guessing.
func Summarize(ctx context.Context, client ChatClient, transcript, model, mode string) (string, error) {
	prompt, ok := Prompts[mode]
	if !ok {
		return "", fmt.Errorf("unknown mode %q", mode)
	}
	user := fmt.Sprintf("Here is the timestamped transcript:\n\n%s", transcript)
	out, err := client.Chat(ctx, model, prompt, user)
	if err != nil {
		return "", fmt.Errorf("chat completion: %w", err)
	}
	return out, nil
}
