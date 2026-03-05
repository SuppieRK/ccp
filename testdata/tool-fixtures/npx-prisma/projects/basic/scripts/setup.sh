#!/usr/bin/env bash
set -euo pipefail

mkdir -p prisma

cat > prisma/schema.prisma <<'EOF'
generator client {
  provider = "prisma-client-js"
}

datasource db {
  provider = "sqlite"
  url      = "file:./dev.db"
}

model User {
  id    Int    @id @default(autoincrement())
  email String @unique
  name  String?
}
EOF

cat > prisma/bad.prisma <<'EOF'
generator client {
  provider = "prisma-client-js"
}

datasource db {
  provider = "sqlite"
  url      = "file:./dev.db"
}

model User {
  id    Int    @id @default(autoincrement())
  email String @unique
}

model User {
  id Int @id
}
EOF

cat > prisma/format.prisma <<'EOF'
generator client {
provider = "prisma-client-js"
}
datasource db {
provider = "sqlite"
url = "file:./dev.db"
}
model Post {
id Int @id @default(autoincrement())
title String
}
EOF

npm install --no-audit --no-fund >/dev/null
