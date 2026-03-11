#!/usr/bin/env sh
set -eu

cat > ./golangci-lint <<'EOF'
#!/usr/bin/env sh
set -eu

if [ "${1:-}" = "run" ]; then
  shift
fi

if [ "${1:-}" = "--output.json.path" ] && [ "${2:-}" = "stdout" ]; then
  printf '%s\n' '{"Issues":[{"FromLinter":"errcheck","Text":"ignored error","Pos":{"Filename":"internal/api/server.go","Line":14,"Column":2}}]}'
  exit 1
fi

printf '%s\n' '{"Issues":['
printf '%s\n' '{"FromLinter":"errcheck","Text":"ignored error","Pos":{"Filename":"internal/api/server.go","Line":14,"Column":2}},'
printf '%s\n' '{"FromLinter":"revive","Text":"missing doc","Pos":{"Filename":"internal/api/server.go","Line":20,"Column":1}},'
printf '%s\n' '{"FromLinter":"gosec","Text":"weak random source","Pos":{"Filename":"internal/api/server.go","Line":27,"Column":9}},'
printf '%s\n' '{"FromLinter":"govet","Text":"Printf format mismatch","Pos":{"Filename":"cmd/app/main.go","Line":9,"Column":3}},'
printf '%s\n' '{"FromLinter":"gosimple","Text":"should use strings.Contains","Pos":{"Filename":"internal/checks/check.go","Line":11,"Column":5}},'
printf '%s\n' '{"FromLinter":"stylecheck","Text":"comment should start with name","Pos":{"Filename":"internal/checks/check.go","Line":22,"Column":1}}'
printf '%s\n' ']}'
exit 1
EOF

chmod +x ./golangci-lint
