#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

readonly BENCHMARK_MATRIX_SED_EXPR='s/^benchmark_matrix=//p'

fail() {
  echo "[planner-test] $*" >&2
  exit 1
  return 1
}

get_output_value() {
  local file="$1"
  local key="$2"
  sed -n "s/^${key}=//p" "$file"
  return 0
}

assert_equals() {
  local actual="$1"
  local expected="$2"
  local msg="$3"
  if [[ "$actual" != "$expected" ]]; then
    fail "${msg}: expected '${expected}', got '${actual}'"
  fi
  return 0
}

assert_tools() {
  local matrix="$1"
  local expected="$2"
  local actual
  actual="$(jq -r 'map(.tool) | sort | join(",")' <<<"$matrix")"
  if [[ "$actual" != "$expected" ]]; then
    fail "tool selection mismatch: expected '${expected}', got '${actual}'"
  fi
  return 0
}

assert_has_tool() {
  local matrix="$1"
  local expected="$2"
  if ! jq -e --arg tool "$expected" 'map(.tool) | index($tool) != null' <<<"$matrix" >/dev/null; then
    fail "expected tool '${expected}' to be selected"
  fi
  return 0
}

run_case() {
  local name="$1"
  local mode="$2"
  local expected_class="$3"
  local expected_validate="$4"
  local expected_bench="$5"
  shift 5

  local output_file
  output_file="$(mktemp)"
  trap 'rm -f "$output_file"' RETURN

  local args=(
    bash ./scripts/benchmark-discover.sh
    --mode "$mode"
    --event-name "$([[ "$mode" == "pr" ]] && echo pull_request || echo push)"
    --output-file "$output_file"
  )
  local changed
  for changed in "$@"; do
    args+=(--changed-file "$changed")
  done

  "${args[@]}"

  assert_equals "$(get_output_value "$output_file" change_class)" "$expected_class" "${name} change class"
  assert_equals "$(get_output_value "$output_file" run_validate)" "$expected_validate" "${name} run_validate"
  assert_equals "$(get_output_value "$output_file" run_benchmarks)" "$expected_bench" "${name} run_benchmarks"

  cat "$output_file"
  rm -f "$output_file"
  trap - RETURN
  return 0
}

filter_output="$(run_case "filter-only" "pr" "benchmark_only" "false" "true" "filters/go.yaml")"
assert_tools "$(printf '%s\n' "$filter_output" | sed -n "$BENCHMARK_MATRIX_SED_EXPR")" "go"

mappings_output="$(run_case "mappings" "pr" "benchmark_only" "false" "true" "filters/.mappings.yaml")"
assert_has_tool "$(printf '%s\n' "$mappings_output" | sed -n "$BENCHMARK_MATRIX_SED_EXPR")" "go"
assert_has_tool "$(printf '%s\n' "$mappings_output" | sed -n "$BENCHMARK_MATRIX_SED_EXPR")" "grep"

fixture_output="$(run_case "fixture-main" "main" "benchmark_only" "false" "true" "testdata/benchmarks/git/case/command.yaml")"
assert_tools "$(printf '%s\n' "$fixture_output" | sed -n "$BENCHMARK_MATRIX_SED_EXPR")" "git"

runtime_output="$(run_case "runtime" "pr" "full_ci" "true" "true" "internal/runner.go")"
assert_has_tool "$(printf '%s\n' "$runtime_output" | sed -n "$BENCHMARK_MATRIX_SED_EXPR")" "go"
assert_has_tool "$(printf '%s\n' "$runtime_output" | sed -n "$BENCHMARK_MATRIX_SED_EXPR")" "grep"

workflow_output="$(run_case "workflow" "pr" "full_ci" "true" "true" ".github/workflows/pr-validation.yml")"
assert_has_tool "$(printf '%s\n' "$workflow_output" | sed -n "$BENCHMARK_MATRIX_SED_EXPR")" "go"

none_output="$(run_case "none" "pr" "none" "false" "false" "assets/readme-banner.png")"
assert_tools "$(printf '%s\n' "$none_output" | sed -n "$BENCHMARK_MATRIX_SED_EXPR")" ""

echo "[planner-test] benchmark-discover routing checks passed"
