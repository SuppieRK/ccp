#!/usr/bin/env bash
set -euo pipefail

if ! kubectl config current-context >/dev/null 2>&1; then
  echo "kubectl context is not configured; run cluster_up.sh first" >&2
  exit 1
fi

# Fast path for CI: if the fixture pod is already Ready, reuse it.
if [[ "${CI:-}" == "true" ]] && kubectl -n ccp-bench get pod ccp-log-basic >/dev/null 2>&1; then
  ready="$(kubectl -n ccp-bench get pod ccp-log-basic -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)"
  if [[ "$ready" == "True" ]]; then
    exit 0
  fi
  # If pod exists but is still coming up, allow a short warm wait before reset.
  if kubectl -n ccp-bench wait --for=condition=Ready pod/ccp-log-basic --timeout=20s >/dev/null 2>&1; then
    exit 0
  fi
fi

kubectl delete namespace ccp-bench --ignore-not-found >/dev/null 2>&1 || true
kubectl create namespace ccp-bench >/dev/null

cat <<'EOF' | kubectl -n ccp-bench apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: ccp-log-basic
spec:
  restartPolicy: Never
  containers:
  - name: logger
    image: alpine:latest
    command:
    - /bin/sh
    - -c
    - |
      for i in $(seq 1 120); do echo 'INFO retrying connection'; done
      for i in $(seq 1 80); do echo 'WARN retrying connection'; done
      echo 'ERROR final failure'
      echo 'stderr-line-1' >&2
      sleep 300
EOF

kubectl -n ccp-bench wait --for=condition=Ready pod/ccp-log-basic --timeout=90s >/dev/null
