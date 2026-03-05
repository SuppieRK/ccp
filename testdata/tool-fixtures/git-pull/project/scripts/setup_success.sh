#!/usr/bin/env bash
set -euo pipefail

unset GIT_DIR GIT_WORK_TREE GIT_CEILING_DIRECTORIES

./scripts/cleanup.sh

mkdir -p tmp

# Create local repository where `git pull` will be executed.
git init >/dev/null
git config user.name "Bench User"
git config user.email "bench@example.com"
cat > tracked.txt <<'EOF'
line 1
EOF
git add tracked.txt
git commit -m "seed" >/dev/null
git branch -M main

# Create origin and push initial commit.
git init --bare remote.git >/dev/null
git remote add origin ./remote.git
git push -u origin main >/dev/null
