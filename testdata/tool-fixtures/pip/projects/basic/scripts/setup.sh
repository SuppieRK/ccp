#!/usr/bin/env bash
set -euo pipefail

if [[ ! -x .venv/bin/pip ]]; then
  python3 -m venv .venv
fi
