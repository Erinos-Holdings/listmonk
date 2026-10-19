// Fork (shop-link attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC I7; shop-link rate,
// SHOP-LINK-RATE-SPEC I6 -- the counts view's sort replaces the parent's rank order). Run: yarn test
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

// The bars as "Label=value|text", for compact assertions.
const bars = (o) => o.labels.map((l, i) => `${l}=${o.data[i]}|${o.texts[i]}`);

test('I6 counts: the four kinds always, zero-filled, ties in rank order', () => {
  const empty = shapeShopLinkBars([], labels);
  assert.deepEqual(empty.labels, ['Self', 'Creator', 'Founder', 'Corporate']);
  assert.deepEqual(empty.data, [0, 0, 0, 0]);
  assert.deepEqual(empty.texts, ['0', '0', '0', '0']);
  assert.deepEqual(shapeShopLinkBars(null, labels).data, [0, 0, 0, 0]);
});

test('I6 counts: sorted by count desc, ties by rank (self, creator, founder, corporate, none, unknown, extras)', () => {
  assert.deepEqual(bars(shapeShopLinkBars([
    { kind: 'founder', count: 34 },
    { kind: 'self', count: 7 },
    { kind: 'corporate', count: 7 },
    { kind: 'none', count: 7 },
    { kind: 'unknown', count: 7 },
    { kind: 'partner', count: 7 },
  ], labels)), [
    'Founder=34|34', 'Self=7|7', 'Corporate=7|7', 'No link=7|7', 'Unknown=7|7', 'partner=7|7', 'Creator=0|0',
  ]);
});

test('I6 counts: No link, Unknown and extras only when non-zero', () => {
  assert.deepEqual(shapeShopLinkBars([
    { kind: 'unknown', count: 3 },
    { kind: 'none', count: 0, audience: 4 },
    { kind: 'partner', count: 0, audience: 2 },
    { kind: 'creator', count: 1 },
  ], labels).labels, ['Unknown', 'Creator', 'Self', 'Founder', 'Corporate']);
});

test('I6 counts: extras verbatim, ties by name; counts coerced; malformed rows ignored; label fallback', () => {
  assert.deepEqual(bars(shapeShopLinkBars([
    { kind: 'zeta', count: 2 },
    { kind: 'alpha', count: 2 },
    { kind: 'self', count: '4' },
    { kind: 'creator', count: null },
    { count: 9 },
    null,
  ], { self: 'Self' })), ['Self=4|4', 'alpha=2|2', 'zeta=2|2', 'creator=0|0', 'founder=0|0', 'corporate=0|0']);
});

test('I6 counts: the text goes through the formatter', () => {
  const fmt = (n) => n.toLocaleString('en-US');
  assert.deepEqual(shapeShopLinkBars([{ kind: 'founder', count: 1234 }], labels, 'counts', fmt).texts[0], '1,234');
});

// The backend repeats snapshots on every row; the shaping does not read it.
const rateRows = [
  { kind: 'founder', count: 34, audience: 1688 },
  { kind: 'self', count: 7, audience: 142 },
  { kind: 'creator', count: 0, audience: 282 },
  { kind: 'corporate', count: 1, audience: 116 },
  { kind: 'none', count: 1, audience: 0 },
  { kind: 'unknown', count: 2, audience: null },
  { kind: 'partner', count: 0, audience: 0 },
];

test('I6 rate: sorted by rate desc; text "x% (n of N)"; unknown and zero-zero kinds hidden', () => {
  const fmt = (n) => n.toLocaleString('en-US');
  const o = shapeShopLinkBars(rateRows, labels, 'rate', fmt);
  // No link (1 of 0) and Creator (0 of 282) tie at rate 0: count desc puts No link first.
  assert.deepEqual(o.labels, ['Self', 'Founder', 'Corporate', 'No link', 'Creator']);
  assert.deepEqual(o.texts, ['4.9% (7 of 142)', '2.0% (34 of 1,688)', '0.9% (1 of 116)', '1 of 0', '0.0% (0 of 282)']);
  assert.ok(Math.abs(o.data[0] - (700 / 142)) < 1e-9);
  // Drift: clicks against audience 0 is shown with value 0.
  assert.equal(o.data[3], 0);
});

test('I6 rate: a rate over 100% is shown as computed; ties by count desc, then rank', () => {
  assert.deepEqual(bars(shapeShopLinkBars([
    { kind: 'self', count: 3, audience: 2 },
    { kind: 'corporate', count: 2, audience: 4 },
    { kind: 'founder', count: 1, audience: 2 },
    { kind: 'creator', count: 1, audience: 2 },
  ], labels, 'rate')), ['Self=150|150.0% (3 of 2)', 'Corporate=50|50.0% (2 of 4)', 'Creator=50|50.0% (1 of 2)', 'Founder=50|50.0% (1 of 2)']);
});

test('hideNone: the No link bar is dropped from both views, nothing else moves', () => {
  const rows = [
    {
      kind: 'founder', count: 54, audience: 1449, snapshots: 1,
    },
    {
      kind: 'none', count: 1, audience: 3, snapshots: 1,
    },
    {
      kind: 'self', count: 7, audience: 118, snapshots: 1,
    },
  ];
  const counts = shapeShopLinkBars(rows, labels, 'counts', String, { hideNone: true });
  assert.deepEqual(counts.labels, ['Founder', 'Self', 'Creator', 'Corporate']);
  const rate = shapeShopLinkBars(rows, labels, 'rate', String, { hideNone: true });
  assert.deepEqual(rate.labels, ['Self', 'Founder']);
  // Default: the bar stays.
  assert.ok(shapeShopLinkBars(rows, labels, 'rate').labels.includes('No link'));
  assert.ok(shapeShopLinkBars(rows, labels, 'counts').labels.includes('No link'));
});

test('I6 rate: every audience null renders no bars (the view owns the no-snapshot note)', () => {
  const o = shapeShopLinkBars([
    { kind: 'founder', count: 3, audience: null },
    { kind: 'unknown', count: 1, audience: null },
  ], labels, 'rate');
  assert.deepEqual(o, { labels: [], data: [], texts: [] });
  assert.deepEqual(shapeShopLinkBars([], labels, 'rate'), { labels: [], data: [], texts: [] });
});
