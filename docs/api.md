●━━━●━━⚡━━●━━━●

# API

Wire rules: JSON field names are the proto field names (snake_case). The
protocol namespace is `redstone.core.v1` ([proto/](../proto/)); breaking
changes belong to a future `v2` package, never to edits of `v1`. Humans get
sparks, machines get JSON — flavor never appears in API responses.

## Bind

```
GET /bind?capability=&level=[&name=][&label.k=v][&stack=][&consumer=][&as=]
```

With `stack`+`consumer`+`as` matching a declared use, `capability` and
`level` may be omitted (inherited from the declaration).

**200** — the instance:

```json
{
  "name": "minio-local",
  "capability": "blob",
  "requested_level": "mutable",
  "effective_level": "mutable",
  "verified": true,
  "endpoint": "http://connector-s3",
  "public": "http://localhost:8110",
  "implementation": "s3",
  "flags": { "direct_read": true, "direct_write": true }
}
```

**409** — nothing satisfies; every candidate's reason listed:

```json
{
  "error": "no instance satisfies blob@mutable",
  "candidates": [
    { "name": "ipfs-overclaimed",
      "reason": "claims mutable, conformance verified only core" }
  ]
}
```

**400** — malformed request, unknown level, config drift (declaration vs
request), or `--strict` violations. Body: `{ "error": "..." }`.

## Catalog, graph, verdicts, edges

| Route | Returns |
|---|---|
| `GET /instances` | catalog merged with verdicts: `{instances: [{name, capability, level, endpoint, public, labels, check?, stack, verified, effective_level}]}` |
| `GET /graph` | declared edges (with live resolution, `at: "declared"`, wire sub-edges as `task/wiretask` paths) + bound edges, latest-wins per (stack\|consumer\|as) |
| `GET /health` | `{ "ok": true }` |
| `GET /svc/registry/edge?stack=&app=&task=` | the bound edge incl. `with` (verbatim) and resolved `wire` — what a shared adapter pulls for a caller; `404` if never bound |
| `GET /svc/{registry,graph,conformance}/manifest` | single-capability manifest (`kind: "core"`) |
| `GET /svc/registry/bind`, `/svc/registry/instances`, `/svc/graph/edges`, `/svc/conformance/verdicts` | core-capability bases (root routes are aliases) |

Edge shape (also the `graph.jsonl` line format):

```json
{ "stack": "prod", "consumer": "admin", "as": "blob",
  "capability": "blob", "level": "mutable", "instance": "minio-local",
  "verified": true, "at": "2026-08-15T14:23:00.000Z",
  "with": { "space": "admin" },
  "wire": { "auth": { "name": "keycloak-oidc", "endpoint": "…", "verified": false } } }
```

## gRPC

`redstone.core.v1.RegisterService` on `:50051` (plaintext):

| RPC | Maps to |
|---|---|
| `Bind(BindRequest) → BindResponse` | `/bind`; refusal = `FAILED_PRECONDITION`, details = the 409 JSON; `INVALID_ARGUMENT` = the 400 |
| `ListInstances` | `/instances` |
| `GetGraph` | `/graph` |

`BindRequest`: capability, level, name, labels (map), consumer, as, stack.
Clients: any generated stub, or dynamic loading of
`proto/redstone/core/v1/core.proto` (Go/Node proven; keep `keepCase`-style
field naming — snake_case is the wire).

## Manifest contract (what instances serve)

Anything with `check: manifest` (the default) must answer `GET /manifest`:

```json
{ "name": "s3-connector", "kind": "connector", "capability": "blob",
  "level": "mutable", "implementation": "s3", "flags": { } }
```

Only `capability` is cross-checked at bind time — `level` is informational
(the instance's claim lives in its stack file; verify is the authority).
