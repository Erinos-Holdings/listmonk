// Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC I7). Run: yarn test
// (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { barLabelPlacement, INSIDE, OUTSIDE } from './barLabels.mjs'; // eslint-disable-line import/extensions

test('I7 inside when the text plus padding on both sides fits the bar', () => {
  assert.equal(barLabelPlacement(100, 84, 8), INSIDE); // exactly fits: 84 + 16 = 100
  assert.equal(barLabelPlacement(300, 40, 8), INSIDE);
});

test('I7 outside when it does not fit', () => {
  assert.equal(barLabelPlacement(99, 84, 8), OUTSIDE);
  assert.equal(barLabelPlacement(20, 30, 8), OUTSIDE);
});

test('I7 outside for a zero bar, whatever the text', () => {
  assert.equal(barLabelPlacement(0, 0, 8), OUTSIDE);
  assert.equal(barLabelPlacement(0, 0, 0), OUTSIDE);
  assert.equal(barLabelPlacement(-5, 1, 8), OUTSIDE);
});

test('I7 malformed input is outside', () => {
  assert.equal(barLabelPlacement(NaN, 10, 8), OUTSIDE);
  assert.equal(barLabelPlacement(100, undefined, 8), OUTSIDE);
});
