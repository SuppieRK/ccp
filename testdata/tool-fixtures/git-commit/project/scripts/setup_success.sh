#!/usr/bin/env bash
set -euo pipefail

./scripts/cleanup.sh

git init >/dev/null
git config user.email bench@example.com
git config user.name bench
git branch -M main
