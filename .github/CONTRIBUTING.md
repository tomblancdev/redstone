# Contributing

●━━━●━━⚡━━●━━━●

Everything runs in containers — nothing is ever installed on your machine.
Open the repo in the **devcontainer** (VS Code: "Reopen in Container") or
use `./dev.sh`, which wraps every task in docker.

```sh
./dev.sh hooks     # once: installs the pre-commit pressure plate
./dev.sh check     # fmt + vet + tests — THE gate, same as CI
./dev.sh build     # static binaries for linux/amd64, linux/arm64, windows
./dev.sh proto     # buf lint + regenerate stubs
./dev.sh smoke     # pressure-plate test against a running kernel
```

## Commits

[Conventional Commits](https://www.conventionalcommits.org):
`type(scope): description` — types `feat` `fix` `docs` `test` `refactor`
`chore` `ci`. Scopes that exist: `bind`, `verify`, `validate`, `cli`,
`graph`, `wire`, `proto`, `vibe`.

```
feat(bind): resolve wire targets self-first
fix(verify): treat missing pack level as unverifiable, never passing
docs(cli): transcript for the creeper rollback
```

Voice in descriptions is welcome; voice in types/scopes is not (tooling
parses those — machines get JSON).

**Your commits ARE the changelog**: release-please turns them into
CHANGELOG entries verbatim, so write the description for a reader of the
release notes. `feat` bumps minor, `fix` bumps patch, a `!` or
`BREAKING CHANGE:` footer bumps major (while 0.x: minor).

## Pull requests

The template walks you through it. The non-negotiables:

1. `./dev.sh check` passes — the gate is not optional, CI reruns it.
2. New bind-path behavior comes with a test (network-free: `check: none`
   gives with `tcp://` endpoints — see existing fixtures).
3. Commits are conventional — they become the changelog, there is no
   separate changelog step.
4. The five policies hold ([docs/dev.md](../docs/dev.md)) — especially:
   the kernel never grows features, and humans get sparks, machines get
   JSON. A change that adds a *concern* is a brick, not a kernel patch.
5. `redstone.core.v1` is frozen. Additive proto changes only; breaking
   changes propose a `v2` package (CI's `buf breaking` will hold the door).

## Releases — the changelog writes itself

SemVer. While 0.x, minor bumps may break anything **except the wire**.

Releasing is merging a bot PR:

1. Land conventional commits on `main`.
2. **release-please** maintains a release PR: generated CHANGELOG section,
   version bumped in `main.go` (the `x-release-please-version` marker) and
   the manifest.
3. Merge that PR → the tag and GitHub Release are created, and the same
   workflow builds and attaches binaries + checksums. Zero manual steps.

Manual fallback: an annotated `vX.Y.Z` tag pushed by a human still triggers
`release.yml` and ships identically.
