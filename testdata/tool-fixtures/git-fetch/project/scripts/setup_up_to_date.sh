#!/usr/bin/env bash
set -euo pipefail
unset GIT_DIR GIT_WORK_TREE GIT_CEILING_DIRECTORIES
./scripts/cleanup.sh

mkdir -p tmp
git init >/dev/null
git config user.name "Bench User"
git config user.email "bench@example.com"
printf "seed\n" > tracked.txt
git add tracked.txt
git commit -m "seed" >/dev/null
git branch -M main

git init --bare tmp/remote.git >/dev/null
git remote add origin ./tmp/remote.git
git push -u origin main >/dev/null
