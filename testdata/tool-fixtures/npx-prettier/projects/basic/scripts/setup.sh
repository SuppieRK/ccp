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

npm install --no-audit --no-fund >/dev/null
