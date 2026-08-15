●━━━●━━⚡━━●━━━●

# CLI

No daemon: the CLI writes files, a running `redstone serve` live-reads them
— your change is visible next tick. Flag order matters (Go convention):
**flags before the positional argument**.

## serve

```
redstone serve --dir ./registry --caps ./capabilities \
               --http :8215 --grpc :8216 --strict
```

| Flag | Default | Meaning |
|---|---|---|
| `--dir` | `.` | state dir (stacks/, conformance.yaml, graph.jsonl) |
| `--caps` | `/capabilities` | capability specs |
| `--http` / `--grpc` | `:80` / `:50051` | listen addresses |
| `--strict` | off | stacked binds must have a declared use |

Boot sequence: validate (issues printed; errors = `⚠ powering on DEGRADED`,
never a refusal to start) → verdict-staleness warning (>7 days: *circuits
corrode*) → graph compaction → `⚡ powering on`.

## verify

```
redstone verify --registry ./registry --caps ./capabilities
```

Runs the conformance packs against every claiming instance, writes
`conformance.yaml` atomically. A reporter, not a gate: always exits 0 — a
lying instance doesn't block boot, it becomes unbindable at the level it
lied about.

```
⚡ minio-local: claimed blob@mutable -> OK
🔌 ipfs-overclaimed: claimed blob@mutable -> DOWNGRADED to core
   FAIL update(create-at-key): PUT 404
```

Tokens `claimed` / `OK` / `DOWNGRADED` / `FAIL` are stable for grepping.

## validate

```
redstone validate --registry ./registry --caps ./capabilities
```

Lints the whole registry — **typos are errors, not silence**: unknown
fields (a `wih:`), levels off the ladder (*"can NEVER bind"*), references
to gives no stack provides, bad names, wire nesting past depth 3. Warnings
for legal-but-notable states (no spec = claimed trust). Exit 1 on any
error. Clean run: `⚡ all circuits clean — registry valid`.

## ls / add / rm

```
redstone ls  --registry ./registry
⛏ 11 stack(s) in this world

redstone add --registry ./registry --name website ./website.yaml
⛏ placed "website": 0 gives, 1 uses — a running redstone sees it next tick

redstone add --registry ./registry --name broken ./typo.yaml
🧨 NOT placed — the creeper got it (registry rolled back). Fix the errors above.

redstone rm  --registry ./registry s3
NOT broken: 2 circuit(s) still draw power from this stack's gives:
  prod/admin.blob -> minio-local
```

`add` validates the **merged** registry and rolls back atomically on error
(`--force` to overwrite an existing stack of the same name). `rm` refuses
while other stacks' uses reference the stack's gives (`--force` to break
anyway; folder-form stacks are removed by deleting their directory).

## version

```
redstone version
redstone 0.1.0 ⚡ the graph is the truth!
```

Splash line varies; machines should parse the first token only. Bare
`redstone` prints the banner (canonical copy: `assets/banner.txt`).
