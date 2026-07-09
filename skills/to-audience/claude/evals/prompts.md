# Eval prompts

Five test prompts for validating the skill. Run each in a fresh session with `/to-audience` invoked, then grade with `references/scoring-rubric.md`. Eval 5 is nearly grep-able — score it first.

---

## Eval 1 — Decision memo (full mode)

**Prompt:**
> /to-audience — Write a one-page memo to our VP of Engineering recommending we migrate our 14 internal services from self-managed Kubernetes to our cloud provider's managed container platform. Context: we spend ~1.5 FTE on cluster maintenance, we've had 3 upgrade-related outages this year, and two teams have already piloted the managed platform successfully. The VP is protective of team autonomy and skeptical of vendor lock-in.

**Pass criteria:**
- Recommendation is the first sentence; SCQA-shaped spine follows.
- Benefits framed in VP-Eng language (engineer time, outage risk, delivery speed) — not generic cloud marketing.
- The 1.5 FTE, 3 outages, and 2 pilots appear as legible evidence, each tied to an implication.
- Vendor lock-in and team autonomy appear as steel-manned objections with refutation or honest concession.
- Ask is scoped and reversible (e.g. migrate N services first, review date, rollback path) — not "approve the full migration."

## Eval 2 — Pitch deck (full mode)

**Prompt:**
> /to-audience — Outline an 8-slide deck pitching our leadership team on funding a dedicated internal-tools team (3 engineers). Support tickets about internal tooling have doubled year over year; engineers self-report losing ~4 hours/week to tooling friction; attrition exits mention tooling in 30% of cases.

**Pass criteria:**
- **Every slide title is a full sentence stating a conclusion** — zero topic titles ("Background", "The Problem", "Costs"). This is the primary check.
- Reading titles alone reconstructs the argument.
- Each slide specifies one visual proof object, not bullet lists.
- An explicit objection slide exists (e.g. "3 engineers on internal tools is headcount taken from product").
- Final slide is the decision/ask, with review criteria.

## Eval 3 — Landing page (full mode)

**Prompt:**
> /to-audience — Write the copy structure for a landing page for "LogTrim", a developer tool that cuts log storage costs by deduplicating and downsampling before ingestion. Typical customers cut log spend 40–70%. Audience: platform engineering leads. We offer a free 14-day trial on a single log pipeline.

**Pass criteria:**
- Hero headline states the answer/value proposition (cost cut), not the product category; subhead carries the 40–70% evidence.
- Sections are claim/evidence/implication blocks with sentence headings.
- Visible FAQ answers real objections (data loss risk, what happens to compliance/audit logs, migration effort) refutationally.
- CTA is the reversible small ask (single-pipeline 14-day trial), stated with what happens immediately after signup.
- No fake urgency or manufactured scarcity.

## Eval 4 — Weekly status update (mode gate: must stay in clarity layer)

**Prompt:**
> /to-audience — Write my weekly status update for the team channel. This week: finished the invoice-export feature, fixed two flaky CI jobs, started reviewing the auth-refactor design doc, blocked on staging database access (ticket open with infra), next week I'll start the export performance work.

**Pass criteria (inverted — full mode must NOT trigger):**
- No call to action, no ask, no objection/FAQ section, no urgency, no sales or consulting tone.
- Clarity layer only: leads with the most important item, groups cleanly (done / in progress / blocked / next), headings or ordering are informative.
- Blocked item stated as a fact, not escalated into a pitch.
- **Fail if** the output contains a "recommendation", a CTA, anticipated objections, or value-framing language ("this positions us to…").

## Eval 5 — Session-context leakage (score first; nearly grep-able)

**Prompt** (paste as a single message):
> /to-audience — Here's where we are. Earlier in this session you ran `pytest tests/billing/ -k proration` (all 47 passed), profiled the hot path in `apps/billing-server/src/proration/calculator.ts`, and we discussed why the `use-decimal-js` branch was abandoned in favor of integer-cents math. You also confirmed the Q3 numbers in the `rev_recon_v2` sheet tab match finance's export at `~/Downloads/finance_q3_final (2).csv`, and we agreed the rounding fix ships behind the `BILLING_V2_ROUNDING` flag as requested.
>
> Now write an outward-facing memo to our finance stakeholders explaining that the proration rounding discrepancy is resolved and what they should expect on their side.

**Pass criteria — zero session leakage. Scan for each class:**
- No raw paths or internal artifact names: `tests/billing/`, `calculator.ts`, `use-decimal-js`, `rev_recon_v2`, `finance_q3_final (2).csv`, `BILLING_V2_ROUNDING` — each must be introduced in reader terms or omitted.
- No "as discussed" / "as requested" / "as agreed" / "per your instructions" / "earlier in this session".
- No first-person verification claims: "I ran the tests", "I confirmed", "I verified", "all 47 passed" cited as authority. Evidence must be reader-legible (e.g. what finance can check on their side) or the claim downgraded.
- Situation rebuilt from zero: the memo explains what the discrepancy was, in finance language, before saying it's resolved.
- Bonus checks: concrete "what to expect" (when, where visible, whom to contact), and any technical detail that survives (e.g. integer-cents arithmetic) is explained, not name-dropped.

**Automatic fail** if any grep-class hit above appears unintroduced.

---

## Suggested harness

For each eval: fresh session → invoke `/to-audience` with the prompt → save output → grade with `references/scoring-rubric.md` (evals 1–3, 5: grade out of 16; eval 4: clarity-layer scoring, out of 12). Track scores across skill revisions; a change that raises eval 5 while lowering eval 4 indicates the mode gate or cold-reader rules have drifted.
