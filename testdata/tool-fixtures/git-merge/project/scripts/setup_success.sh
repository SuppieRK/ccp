#!/usr/bin/env bash
set -euo pipefail

./scripts/cleanup.sh

git init >/dev/null
git config user.email bench@example.com
git config user.name bench
git branch -M main

echo 'base' > tracked.txt
git add tracked.txt
git commit -m 'init' >/dev/null

git checkout -b feature >/dev/null
echo 'feature' >> tracked.txt
git add tracked.txt
git commit -m 'feature' >/dev/null

git checkout main >/dev/null
