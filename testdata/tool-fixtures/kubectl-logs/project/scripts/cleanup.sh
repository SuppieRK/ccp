#!/usr/bin/env bash
set -euo pipefail

# In CI, keep the namespace/resources between scenario passes to avoid
# repeated expensive teardown/bootstrap cycles.
if [[ "${CI:-}" == "true" ]]; then
  exit 0
fi

kubectl delete namespace ccp-bench --ignore-not-found >/dev/null 2>&1 || true
