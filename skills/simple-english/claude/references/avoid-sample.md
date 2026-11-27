## AI Generated Description:

Define values once. Make their construction rules, identity, and permitted observations part of the API.

This package is for TypeScript codebases where an object with the right properties is not enough. You need to know how it was created. TypeScript usually compares types structurally: objects with matching members can be compatible. A nominal value belongs to a specific declared type; this library also enforces membership at runtime with ES private fields.

This matters particularly in agent-heavy codebases. An engineer may remember that a value must pass a shared validator. An agent working on one function may instead assemble a matching object and add local helpers. The change can look reasonable while bypassing rules elsewhere in the application.

A carefully written class with a private constructor and ES private fields can enforce controlled construction manually. TypeScript and JavaScript can implement these mechanisms without this package. `sealed-semantics` standardizes the complete convention: runtime nominal identity, Zod codec composition, hidden state, controlled projections, semantic identity, mint-event provenance, executable documentation, and laws.

## Two primitives

**`defineSeal` represents a semantic value.** Equivalent decoded values from one completed definition are the same live JavaScript object. Compare them with `===`. Use native `Map` and `Set`.

**`defineMint` represents a successful mint event.** Every successful `.mint(input)` call creates a distinct object. A downstream function can require that object as evidence that the configured producer succeeded.

You supply the checks. The library prevents callers from creating a genuine instance without passing through its construction path. It does not prove that your checks are correct. Accepting a user ID does not prove that a database row exists, or establish authorization, persistence, or currentness.

## What the runtime guarantee covers

A genuine instance can only be created through its definition. TypeScript rejects ordinary objects passed as sealed values. However, `any`, casts, and suppressed errors can bypass that static protection; a type annotation does not authenticate a reference at runtime.

Observe through the kind: `UserId.suffix(user)`, `UserId.read(user)`, or `UserId.assert(user)`. Those functions unseal first. A duck-typed `{ view: { suffix } }` plus `as UserId` throws when observed or encoded. Use the definition's codec for external representations and `UserId.is(value)` to check membership when origin is uncertain.

Typed internal functions normally trust their parameters when callers preserve those types. If the input can come from untyped code, deliberate casts, plugins, or a different definition instance, validate it before relying on its observations or performing effects. This applies to internal functions too: the distinction is the trust boundary, not whether the function is exported. The package is not a sandbox against code that deliberately bypasses or replaces validation.

## Requirements

Use Node 22+, TypeScript 5.7+, and Zod >=4.6.6 <5. The core is browser-compatible. It requires `WeakRef` and `FinalizationRegistry` and throws a clear error at import if either is unavailable. There is no fallback.

