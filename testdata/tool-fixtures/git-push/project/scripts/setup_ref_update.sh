#!/usr/bin/env bash
set -euo pipefail
./scripts/setup_up_to_date.sh
printf "line 2\n" >> tracked.txt
git add tracked.txt
git commit -m "local change" >/dev/null
