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
NAME STATUS ROLES AGE VERSION
node-a Ready worker 3d v1.29.0
node-b Ready worker 3d v1.29.0
node-c NotReady worker 2m v1.29.0
OUT
EOF
chmod +x ./kubectl
