#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"
export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$ROOT_DIR/.cache}"
mkdir -p "$GOCACHE" "$XDG_CACHE_HOME" "$ROOT_DIR/.bin"

build_ccp() {
  echo "[benchmark-local] go build -o .bin/ccp ./cmd/ccp"
  go build -o .bin/ccp ./cmd/ccp
  return 0
}

collect_tools() {
  local filter_path
  for filter_path in filters/*.yaml; do
    [[ -f "$filter_path" ]] || continue
    [[ "$filter_path" == "filters/.mappings.yaml" ]] && continue
    basename "${filter_path%.yaml}"
  done | sort
  return 0
}

resolve_tools() {
  local -a available_tools
  mapfile -t available_tools < <(collect_tools)
  if [[ ${#available_tools[@]} -eq 0 ]]; then
    echo "no benchmark tools found from filters/*.yaml" >&2
    exit 1
  fi

  if [[ $# -gt 0 ]]; then
    local tool
    for tool in "$@"; do
      if [[ ! -f "filters/${tool}.yaml" ]]; then
        echo "unknown benchmark tool: ${tool}" >&2
        exit 1
      fi
      printf '%s\n' "$tool"
    done
    return 0
  fi

  printf '%s\n' "${available_tools[@]}"
}

run_tool_benchmark() {
  local tool="$1"
  local out_dir="$ROOT_DIR/.artifacts/benchmark/${tool}"
  local history_dir="$ROOT_DIR/.artifacts/benchmark-history/${tool}"
  local prev_report="$history_dir/report.json"

  mkdir -p "$out_dir" "$history_dir"

  echo "[benchmark-local] running ${tool}"
  go run ./cmd/ccp-ci \
    -artifacts-dir "$out_dir" \
    -previous-report "$prev_report" \
    -tool "$tool"

  if [[ -f "${out_dir}/report.json" ]]; then
    cp "${out_dir}/report.json" "$prev_report"
  fi
  return 0
}

build_ccp
export PATH="$ROOT_DIR/.bin:$PATH"

mapfile -t TOOLS < <(resolve_tools "$@")
if [[ ${#TOOLS[@]} -eq 0 ]]; then
  echo "no benchmark tools found under testdata/benchmarks" >&2
  exit 1
fi

for tool in "${TOOLS[@]}"; do
  run_tool_benchmark "$tool"
done
