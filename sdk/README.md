●━━━●━━⚡━━●━━━●

# SDKs

The binders: how an app in language X joins the graph. Three laws govern
this directory:

1. **Micro or failing.** An SDK is bind + optional + edge identity —
   ~100 lines. The moment one grows retries-DSLs, middleware or opinions,
   it is becoming a framework, and frameworks are bricks, not SDK residents.
2. **A folder appears when a consumer exists, not before.** `go`, `js`, `py`
   are seeded because real apps proved them. Rust/C#/anything joins when
   something written in it actually wants to bind. An empty SDK is a promise
   made to nobody.
3. **The wire is the spec.** SDKs track `proto/redstone/core/v1/core.proto`
   and nothing else. One PR changes proto + kernel + every SDK atomically —
   that is the entire point of the monorepo.

## The shape (identical in every language)

```
client  = connect(register, stack, app)
binding = client.bind("uploads")            # declaration-driven: the stack
                                            # file decides capability+level
maybe   = client.optional("mail")           # unresolved -> feature off
binding.header()                            # ("X-Edge", "stack/app/task") —
                                            # send on capability calls so
                                            # shared adapters can pull this
                                            # edge's with/wire config
```

Refusals surface the register's written reasons verbatim — an SDK never
swallows a verdict.

| SDK | Style | Install |
|---|---|---|
| [go/](go/) | generated stubs, same module as the kernel | `go get github.com/tomblancdev/redstone/sdk/go` |
| [js/](js/) | **JS + TS in one package**: runtime proto loading + hand-written `index.d.mts`, type-tested in CI (`types-test.mts`, tsc --strict) | `npm install` in `sdk/js` |
| [py/](py/) | generated stubs shipped inside the package | `pip install ./sdk/py` |

TypeScript gets no separate folder on purpose (law 1): TS consumers need
*types*, not a second binder — one package serves both, and the declaration
file compiles against real usage in CI so it cannot drift.

Check them: `./dev.sh sdk` (js+py load smokes) — the Go SDK rides the main
gate (`./dev.sh check`).
