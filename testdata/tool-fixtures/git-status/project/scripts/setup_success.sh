#!/usr/bin/env bash
set -euo pipefail
unset GIT_DIR GIT_WORK_TREE GIT_CEILING_DIRECTORIES

./scripts/cleanup.sh

git init >/dev/null
git config user.name "Bench User"
git config user.email "bench@example.com"

cat > base.txt <<'EOF'
base
EOF
cat > staged.txt <<'EOF'
staged-0
EOF
cat > modified.txt <<'EOF'
modified-0
EOF

git add base.txt staged.txt modified.txt
git commit -m "seed" >/dev/null
git branch -M main

printf "staged-1\n" >> staged.txt
git add staged.txt

printf "modified-1\n" >> modified.txt

touch untracked.txt
