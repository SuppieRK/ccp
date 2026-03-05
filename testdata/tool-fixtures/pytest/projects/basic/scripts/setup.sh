#!/usr/bin/env bash
set -euo pipefail

if [[ ! -x .venv/bin/python ]]; then
  python3 -m venv .venv
fi

if [[ ! -x .venv/bin/pytest ]]; then
  ./.venv/bin/pip install -q pytest
fi
