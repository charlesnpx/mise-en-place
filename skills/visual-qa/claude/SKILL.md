---
name: visual-qa
description: >
  Compare a reference design image (screenshot, Figma export, mockup) against a generated PDF
  to find and fix visual discrepancies. Use this skill whenever the user asks to match a PDF
  to a design, compare a PDF against a screenshot or mockup, do visual QA on a generated
  document, check if a PDF "looks like" an image, or iteratively fix a PDF until it matches
  a reference. Also trigger when the user provides both an image and a PDF and asks about
  differences, or says things like "make the PDF match this", "why doesn't this look right",
  "compare these two", or "iterate until it matches". This skill is about VISUAL fidelity —
  not content accuracy.
---

# Visual QA: Image-to-PDF Comparison

## Purpose

You have a reference image (the "target") and a generated PDF (the "candidate"). Your job is
to systematically identify every visual difference and fix them iteratively until a designer
would struggle to tell them apart.

**Critical mindset**: You are comparing VISUAL DESIGN, not content/structure. Two documents
can have identical content and completely different visual treatments. "Same sections in the
same order" is NOT "visually matching."

## Inputs

You need two things:
1. **Reference image** — a screenshot, Figma export, or mockup (PNG/JPG). This may arrive
   before this skill is triggered (check the conversation history and uploaded files).
2. **Candidate PDF** — the generated PDF to compare. This may also already be uploaded, or
   may need to be generated first.

Check `/mnt/user-data/uploads/` for both files. The image will be visible in your context
window. For the PDF, convert it to images so you can compare visually.

## Workflow

### Step 1: Convert the PDF to Images

```bash
pip install pdf2image --break-system-packages -q
```

```python
from pdf2image import convert_from_path

images = convert_from_path('/path/to/candidate.pdf', dpi=200)
for i, img in enumerate(images):
    img.save(f'/home/claude/candidate_page_{i+1}.png', 'PNG')
```

Then use the `view` tool to look at each candidate page image alongside the reference image.

### Step 2: Run the Element-by-Element Audit

Go through EVERY item on this checklist. For each item, assign a score from 1-5:
- **5** = Pixel-perfect or indistinguishable
- **4** = Very close, minor tweaks needed (e.g., 2px padding off)
- **3** = Noticeably different but same concept (e.g., right shade of blue but wrong weight)
- **2** = Wrong approach (e.g., black header instead of green banner)
- **1** = Missing or completely wrong

#### The Checklist

**A. Page-Level Layout**
- [ ] Page size and orientation (Letter/A4, portrait/landscape)
- [ ] Margins (top, bottom, left, right)
- [ ] Overall content density and whitespace distribution

**B. Header / Title Banner**
- [ ] Background color (compare exact shade, not just "it's green")
- [ ] Height / vertical padding
- [ ] Text content, font size, font weight, font color
- [ ] Text alignment (left/center/right)
- [ ] Border or shadow effects
- [ ] Logo placement, size, and spacing

**C. Section Headings**
- [ ] Font family, size, weight, color
- [ ] Spacing above and below
- [ ] Any underlines, borders, or decorative elements
- [ ] Numbering or bullet style if present

**D. Body Text**
- [ ] Font family, size, weight, color
- [ ] Line height / leading
- [ ] Paragraph spacing

**E. Tables**
- [ ] Header row background color
- [ ] Header row text color, weight, alignment
- [ ] Cell borders (color, weight, which sides)
- [ ] Cell padding (internal spacing)
- [ ] Row height
- [ ] Alternating row colors if present
- [ ] Column widths (proportional match)
- [ ] Text alignment per column (left/center/right)

**F. Special Text Treatments**
- [ ] Highlighted text (background color, text color)
- [ ] Bold/italic usage
- [ ] Any colored text (match the exact color)
- [ ] Links or special formatting

**G. Footer**
- [ ] Background color
- [ ] Text content, alignment, font
- [ ] Borders or separator lines
- [ ] Page numbers (position, format)

**H. Iconography & Decorative Elements**
- [ ] Circled numbers or badges (size, color, border)
- [ ] Separator lines (weight, color, position)
- [ ] Background shading on sections
- [ ] Any rounded corners, shadows, or effects

**I. Spacing & Rhythm**
- [ ] Vertical spacing between sections
- [ ] Consistency of spacing throughout
- [ ] Indentation levels
- [ ] Alignment grid (do elements align vertically across sections?)

### Step 3: Report the Gaps

Present findings as a structured gap report. Example format:

```
VISUAL QA REPORT
================

Overall Score: 2.8 / 5.0

Critical Gaps (score 1-2):
- B. Title Banner: Score 1 — PDF has plain black text on white; reference has white
  text on #4CAF50 green banner, ~60px tall, full-width
- E. Table Headers: Score 2 — PDF uses light gray (#f0f0f0); reference uses
  steel blue (#4682B4) with white text

Moderate Gaps (score 3):
- C. Section Headings: Score 3 — Font weight is bold (correct) but size is 14pt
  vs reference ~18pt; missing the subtle bottom border
- I. Spacing: Score 3 — Sections are too tightly packed; reference has ~20px
  between sections, PDF has ~8px

Minor Gaps (score 4):
- D. Body Text: Score 4 — Correct font and size, but line-height is slightly
  tighter than reference

Passing (score 5):
- A. Page Layout: Score 5 — Correct page size and orientation
```

### Step 4: Fix and Re-Compare

1. Fix the **Critical Gaps first** (scores 1-2), then Moderate (3), then Minor (4).
2. After each round of fixes, re-convert the PDF to images and re-run the checklist.
3. **Do NOT declare success until ALL items score 4 or above.**
4. **Do NOT declare success after only one iteration** — always do at least two comparison
   passes. The first fix round often introduces new issues.

### Step 5: Extracting Design Values from the Reference

When you can see the reference image, extract approximate values:

- **Colors**: Describe what you see precisely. "Dark green" is not enough — estimate the
  hex value. Look at the hue, saturation, and brightness carefully. Forest green (#228B22)
  vs lime green (#32CD32) vs teal green (#008080) are very different.
- **Font sizes**: Estimate relative to the page. Title text taking up ~3% of page width
  per character suggests ~24-28pt. Body text at ~1.5% suggests ~10-12pt.
- **Spacing**: Estimate in points or mm relative to known elements (e.g., "the gap between
  the header and first section is approximately the same height as two table rows").
- **Proportions**: Note column width ratios (e.g., "first column is ~15% of table width,
  second column is ~25%").

If the user has access to the original Figma/design file, ASK for exact hex values, font
specs, and measurements. Don't guess when you can get real values.

## Common Failure Modes (watch for these)

1. **"Close enough" bias** — LLMs tend to declare victory too early. If you catch yourself
   thinking "this is basically the same," stop and re-examine each checklist item.
2. **Structural match ≠ visual match** — Having the same sections and tables is necessary
   but NOT sufficient. The visual treatment (colors, fonts, spacing) is what this skill
   is about.
3. **Color blindness** — Be especially careful with greens vs grays, blues vs purples,
   and similar hues. When in doubt, describe the color in terms of RGB components.
4. **Spacing amnesia** — After fixing colors and fonts, spacing often gets forgotten.
   Always check spacing in the final pass.
5. **Regression** — Fixing one thing often breaks another. Always do a full re-check
   after changes, not just verifying the thing you fixed.

## When to Stop

You're done when:
- All checklist items score 4 or above
- You've done at least 2 full comparison passes after changes
- You've explicitly confirmed no regressions from previous fixes
- The overall score is 4.0 or above

Present the final PDF to the user with the final gap report showing all scores.
