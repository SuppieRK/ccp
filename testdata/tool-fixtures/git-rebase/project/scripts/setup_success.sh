#!/usr/bin/env bash
set -euo pipefail
unset GIT_DIR GIT_WORK_TREE GIT_CEILING_DIRECTORIES

./scripts/cleanup.sh

git init >/dev/null
git config user.name "Bench User"
git config user.email "bench@example.com"

cat > tracked.txt <<'EOF'
base
EOF

git add tracked.txt
git commit -m "base" >/dev/null
git branch -M main

git checkout -b feature >/dev/null
cat > feature.txt <<'EOF'
feature
EOF
git add feature.txt
git commit -m "feature commit" >/dev/null

git checkout main >/dev/null
cat > main.txt <<'EOF'
main
EOF
git add main.txt
git commit -m "main commit" >/dev/null

git checkout feature >/dev/null
# On feature branch now; `git rebase main` is the scenario command.
