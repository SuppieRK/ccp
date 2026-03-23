#!/usr/bin/env bash
set -euo pipefail

normalize_output() {
  printf '%s' "$1" | tr -d '\r'
}

runner_os="${RUNNER_OS:-}"
ccp_bin="ccp"
pass_cmd=(sh -c "printf 'smoke-filtered\\n'")
raw_cmd=(sh -c "printf 'smoke-raw\\n'")
blocked_cmd=(sh -c "printf 'smoke-audit\\n'")
fail_cmd=(sh -c "exit 7")

if [[ "$runner_os" == "Windows" ]]; then
  ccp_bin="ccp.exe"
  pass_cmd=(cmd /c echo smoke-filtered)
  raw_cmd=(cmd /c echo smoke-raw)
  blocked_cmd=(cmd /c echo smoke-audit)
  fail_cmd=(cmd /c exit 7)
fi

if ! command -v "$ccp_bin" >/dev/null 2>&1; then
  echo "installed $ccp_bin not found on PATH" >&2
  exit 1
fi

workspace="$(mktemp -d)"
trap 'rm -rf "$workspace"' EXIT

pushd "$workspace" >/dev/null

version_out="$($ccp_bin --version)"
if [[ -z "${version_out}" ]]; then
  echo "ccp --version returned empty output" >&2
  exit 1
fi

filtered_out="$($ccp_bin "${pass_cmd[@]}")"
if [[ "$(normalize_output "$filtered_out")" != "smoke-filtered" ]]; then
  echo "unexpected filtered smoke output: $filtered_out" >&2
  exit 1
fi

raw_out="$($ccp_bin --raw "${raw_cmd[@]}")"
if [[ "$(normalize_output "$raw_out")" != "smoke-raw" ]]; then
  echo "unexpected raw smoke output: $raw_out" >&2
  exit 1
fi

blocked_home="$workspace/blocked-home"
mkdir -p "$blocked_home"
printf 'block' > "$blocked_home/.config"
blocked_out="$(HOME="$blocked_home" USERPROFILE="$blocked_home" $ccp_bin "${blocked_cmd[@]}")"
if [[ "$(normalize_output "$blocked_out")" != "smoke-audit" ]]; then
  echo "unexpected blocked-audit smoke output: $blocked_out" >&2
  exit 1
fi

set +e
$ccp_bin "${fail_cmd[@]}"
status=$?
set -e
if [[ $status -ne 7 ]]; then
  echo "expected ccp to preserve exit code 7, got $status" >&2
  exit 1
fi

popd >/dev/null
