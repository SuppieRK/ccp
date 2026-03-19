#!/usr/bin/env sh
set -eu

# Installs ccp from GitHub Releases.
#
# Usage:
#   curl --proto "=https" --tlsv1.2 -sSfL https://raw.githubusercontent.com/SuppieRK/ccp/main/scripts/install.sh | sh
#   curl --proto "=https" --tlsv1.2 -sSfL ... | VERSION=0.1.0 sh
#
# Env:
#   VERSION    Release tag (default: latest)
# Installer behavior:
#   - Repository is fixed to SuppieRK/ccp.
#   - Install directory is selected automatically:
#       1) /usr/local/bin (if writable)
#       2) $HOME/.local/bin
#       3) ./bin

REPO="SuppieRK/ccp"
VERSION="${VERSION:-latest}"
BIN_NAME="ccp"
PROFILE_NOTE="# added by ccp installer"
REPAIR_CUTOFF_VERSION="0.5.1"
curl_secure() {
  curl --proto "=https" --tlsv1.2 -sSfL "$@"
  return 0
}

need_cmd() {
  cmd="$1"
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "missing required command: $cmd" >&2
    exit 1
  }
  return 0
}

need_cmd uname
need_cmd curl
need_cmd unzip

normalize_release_version() {
  ver="$(printf '%s' "$1" | tr -d '\r\n')"
  ver="${ver#v}"
  ver="${ver%%-*}"
  ver="${ver%%+*}"
  case "$ver" in
    ''|*[!0-9.]*) return 1 ;;
  esac
  printf '%s' "$ver"
  return 0
}

version_lt_cutoff() {
  ver="$(normalize_release_version "$1")" || return 1
  cutoff="$(normalize_release_version "$REPAIR_CUTOFF_VERSION")" || return 1

  old_ifs="$IFS"
  IFS=.
  set -- $ver
  [ "$#" -eq 3 ] || return 1
  ver_major="${1:-}" ver_minor="${2:-}" ver_patch="${3:-}"
  set -- $cutoff
  [ "$#" -eq 3 ] || return 1
  cutoff_major="${1:-}" cutoff_minor="${2:-}" cutoff_patch="${3:-}"
  IFS="$old_ifs"

  case "$ver_major:$ver_minor:$ver_patch:$cutoff_major:$cutoff_minor:$cutoff_patch" in
    *::*) return 1 ;;
    *[!0-9:]*) return 1 ;;
  esac

  if [ "$ver_major" -lt "$cutoff_major" ]; then
    return 0
  fi
  if [ "$ver_major" -gt "$cutoff_major" ]; then
    return 1
  fi
  if [ "$ver_minor" -lt "$cutoff_minor" ]; then
    return 0
  fi
  if [ "$ver_minor" -gt "$cutoff_minor" ]; then
    return 1
  fi
  if [ "$ver_patch" -lt "$cutoff_patch" ]; then
    return 0
  fi
  return 1
}

probe_installed_version() {
  candidate="$1"
  if [ -n "$candidate" ] && [ -x "$candidate" ]; then
    "$candidate" --version 2>/dev/null || true
    return 0
  fi
  return 1
}

choose_install_dir() {
  if [ -w "/usr/local/bin" ]; then
    echo "/usr/local/bin"
    return 0
  fi
  if [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
    echo "$HOME/.local/bin"
    return 0
  fi
  if [ -d "./bin" ] || mkdir -p "./bin" 2>/dev/null; then
    # Keep fallback deterministic and local to current directory.
    echo "$(pwd)/bin"
    return 0
  fi
  echo "failed to determine writable install directory" >&2
  exit 1
}

path_contains_dir() {
  dir="$1"
  case ":$PATH:" in
    *":$dir:"*) return 0 ;;
    *) return 1 ;;
  esac
}

append_path_export_once() {
  conf_file="$1"
  install_dir="$2"
  if [ -f "$conf_file" ] && grep -Fqs "$install_dir" "$conf_file"; then
    echo "PATH already configured in $conf_file"
    return 0
  fi
  if [ ! -f "$conf_file" ]; then
    : > "$conf_file"
  fi
  {
    echo ""
    echo "$PROFILE_NOTE"
    echo "export PATH=\"\$PATH:$install_dir\""
  } >> "$conf_file"
  echo "Added $install_dir to $conf_file"
  return 0
}

