# sealed-semantics

## Why

TypeScript types primarily describe structure:

> Is this thing a string or not? If this thing is an object instead of a number, then this property on that object needs this type. etc.

AI agents often confuse locally coherent structural rules for globally coherent semantic ones. Human programmers are different in that they implicitly carry lots of semantic context in their heads when working in a repository they're familiar with, which they apply when making decisions.

A senior engineer who sees:

```ts
submitPayment(payment)
```
understands 20 necessary preconditions instantly. An AI doesn't necessarily see more than a callable function that needs objects of the payment shape.

You can try to address this with better context selection: graph traversal or RAG-like systems, or adding rules and heuristics like many people do with CLAUDE.md or AGENTS.md files, where you can try to solve this with updating CLAUDE.md or AGENTS.md stating:

> “Never call capturePayment unless…”

but then you're relying on the agent to remember. you're also encoding just one rule of the N you'll need with this approach where their ability to remember decreases proportional to N increasing.

So why don't we strive to make these implicit rules explicit, so that they're impossible to use incorrectly? AI agents don't get annoyed with strictness and rules in the way humans do, so the cost of making these things impossible by construction seems marginal today.

___

The goal of this repository is to add two semantic datatype producers, both of which strive to create much stronger guarantees around making invalid semantic state impossible to use.

This comes from two distinct things: semantic seals, which are values, and semantic mints, which are wrapped datatypes that include a contract about rules and transformations any value of this mint type has undergone.

**defineSeal** creates seals. They provide runtime guarantees beyond TypeScript types alone because seals enforce the construction path of their data. Once a definition for a seal is provided, new values of that seal must pass through its configured Zod codec. The only methods that can be used to interact with a value of that seal are those in the definition. This allows you to distinguish between something that was merely produced, from something that is legitimately entitled to be relied upon.

```ts
const UserId = defineSeal({
  key: id => id, // Same string means the same sealed value. For object data, combine the properties that identify the value, e.g. p => `${p.lat}:${p.lng}`.
  name: 'app/user-id', // Diagnostic label for this definition; uniqueness is not enforced.
  schema: z.string().toLowerCase().regex(/^usr_[a-f0-9]+$/), // regular zod schema for handling serialization
})
  .view({ // Declare the immutable observations callers can read.
    suffix: id => id.slice(-6),
    uiDisplay: id => `User ID: ${id}`
   })
  .seal(); // Complete this definition.

type UserId = ValueOf<typeof UserId>;
```

```ts
// Decode incoming data into a UserId seal. Zod validates and normalizes it.
const incoming = 'USR_0123456789abcdef';
const userId: UserId = z.decode(UserId.codec, incoming);

// Encode the seal into its external representation, ready for transport.
const outgoing: string = z.encode(UserId.codec, userId);
const jsonBody = JSON.stringify(outgoing); // '"usr_0123456789abcdef"'
```

Seals are meant to transit across serialization boundaries with no issues. They use Zod for encoding and decoding as you already would.

---

**defineMint** creates contracts that couple data (satisfying a TypeScript type)
to a set of rules provided in its definition.
For example, let's say we have a function that wants to only accept a PaymentObject.
What that function often means by this is that they want some data which
describes a payment which has been validated and time-stamped through
the officially sanctioned paths for doing so in this system.
What PaymentObject often means in TypeScript is "a JavaScript object with 3 properties with these names that have string values, and one property with this name that has a boolean value" or whatever.

If PaymentObject were a mint (we might call a minted `PaymentObject` object `PaymentReady` or something), those requirements could
be added to its mint function. As a result every `PaymentReady` object guarantees that it is a `PaymentObject` that has successfully
had the rules needed for it to be ready applied, and only values produced by `mint()` can be passed
to functions expecting PaymentReady.

```ts

type PaymentObject = {
  recipientId: UserId,
  amountCents: number,
  currency: 'USD' | 'EUR'
};

const PaymentReady = defineMint({
  name: 'app/payment',
  mint: (input: PaymentObject) => {
    const recipientId = UserId.assert(input.recipientId);
    if (
      !Number.isSafeInteger(input.amountCents) ||
      input.amountCents <= 0
    ) {
      return {
        ok: false,
        error: {
          code: 'invalid_payment',
          message: 'Expected a positive safe-integer amount in cents.',
        },
      };
    }

    return {
      ok: true,
      value: {
        recipientId,
        amountCents: input.amountCents,
        currency: input.currency,
        preparedAt: Date.now(),
      },
    };
  },
})
  .view({
    recipientId: p => p.recipientId,
    amountCents: p => p.amountCents,
    currency: p => p.currency,
    preparedAt: p => p.preparedAt,
  })
  .seal();

type PaymentReady = ValueOf<typeof PaymentReady>;

```

Then any function taking `PaymentReady` as an argument can work from the guarantees this creates.

You can also create minted datatypes which include seals, or other mints. This allows the semantic attestations to compose naturally.

One result of this approach is that AI-generated code can't merely discover the shape of an execution-eligible value  and recreate it locally. It has to find the producer that is capable of minting one.

Mints should never cross serialization boundaries and are non-serializable. This is because producing a mint from plain JSON means making a claim about the data outside of what its contract can attest to as it's currently constructed. (There are of course some fun ways one might think about making this not an issue! Not for now though.)

