#!/usr/bin/env bash
set -euo pipefail
unset GIT_DIR GIT_WORK_TREE GIT_CEILING_DIRECTORIES
./scripts/setup_up_to_date.sh

git clone ./tmp/remote.git ./tmp/seed >/dev/null 2>&1
cd ./tmp/seed
git config user.name "Bench User"
git config user.email "bench@example.com"
git checkout main >/dev/null 2>&1 || git checkout -b main >/dev/null
printf "next\n" >> tracked.txt
git add tracked.txt
git commit -m "next" >/dev/null
git push origin HEAD:main >/dev/null
for branch in feature-a feature-b feature-c; do
  git branch "$branch"
  git push origin "$branch" >/dev/null
done
git tag v2
git tag v3
git push origin v2 v3 >/dev/null
