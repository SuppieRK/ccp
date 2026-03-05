#!/usr/bin/env bash
set -euo pipefail

if ! kubectl config current-context >/dev/null 2>&1; then
  echo "kubectl context is not configured; run cluster_up.sh first" >&2
  exit 1
fi

kubectl delete namespace ccp-bench --ignore-not-found >/dev/null 2>&1 || true
kubectl create namespace ccp-bench >/dev/null

# Healthy pod.
cat <<'EOF' | kubectl -n ccp-bench apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: ccp-healthy
spec:
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
EOF

# Unhealthy pod with guaranteed image pull failure.
cat <<'EOF' | kubectl -n ccp-bench apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: ccp-broken
spec:
  containers:
  - name: broken
    image: invalid.invalid/does-not-exist:latest
EOF

# Wait for the healthy pod to become Running, and broken pod to surface a non-Running state.
kubectl -n ccp-bench wait --for=condition=Ready pod/ccp-healthy --timeout=90s >/dev/null

for _ in $(seq 1 30); do
  status=$(kubectl -n ccp-bench get pod ccp-broken -o jsonpath='{.status.phase}' 2>/dev/null || true)
  reason=$(kubectl -n ccp-bench get pod ccp-broken -o jsonpath='{.status.containerStatuses[0].state.waiting.reason}' 2>/dev/null || true)
  if [[ "$status" != "Running" && "$status" != "" ]]; then
    break
  fi
  if [[ "$reason" == "ErrImagePull" || "$reason" == "ImagePullBackOff" ]]; then
    break
  fi
  sleep 2
done
