---
name: to-audience
description: Structures audience-facing reports, decks, websites, memos, executive updates, and presentations around a clear decision, audience-value framing, evidence, objections, and action.
disable-model-invocation: true
---

<!-- Manual-only (/to-audience). To promote to auto-trigger, remove disable-model-invocation. -->

# to-audience

This skill changes the agent's default output from:
> "Here is a well-written explanation of the topic."

to:
> "Here is the answer this audience needs, why they should believe it, what objection they will raise, and what concrete next step follows."

This is decision communication: help a real audience understand, believe, decide, and act.

## Rule strength

- **Mandatory in full mode:** answer-first structure · audience-value framing · claim→evidence→implication→ask blocks · fair treatment of the strongest resistance · sentence-headline slides for decks.
- **Mandatory in clarity layer:** answer-first where useful · sentence headings · clean grouping · no forced objection handling, CTA, urgency, or sales tone.
- **Conditional — use only if it clarifies stakes:** one compact narrative · near-peer social proof.
- **Reason count:** use 2–4 grouped reasons; three is the default, but never pad weak reasons to reach three.

## Step 0 — Mode gate

The full workflow applies ONLY when the artifact should help an audience decide, approve, fund, adopt, change behavior, or act. If the task is neutral status, documentation, notes, or exploratory writing, apply ONLY the clarity layer: answer-first where useful, sentence headings, clean grouping — no objection handling, no call to action, no urgency, no consulting tone. This selects a layer, not an on/off switch.

## Step 1 — Define the communication situation before drafting

Identify all of the following before writing a word of the artifact:

- **Audience** — who actually reads or watches this
- **Decision / action** the artifact should enable
- **Current belief → desired belief**
- **Value frame** — what this audience weighs: cost, risk, speed, compliance, outcomes
- **Strongest resistance** they will raise
- **Available evidence**
- **Smallest concrete ask**
- **Reader's knowledge state**

**Reader knowledge state rule:** assume the reader has ZERO access to this session — no conversation history, no file system, no tool outputs, no operations you performed. List what the reader independently knows; everything else must be established inside the artifact or cut.

**Missing items — asymmetric handling:** if **audience** or **decision** cannot be recovered from context, ask the user ONE clarifying question — do not guess; a wrong audience silently poisons the framing, the objections, and the ask. For every other missing item, infer a reasonable default AND state the inference visibly at the top of the output (e.g. "Written for: VP Eng deciding whether to fund a pilot") so a wrong guess is correctable.

## Step 2 — Default argument spine

1. Answer / recommendation
2. Situation — rebuilt from zero for a reader who wasn't in the room; never compress it away just because you know it
3. Complication — the cost of the status quo
4. Grouped reasons (2–4; see Rule strength)
5. Evidence under each reason
6. Strongest objection stated fairly, then refuted
7. Implication for this audience
8. Concrete ask

## Step 3 — Claim → evidence → implication → ask blocks

The default unit for every major paragraph, section, or slide.

- Evidence must be **reader-legible**: show the number, table, quote, or excerpt itself.
- Never cite your own actions as proof ("I ran the tests", "I verified the data"). If evidence exists only in your session, surface the artifact or downgrade the claim.
- No claim without evidence and an implication.

## Step 4 — Format adapters

Skeletons live in `references/templates.md` — read it when drafting.

- **Memo / report:** executive answer first · grouped reasons · proof and tradeoffs · decision/ask at the end.
- **Deck:** every slide title is a full-sentence conclusion, never a topic · one visual proof per slide · no bullet walls · explicit objection slide · final slide is the decision.
- **Website / static page:** hero states the answer/value proposition · sections are claim/evidence/implication blocks · visible FAQ handles objections · CTA is specific, low-friction, reversible.
- **Conversation prep** (you produce a prep document; you are never in the room): questions to ask · anticipated concerns with reflections · what evidence would change their mind · proposed small next step.

## Step 5 — Quality gates

Binary checklist; run before finalizing, every box must pass.

- [ ] Main point identifiable in 10 seconds; first screen/page/slide is answer-first.
- [ ] Framed in the audience's decision language, not the author's interests.
- [ ] Every major claim has reader-legible proof.
- [ ] Strongest objection stated in a form its proponents would recognize, then answered.
- [ ] Ask is concrete, small, and reversible where possible.
- [ ] **Cold-reader pass:** every file path, proper noun, acronym, and back-reference is introduced or deleted; no "as discussed" / "as requested" / "per your instructions"; no first-person verification claims; no naked internal artifact names (branches, sheet tabs, function names, upload paths).
- [ ] No fake urgency, manipulative scarcity, or overclaiming.
- [ ] Structural variety: keep the skeleton, vary the surface — do not produce a recognizable house style across artifacts.

## Lane separation

This skill controls argument structure and audience adaptation only. It does NOT control visual styling, chart design, brand voice, prose humanization, or document production mechanics — defer to dedicated skills for those.

## References

- `references/templates.md` — memo, deck, website, and conversation-prep skeletons. Read when drafting.
- `references/objection-bank.md` — prompt questions for building the objection/FAQ section.
- `references/scoring-rubric.md` — 0–2 rubric for HUMAN evaluation of outputs; not runtime instructions.
