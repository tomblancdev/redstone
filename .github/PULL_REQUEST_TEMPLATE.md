## What this wires

<!-- One paragraph. Link the issue if one exists. -->

## Pressure plate

- [ ] `./dev.sh check` passes (CI reruns it; the gate is not optional)
- [ ] New bind-path behavior has a network-free test (`check: none` fixtures)
- [ ] Commits are conventional and readable — **they become the changelog**
- [ ] Humans get sparks, machines get JSON — no flavor in API responses,
      refusal reasons, or the tokens `claimed`/`OK`/`DOWNGRADED`/`FAIL`
- [ ] `redstone.core.v1` untouched (additive only) — or this PR proposes
      `v2` and says so loudly
- [ ] The kernel did not grow a feature (or: this really couldn't be a
      brick, explained below)
