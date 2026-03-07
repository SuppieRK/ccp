#!/usr/bin/env bash
set -euo pipefail

mode=""
event_name=""
base_sha=""
head_sha=""
output_file=""
summary_file=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      mode="${2:-}"
      shift 2
      ;;
    --event-name)
      event_name="${2:-}"
      shift 2
      ;;
    --base-sha)
      base_sha="${2:-}"
      shift 2
      ;;
    --head-sha)
      head_sha="${2:-}"
      shift 2
      ;;
    --output-file)
      output_file="${2:-}"
      shift 2
      ;;
    --summary-file)
      summary_file="${2:-}"
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "$mode" || -z "$output_file" ]]; then
  echo "usage: benchmark-discover.sh --mode <main|pr> --output-file <path> [--event-name <name>] [--base-sha <sha>] [--head-sha <sha>] [--summary-file <path>]" >&2
  exit 1
fi

mapfile -t all_tools < <(find testdata/tool-fixtures -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort)
if [[ ${#all_tools[@]} -eq 0 ]]; then
  echo "No benchmark tools found under testdata/tool-fixtures" >&2
  exit 1
fi

tool_matrix_entry() {
  local tool="$1"
  local needs_python=false
  local needs_java=false
  local needs_rust=false
  local needs_deno=false
  local needs_node=false
  local needs_pnpm=false
  local needs_kind=false
  local needs_docker_images=false

  case "$tool" in
    pip|pytest)
      needs_python=true
      ;;
    gradle|maven)
      needs_java=true
      ;;
    cargo-*)
      needs_rust=true
      ;;
    deno)
      needs_deno=true
      ;;
    node|npm|pnpm|yarn|npx-*)
      needs_node=true
      ;;
  esac

  if [[ "$tool" == "pnpm" ]]; then
    needs_pnpm=true
  fi

  if [[ "$tool" == kubectl-* ]]; then
    needs_kind=true
  fi

  if [[ "$tool" == docker-* ]]; then
    needs_docker_images=true
  fi

  jq -nc \
    --arg tool "$tool" \
    --argjson needs_python "$needs_python" \
    --argjson needs_java "$needs_java" \
    --argjson needs_rust "$needs_rust" \
    --argjson needs_deno "$needs_deno" \
    --argjson needs_node "$needs_node" \
    --argjson needs_pnpm "$needs_pnpm" \
    --argjson needs_kind "$needs_kind" \
    --argjson needs_docker_images "$needs_docker_images" \
    '{
      tool: $tool,
      needs_python: $needs_python,
      needs_java: $needs_java,
      needs_rust: $needs_rust,
      needs_deno: $needs_deno,
      needs_node: $needs_node,
      needs_pnpm: $needs_pnpm,
      needs_kind: $needs_kind,
      needs_docker_images: $needs_docker_images
    }'
}

build_matrix_json() {
  local tool
  for tool in "$@"; do
    tool_matrix_entry "$tool"
  done | jq -s -c '.'
}

if [[ "$mode" == "main" ]]; then
  benchmark_matrix=$(build_matrix_json "${all_tools[@]}")
  {
    echo "benchmark_matrix=${benchmark_matrix}"
  } >> "$output_file"
  exit 0
fi

if [[ "$mode" != "pr" ]]; then
  echo "unsupported mode: $mode" >&2
  exit 1
fi

declare -A selected=()
run_all=0
changed=()

add_prefix() {
  local prefix="$1"
  local t
  for t in "${all_tools[@]}"; do
    if [[ "$t" == "$prefix" || "$t" == "$prefix"-* ]]; then
      selected["$t"]=1
    fi
  done
}

add_exact_if_exists() {
  local exact="$1"
  local t
  for t in "${all_tools[@]}"; do
    if [[ "$t" == "$exact" ]]; then
      selected["$t"]=1
      return
    fi
  done
}

if [[ "$event_name" == "pull_request" ]]; then
  if [[ -z "$base_sha" || -z "$head_sha" ]]; then
    echo "pull_request mode requires --base-sha and --head-sha" >&2
    exit 1
  fi
  mapfile -t changed < <(git diff --name-only "$base_sha" "$head_sha")
fi

if [[ ${#changed[@]} -eq 0 && "$event_name" != "pull_request" ]]; then
  run_all=1
fi

for path in "${changed[@]}"; do
  [[ -z "$path" ]] && continue
  case "$path" in
    testdata/tool-fixtures/*)
      tool="${path#testdata/tool-fixtures/}"
      tool="${tool%%/*}"
      add_exact_if_exists "$tool"
      ;;
    openspec/specs/*/spec.md)
      cap="${path#openspec/specs/}"
      cap="${cap%%/*}"
      add_exact_if_exists "$cap"
      ;;
    internal/engine/filters/*)
      rel="${path#internal/engine/filters/}"
      first="${rel%%/*}"
      first="${first%%.*}"
      case "$first" in
        cargo|docker|git|go|kubectl|npx)
          add_prefix "$first"
          ;;
        *)
          add_exact_if_exists "$first"
          ;;
      esac
      ;;
    cmd/ccp/*|internal/runner/*|internal/engine/*|internal/metrics/*|tools/benchmark/*|go.mod|go.sum)
      run_all=1
      ;;
  esac
done

if [[ "$run_all" -eq 1 ]]; then
  benchmark_matrix=$(build_matrix_json "${all_tools[@]}")
  has_tools=true
elif [[ ${#selected[@]} -gt 0 ]]; then
  mapfile -t selected_tools < <(printf "%s\n" "${!selected[@]}" | sort)
  benchmark_matrix=$(build_matrix_json "${selected_tools[@]}")
  has_tools=true
else
  benchmark_matrix="[]"
  has_tools=false
fi

{
  echo "benchmark_matrix=${benchmark_matrix}"
  echo "has_tools=${has_tools}"
} >> "$output_file"

if [[ -n "$summary_file" ]]; then
  {
    echo "## PR Benchmark Tool Selection"
    echo ""
    echo "- Changed files analyzed: ${#changed[@]}"
    echo "- Selected tools: \`$(jq -c 'map(.tool)' <<< "${benchmark_matrix}")\`"
  } >> "$summary_file"
fi
