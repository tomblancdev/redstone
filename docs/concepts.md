●━━━●━━⚡━━●━━━●

# Concepts

Everything in Redstone follows one rule:

> **A consumer names a capability and a level — `blob@mutable` — never an
> implementation.** Which instance answers is configuration, verified by
> tests, recorded on a graph.

## Capability

A capability is what apps depend on *instead of* a product: `blob`, `mail`,
`secrets`. Each has a spec file (`capabilities/<name>.yaml`) whose `levels:`
form a **ladder** — key order, weakest first:

```yaml
capability: blob
levels:
  core:    { operations: [put, get, resolve] }
  mutable: { operations: [update, delete, list] }
```

Levels express honest partial compatibility: a content-addressed store can
provide `blob@core` but can never honestly provide `blob@mutable` (nothing
content-addressed can delete). A consumer asks for the level it needs; an
instance satisfies any request at or below its own.

## Conformance

A claim is worth nothing until tested. `redstone verify` runs an executable
pack against every claiming instance, level by level (a level must pass in
full before the next runs; a level with no pack is *unverifiable*, never
auto-passed), and writes verdicts to `conformance.yaml`.

**The register enforces verdicts at bind time.** An instance claiming
`mutable` but verified only `core` is unbindable at `mutable` — refused with
the verdict as the reason. Verification is the authority; the claim is just
an application.

Capabilities without a pack bind on *claimed trust*, visibly marked
`verified: false`.

## Graph

Every bind appends an edge: who (stack/app/task) consumed what, from whom,
verified or not, when. Every *declared* use also renders as an edge with its
live resolution — including `unresolved` when nothing satisfies it, visible
**before** anything breaks. The catalog cannot go stale because it is the
wiring itself.

## Stacks

A stack is a self-contained unit: one file declaring what it **gives** the
fleet and what its apps **use** from it — never another stack's internals
(only give-*names* cross stack boundaries, and only as deliberate pins).
Resolution is lexically scoped: a stack's own gives first, then the fleet.

Declared uses are the single source of truth: an app may bind by task name
alone and inherit capability + level from its declaration; a request that
contradicts its declaration is **config drift, refused loudly**. Full format:
[stack-spec.yaml](stack-spec.yaml).

## Edge properties

A binding between a consumer and a shared service can carry:

- **`with`** — an opaque dict (space, tenant, ttl…). The kernel stores and
  delivers it verbatim, never interprets a key. Non-secret config only.
- **`wire`** — per-edge capability bindings for the service's own needs
  (e.g. *this* app's storage authenticates against *that* IdP). Resolved and
  verified by the kernel like any bind; recursive (a wire value is a
  use-value), depth-limited, cycle-refused.

Adapters retrieve a caller's edge properties from the register
(`/svc/registry/edge`) using the caller's identity — config reaches shared
services without ever being baked into them.
