#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CCP_KIND_CLUSTER:-ccp-benchmark}"

if ! command -v kind >/dev/null 2>&1; then
  echo "kind is required for kubectl benchmark scenarios" >&2
  exit 1
fi

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required for kubectl benchmark scenarios" >&2
  exit 1
fi

if ! kind get clusters | grep -Fxq "$CLUSTER_NAME"; then
  kind create cluster --name "$CLUSTER_NAME" --wait 90s >/dev/null
fi

# Ensure kubectl context points to the benchmark cluster.
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null
