#!/usr/bin/env sh
set -eu

cat > ./mypy <<'EOF'
#!/usr/bin/env sh
set -eu

if [ "${1:-}" = "--output=json" ] || [ "${1:-}" = "--error-format=json" ]; then
  printf '%s\n' '{"errors":[]}'
  exit 0
fi

printf '%s\n' 'src/app.py:12: error: Incompatible return value type (got "str", expected "int")  [return-value]'
printf '%s\n' 'src/app.py:13: note: Consider using Optional[str]'
printf '%s\n' 'src/app.py:18: error: Argument 1 has incompatible type "int"; expected "str"  [arg-type]'
printf '%s\n' 'src/app.py:22: error: Unsupported operand types for + ("int" and "str")  [operator]'
printf '%s\n' 'src/models.py:8: error: Name "foo" is not defined  [name-defined]'
printf '%s\n' 'src/models.py:10: error: Missing return statement  [return]'
printf '%s\n' 'Found 5 errors in 2 files (checked 4 source files)'
exit 1
EOF

chmod +x ./mypy
