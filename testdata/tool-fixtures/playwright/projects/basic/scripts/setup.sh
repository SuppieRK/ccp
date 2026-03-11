#!/usr/bin/env sh
set -eu

cat > ./playwright <<'EOF'
#!/usr/bin/env sh
set -eu

if [ "${1:-}" != "test" ]; then
  echo 'unsupported'
  exit 2
fi
shift

if [ "${1:-}" = "--reporter=json" ]; then
  echo '{"reporter":"json"}'
  exit 0
fi

if [ "${1:-}" = "--failure" ]; then
  echo 'Running 3 tests using 1 worker'
  echo '  ✓  1 tests/auth.spec.ts:3:1 › auth › logs in (1.1s)'
  echo '  ✘  2 tests/auth.spec.ts:8:1 › auth › rejects bad password (2.0s)'
  echo '    Error: expect(received).toBeTruthy()'
  echo '    screenshot: test-results/auth-failure.png'
  echo '  2 passed, 1 failed (3.1s)'
  exit 1
fi

echo 'Running 2 tests using 1 worker'
echo '  ✓  1 tests/auth.spec.ts:3:1 › auth › logs in (1.1s)'
echo '  ✓  2 tests/auth.spec.ts:8:1 › auth › logs out (2.0s)'
echo '  2 passed (3.1s)'
exit 0
EOF

chmod +x ./playwright
