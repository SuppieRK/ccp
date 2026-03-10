#!/usr/bin/env bash
set -euo pipefail

./scripts/cleanup.sh

git init >/dev/null
git config user.email bench@example.com
git config user.name bench
printf 'base\n' > tracked.txt
git add tracked.txt
git commit -m "seed" >/dev/null
git worktree add ../wt-feature -b feature >/dev/null
