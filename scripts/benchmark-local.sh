#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURES_ROOT="${FIXTURES_ROOT:-$ROOT_DIR/testdata/tool-fixtures}"
MAX_ARTIFACT_BYTES="${MAX_ARTIFACT_BYTES:-5242880}"
BIN_DIR="$ROOT_DIR/.bin"
CCP_BIN="$BIN_DIR/ccp"
OVERRIDE=0

usage() {
  cat <<EOF
Usage: $0 [--override] [--tool <name>]

Modes:
  default     read scenarios from testdata/tool-fixtures and write artifacts to .artifacts/benchmark
  --override  read scenarios from testdata/tool-fixtures and overwrite artifacts in testdata/tool-fixtures

Options:
  --tool, -t        run only one tool folder under testdata/tool-fixtures (example: --tool ls)

Environment:
  FIXTURES_ROOT      scenario source root (default: \$ROOT_DIR/testdata/tool-fixtures)
  ARTIFACTS_DIR      artifact output root (default: depends on mode)
  MAX_ARTIFACT_BYTES max artifact size in bytes (default: 5242880)
EOF
  return 0
}

TOOL_NAME=""
while (($# > 0)); do
  case "$1" in
    --override)
      OVERRIDE=1
      ;;
    --tool|-t)
      if (($# < 2)); then
        echo "[benchmark] missing value for $1" >&2
        usage >&2
        exit 2
      fi
      TOOL_NAME="$2"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[benchmark] unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if [[ -n "$TOOL_NAME" ]]; then
  TOOL_FIXTURES_DIR="$FIXTURES_ROOT/$TOOL_NAME"
  if [[ ! -d "$TOOL_FIXTURES_DIR" ]]; then
    echo "[benchmark] tool fixtures not found: $TOOL_FIXTURES_DIR" >&2
    exit 2
  fi
fi

if [[ "$OVERRIDE" -eq 1 ]]; then
  if [[ -n "$TOOL_NAME" ]]; then
    ARTIFACTS_DIR="${ARTIFACTS_DIR:-$FIXTURES_ROOT/$TOOL_NAME}"
  else
    ARTIFACTS_DIR="${ARTIFACTS_DIR:-$FIXTURES_ROOT}"
  fi
else
  if [[ -n "$TOOL_NAME" ]]; then
    ARTIFACTS_DIR="${ARTIFACTS_DIR:-$ROOT_DIR/.artifacts/benchmark-$TOOL_NAME}"
  else
    ARTIFACTS_DIR="${ARTIFACTS_DIR:-$ROOT_DIR/.artifacts/benchmark}"
  fi
fi

mkdir -p "$BIN_DIR"
mkdir -p "$ARTIFACTS_DIR"

if [[ -n "$TOOL_NAME" ]]; then
  CLEAN_ROOT="$FIXTURES_ROOT/$TOOL_NAME"
else
  CLEAN_ROOT="$FIXTURES_ROOT"
fi
mapfile -t STALE_GAIN_DBS < <(find "$CLEAN_ROOT" -type f -path "*/.ccp/gain.db" | sort -u)
if [[ "${#STALE_GAIN_DBS[@]}" -gt 0 ]]; then
  echo "[benchmark] removing stale gain.db files under: $CLEAN_ROOT"
  for db in "${STALE_GAIN_DBS[@]}"; do
    rm -f "$db"
  done
fi

echo "[benchmark] building ccp -> $CCP_BIN"
GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}" go build -o "$CCP_BIN" "$ROOT_DIR/cmd/ccp"

echo "[benchmark] running ccp-ci"
echo "[benchmark] fixtures root: $FIXTURES_ROOT"
echo "[benchmark] artifacts dir: $ARTIFACTS_DIR"
if [[ -n "$TOOL_NAME" ]]; then
  echo "[benchmark] tool: $TOOL_NAME"
fi
(
  cd "$ROOT_DIR/tools/benchmark"
  CCI_ARGS=(
    -fixtures-root "$FIXTURES_ROOT"
    -artifacts-dir "$ARTIFACTS_DIR"
    -max-artifact-bytes "$MAX_ARTIFACT_BYTES"
  )
  if [[ -n "$TOOL_NAME" ]]; then
    CCI_ARGS+=(-tool "$TOOL_NAME")
  fi
  PATH="$BIN_DIR:$PATH" GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}" GITHUB_STEP_SUMMARY="" \
    go run ./cmd/ccp-ci "${CCI_ARGS[@]}"
)

echo "[benchmark] artifacts stored to: $ARTIFACTS_DIR"

echo "[benchmark] running ccp gain"
if [[ -n "$TOOL_NAME" ]]; then
  GAIN_SEARCH_ROOT="$FIXTURES_ROOT/$TOOL_NAME"
else
  GAIN_SEARCH_ROOT="$FIXTURES_ROOT"
fi

mapfile -t GAIN_DBS < <(find "$GAIN_SEARCH_ROOT" -type f -path "*/.ccp/gain.db" | sort -u)
if [[ "${#GAIN_DBS[@]}" -eq 0 ]]; then
  echo "[benchmark] no gain.db files found under: $GAIN_SEARCH_ROOT"
else
  for db in "${GAIN_DBS[@]}"; do
    project_dir="$(dirname "$(dirname "$db")")"
    rel_project="${project_dir#"$ROOT_DIR"/}"
    echo
    echo "[benchmark] ccp gain @ ${rel_project:-$project_dir}"
    (
      cd "$project_dir"
      PATH="$BIN_DIR:$PATH" "$CCP_BIN" gain
    )
  done
fi

if [[ -f "$ARTIFACTS_DIR/report.json" ]]; then
  rm -f "$ARTIFACTS_DIR/report.json"
  echo
  echo
  echo "[benchmark] removed transient report: $ARTIFACTS_DIR/report.json"
fi
