---
name: gemini-vision
description: >
  Use Gemini CLI in headless prompt mode for computer-vision and visual
  interpretation tasks. Trigger when the user asks to read, summarize,
  describe, compare, inspect, interpret, or extract visual information from an
  image, screenshot, diagram, chart, UI mockup, photo, scanned visual, or other
  image-like file, especially when they explicitly mention Gemini.
---

# Gemini Vision

Use Gemini CLI as an external vision interpreter for image-centric tasks.
Delegate visual understanding to Gemini, then report the result faithfully.

## Workflow

1. Resolve the image path or paths from the user's request, attachments, or local files.
2. If the image may be private, sensitive, or regulated and the user did not explicitly ask to use Gemini, ask before sending it to Gemini.
3. Run the helper script from this skill directory:

```bash
python scripts/gemini_vision.py --image /path/to/image.png --prompt "Summarize this image." --model pro
```

4. Use the output as Gemini's interpretation. If the image is ambiguous, say that Gemini's answer is an interpretation rather than a confirmed fact.
5. If Gemini fails because it is missing, unauthenticated, blocked, or out of quota, report that clearly and fall back to local image inspection if available.

## Command Rules

- Prefer the helper script instead of hand-writing the command.
- If you must call Gemini directly, use `gemini -p "..."`; do not use bare `gemini "..."`, because that starts or continues an interactive session in a TTY.
- Use `@{/absolute/path/to/image}` injection for image files in Gemini prompts.
- Keep Gemini in read-only/headless mode with `--approval-mode plan`, `--skip-trust`, and `--output-format json` when calling it directly.
- Do not use Gemini for ordinary code, text, or repository work just because this skill is available.

## Examples

Summarize one image:

```bash
python scripts/gemini_vision.py \
  --image /tmp/screenshot.png \
  --prompt "Describe the UI state and call out any visible errors."
```

Compare two images:

```bash
python scripts/gemini_vision.py \
  --image /tmp/reference.png \
  --image /tmp/candidate.png \
  --prompt "Compare these images and list the most important visual differences."
```

Extract structured details:

```bash
python scripts/gemini_vision.py \
  --image /tmp/chart.png \
  --prompt "Read the chart. Return the title, axes, trend, and any visible values."
```
