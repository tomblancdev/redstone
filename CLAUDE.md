# CLAUDE.md

Working rules for this repo — the project explains itself in [README.md](README.md).

## The loop

- **Everything runs in containers** — never install toolchains on the host.
  `./dev.sh` wraps every task: `check` (gate) · `build` · `fmt` · `proto` ·
  `sdk` · `smoke` · `hooks`.
- **`./dev.sh check` before claiming anything works.** It is the pre-commit
  hook and CI; never build without it.
- New bind-path behavior needs a test in `cmd/redstone/redstone_test.go` —
  network-free: fixtures use `check: none` gives with `tcp://` endpoints.
- Touched `proto/`? Run `./dev.sh proto` and **commit the regenerated
  stubs** (`gen/`, `sdk/py/redstone_sdk/_gen`) — CI fails on stale stubs.

## Hard rules (details: docs/dev.md "Policies")

1. The kernel never grows features — new concerns are bricks, not patches.
2. `redstone.core.v1` is frozen: additive proto changes only; breaking → v2.
3. Humans get sparks, machines get JSON — never flavor API responses,
   refusal reasons, or the tokens `claimed`/`OK`/`DOWNGRADED`/`FAIL`.
4. Typos are errors, not silence — new config surfaces must fail loudly.
5. Never hand-edit `CHANGELOG.md` or the version in `cmd/redstone/main.go`
   (release-please owns both; commits become the changelog).

## Commits

Conventional, enforced by the commit-msg hook:
`type(scope): description` — types `feat fix docs test refactor perf chore
ci`, scopes `bind verify validate cli graph wire proto vibe sdk`.
Write descriptions for release-notes readers.

## Check first

| Changing… | Read |
|---|---|
| kernel behavior | `cmd/redstone/*.go` + [docs/architecture.md](docs/architecture.md) |
| the stack format | [docs/stack-spec.yaml](docs/stack-spec.yaml) (locked) + `schemas/stack.schema.json` |
| the wire / proto | [docs/api.md](docs/api.md) + `proto/redstone/core/v1/core.proto` |
| SDKs | [sdk/README.md](sdk/README.md) — the three laws |
| CLI output / vibe | [docs/cli.md](docs/cli.md) — the vocabulary table |
| process / releases | [.github/CONTRIBUTING.md](.github/CONTRIBUTING.md) |
