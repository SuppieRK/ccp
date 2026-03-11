#!/usr/bin/env sh
set -eu

cat > ./ruff <<'EOF'
#!/usr/bin/env sh
set -eu

if [ "${1:-}" = "check" ]; then
  shift
fi

if [ "${1:-}" = "--output-format" ] || [ "${1:-}" = "--output-format=json" ]; then
  if [ "${1:-}" = "--output-format" ]; then
    shift 2
  else
    shift 1
  fi
fi

printf '%s\n' '['
printf '%s\n' '{"code":"F401","message":"`os` imported but unused","location":{"row":1,"column":8},"filename":"src/app.py","fix":{"applicability":"safe"}},'
printf '%s\n' '{"code":"F841","message":"Local variable `x` is assigned to but never used","location":{"row":4,"column":5},"filename":"src/app.py","fix":{"applicability":"safe"}},'
printf '%s\n' '{"code":"E501","message":"Line too long (100 > 88 characters)","location":{"row":10,"column":89},"filename":"src/utils.py","fix":null}'
printf '%s\n' ']'
exit 1
EOF

chmod +x ./ruff
