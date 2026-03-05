#!/usr/bin/env bash
set -euo pipefail
kubectl delete namespace ccp-bench --ignore-not-found >/dev/null 2>&1 || true
