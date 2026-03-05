#!/usr/bin/env bash
set -euo pipefail
./scripts/cleanup.sh
cat > ./kubectl <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "get" || "${2:-}" != "nodes" ]]; then
  echo "error: unsupported args" >&2
  exit 1
fi
cat <<'OUT'
CUSTOM COLS HERE
x y z
OUT
EOF
chmod +x ./kubectl
