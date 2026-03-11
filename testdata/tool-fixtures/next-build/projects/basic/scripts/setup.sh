#!/usr/bin/env sh
set -eu

cat > ./next <<'EOF'
#!/usr/bin/env sh
set -eu

if [ "${1:-}" != "build" ]; then
  echo "unsupported"
  exit 2
fi
shift

if [ "${1:-}" = "--cached" ]; then
  echo '▲ Next.js 15.2.0'
  echo 'Build already optimized using cache'
  exit 0
fi

if [ "${1:-}" = "--failure" ]; then
  echo '▲ Next.js 15.2.0'
  echo 'Creating an optimized production build ...'
  echo 'Failed to compile.'
  echo './app/page.tsx'
  echo 'Error: Missing required export'
  exit 1
fi

echo '▲ Next.js 15.2.0'
echo 'Creating an optimized production build ...'
echo '✓ Compiled successfully'
echo 'Route (app)                    Size     First Load JS'
echo '├ ○ /                          1.2 kB        132 kB'
echo '└ ● /dashboard                 2.5 kB        156 kB'
echo '✓ Built in 34.2s'
exit 0
EOF

chmod +x ./next
