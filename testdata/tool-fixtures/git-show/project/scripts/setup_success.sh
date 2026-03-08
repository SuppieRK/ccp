#!/usr/bin/env bash
set -euo pipefail

./scripts/cleanup.sh

git init >/dev/null
git config user.email bench@example.com
git config user.name bench
git branch -M main

echo 'v1' > tracked.txt
git add tracked.txt
git commit -m 'initial commit' >/dev/null

echo 'v2' > tracked.txt
git add tracked.txt
git commit -m 'second commit' >/dev/null

echo 'v3' > tracked.txt
git add tracked.txt
git commit -m 'third commit' >/dev/null
