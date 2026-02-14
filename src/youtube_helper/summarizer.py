from openai import OpenAI

SYSTEM_PROMPT = """\
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


def summarize_transcript(
    transcript_text: str, client: OpenAI, model: str = "gpt-4.1"
) -> str:
    """Send transcript to GPT and get structured markdown summary."""
    response = client.chat.completions.create(
        model=model,
        messages=[
            {"role": "system", "content": SYSTEM_PROMPT},
            {
                "role": "user",
                "content": f"Here is the timestamped transcript:\n\n{transcript_text}",
            },
        ],
        temperature=0.3,
    )

    return response.choices[0].message.content
