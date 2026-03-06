#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"
export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$ROOT_DIR/.cache}"
mkdir -p "$GOCACHE" "$XDG_CACHE_HOME"
SUMMARY_OUT="${SUMMARY_OUT:-$ROOT_DIR/.artifacts/coverage/summary.md}"

warn_missing_tool() {
  local tool="$1"
  local install="$2"
  echo "[validate] warning: ${tool} not found; install with:" >&2
  echo "[validate]   ${install}" >&2
}

run_if_available() {
  local tool="$1"
  local install="$2"
  shift 2
  if command -v "$tool" >/dev/null 2>&1; then
    echo "[validate] $*"
    "$@"
    return 0
  fi
  warn_missing_tool "$tool" "$install"
}

mapfile -t GO_FILES < <(find cmd internal -name '*.go' | sort)
if [[ "${#GO_FILES[@]}" -gt 0 ]]; then
  echo "[validate] gofmt -w ${#GO_FILES[@]} files"
  gofmt -w "${GO_FILES[@]}"
fi

echo "[validate] go vet ./..."
go vet ./...

echo "[validate] go test -count=1 ./..."
go test -count=1 ./...

echo "[validate] go mod tidy"
go mod tidy

echo "[validate] go test -count=1 -race ./..."
go test -count=1 -race ./...

run_if_available \
  staticcheck \
  "go install honnef.co/go/tools/cmd/staticcheck@latest" \
  staticcheck ./...

run_if_available \
  ineffassign \
  "go install github.com/gordonklaus/ineffassign@latest" \
  ineffassign ./...

run_if_available \
  gocyclo \
  "go install github.com/fzipp/gocyclo/cmd/gocyclo@latest" \
  gocyclo -over 15 .

mkdir -p .artifacts/coverage
echo "[validate] go test -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=.artifacts/coverage/internal.cover ./..."
go test -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=.artifacts/coverage/internal.cover ./...

echo "[validate] go run ./cmd/coverage-gate -coverprofile .artifacts/coverage/internal.cover -module go-command-compression-proxy -internal-prefix internal/ -threshold 80 -summary-out ${SUMMARY_OUT}"
go run ./cmd/coverage-gate -coverprofile .artifacts/coverage/internal.cover -module go-command-compression-proxy -internal-prefix internal/ -threshold 80 -summary-out "$SUMMARY_OUT"
