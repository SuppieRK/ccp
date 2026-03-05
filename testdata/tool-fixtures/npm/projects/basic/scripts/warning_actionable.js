const { deprecate } = require('node:util');
const { normalizeName } = require('../src/legacy');

const deprecatedNormalize = deprecate(
  normalizeName,
  'normalizeName() is deprecated; migrate to normalizeUserName()'
);

deprecatedNormalize('  Example  ');
process.stdout.write('Checked 1 file\n');
