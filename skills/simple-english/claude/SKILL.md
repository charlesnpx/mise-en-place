---
name: simple-english
description: >
  Write or rewrite answers in simple English that a reader can follow without
  outside context. Use when the user invokes simple-english, /simple-english, or
  $simple-english, or asks to simplify something or explain it in simple English.
---

# Simple English

When asked to simplify something, or to say it in simple English, treat the request as:

> Simplify this in simple English. Do not use jargon that assumes the reader already has the context it needs. Do not compress the answer in a way that makes understanding it depend on things that are not in the response.

Before writing, read both reference files. They are the two halves of the `sealed-semantics` README:

- [references/voice-sample.md](references/voice-sample.md) is the human-written opening. This is the style to match.
- [references/avoid-sample.md](references/avoid-sample.md) is the part after the `## AI Generated Description:` heading. This is the style to avoid. Read it only to recognize what not to write; do not copy its phrasing or structure.

What the sample does that you should do:

- It starts from something the reader already knows (TypeScript types describe structure) and builds the new idea from there.
- It explains a term the first time it is used, in ordinary words, often with a small concrete example (`submitPayment(payment)`, what `PaymentObject` "often means").
- It says why something matters in the same place it says what it is.
- Sentences are full sentences, with articles and connecting words ("so", "because", "but"). It reads like a person explaining something to a colleague.

What the AI-written part does that you should not do:

- Short, clipped sentences and slogans ("Define values once.").
- Lists of named concepts that are never explained ("runtime nominal identity, Zod codec composition, hidden state, controlled projections, ...").
- Terms that only make sense if you already know the subject.

Rules:

1. Define or replace every term the reader may not know. If a term is needed, explain it the first time in plain words.
2. Keep the answer self-contained. Do not point to earlier messages, files, or ideas the reader has to go look up to understand the point. If something is needed, say it here.
3. Do not shorten by dropping words or steps. Shorten by removing content the reader does not need, not by compressing what remains.
4. Prefer concrete examples over abstract descriptions.
5. Keep the facts and meaning of the original. Simpler words, same claims.
