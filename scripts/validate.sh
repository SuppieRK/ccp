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
  return 0
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

declare -a PARALLEL_PIDS=()
declare -a PARALLEL_NAMES=()

run_in_background() {
  local name="$1"
  shift
  echo "[validate] $*"
  "$@" &
  PARALLEL_PIDS+=("$!")
  PARALLEL_NAMES+=("$name")
  return 0
}

wait_for_background_jobs() {
  local failed=0
  local index
  for index in "${!PARALLEL_PIDS[@]}"; do
    local pid="${PARALLEL_PIDS[$index]}"
    local name="${PARALLEL_NAMES[$index]}"
    if ! wait "$pid"; then
      echo "[validate] ${name} failed" >&2
      failed=1
    fi
  done

  PARALLEL_PIDS=()
  PARALLEL_NAMES=()

  if [[ "$failed" -ne 0 ]]; then
    return 1
  fi
  return 0
}

collect_go_files() {
  find cmd internal -name '*.go' | sort
  return 0
}

GO_FILES="$(collect_go_files)"
if [[ -n "$GO_FILES" ]]; then
  GO_FILE_COUNT="$(printf '%s\n' "$GO_FILES" | wc -l | tr -d '[:space:]')"
  echo "[validate] gofmt -w ${GO_FILE_COUNT} files"
  while IFS= read -r go_file; do
    [[ -z "$go_file" ]] && continue
    gofmt -w "$go_file"
  done <<EOF
$GO_FILES
EOF
fi

echo "[validate] go mod tidy"
go mod tidy

run_in_background "go vet ./..." go vet ./...
run_in_background "go test -count=1 -race ./..." go test -count=1 -race ./...

run_in_background \
  "staticcheck ./..." \
  run_if_available \
  staticcheck \
  "go install honnef.co/go/tools/cmd/staticcheck@latest" \
  staticcheck ./...

#run_in_background \
#  "gosec ./..." \
#  run_if_available \
#  gosec \
#  "go install github.com/securego/gosec/v2/cmd/gosec@latest" \
#  gosec ./...

run_in_background \
  "govulncheck ./..." \
  run_if_available \
  govulncheck \
  "go install golang.org/x/vuln/cmd/govulncheck@latest" \
  govulncheck ./...

run_in_background \
  "golangci-lint run ./..." \
  run_if_available \
  golangci-lint \
  "curl --proto \"=https\" -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.11.3" \
  golangci-lint run ./...

run_in_background \
  "ineffassign ./..." \
  run_if_available \
  ineffassign \
  "go install github.com/gordonklaus/ineffassign@latest" \
  ineffassign ./...

run_in_background \
  "gocyclo -over 15 ." \
  run_if_available \
  gocyclo \
  "go install github.com/fzipp/gocyclo/cmd/gocyclo@latest" \
  gocyclo -over 15 .

wait_for_background_jobs

mkdir -p .artifacts/coverage
echo "[validate] go test -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=.artifacts/coverage/internal.cover ./..."
go test -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=.artifacts/coverage/internal.cover ./...

echo "[validate] go run ./cmd/coverage-gate -coverprofile .artifacts/coverage/internal.cover -module go-command-compression-proxy -internal-prefix internal/ -threshold 80 -summary-out ${SUMMARY_OUT}"
go run ./cmd/coverage-gate -coverprofile .artifacts/coverage/internal.cover -module go-command-compression-proxy -internal-prefix internal/ -threshold 80 -summary-out "$SUMMARY_OUT"
