●━━━●━━⚡━━●━━━●

# Development

## Layout

```
cmd/redstone/        the kernel — flat package main, readable in a sitting
  main.go            CLI + subcommands
  catalog.go         stack files, catalog, ladders, crossCheck
  bind.go            the bind pipeline, wire resolution, graph
  validate.go        registry lint (validate / add-gate / boot warnings)
  verify.go          conformance packs + runner
  httpserver.go      HTTP transport (+ core-capability bases)
  grpcserver.go      gRPC transport
  redstone_test.go   the regression net — 18 tests, network-free
gen/                 generated Go stubs (committed)
proto/               core.proto + buf configs — the wire contract
sdk/                 the binders (go, js/ts, py) — see sdk/README.md
capabilities/        the three core specs (registry, graph, conformance)
schemas/             stack.schema.json (editor validation)
docs/                you are here · stack-spec.yaml is the locked format
assets/banner.txt    the banner, canonical copy
scripts/smoke.sh     the pressure-plate test (deployment battery)
dev.sh               the task runner — every task in a container
.github/             CI/CD, templates, CONTRIBUTING, SECURITY,
                     release-please config
```

## Build — the gate is not optional

```sh
docker run --rm -v "$PWD:/src" -w /src -e CGO_ENABLED=0 golang:1.24-alpine \
  sh -c "go vet ./... && go test ./... && \
         go build -trimpath -ldflags '-s -w' -o bin/redstone ./cmd/redstone"
```

Never build without `go vet && go test`. Cross-compile with
`GOOS`/`GOARCH` (linux/amd64, linux/arm64, windows/amd64 are the tested
targets). Static binaries — no CGO, no runtime.

## Tests

Unit tests build their own registries in `t.TempDir()` and use
`check: none` gives with `tcp://` endpoints — **crossCheck never dials in
tests**. Cover any new bind-path behavior the same way. After any edit that
touches a running deployment, run the smoke battery:

```sh
REDSTONE_URL=http://localhost:8215 sh scripts/smoke.sh   # assumes --strict
```

## Protocol changes

`proto/redstone/core/v1/core.proto` is the wire contract. Additive changes
only (new fields, new field numbers); regenerate with:

```sh
docker run --rm -v "$PWD:/w" -w /w/proto bufbuild/buf:1.71.0 generate
```

`redstone.core.v1` is where compatibility STARTS: from here on, renaming a
binary stays free but the wire namespace never moves again — breaking
changes belong to a future `v2` package, not to edits of `v1`. (The
prototype-era `mystack.core.v1` namespace was retired in one deliberate
break before any external consumer existed.)

## Policies (the ones that keep this small)

1. **The kernel never grows features.** No plugin API, no auth, no UI, no
   bus, no scheduler. Extension = things *outside* that bind capabilities.
   If a change adds a concern rather than deepening an existing one, it's a
   brick, not a kernel patch.
2. **Files, not a database.** State must remain human-readable,
   live-editable, git-diffable.
3. **Typos are errors, not silence.** Anything a stack author can misspell
   must fail loudly at validate/parse — never degrade into ignored config.
4. **Humans get sparks, machines get JSON.** Flavor lives in CLI output,
   logs and docs only. API responses, refusal reasons and the tokens
   `claimed`/`OK`/`DOWNGRADED`/`FAIL` are frozen interfaces.
5. **Fail safe.** Bad input degrades to loud refusals, never wrong answers;
   kernel downtime must never affect already-bound traffic.
