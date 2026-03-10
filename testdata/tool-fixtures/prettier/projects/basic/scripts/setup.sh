#!/usr/bin/env bash
set -euo pipefail

cat > src/bad.js <<'EOF'
function  sum(a,b){return a+b}
module.exports={sum}
EOF

cat > src/bad2.js <<'EOF'
const  pair={left:1,right:2}
module.exports=pair
EOF

cat > src/write_bad.js <<'EOF'
const  data=[1,2,3]
console.log(data)
EOF

mkdir -p node_modules/.bin
cat > node_modules/.bin/prettier <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

mode=""
args=()
for arg in "$@"; do
  case "$arg" in
    --check|--write)
      mode="$arg"
      ;;
    *)
      args+=("$arg")
      ;;
  esac
done

case "$mode" in
  --check)
    echo "Checking formatting..."
    if ((${#args[@]} == 1)) && [[ "${args[0]}" == "src/good.js" ]]; then
      echo "All matched files use Prettier code style!"
      exit 0
    fi
    if ((${#args[@]} == 1)) && [[ "${args[0]}" == "src/bad.js" ]]; then
      echo "[warn] src/bad.js" >&2
      echo "[warn] Code style issues found in the above file. Run Prettier with --write to fix." >&2
      exit 1
    fi
    echo "[warn] src/bad.js" >&2
    echo "[warn] src/bad2.js" >&2
    echo "[warn] Code style issues found in 2 files. Run Prettier with --write to fix." >&2
    exit 1
    ;;
  --write)
    echo "src/write_bad.js 28ms"
    exit 0
    ;;
esac

echo "unsupported prettier fixture invocation" >&2
exit 9
EOF
chmod +x node_modules/.bin/prettier
