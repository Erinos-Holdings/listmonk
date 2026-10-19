// Fork (shop-link attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC I7). Run: yarn test
// (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { shapeShopLinkBars } from './shopLinkBars.mjs'; // eslint-disable-line import/extensions

const labels = {
  self: 'Self',
  creator: 'Creator',
  founder: 'Founder',
  corporate: 'Corporate',
  none: 'No link',
  unknown: 'Unknown',
};

test('I7 the four kinds always, in rank order, zero-filled', () => {
  assert.deepEqual(shapeShopLinkBars([], labels), {
    labels: ['Self', 'Creator', 'Founder', 'Corporate'],
    data: [0, 0, 0, 0],
  });
  assert.deepEqual(shapeShopLinkBars(null, labels).data, [0, 0, 0, 0]);
  // Backend order is count desc; the bars keep rank order regardless.
  assert.deepEqual(shapeShopLinkBars([
    { kind: 'founder', count: 90 },
    { kind: 'self', count: 4 },
    { kind: 'corporate', count: 2 },
  ], labels), {
    labels: ['Self', 'Creator', 'Founder', 'Corporate'],
    data: [4, 0, 90, 2],
  });
});

test('I7 No link and Unknown only when non-zero, after the four kinds', () => {
  assert.deepEqual(shapeShopLinkBars([
    { kind: 'unknown', count: 3 },
    { kind: 'none', count: 0 },
    { kind: 'creator', count: 1 },
  ], labels), {
    labels: ['Self', 'Creator', 'Founder', 'Corporate', 'Unknown'],
    data: [0, 1, 0, 0, 3],
  });
  assert.deepEqual(shapeShopLinkBars([
    { kind: 'unknown', count: 3 },
    { kind: 'none', count: 5 },
  ], labels), {
    labels: ['Self', 'Creator', 'Founder', 'Corporate', 'No link', 'Unknown'],
    data: [0, 0, 0, 0, 5, 3],
  });
});

test('I7 extra kinds verbatim, after None/Unknown, by count desc then name', () => {
  assert.deepEqual(shapeShopLinkBars([
    { kind: 'partner', count: 2 },
    { kind: 'none', count: 1 },
    { kind: 'zeta', count: 7 },
    { kind: 'alpha', count: 2 },
    { kind: 'self', count: 1 },
  ], labels), {
    labels: ['Self', 'Creator', 'Founder', 'Corporate', 'No link', 'zeta', 'alpha', 'partner'],
    data: [1, 0, 0, 0, 1, 7, 2, 2],
  });
});

test('I7 counts coerced to numbers; malformed rows ignored; a missing label falls back to the kind', () => {
  assert.deepEqual(shapeShopLinkBars([
    { kind: 'self', count: '4' },
    { kind: 'creator', count: null },
    { count: 9 },
    null,
  ], { self: 'Self' }), {
    labels: ['Self', 'creator', 'founder', 'corporate'],
    data: [4, 0, 0, 0],
  });
});
