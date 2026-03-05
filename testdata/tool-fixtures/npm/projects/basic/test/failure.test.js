const test = require('node:test');
const assert = require('node:assert/strict');
const { divide } = require('../src/calculator');

test('divide returns integer result', () => {
  assert.equal(divide(7, 2), 4);
});
