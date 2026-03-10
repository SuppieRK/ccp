#!/usr/bin/env sh
set -eu

docker compose down --rmi local --remove-orphans >/dev/null 2>&1 || true
