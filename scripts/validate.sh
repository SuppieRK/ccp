#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

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

run_validation() {
  local summary_out go_files go_file_count
  cd "$ROOT_DIR"
  export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"
  export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$ROOT_DIR/.cache}"
  mkdir -p "$GOCACHE" "$XDG_CACHE_HOME"
  summary_out="${SUMMARY_OUT:-$ROOT_DIR/.artifacts/coverage/summary.md}"

  go_files="$(collect_go_files)"
  if [[ -n "$go_files" ]]; then
    go_file_count="$(printf '%s\n' "$go_files" | wc -l | tr -d '[:space:]')"
    echo "[validate] gofmt -w ${go_file_count} files"
    while IFS= read -r go_file; do
      [[ -z "$go_file" ]] && continue
      gofmt -w "$go_file"
    done <<EOF
$go_files
EOF
  fi

  echo "[validate] go mod tidy"
  go mod tidy

  echo "[validate] bash ./scripts/test-benchmark-discover.sh"
  bash ./scripts/test-benchmark-discover.sh

  run_in_background "go vet ./..." go vet ./...
  run_in_background "go test -count=1 -race ./..." go test -count=1 -race ./...

  run_in_background \
    "go tool staticcheck ./..." \
    go tool staticcheck ./...

  #run_in_background \
  #  "gosec ./..." \
  #  run_if_available \
  #  gosec \
  #  "go install github.com/securego/gosec/v2/cmd/gosec@latest" \
  #  gosec ./...

  run_in_background \
    "go tool govulncheck ./..." \
    go tool govulncheck ./...

  run_in_background \
    "go tool golangci-lint run ./..." \
    go tool golangci-lint run ./...

  run_in_background \
    "go tool ineffassign ./..." \
    go tool ineffassign ./...

  run_in_background \
    "go tool gocyclo -over 15 ." \
    go tool gocyclo -over 15 .

  wait_for_background_jobs

  mkdir -p .artifacts/coverage
  echo "[validate] go test -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=.artifacts/coverage/internal.cover ./..."
  go test -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=.artifacts/coverage/internal.cover ./...

  echo "[validate] go run ./cmd/coverage-gate -coverprofile .artifacts/coverage/internal.cover -module go-command-compression-proxy -internal-prefix internal/ -threshold 80 -summary-out ${summary_out}"
  go run ./cmd/coverage-gate -coverprofile .artifacts/coverage/internal.cover -module go-command-compression-proxy -internal-prefix internal/ -threshold 80 -summary-out "$summary_out"
  return 0
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  run_validation "$@"
fi
