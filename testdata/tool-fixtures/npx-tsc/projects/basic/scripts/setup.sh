#!/usr/bin/env bash
set -euo pipefail

mkdir -p src

cat > src/fail.ts <<'EOF'
const n: number = "boom";
console.log(n);

const n2: number = "boom";
console.log(n2);
EOF

cat > src/fail2.ts <<'EOF'
function usesUnknown(): number {
  return missingSymbol;
}
console.log(usesUnknown());
EOF

cat > src/ok.ts <<'EOF'
const n: number = 42;
console.log(n);
EOF

cat > tsconfig.fail.json <<'EOF'
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "noEmit": true
  },
  "include": ["src/fail.ts"]
}
EOF

cat > tsconfig.ok.json <<'EOF'
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "noEmit": true
  },
  "include": ["src/ok.ts"]
}
EOF

cat > tsconfig.multi.json <<'EOF'
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "noEmit": true
  },
  "include": ["src/fail.ts", "src/fail2.ts"]
}
EOF

npm install --no-audit --no-fund >/dev/null
