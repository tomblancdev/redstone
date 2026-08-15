●━━━●━━⚡━━●━━━●

# Install & update

Redstone installs the way docker does: **one static binary on the machine,
on your `PATH`**. No runtime, no package manager, no migrations — state is
files the binary reads next tick. A docker image exists too, but it is the
alternative, not the default: the wiring layer usually belongs *on* the
host, next to the things it wires.

## Install from a release build

Every release ships static builds — `redstone-linux-amd64`,
`redstone-linux-arm64`, `redstone-windows-amd64.exe` — plus a
`checksums.txt`. Grab, verify, place on `PATH`, done:
<https://github.com/tomblancdev/redstone/releases>

### The one-liner

Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/tomblancdev/redstone/main/scripts/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/tomblancdev/redstone/main/scripts/install.ps1 | iex
```

Latest release by default, always verified against `checksums.txt` before
it is placed. Pin a version or pick the directory:

```sh
curl -fsSL .../install.sh | sh -s -- --version v0.1.0
REDSTONE_INSTALL=$HOME/.local/bin  curl -fsSL .../install.sh | sh
```

```powershell
$env:REDSTONE_VERSION = "v0.1.0"; irm .../install.ps1 | iex
$env:REDSTONE_INSTALL = "C:\Tools"; irm .../install.ps1 | iex   # custom dirs are left off PATH
```

The linux script installs to `/usr/local/bin` (sudo only if needed); the
windows script installs to `%LOCALAPPDATA%\Programs\redstone` and adds it
to your user `PATH`.

### By hand — Linux

```sh
V=v0.1.0    # pick a version from the releases page
curl -fsSLO "https://github.com/tomblancdev/redstone/releases/download/$V/redstone-linux-amd64"
curl -fsSLO "https://github.com/tomblancdev/redstone/releases/download/$V/checksums.txt"
sha256sum --check --ignore-missing checksums.txt      # redstone-linux-amd64: OK
sudo install -m 0755 redstone-linux-amd64 /usr/local/bin/redstone
redstone version
```

arm64: same steps with `redstone-linux-arm64`.

### By hand — Windows (PowerShell)

```powershell
$V = "v0.1.0"
irm "https://github.com/tomblancdev/redstone/releases/download/$V/redstone-windows-amd64.exe" -OutFile redstone.exe
irm "https://github.com/tomblancdev/redstone/releases/download/$V/checksums.txt" -OutFile checksums.txt
(Get-FileHash redstone.exe).Hash                       # compare with checksums.txt
# then put redstone.exe in a folder that is on $env:PATH
.\redstone.exe version
```

### With a Go toolchain

```sh
go install github.com/tomblancdev/redstone/cmd/redstone@latest   # or @vX.Y.Z
```

## Docker (the alternative)

`ghcr.io/tomblancdev/redstone` — `FROM scratch`, just the kernel, amd64 +
arm64, published from `v0.2.0` onward. Tags: one per release (`0.2.0`) plus
`latest`. Mount your state, pass the same flags as anywhere else:

```sh
docker run --rm \
  -v "$PWD/registry:/registry" -v "$PWD/capabilities:/capabilities" \
  -p 8215:8215 -p 8216:8216 \
  ghcr.io/tomblancdev/redstone:latest \
  serve --dir /registry --caps /capabilities --http :8215 --grpc :8216 --strict
```

The CLI works the same way (state is files, no daemon needed):

```sh
docker run --rm -v "$PWD/registry:/registry" -v "$PWD/capabilities:/capabilities" \
  ghcr.io/tomblancdev/redstone:latest validate --registry /registry --caps /capabilities
```

## First power-on

```sh
redstone validate --registry ./registry --caps ./capabilities
redstone verify   --registry ./registry --caps ./capabilities
redstone serve    --dir ./registry --caps ./capabilities --http :8215 --grpc :8216 --strict
```

Every command and flag: [cli.md](cli.md). What the files mean:
[concepts.md](concepts.md).

## Update

An update is a **binary swap**. State (`stacks/`, `conformance.yaml`,
`graph.jsonl`) lives outside the binary and is never touched by an install
— there is nothing to migrate.

1. Read the release notes ([CHANGELOG](../CHANGELOG.md)) for the jump you
   are making.
2. Download the new build and verify checksums — same steps as install.
   **The install script is also the updater**: run the one-liner again and
   it fetches the latest (or `--version`) and swaps the binary in place.
3. Stop `redstone serve`, replace the binary, start it again.
4. Confirm the circuit: `redstone version`, then `redstone validate`
   (still lints clean?) and `redstone verify` (verdicts corrode — re-test
   claims on every upgrade).

**Rollback** is the same move in reverse: keep the previous binary around
(`redstone-0.1.0`) and swap back. State is files; both directions are safe.

- docker: `docker pull ghcr.io/tomblancdev/redstone:X.Y.Z` and recreate the
  container — pin versions; `latest` is for the curious.
- go: `go install github.com/tomblancdev/redstone/cmd/redstone@vX.Y.Z`

## Versioning

SemVer. The wire protocol (`redstone.core.v1`) is frozen — a breaking wire
change means a new namespace (`v2`) and a major version, announced loudly.
*Watch → Custom → Releases* on GitHub to hear about new ones.
