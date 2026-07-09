# Scoring rubric (0–2 per dimension)

**For HUMAN evaluation of finished outputs — NOT runtime instructions.** The agent's runtime checklist is the binary quality-gate list in SKILL.md Step 5; do not load this file while drafting. Use this rubric to grade artifacts after the fact (e.g. when running `evals/prompts.md`) and to compare revisions.

Score each dimension 0, 1, or 2. Maximum 16.

| # | Dimension | 0 | 1 | 2 |
|---|---|---|---|---|
| 1 | **Answer-first** | Main point buried or absent; opens with background/methodology | Main point present early but hedged, split, or preceded by throat-clearing | Recommendation is the first thing a reader sees; identifiable in 10 seconds |
| 2 | **Audience framing** | Written in the author's interests or generic corporate language | Audience named but benefits not translated into their value frame | Every benefit phrased in the audience's decision language (cost, risk, speed, compliance, outcomes) |
| 3 | **Evidence legibility** | Claims unsupported, or supported only by author assertion ("I verified…") | Evidence referenced but not shown (named without the number/table/quote) | Every major claim carries proof the reader can inspect in the artifact |
| 4 | **Objection handling** | No objections addressed, or only strawmen | Objections listed but weakly stated or deflected rather than refuted | Hardest objections steel-manned and refuted (or honestly conceded with mitigation) |
| 5 | **Quality of the ask** | No ask, or a vague/irreversible "approve everything" ask | Ask present but missing scope, owner, metrics, or review point | Ask concrete, small, reversible where possible, with success/stop criteria |
| 6 | **Cold-reader integrity** | Session leakage: raw file paths, "as discussed", undefined internal names, first-person verification claims | Mostly self-contained; one or two unintroduced references | Fully self-contained; a reader with zero session access loses nothing |
| 7 | **Tone integrity** | Fake urgency, manipulative scarcity, or overclaiming | Honest but noticeably salesy or consulting-toned for the context | Confident, plain, evidence-led; mode matches the Step 0 gate (clarity-only tasks show no pitch apparatus) |
| 8 | **Structure fit** | Wrong skeleton for the format (topic slide titles, memo with no spine, hero that doesn't state the answer) | Skeleton mostly followed with gaps (missing objection slide, FAQ, or decision close) | Format adapter fully applied; titles-only read reconstructs the argument (decks) or equivalent |

## Interpreting scores

- **14–16** — ship it.
- **10–13** — revise the dimensions scoring ≤1; usually framing, evidence legibility, or the ask.
- **≤9** — structural failure; re-run the workflow from Step 1 rather than editing prose.
- **Any 0 on dimension 6 or 7 is an automatic fail** regardless of total — session leakage and manipulative tone are disqualifying, not averaging-out flaws.

## Scoring notes

- Score dimension 6 first; it is nearly mechanical (scan for paths, "as discussed"-class phrases, "I ran/checked/verified", undefined internal names).
- For clarity-layer outputs (Step 0 gate not triggered), dimensions 4 and 5 score N/A; grade out of 12 and dimension 7 checks that no pitch apparatus appeared.
- Grade against the artifact's stated audience, not the grader's preferences.
