#!/usr/bin/env bash
set -euo pipefail

docker compose down --remove-orphans >/dev/null 2>&1 || true
docker compose up -d >/dev/null
