---
name: humanizer
description: >
  Rewrite text so it sounds less AI-generated and more naturally human.
  Use when the user asks to humanize writing, remove AI writing patterns,
  make something sound less robotic, less corporate, less ChatGPT-like, or
  more natural. Trigger on: humanizer, sound more human, less AI, remove AI
  tells, de-corporatize, make this read naturally.
---

# Humanizer

Rewrite text to remove common AI writing tells while preserving meaning, tone, and required structure.

## Hard rule

No em dashes in the final output. Replace them with commas, periods, semicolons, colons, parentheses, or a sentence rewrite.

## Use this skill for

- PR descriptions, emails, docs, comments, proposals, and summaries that sound too AI-generated
- Text that feels stiff, overly polished, vague, promotional, or mechanically structured
- Requests to make writing sound more natural, direct, specific, or human-written

## Workflow

1. Read the source text and identify the intended tone and audience.
2. Preserve required structure, headings, checklist items, links, and factual content.
3. Remove common AI tells:
   - inflated significance language
   - promotional or brochure-like wording
   - vague attributions and weasel phrases
   - filler and excessive hedging
   - repetitive sentence rhythm
   - rule-of-three phrasing
   - synonym cycling
   - negative parallelisms like "not just X, but Y"
   - excessive bolding, emojis, title case headings, chatbot filler, and knowledge-cutoff disclaimers
   - overused AI vocabulary such as "crucial", "delve", "showcase", "underscores", "vibrant", "landscape", and similar phrasing
4. Prefer concrete nouns, specific verbs, and plain sentence constructions.
5. Add some voice when appropriate. Clean writing is not enough if it still sounds sterile.
6. Do a final anti-AI audit:
   - Briefly ask yourself: "What still makes this sound AI-generated?"
   - Remove the remaining tells.
7. Verify the final output contains no em dashes.

## Output guidance

Default output:
1. Revised text
2. Optional brief note listing the main changes, only if useful

If the user asks for a deeper pass, provide:
1. Draft rewrite
2. Brief bullets on remaining AI tells
3. Final rewrite

## Editing rules

- Keep the original meaning intact unless the user asks for substantive rewriting.
- Keep the requested format intact. If the user gives a template, do not break it.
- Do not invent facts, citations, examples, or quotes.
- Do not replace specificity with generic polish.
- When editing a file, modify only the requested text.
- When asked to write into a target file, save the rewritten output there.

## Style targets

Aim for writing that:
- sounds natural aloud
- varies rhythm without feeling random
- uses direct wording instead of inflated abstractions
- feels written by a person with judgment, not assembled from stock phrases
