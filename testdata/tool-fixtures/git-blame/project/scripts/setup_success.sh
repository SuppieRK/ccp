#!/usr/bin/env bash
set -euo pipefail

./scripts/cleanup.sh

git init >/dev/null
git config user.email bench@example.com
git config user.name bench

printf 'blame-line-1\nblame-line-2\n' > tracked.txt
git add tracked.txt
GIT_AUTHOR_DATE="2024-01-01T00:00:00Z" \
GIT_COMMITTER_DATE="2024-01-01T00:00:00Z" \
  git commit -m "seed" >/dev/null