update_path_if_needed() {
  install_dir="$1"
  shell_name=""
  conf_file=""
  case "$install_dir" in
    /usr/local/bin) return 0 ;;
    *) ;;
  esac
  if path_contains_dir "$install_dir"; then
    return 0
  fi

  shell_name="${SHELL:-}"
  case "$shell_name" in
    */zsh)
      append_path_export_once "$HOME/.zshrc" "$install_dir"
      echo "Run: source \"$HOME/.zshrc\""
      ;;
    */bash)
      if [ "$(uname -s)" = "Darwin" ]; then
        conf_file="$HOME/.bash_profile"
      else
        conf_file="$HOME/.bashrc"
      fi
      append_path_export_once "$conf_file" "$install_dir"
      echo "Run: source \"$conf_file\""
      ;;
    */fish)
      if command -v fish >/dev/null 2>&1; then
        fish -c "fish_add_path '$install_dir'" >/dev/null 2>&1 || true
        echo "Added $install_dir to fish PATH configuration"
      fi
      ;;
    *)
      append_path_export_once "$HOME/.profile" "$install_dir"
      echo "Run: source \"$HOME/.profile\""
      ;;
  esac
  return 0
}

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "unsupported arch: $ARCH" >&2
    exit 1
    ;;
esac

case "$OS" in
  linux) OS="linux" ;;
  darwin) OS="darwin" ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *)
    echo "unsupported os: $OS" >&2
    exit 1
    ;;
esac

if [ "$VERSION" = "latest" ]; then
  API_URL="https://api.github.com/repos/$REPO/releases/latest"
  VERSION="$(curl_secure "$API_URL" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' | head -n1)"
  if [ -z "$VERSION" ]; then
    echo "failed to resolve latest version from $API_URL" >&2
    exit 1
  fi
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

ASSET="${BIN_NAME}_${VERSION}_${OS}_${ARCH}.zip"
URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"

echo "Downloading $URL"
curl_secure "$URL" -o "$TMP_DIR/$ASSET"

unzip -oq "$TMP_DIR/$ASSET" -d "$TMP_DIR"

if [ "$OS" = "windows" ]; then
  SRC="$TMP_DIR/${BIN_NAME}.exe"
  INSTALL_DIR="$(choose_install_dir)"
  DST="$INSTALL_DIR/${BIN_NAME}.exe"
else
  SRC="$TMP_DIR/$BIN_NAME"
  INSTALL_DIR="$(choose_install_dir)"
  DST="$INSTALL_DIR/$BIN_NAME"
fi

PREVIOUS_VERSION=""
if [ -x "$DST" ]; then
  PREVIOUS_VERSION="$(probe_installed_version "$DST")"
else
  EXISTING_BIN="$(command -v "$BIN_NAME" 2>/dev/null || true)"
  PREVIOUS_VERSION="$(probe_installed_version "$EXISTING_BIN")"
fi

if [ ! -f "$SRC" ]; then
  echo "archive did not contain expected binary: $SRC" >&2
  exit 1
fi

if [ "$OS" != "windows" ]; then
  chmod +x "$SRC"
fi

if ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
  echo "install directory is not writable: $INSTALL_DIR" >&2
  exit 1
fi

install -m 0755 "$SRC" "$DST"
update_path_if_needed "$INSTALL_DIR"
echo "Installed $BIN_NAME $VERSION to $DST"

if version_lt_cutoff "$PREVIOUS_VERSION"; then
  if REPAIR_OUTPUT="$("$DST" repair --yes 2>&1)"; then
    printf '%s\n' "$REPAIR_OUTPUT"
  else
    case "$REPAIR_OUTPUT" in
      *"executable file not found"*|*"not found"*|*"Usage:"*)
        echo "Installed binary does not support 'ccp repair'; skipping managed state rewrite"
        ;;
      *)
        printf '%s\n' "$REPAIR_OUTPUT" >&2
        echo "ccp repair failed after install" >&2
        exit 1
        ;;
    esac
  fi
fi
