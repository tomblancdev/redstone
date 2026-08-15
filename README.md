# Redstone

```text
█▀▄ █▀▀ █▀▄ █▀▀ ▀█▀ █▀█ █▄ █ █▀▀
█▀▄ █▀▀ █ █ ▀▀█  █  █ █ █ ▀█ █▀▀
▀ ▀ ▀▀▀ ▀▀▀ ▀▀▀  ▀  ▀▀▀ ▀  ▀ ▀▀▀
●━━━●━━⚡━━●  the wiring layer  ●━━⚡━━●━━━●
```

Named for the wiring layer of a certain block game: redstone is the stuff
that connects your components and makes the build actually *do* something.

Redstone is a **capability kernel**: apps bind *capabilities*
(`blob@mutable`), never products. Claims are **verified** by executable
conformance before anything may bind them, every binding lands on a
**graph** you can read — and a provider that can't honor its claim is
refused *with the reason in writing*. One static binary (~12 MB, no
runtime, no database — state is files), one protocol over HTTP+JSON and
gRPC, and a docker-style CLI that needs no daemon.

```
redstone serve      the register: catalog, bind, graph   (HTTP + gRPC)
redstone verify     conformance: test every claim, write the verdicts
redstone validate   lint the registry — typos are errors, not silence
redstone ls         list the stacks in this world
redstone add        place a stack (validation-gated, atomic rollback)
redstone rm         break a stack (refuses while circuits still draw power)
```

## Quickstart

Install like docker — one line, one static binary on your `PATH`,
checksum-verified (all paths incl. windows, pinning and the ghcr image in
[docs/install.md](docs/install.md)):

```sh
curl -fsSL https://raw.githubusercontent.com/tomblancdev/redstone/main/scripts/install.sh | sh
```

```sh
redstone validate --registry ./registry --caps ./capabilities
redstone serve    --dir ./registry --caps ./capabilities \
                  --http :8215 --grpc :8216 --strict
redstone verify   --registry ./registry --caps ./capabilities

curl 'localhost:8215/bind?stack=prod&consumer=my-app&as=blob'
curl 'localhost:8215/graph'
```

A stack is one file — what it **gives** the fleet, what its apps **use**:

```yaml
version: 1
gives:
  blob-s3: { is: blob@mutable, at: http://connector-s3 }
uses:
  my-app:
    blob: blob@mutable          # kernel picks, verified
    docs: blob-ipfs             # a specific give, verified too
```

## Documentation

| Doc | What it explains |
|---|---|
| [docs/install.md](docs/install.md) | install from a release build (or docker/ghcr), updates, rollback |
| [docs/concepts.md](docs/concepts.md) | capabilities & levels, conformance, the graph, stacks, `with`/`wire` |
| [docs/architecture.md](docs/architecture.md) | state files, the bind pipeline, transports, failure semantics |
| [docs/stack-spec.yaml](docs/stack-spec.yaml) | the locked stack-file format, annotated |
| [docs/api.md](docs/api.md) | every HTTP route, gRPC, response shapes, the manifest contract |
| [docs/cli.md](docs/cli.md) | every command, every flag, example transcripts |
| [docs/dev.md](docs/dev.md) | layout, build gate, tests, proto policy, the five policies |
| [sdk/](sdk/README.md) | the binders — Go, JS, Python; micro by law |
| [schemas/](schemas/stack.schema.json) | JSON Schema for stack.yaml — editor autocomplete + validation |

## Posture

Fails safe (bad config → loud refusals, never wrong answers) · kernel-down
doesn't touch running apps · graph self-compacts with an audit archive ·
**run it network-internal** (no auth by design — the perimeter is an
ingress brick's job) · put the registry in git · the kernel never grows
features.

The CLI speaks builder — stacks are *placed* and *broken*, circuits *power*
or *don't*, validation failures get *creeper'd* — under one rule: **humans
get sparks, machines get JSON** (see [docs/cli.md](docs/cli.md)).

## Contributing

`./dev.sh check` is the gate, the devcontainer is the workshop, commits are
conventional (they become the changelog), releasing is merging the bot's
PR — see [CONTRIBUTING](.github/CONTRIBUTING.md). MIT licensed.
Vulnerabilities: [SECURITY](.github/SECURITY.md). History:
[CHANGELOG.md](CHANGELOG.md).
