#!/usr/bin/env bash
set -euo pipefail

if ! kubectl config current-context >/dev/null 2>&1; then
  echo "kubectl context is not configured; run cluster_up.sh first" >&2
  exit 1
fi

kubectl delete namespace ccp-bench --ignore-not-found >/dev/null 2>&1 || true
kubectl create namespace ccp-bench >/dev/null

cat <<'EOF' | kubectl -n ccp-bench apply -f - >/dev/null
apiVersion: v1
kind: Service
metadata:
  name: svc-a
spec:
  selector:
    app: noop-a
  ports:
  - name: http
    port: 80
    targetPort: 8080
EOF

cat <<'EOF' | kubectl -n ccp-bench apply -f - >/dev/null
apiVersion: v1
kind: Service
metadata:
  name: svc-b
spec:
  selector:
    app: noop-b
  ports:
  - name: http
    port: 80
    targetPort: 8080
EOF

