---
name: visual-qa
description: >
  Compare a reference design image against a generated PDF, identify visual discrepancies,
  and iteratively fix the PDF until it closely matches the reference. Use this skill when
  the user wants a PDF to match a screenshot, mockup, or Figma export; asks whether a PDF
  "looks right"; provides both an image and a PDF and wants differences called out; or asks
  for visual QA, styling fixes, or repeated comparison passes. This skill is for visual
  fidelity, not content accuracy.
---

# Visual QA

## Overview

Compare a reference image to a candidate PDF page by page. Audit visual design, report gaps,
fix the PDF, and repeat until the result is difficult to distinguish from the reference.

Treat this as design QA. Matching content or section order is necessary but not sufficient.

## Required Inputs

You need:

1. A reference image: screenshot, Figma export, or mockup.
2. A candidate PDF: uploaded by the user or generated from the repo.

Check attached files first. If files are not obvious in the conversation, inspect
`/mnt/user-data/uploads/`.

## Workflow

### 1. Convert the PDF to page images

Resolve `scripts/pdf_to_images.py` relative to the skill directory and use it instead of
rewriting the conversion step.

```bash
python scripts/pdf_to_images.py /path/to/candidate.pdf --output-dir /tmp/visual-qa
```

If the helper script cannot run because of missing dependencies, fall back to a one-off
environment-specific conversion command and state the limitation.

### 2. Inspect the reference and candidate pages visually

Use `view_image` on the reference image and each rendered PDF page. Compare them side by
side when possible.

Do not stop after a quick scan. Work through the full checklist below.

### 3. Score every checklist section

Use this scale:

- `5`: indistinguishable or effectively pixel-perfect
- `4`: very close, minor tweaks needed
- `3`: noticeably different, same general idea
- `2`: wrong treatment or poor match
- `1`: missing or completely wrong

## Audit Checklist

### A. Page-Level Layout

- Page size and orientation
- Margins on all sides
- Overall whitespace and content density

### B. Header or Title Banner

- Background color
- Height and vertical padding
- Title font size, weight, color, and alignment
- Border, shadow, or divider treatment
- Logo placement, size, and spacing

### C. Section Headings

- Font family, size, weight, and color
- Spacing above and below
- Underlines, borders, or decorative rules
- Numbering or bullet treatment

### D. Body Text

- Font family, size, weight, and color
- Line height
- Paragraph spacing

### E. Tables

- Header background color
- Header text color, weight, and alignment
- Cell borders: color, weight, and visible edges
- Cell padding
- Row height
- Alternating row fills
- Column width proportions
- Text alignment per column

### F. Special Text Treatments

- Highlight fills
- Bold and italic usage
- Colored text
- Links or other special formatting

### G. Footer

- Background color
- Text content, alignment, and font treatment
- Separator line or border
- Page number position and format

### H. Iconography and Decorative Elements

- Badges, circled numbers, or callouts
- Separator lines
- Background shading
- Rounded corners, shadows, or other effects

### I. Spacing and Rhythm

- Vertical spacing between sections
- Consistency of spacing throughout
- Indentation depth
- Alignment grid across sections

## Gap Report Format

Report findings in a compact, structured format:

```text
VISUAL QA REPORT
================

Overall Score: 2.8 / 5.0

Critical Gaps (score 1-2):
- B. Title Banner: Score 1 — PDF uses black text on white; reference uses white text on a green full-width banner.
- E. Table Headers: Score 2 — PDF header fill is light gray; reference uses a medium blue fill with white text.

Moderate Gaps (score 3):
- C. Section Headings: Score 3 — weight is close, but size is smaller and the bottom rule is missing.
- I. Spacing: Score 3 — sections are packed too tightly relative to the reference.

Minor Gaps (score 4):
- D. Body Text: Score 4 — font is close, but line height is slightly tight.

Passing (score 5):
- A. Page Layout: Score 5 — page size and orientation match.
```

Always describe what is visually different. Avoid vague statements such as "looks off."

## Fix Loop

1. Fix all critical gaps first.
2. Re-render the PDF to images.
3. Re-run the entire checklist, not just the changed areas.
4. Fix moderate gaps.
5. Re-render and compare again.
6. Address minor gaps.

Do at least two full comparison passes after making changes. The first pass often misses
spacing regressions or new styling issues.

## Extracting Design Values

Estimate concrete design values from the reference when exact specs are unavailable:

- Colors: estimate approximate hex values, not just color names.
- Font sizes: estimate relative scale from known page elements.
- Spacing: estimate in points or pixels based on nearby rows, padding, and margins.
- Proportions: note ratios such as column widths or banner height.

If the user can provide Figma values, exact hex colors, fonts, or spacing tokens, ask for
them instead of guessing.

## Failure Modes

Watch for these mistakes:

- Declaring victory because the structure matches while the styling still differs.
- Ignoring spacing after fixing color and typography.
- Re-checking only the modified area and missing regressions elsewhere.
- Using vague color labels instead of approximate RGB or hex descriptions.
- Stopping after one comparison pass.

## Stop Conditions

Stop only when all of the following are true:

- Every checklist section scores `4` or higher.
- The overall score is `4.0` or higher.
- You completed at least two full comparison passes after changes.
- You explicitly checked for regressions after the latest fixes.
