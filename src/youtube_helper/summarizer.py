from openai import OpenAI

SUMMARY_PROMPT = """\
You are a video content analyst. Given a timestamped transcript, produce a structured \
markdown summary following this EXACT format. Do not include a top-level heading — \
start directly with sections.

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
"""

LECTURE_PROMPT = """\
You are an expert note-taker. Given a timestamped transcript of a lecture, produce a \
detailed conspect (structured lecture notes) in markdown. Do not include a top-level \
heading — start directly with sections. Preserve technical accuracy: definitions, \
formulas, code, names, and numbers must be faithful to the source.
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
List every question raised during this part — by the audience, an interviewer, or the \
lecturer themselves (rhetorical or self-posed). Reproduce the FULL question as asked, \
not a paraphrase. If the question was answered in the video, include the answer; \
otherwise mark it unanswered. Omit this subsection entirely if no questions were asked \
in this part.

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
- For "Questions Asked": capture the actual question wording, include who asked if \
clear from context (e.g. "audience member", "interviewer", "lecturer rhetorically"), \
and never invent questions that were not asked
"""

PROMPTS = {
    "summary": SUMMARY_PROMPT,
    "lecture": LECTURE_PROMPT,
}


def summarize_transcript(
    transcript_text: str,
    client: OpenAI,
    model: str = "gpt-4.1",
    mode: str = "summary",
) -> str:
    """Send transcript to GPT and get structured markdown output for the given mode."""
    response = client.chat.completions.create(
        model=model,
        messages=[
            {"role": "system", "content": PROMPTS[mode]},
            {
                "role": "user",
                "content": f"Here is the timestamped transcript:\n\n{transcript_text}",
            },
        ],
        temperature=0.3,
    )

    return response.choices[0].message.content
