// Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC I10 / T10). Run:
// npm run test:conversions (node --test src/conversionCells.test.mjs). Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  MISSING, analyticsCells, aovText, conversionCell, conversionTooltip, conversionsComputedLabel, formatUsd, normalizeMeta, scopedCell, sumCovered,
} from './conversionCells.mjs'; // eslint-disable-line import/extensions

const META = {
  as_of: '2026-10-28T14:45:00Z', tracked_floor: '2026-04-01', matched_floor: '2026-08-31T00:00:00Z', tracked_stale: false,
};

test('uncovered (null) renders a dash, never a zero', () => {
  assert.deepEqual(conversionCell(null), { orders: MISSING, sales: '' });
  assert.deepEqual(conversionCell(undefined), { orders: MISSING, sales: '' });
});

test('covered with no rows renders a real zero', () => {
  assert.deepEqual(conversionCell({ orders: 0, sales_usd: 0 }), { orders: '0', sales: '$0' });
});

test('covered with rows: orders, then sales in whole dollars', () => {
  assert.deepEqual(conversionCell({ orders: 1234, sales_usd: 5678.5 }), { orders: '1,234', sales: '$5,679' });
  // The camelCased dashboard counts carry salesUsd.
  assert.deepEqual(conversionCell({ orders: 2, salesUsd: 3 }), { orders: '2', sales: '$3' });
  assert.equal(formatUsd(12509.38), '$12,509');
});

test('Campaign Analytics: one campaign uncovered for a measure shows a dash for it', () => {
  const c = analyticsCells({ ...META, campaigns: [{ campaign_id: 1, tracked: null, matched: { orders: 0, sales_usd: 0 } }] });
  assert.equal(c.tracked.orders, MISSING);
  assert.equal(c.matched.orders, '0');
});

test('Campaign Analytics: sums over covered campaigns only', () => {
  const campaigns = [
    { campaign_id: 1, tracked: { orders: 2, sales_usd: 10 }, matched: { orders: 1, sales_usd: 4 } },
    { campaign_id: 2, tracked: null, matched: { orders: 3, sales_usd: 6.25 } },
  ];
  assert.deepEqual(sumCovered(campaigns, 'tracked'), { orders: 2, sales: 10 });
  assert.deepEqual(sumCovered(campaigns, 'matched'), { orders: 4, sales: 10.25 });
  const c = analyticsCells({ ...META, campaigns });
  assert.deepEqual(c.tracked, { orders: '2', sales: '$10', aov: '$5.00' });
  assert.deepEqual(c.matched, { orders: '4', sales: '$10', aov: '$2.56' });
});

// Fork (matrix polish, integrations MATRIX-POLISH-SPEC I10) -- AOV = sales / orders, two decimals;
// absent when orders is 0 or the measure is not covered.
test('MATRIX-POLISH I10: AOV is sales / orders with two decimals, absent at 0 orders or uncovered', () => {
  assert.equal(aovText({ orders: 3, sales: 100 }), '$33.33');
  assert.equal(aovText({ orders: 1, sales: 1234.5 }), '$1,234.50');
  assert.equal(aovText({ orders: 0, sales: 0 }), '');
  assert.equal(aovText(null), '');
  const c = analyticsCells({ ...META, campaigns: [{ campaign_id: 1, tracked: null, matched: { orders: 0, sales_usd: 0 } }] });
  assert.equal(c.tracked.aov, '', 'not covered');
  assert.equal(c.matched.aov, '', 'covered, zero orders');
  assert.equal(analyticsCells({ as_of: null, campaigns: [] }).matched.aov, '', 'no doc');
});

test('no conversions doc: every cell is a dash', () => {
  const noDoc = { as_of: null, campaigns: [{ campaign_id: 1, tracked: null, matched: null }] };
  const c = analyticsCells(noDoc);
  assert.equal(c.meta, null);
  assert.equal(c.tracked.orders, MISSING);
  assert.equal(c.matched.orders, MISSING);
  assert.equal(scopedCell({ orders: 0, sales_usd: 0 }, null).orders, MISSING);
  assert.equal(scopedCell({ orders: 0, salesUsd: 0 }, { asOf: '2026-10-28T14:45:00Z' }).orders, '0');
});

test('normalizeMeta reads both key styles', () => {
  assert.deepEqual(normalizeMeta(META), {
    asOf: '2026-10-28T14:45:00Z', trackedFloor: '2026-04-01', matchedFloor: '2026-08-31T00:00:00Z', trackedStale: false,
  });
  assert.equal(normalizeMeta({ asOf: 'x', trackedStale: true }).trackedStale, true);
  assert.equal(normalizeMeta(null), null);
});

test('tooltip: definition with its floor, the computed time, and the stale note on Tracked only', () => {
  const t = (k, p) => `${k}${p ? JSON.stringify(p) : ''}`;
  const stale = { ...META, tracked_stale: true };
  // asOf 2026-10-28T14:45Z = 8:45 am MDT on Oct 28; `now` the same Mountain day -> no date prefix.
  const now = Date.parse('2026-10-28T20:00:00Z');
  const computed = 'analytics.matrix.conversionsComputed{"time":"8:45 am"}';
  const wantTracked = `analytics.conversions.trackedHelp{"floor":"2026-04-01"} ${computed} analytics.conversions.stale`;
  const wantMatched = `analytics.conversions.matchedHelp{"floor":"2026-08-31"} ${computed}`;
  assert.equal(conversionTooltip('tracked', stale, t, now), wantTracked);
  assert.equal(conversionTooltip('matched', stale, t, now), wantMatched);
  assert.equal(conversionTooltip('matched', null, t, now), 'analytics.conversions.matchedHelp{"floor":""} analytics.conversions.none');
});

// integrations SYNC-ALL-SPEC I5: the Matrix's Conversions header is the asOf as a Mountain clock
// time, date-prefixed when not today -- never a "through <day>".
test('SYNC-ALL I5: conversionsComputedLabel -- Mountain clock time, date only when not today, never "through"', () => {
  const t = (k, p) => `${k}${p ? JSON.stringify(p) : ''}`;
  const meta = { as_of: '2026-11-06T21:01:05Z' }; // 2:01 pm MST
  assert.equal(conversionsComputedLabel(meta, t, Date.parse('2026-11-06T21:14:05Z')), 'analytics.matrix.conversionsComputed{"time":"2:01 pm"}');
  assert.equal(conversionsComputedLabel(meta, t, Date.parse('2026-11-07T16:00:00Z')), 'analytics.matrix.conversionsComputed{"time":"Nov 6, 2:01 pm"}');
  assert.equal(conversionsComputedLabel({ asOf: '2026-11-06T21:01:05Z' }, t, Date.parse('2026-11-06T21:14:05Z')), 'analytics.matrix.conversionsComputed{"time":"2:01 pm"}');
  assert.equal(conversionsComputedLabel(null, t), '');
  assert.equal(conversionsComputedLabel({}, t), '');
  assert.equal(conversionsComputedLabel({ as_of: 'garbage' }, t), '');
  assert.doesNotMatch(conversionsComputedLabel(meta, t, Date.parse('2026-11-07T16:00:00Z')), /through/i);
});
