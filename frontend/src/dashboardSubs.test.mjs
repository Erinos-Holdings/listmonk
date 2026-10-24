// Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC I3, D2/D3; Revision 2 I10/I11). Run: npm run
// test:dashboard-subs (node --test src/dashboardSubs.test.mjs src/gridLines.test.mjs).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  engagementLines, pctCount, segmentCell, subscriberColumns, subscriberRows,
} from './dashboardSubs.mjs'; // eslint-disable-line import/extensions
import { gridLines } from './gridLines.mjs'; // eslint-disable-line import/extensions

const cell = (o) => ({
  active: 0, held: 0, unsubscribed: 0, pending: 0, blocked: 0, total: 0, ...o,
});

test('rows: the D2 order, Pending hidden at zero', () => {
  const g = { all: cell({ active: 3, total: 3 }) };
  assert.deepEqual(subscriberRows(g).map((r) => r.key), ['active', 'held', 'unsubscribed', 'blocked']);
});

test('rows: Pending shown when the all row counts someone pending, after Blocklisted', () => {
  const g = { all: cell({ active: 3, pending: 1, total: 4 }) };
  assert.deepEqual(subscriberRows(g).map((r) => r.key), ['active', 'held', 'unsubscribed', 'blocked', 'pending']);
});

test('rows: labels reuse the Lists grid keys, Blocklisted the subscriber status word', () => {
  const g = { all: cell({ pending: 1, total: 1 }) };
  assert.deepEqual(subscriberRows(g).map((r) => r.label), [
    'lists.grid.active', 'lists.grid.held', 'lists.grid.unsubscribed',
    'subscribers.status.blocklisted', 'lists.grid.pending',
  ]);
});

test('rows: an empty or missing grid still lists the four segments', () => {
  assert.equal(subscriberRows(undefined).length, 4);
  assert.equal(subscriberRows({}).length, 4);
});

test('columns: all then the gridLines keys -- the one rule', () => {
  const grids = [
    { all: cell({ total: 5 }), en: cell({ total: 5 }) },
    { all: cell({ total: 5 }), en: cell({ total: 3 }), fr: cell({ total: 2 }) },
    { all: cell({ total: 5 }), en: cell({ total: 5 }), none: cell({ total: 1 }) },
    { all: cell({ total: 1 }), other: cell({ total: 1 }) },
  ];
  grids.forEach((g) => {
    [true, false].forEach((on) => {
      assert.deepEqual(subscriberColumns(g, on), gridLines(g, on));
    });
  });
  assert.deepEqual(subscriberColumns(grids[1], true).map((c) => c.key), ['all', 'en', 'fr']);
  assert.deepEqual(subscriberColumns(grids[1], false).map((c) => c.key), ['all']);
});

// Revision 2 -- I10: pctCount is rate-first with the count, the count alone at a 0 denominator,
// one decimal.
test('pctCount: one decimal, the count beside it', () => {
  assert.deepEqual(pctCount(751, 1901), { pct: '39.5%', count: '751' });
  assert.deepEqual(pctCount(1, 3), { pct: '33.3%', count: '1' });
  assert.deepEqual(pctCount(0, 10), { pct: '0.0%', count: '0' });
  assert.deepEqual(pctCount(12345, 12345), { pct: '100.0%', count: '12,345' });
});

test('pctCount: a 0, null or missing denominator leaves the count alone', () => {
  [0, null, undefined, NaN].forEach((d) => {
    assert.deepEqual(pctCount(7, d), { pct: null, count: '7' });
  });
});

test('pctCount: fmt formats the count', () => {
  assert.deepEqual(pctCount(1500, 3000, (n) => `<${n}>`), { pct: '50.0%', count: '<1500>' });
});

// I11: segment rates are over THAT column's total; Active and Pending are plain counts.
test('segmentCell: Held/Unsubscribed/Blocklisted over the column total, Active/Pending plain', () => {
  const g = {
    all: cell({
      active: 6, held: 2, unsubscribed: 1, blocked: 1, pending: 1, total: 11,
    }),
    en: cell({ active: 4, held: 2, total: 6 }),
    fr: cell({
      active: 2, unsubscribed: 1, blocked: 1, pending: 1, total: 5,
    }),
  };
  const rows = subscriberRows(g);
  const cols = subscriberColumns(g, true);
  assert.deepEqual(cols.map((c) => c.key), ['all', 'en', 'fr']);
  const seg = (k) => rows.find((r) => r.key === k);
  cols.forEach((col) => {
    ['held', 'unsubscribed', 'blocked'].forEach((k) => {
      assert.deepEqual(segmentCell(col, seg(k)), { count: col.row[k], denominator: col.row.total });
    });
    ['active', 'pending'].forEach((k) => {
      assert.deepEqual(segmentCell(col, seg(k)), { count: col.row[k], denominator: null });
      assert.equal(pctCount(col.row[k], segmentCell(col, seg(k)).denominator).pct, null);
    });
  });
  // The en column's Held is 2 of en's 6, not of all's 11.
  assert.equal(pctCount(2, segmentCell(cols[1], seg('held')).denominator).pct, '33.3%');
});

test('segmentCell: an empty column has a 0 denominator, so the count stands alone', () => {
  const c = segmentCell({ key: 'all', row: cell({}) }, subscriberRows({})[1]);
  assert.deepEqual(c, { count: 0, denominator: 0 });
  assert.equal(pctCount(c.count, c.denominator).pct, null);
  assert.deepEqual(segmentCell(undefined, subscriberRows({})[1]), { count: 0, denominator: 0 });
});

// I11 + R2, BRAND-MATRIX-SPEC D8: Views, Clicks, H. Bounces, Complaints, in that order, every one
// over Messages sent; the every-row bounces field is not a line.
test('engagementLines: the D8 order, each over messages', () => {
  const lines = engagementLines({
    views: 19025, clicks: 1136, bounces: 265, hardBounces: 200, complaints: 4,
  }, 40610);
  assert.deepEqual(lines.map((l) => l.key), ['views', 'clicks', 'hardBounces', 'complaints']);
  assert.deepEqual(lines.map((l) => l.label), [
    'campaigns.views', 'campaigns.clicks', 'dashboard.hardBouncesLine', 'dashboard.complaintsLine',
  ]);
  lines.forEach((l) => assert.equal(l.denominator, 40610));
  assert.deepEqual(lines.map((l) => l.count), [19025, 1136, 200, 4]);
  lines.forEach((l) => assert.equal(l.help, undefined));
  assert.equal(pctCount(lines[2].count, lines[2].denominator).pct, '0.5%');
});

test('engagementLines: nothing sent or no payload yet -- zeros, counts alone', () => {
  const lines = engagementLines(undefined, undefined);
  assert.equal(lines.length, 4);
  lines.forEach((l) => {
    assert.equal(l.count, 0);
    assert.equal(pctCount(l.count, l.denominator).pct, null);
  });
});
