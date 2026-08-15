

---

```text
●━━━●━━⚡━━●  the wiring layer  ●━━⚡━━●━━━●
```

### Get it

Grab the binary for your platform below, then:

```sh
chmod +x redstone-linux-amd64          # (not needed on Windows)
./redstone-linux-amd64 version         # expect this release's number + a splash
sha256sum -c checksums.txt --ignore-missing   # trust, but verify
```

New world? `redstone validate` before `redstone serve` — typos are errors,
not silence. Docs: [README](../../blob/main/README.md) ·
[quickstart](../../blob/main/README.md#quickstart) ·
[stack spec](../../blob/main/docs/stack-spec.yaml).

*The notes above were written by the commits themselves — humans get
sparks, machines get JSON.* ⚡
