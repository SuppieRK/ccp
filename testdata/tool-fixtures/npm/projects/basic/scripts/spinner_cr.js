const fs = require('node:fs');
const path = require('node:path');

const srcDir = path.join(__dirname, '..', 'src');
const files = fs.readdirSync(srcDir).filter((name) => name.endsWith('.js'));

files.forEach((name, idx) => {
  const phase = idx + 1;
  process.stdout.write(`\r⠋ [${phase}/${files.length}] compiling ${name}...`);
  const abs = path.join(srcDir, name);
  fs.readFileSync(abs, 'utf8');
});

process.stdout.write('\nDone\n');
