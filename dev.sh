#!/bin/sh
# ⚡ redstone task runner — every task runs in a container, nothing is ever
# installed on the host. Works from Git Bash (Windows), Linux, macOS.
set -eu

GO_IMG=golang:1.25-alpine
BUF_IMG=bufbuild/buf:1.71.0

# Git Bash mangles /paths in docker args; pwd -W gives the Windows form there.
WD="$(pwd -W 2>/dev/null || pwd)"
run_go() { MSYS_NO_PATHCONV=1 docker run --rm -v "$WD:/src" -w /src \
  -v redstone-go-cache:/go/pkg/mod -v redstone-go-build:/root/.cache/go-build -e CGO_ENABLED=0 "$GO_IMG" sh -c "$1"; }
run_buf() { MSYS_NO_PATHCONV=1 docker run --rm -v "$WD:/w" -w /w/proto "$BUF_IMG" "$@"; }

case "${1:-help}" in
  check)
    echo "⚡ pressure plate: fmt + vet + test"
    run_go 'test -z "$(gofmt -l . | grep -v ^gen/)" || { gofmt -l . | grep -v ^gen/; echo "🧨 gofmt: fix the files above"; exit 1; }
            go vet ./... && go test ./...'
    echo "⚡ all circuits power"
    ;;
  build)
    run_go 'go vet ./... && go test ./... &&
      GOOS=linux  GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/redstone-linux-amd64 ./cmd/redstone &&
      GOOS=linux  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/redstone-linux-arm64 ./cmd/redstone &&
      GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/redstone-windows-amd64.exe ./cmd/redstone'
    echo "⛏ binaries in bin/"
    ;;
  fmt)
    run_go 'gofmt -w cmd sdk'
    ;;
  proto)
    run_buf lint
    run_buf generate
    echo "⚡ proto linted + stubs regenerated"
    ;;
  smoke)
    REDSTONE_URL="${REDSTONE_URL:-http://localhost:8215}" sh scripts/smoke.sh
    ;;
  sdk)
    echo "⚡ js/ts sdk"
    MSYS_NO_PATHCONV=1 docker run --rm -v "$WD:/src" -w /src/sdk/js node:22-alpine sh -c \
      'npm install --no-audit --no-fund >/dev/null && node -e "import(\"./index.mjs\").then(m => { const c = m.createClient({register:\"localhost:1\", stack:\"dev\", app:\"smoke\"}); console.log(\"  loads, bind =\", typeof c.bind); c.close(); })" &&
       npx -y -p typescript@5.6 tsc --strict --noEmit --module nodenext --moduleResolution nodenext types-test.mts &&
       echo "  types hold (tsc --strict)"'
    echo "⚡ py sdk"
    MSYS_NO_PATHCONV=1 docker run --rm -v "$WD:/src" -w /src/sdk/py python:3.12-slim sh -c \
      'pip install --quiet . >/dev/null 2>&1 && python -c "import redstone_sdk; print(\"  loads, header =\", redstone_sdk.Binding(\"\",\"\",\"\",False,\"\",\"\",\"t\",\"dev/smoke/t\").header())"'
    echo "⚡ sdk circuits power (go sdk is gated by ./dev.sh check)"
    ;;
  hooks)
    git config core.hooksPath .githooks
    echo "⚡ pressure plate installed at the door (.githooks/pre-commit)"
    ;;
  help|*)
    echo "usage: ./dev.sh <check|build|fmt|proto|smoke|sdk|hooks>"
    ;;
esac
