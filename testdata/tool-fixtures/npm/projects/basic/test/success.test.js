const test = require('node:test');
const assert = require('node:assert/strict');
const { add, divide } = require('../src/calculator');

test('add computes sums', () => {
  assert.equal(add(2, 3), 5);
});

test('divide computes quotient', () => {
  assert.equal(divide(10, 2), 5);
});