For Cloudflare Workers with weak references disabled, enable `enable_weak_ref` and remove `disable_weak_ref`. See [runtime requirements](docs/guarantees.md#runtime-requirements).

```sh
npm install sealed-semantics zod
```

## Define an identifier

```ts
import { z } from 'zod';
import { defineSeal, type ValueOf } from 'sealed-semantics';

const UserId = defineSeal({
  key: parts => parts,
  name: 'app/user-id',
  schema: z.string().toLowerCase().regex(/^usr_[a-f0-9]+$/),
})
  .view({ suffix: spelling => spelling.slice(-6) })
  .seal();
type UserId = ValueOf<typeof UserId>;

const a = UserId.codec.parse('USR_0123456789ABCDEF');
const b = UserId.codec.parse('usr_0123456789abcdef');
console.log(a === b); // true
console.log(UserId.suffix(a)); // abcdef
console.log(z.encode(UserId.codec, a)); // usr_0123456789abcdef
const users = new Map<UserId, string>([[a, 'Alice']]);
console.log(users.get(b)); // Alice
```

The schema validates and normalizes input. Its output is copied into frozen private **Parts** before identity is computed. Producer-owned containers are not frozen or retained. Interning happens after that normalization. It makes semantic identity coincide with object identity; it is not a promise of faster parsing.

Call `.docs(...)`, `.view(...)`, and `.copy(...)` in any order; `.seal()` must come last and checks documentation against the final view and copy observations. Completed definitions have no builder methods.

`key`, `debug`, `view`, and `copy` callbacks receive `DeepReadonly<Parts>` in TypeScript. Normalize and copy mutable data before those callbacks run. This readonly callback contract is a public breaking change for code that mutated Parts in callbacks.

`ValueOf<typeof UserId>` gives the instance type. A cast cannot construct an instance. `UserId.is(value)` checks the actual private brand.

## You can decode individual fields, or a whole response

Zod owns representation boundaries. Use `Seal.codec.parse(unknown)` or `safeParse(unknown)` for external input. Use `z.decode` for statically typed input, and `z.encode` for output. A composed schema handles nested seals and arrays without field-by-field conversion.

```ts
import { z } from 'zod';
import { defineSeal, defineMint, type ValueOf } from 'sealed-semantics';

const UserId = defineSeal({
  key: parts => parts,
  name: 'app/user-id',
  schema: z.string().toLowerCase().regex(/^usr_[a-f0-9]+$/),
}).seal();
type UserId = ValueOf<typeof UserId>;

const ProjectId = defineSeal({
  key: parts => parts,
  name: 'app/project-id',
  schema: z.string().regex(/^prj_[a-f0-9]+$/),
}).seal();
type ProjectId = ValueOf<typeof ProjectId>;

const ResponseSchema = z.object({
  user_id: UserId.codec,
  project_id: ProjectId.codec,
});
const response = ResponseSchema.parse({
  user_id: 'USR_123ABC',
  project_id: 'prj_123abc',
});
console.log(z.encode(ResponseSchema, response));
// { user_id: 'usr_123abc', project_id: 'prj_123abc' }
```

Use a `z.codec(...)` as the definition's schema when external input and Parts have different types. Normalize in that schema before identity is computed. One-way Zod transforms can decode but cannot encode backward. When `z.encode(Kind.codec, value)` runs, the sealed codec reads the frozen Parts snapshot. Zod may validate or copy that value before a schema encoder runs. Write schema encoders as read-only code; do not rely on mutating Parts.

## Every seal declares its identity key

Every `defineSeal` requires `key`, including primitive Parts. Use `key: id => id` for a normalized string identifier. Supported keys are strings, numbers, bigints, booleans, `null`, and `undefined`. Symbols are not semantic keys.

For structured Parts, choose a key that identifies the complete value:

```ts
const Coordinate = defineSeal({
  name: 'geo/coordinate',
  schema: z.object({ lat: z.number(), lng: z.number() }),
  key: p => `${p.lat}:${p.lng}`,
}).seal();
const c1 = Coordinate.codec.parse({ lat: 1, lng: 2 });
const c2 = Coordinate.codec.parse({ lat: 1, lng: 2 });
console.log(c1 === c2); // true
```

To key on the canonical wire form, encode Parts with `encodeParts(schema, parts)`. Callbacks receive `DeepReadonly<Parts>`, which `z.encode` does not accept because a Zod schema outputs mutable arrays. `encodeParts` accepts readonly Parts, or any readonly part of them with its own sub-schema. Use it in `key`, `view`, `copy`, and `debug`. The schema must be able to encode. One-way `.transform` schemas throw; use `z.codec` for a normalization that must encode.

```ts
import { encodeParts } from 'sealed-semantics';

const member = z.object({ user: UserId.codec, roles: z.array(z.string()) });
const team = z.object({ name: z.string(), members: z.array(member) });
const Team = defineSeal({
  name: 'app/team',
  schema: team,
  key: parts => JSON.stringify(encodeParts(team, parts)),
})
  .view({ firstMember: parts => encodeParts(member, parts.members[0]!) })
  .seal();
const core = Team.codec.parse({ name: 'core', members: [{ user: 'usr_1', roles: ['admin'] }] });
console.log(core === Team.codec.parse({ name: 'core', members: [{ user: 'usr_1', roles: ['admin'] }] })); // true
console.log(Team.firstMember(core)); // { user: 'usr_1', roles: [ 'admin' ] }
```

The key is the declared identity, not a hash hint. Parts must already be normalized for that identity. On a live hit, the library compares the Parts structurally. A key collision between different Parts throws instead of returning the wrong value. Errors include the seal and key, not private Parts. The error occurs at the conflicting parse, which can be far from the definition. Test keys beside their definitions with the [generated-pair collision test](docs/laws.md#testing-key-collisions). The law harness checks individual generated values and configured aliases; it does not exhaustively prove key injectivity.

Decoded Parts are snapshotted before `key(parts)` runs. Snapshot failure happens before key evaluation and before the intern table can change. Supported Parts are primitives, dense arrays, plain data objects, and genuine sealed values from this installed package copy. Plain-object snapshot properties are canonicalized before any callback receives them: JavaScript array-index keys appear in numeric order, followed by other string keys in UTF-16 lexical order. Insertion order is non-semantic because callbacks never receive unsnapshotted plain objects. Use timestamps or strings instead of `Date` Parts. Dates, collections, array buffers/views, functions, arbitrary class instances, accessors, hidden properties, symbol-keyed structures, and cycles are rejected on the first decode. Shared acyclic children are preserved in the frozen snapshot.

Alias topology is semantic. Two Parts graphs with the same property values but different shared-child arrangements are different when callbacks could observe that with reference equality.

Keys use `Object.is` value semantics: `0` and `-0` are distinct keys, and all `NaN` keys share one key. The schema must accept or produce those values first. This does not make non-finite numbers valid JSON.

## Require checks before saving

A save function can require a minted batch containing validated IDs. The producer below requires a non-empty list and removes repeated users.

```ts
const MembershipBatch = defineMint({
  name: 'app/membership-batch',
  mint: (input: { projectId: ProjectId; userIds: readonly UserId[] }) => {
    if (input.userIds.length === 0) {
      return {
        ok: false,
        error: {
          code: 'invalid_membership_batch',
          message: 'Expected at least one user.',
        },
      };
    }
    return { ok: true, value: {
      projectId: input.projectId,
      userIds: [...new Set(input.userIds)],
    } };
  },
})
  .view({
    projectId: p => p.projectId,
    userIds: p => p.userIds
  })
  .seal();

type MembershipBatch = ValueOf<typeof MembershipBatch>;

function saveMembershipBatch(batch: MembershipBatch) {
  console.log('Save:', {
    project: z.encode(ProjectId.codec, MembershipBatch.projectId(batch)),
    users: MembershipBatch.userIds(batch).map(id => z.encode(UserId.codec, id)),
  });
}
const input = { projectId: response.project_id, userIds: [response.user_id] };
const first = MembershipBatch.mint(input);
const second = MembershipBatch.mint(input);
if (first.ok && second.ok) {
  console.log(first.value === second.value); // false: two mint events
  saveMembershipBatch(first.value);
}
```

The producer owns its error format. `ProducerResult<Parts, Error>` can describe object unions or primitive errors; `.mint()` returns failures unchanged. These typed examples assume their identifiers have already passed through the correct codecs. They check additional business rules, not the same input shape again. Use runtime membership checks when an object's origin is uncertain, including in internal functions that cross such a boundary.

The library ensures that the producer succeeded. The producer defines what that success means. This example does not check user existence or permission to change project membership.

## Stable, immutable views

Each view projection runs lazily. Its first successful result is validated, snapshotted as frozen data, and cached. Already snapshotted library-owned nodes can be reused by reference. Later reads return exactly that result, including `undefined` and other falsy values. Failed projections can be retried.

Views allow primitives, sealed values, dense arrays, and plain data objects. Structured observations are deeply readonly in TypeScript and plain-object result properties are canonicalized before callers receive them. Functions, Dates, collections, array buffers/views, other class instances, accessors, hidden properties, symbol-keyed structures, and cycles are rejected on access. Genuine sealed leaves from this installed package copy retain their exact type and remain usable. An ES-private brand authenticates them. The well-known name symbol is only a diagnostic label. Rejected sealed-looking objects may come from another installed package copy or may be imitations; the runtime does not need to decide which.

For digests and other byte-valued data, keep canonical string Parts and declare `.copy({ bytes: ... })` for a stable byte observation returned as a fresh `Uint8Array` at the binary API boundary. Buffers are mutable representations, not identity-bearing values or view outputs. See [digests and byte buffers](docs/bytes.md).

Successful semantic Parts and mint Parts are copied into frozen private snapshots. Anything exposed through `view` becomes a frozen snapshot; already snapshotted library-owned nodes can be reused by reference. If a projection returns an internal array, callers receive a frozen array. Mutating a retained producer alias cannot change an existing value, but a later decode or mint uses a new snapshot. The author still owns semantic correctness, normalization, units, authorization checks, and other domain facts.

## Definition identity and development

A sealed value belongs to exactly one completed definition instance. `Seal.is(value)` is true only for that instance's values. Two definitions may use the same `name` string. They are unrelated, and their values are foreign to each other. The string is a diagnostic label, not runtime identity. Define each semantic meaning once in an owning module, export the completed definition, and import it everywhere it is used. Repeating `.seal()` creates another definition even from the same builder. A copied definition with the same name is not interchangeable with the original.

Re-executing a definition module creates a new definition instance. Older values still work, but the new codec rejects them with a diagnostic. After editing a definition, refresh the page or restart the process to clear preserved state.

Next.js dev, Vite, Vitest, Jest, and Node's test runner need no package-specific configuration. The package holds no global state that can survive a module reset. See [framework guidance](docs/frameworks.md).

TypeScript cannot generate a fresh nominal type for each factory call. Literal definition names distinguish types statically, but two definitions using the same name can have compatible TypeScript types. Runtime brands and codecs remain authoritative. The law harness tests the definitions you supply individually; it does not discover duplicate definitions across your application. There is no duplicate-name registry.

## React and representation boundaries

Equivalent decodes from one definition return the same live object. Semantic values can be used in React state and dependency arrays. Cached structured views do not need caller-written `useMemo` just to stabilize their references.

Object identity does not cross workers, processes, server/client, network, or storage boundaries. Encode through Zod, transfer plain data, then decode with the receiving definition.

Console inspection shows `Sealed<app/user-id>` without encoding, revealing Parts, or calling `debug()`. In development it also includes a relative `defined at file:line`. `JSON.stringify(value)` still throws. Structured/JSON logging requires explicit Zod encoding. Minted values are local construction attestations in the current API. Crossing a serialization boundary requires establishing the claim again on the receiving side.

In Jest and Vitest, use `toBe` to test identity. Two distinct minted values can pass `toEqual` because their private state is not enumerable.

## Executable documentation with `.docs()`

`.docs()` adds descriptions and examples to a builder. `.seal()` checks the completed documentation: it parses each input, compares its encoded output, and verifies round-trip identity. An incorrect example throws during sealing, so documentation errors can fail module initialization. This is optional, but a semantic docs block must contain at least one input/encoded example.

```ts
const DocumentedUserId = defineSeal({
  name: 'example/documented-user-id',
  schema: z.string().toLowerCase().regex(/^usr_[a-f0-9]+$/),
  key: id => id,
})
  .view({ suffix: id => id.slice(-6) })
  .docs({
    description: 'A normalized user identifier.',
    examples: [{ input: 'USR_ABCDEF', encoded: 'usr_abcdef' }],
    view: { suffix: { description: 'The final six characters.' } },
  })
  .seal();
console.log(DocumentedUserId.documentation.description);
```

When docs are present, every declared view or copy observation needs a description. Observation samples are illustrative; they are not expected outputs linked to each input. Semantic examples exercise those observations during sealing. Minted documentation has no wire examples and does not execute the mint producer. Completed definitions expose the metadata as `.documentation`. Metadata does not generate editor JSDoc text. See the [documentation contract](docs/documentation.md).

## What a law tests

A law is an invariant that should hold for every valid input, such as “decoding the same semantic value twice returns the same live object.” The law harness uses fast-check to test generated inputs for identity, codec round trips, immutable views, independent byte copies, and other package guarantees. Minted laws check distinct mint events instead of semantic interning.

```ts
import * as fc from 'fast-check';
import { assertValueLaws } from 'sealed-semantics/laws';

assertValueLaws(Coordinate, {
  validWire: fc.record({ lat: fc.integer(), lng: fc.integer() }),
});
```

Install fast-check as a development dependency. Generators must produce accepted inputs. Sampling can expose counterexamples, but it does not prove all inputs are correct or that the producer implements the intended business meaning. Keep domain-specific tests as well. See [laws and key-collision testing](docs/laws.md).

## Runtime cost

Seals add work beyond the same plain Zod schema. Each decode snapshots the decoded Parts graph, computes a key from that frozen snapshot, and looks up the per-definition weak intern table. A live hit still allocates the new snapshot and compares it with the stored snapshot to detect collisions. A miss allocates a frozen sealed instance, a WeakRef, and finalization bookkeeping. Zod validation still runs on hits.

For primitive Parts with a constant-cost key, this extra work is constant per parse. For an object graph with N visited properties or elements, snapshotting and comparison generally require work proportional to N, plus whatever the key callback costs. A hit does not skip snapshot allocation or comparison just because the value was seen before. Large structured values can therefore cost much more than small string IDs. Seal identifiers and small canonical values. Keep large records as ordinary Zod output that contains sealed leaves. A 500-property record is usually data, not identity.

Each distinct live value retains its frozen Parts snapshot and interning bookkeeping. Finalizer cleanup is delayed, not immediate. The first view access snapshots its result, reusing already snapshotted library-owned nodes when present; later reads return the cached reference. A byte copy observation retains a private snapshot after first use and allocates/copies B bytes for each B-byte result.

0.4.1 includes measured snapshot hot-path optimizations. In the first optimization, descriptor reads and per-property definition dominated sorting. After that change, comparator-based canonical sorting became the largest avoidable `snapshotData` cost for plain objects. The final path partitions array-index keys from string keys before sorting, so array-index checks are computed once per key instead of once per comparison. There is still no qualified timing ratio against plain Zod for this exact implementation. Measure representative schemas, sizes, and hit/miss rates before adopting seals for a performance-sensitive path. Interning guarantees identity; it is not a claim that parsing is faster.

## Examples and verification

```sh
npm ci
npm run check
npm run examples --prefix examples
```

The [consumer walkthrough](examples/src/examples/README.md) demonstrates HTTP requests, native caches, and a save function that requires minted plans and batches. It imports the package through its public exports.

Read the [guarantees](docs/guarantees.md), [specification](docs/specification.md), [documentation guide](docs/documentation.md), [laws](docs/laws.md), [error guide](docs/producer-results.md), and [agent instructions](docs/AGENTS.md). See [CONTRIBUTING.md](CONTRIBUTING.md), [release checks](docs/releasing.md), and [SECURITY.md](SECURITY.md). Licensed under [MIT](LICENSE).

### Owned bytes belong in `.copy`

Use `.view({ hex: ... })` for stable immutable observations and `.copy({ bytes: ... })` for fresh owned `Uint8Array` storage. On the first successful `Digest.bytes(digest)` call, the producer runs and the library copies its bytes into a private snapshot. Every call returns a new array from that snapshot. Mutating a returned array or a retained producer result cannot change later copies. Prefer `Kind.hex(value)` on a hot path; `Kind.read(value)` evaluates every projection.

Only Uint8Array is supported. Buffer producer output is accepted but returned as plain Uint8Array. Shared backing memory is rejected. The definition author is responsible for what the bytes mean; the library enforces storage isolation and stable contents, not purity or semantic correctness. Copy observations do not replace Zod encoding or give minted values a codec. See [the complete digest example and copying rules](docs/bytes.md).
