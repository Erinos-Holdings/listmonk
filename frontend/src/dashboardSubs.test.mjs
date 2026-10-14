// Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC I3, D2/D3). Run: npm run
// test:dashboard-subs (node --test src/dashboardSubs.test.mjs src/gridLines.test.mjs).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { subscriberColumns, subscriberRows } from './dashboardSubs.mjs'; // eslint-disable-line import/extensions
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
