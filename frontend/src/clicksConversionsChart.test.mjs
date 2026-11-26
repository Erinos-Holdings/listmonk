// Fork (campaign conversions, the Clicks + conversions chart). Run: node --test
// src/clicksConversionsChart.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  shapeClicksConversions, conversionsAxisMax, clicksConversionsOptions, normalizeConvState,
  loadConvState, saveConvState, CLICKS_FILL, CONV_COLOR, MATCHED_FILL, CONV_STORAGE_KEY,
} from './clicksConversionsChart.mjs'; // eslint-disable-line import/extensions
import { CLICKS_COLOR, withAlpha, TODAY_POINT_RADIUS } from './sendsClicksChart.mjs'; // eslint-disable-line import/extensions

const labels = {
  labelOf: (d) => d.slice(5), clicksLabel: 'Clicks', matchedLabel: 'Matched', trackedLabel: 'Tracked',
};
const day = (date, t, m) => ({ date, tracked: { orders: t[0], sales: t[1] }, matched: { orders: m[0], sales: m[1] } });
const conv = [day('2026-10-21', [1, 40], [3, 120.5]), day('2026-10-22', [0, 0], [2, 80])];
const clicks = [{ date: '2026-10-21', count: 207 }, { date: '2026-10-22', count: 395 }];
const byLabel = (out, l) => out.datasets.find((d) => d.label === l);

test('no conversion days (no doc) renders nothing', () => {
  assert.deepEqual(shapeClicksConversions(clicks, [], labels), {});
  assert.deepEqual(shapeClicksConversions(clicks, null, labels), {});
});

test('default: Clicks bars on y, Tracked dollars line on y1 drawn on top, Matched fill beneath', () => {
  const out = shapeClicksConversions(clicks, conv, labels);
  assert.deepEqual(out.labels, ['10-21', '10-22']);
  const b = byLabel(out, 'Clicks');
  const t = byLabel(out, 'Tracked');
  const m = byLabel(out, 'Matched');
  assert.deepEqual([b.type, b.yAxisID, b.backgroundColor, b.data], ['bar', 'y', CLICKS_FILL, [207, 395]]);
  assert.deepEqual([t.type, t.yAxisID, t.borderColor, t.data], ['line', 'y1', CONV_COLOR, [40, 0]]);
  assert.deepEqual([m.fill, m.backgroundColor, m.borderWidth, m.pointRadius, m.data], ['origin', MATCHED_FILL, 0, 0, [120.5, 80]]);
  assert.ok(t.order < m.order && m.order < b.order);
});

test('# switches both to orders; Show Matched off drops the fill and keeps the line', () => {
  const out = shapeClicksConversions(clicks, conv, { ...labels, unit: 'orders' });
  assert.deepEqual([byLabel(out, 'Tracked').data, byLabel(out, 'Matched').data], [[1, 0], [3, 2]]);
  const off = shapeClicksConversions(clicks, conv, { ...labels, showMatched: false });
  assert.equal(byLabel(off, 'Matched'), undefined);
  assert.deepEqual(byLabel(off, 'Tracked').data, [40, 0]);
});

test('a day only clicks has is a gap (null) on the conversion line, never 0', () => {
  const out = shapeClicksConversions([...clicks, { date: '2026-10-23', count: 5 }], conv, labels);
  assert.deepEqual(byLabel(out, 'Tracked').data, [40, 0, null]);
});

test('the right axis max is exactly twice the top drawn series peak: Matched when shown, else Tracked', () => {
  assert.equal(conversionsAxisMax(conv), 241); // Matched peak 120.5 sits at 50%
  assert.equal(conversionsAxisMax(conv, { unit: 'orders' }), 6);
  assert.equal(conversionsAxisMax(conv, { showMatched: false }), 80); // Tracked peak 40
  assert.equal(conversionsAxisMax(conv, { unit: 'orders', showMatched: false }), 2);
  assert.equal(conversionsAxisMax([day('2026-10-21', [0, 0], [3, 120])], { showMatched: false }), undefined);
  assert.equal(conversionsAxisMax([day('2026-10-21', [0, 0], [0, 0])]), undefined);
});

test('the tooltip lists every drawn series (Clicks, Matched, Tracked) in the chart unit', () => {
  const out = shapeClicksConversions(clicks, conv, labels);
  const item = (l, raw) => ({ dataset: byLabel(out, l), raw });
  const opts = clicksConversionsOptions({ clicksLabel: 'Clicks', axisLabel: 'Sales', dates: ['2026-10-21'] });
  const { label } = opts.plugins.tooltip.callbacks;
  assert.equal(opts.plugins.tooltip.filter, undefined);
  const lines = [label(item('Clicks', 1207)), label(item('Matched', 120.5)), label(item('Tracked', 40))];
  assert.deepEqual(lines, ['Clicks: 1,207', 'Matched: $121', 'Tracked: $40']);
  const items = [item('Tracked', 40), item('Clicks', 207), item('Matched', 120.5)];
  assert.deepEqual([...items].sort(opts.plugins.tooltip.itemSort).map((i) => i.dataset.label), ['Clicks', 'Matched', 'Tracked']);
  const orders = clicksConversionsOptions({ clicksLabel: 'Clicks', axisLabel: 'Orders', unit: 'orders' });
  assert.equal(orders.plugins.tooltip.callbacks.label(item('Matched', 3)), 'Matched: 3');
  assert.equal(opts.scales.y1.ticks.callback(12509.38), '$12,509');
});

// Fork (integrations TODAY-SO-FAR-SPEC I6) -- the same partial-day treatment as the top chart,
// through its one helper: the Clicks bar pale with the dashed outline mark (the Clicks axis color),
// the Tracked segment into today dashed and today's point hollow; the Matched fill unchanged.
test('today: Clicks bar 45% + outline mark, Tracked dashed into today with a hollow point, Matched fill untouched', () => {
  const out = shapeClicksConversions(clicks, conv, { ...labels, today: '2026-10-22' });
  const b = byLabel(out, 'Clicks');
  const m = byLabel(out, 'Tracked');
  const t = byLabel(out, 'Matched');
  assert.equal(b.backgroundColor({ dataIndex: 0 }), CLICKS_FILL);
  assert.equal(b.backgroundColor({ dataIndex: 1 }), withAlpha(CLICKS_FILL, 0.45));
  assert.deepEqual(b.todayOutline, { index: 1, color: CLICKS_COLOR });
  assert.deepEqual(m.segment.borderDash({ p1DataIndex: 1 }), [3, 2]);
  assert.equal(m.segment.borderDash({ p1DataIndex: 0 }), undefined);
  assert.deepEqual([m.pointBackgroundColor({ dataIndex: 1 }), m.pointBorderColor, m.pointRadius({ dataIndex: 1 })], ['#fff', CONV_COLOR, TODAY_POINT_RADIUS]);
  assert.deepEqual([m.pointBackgroundColor({ dataIndex: 0 }), m.pointRadius({ dataIndex: 0 })], [CONV_COLOR, 2]);
  assert.deepEqual([t.backgroundColor, t.segment, t.todayOutline], [MATCHED_FILL, undefined, undefined]);
});

test('no today (or not drawn): the datasets exactly as before; the tooltip title marks today only', () => {
  const plain = shapeClicksConversions(clicks, conv, labels);
  assert.deepEqual(shapeClicksConversions(clicks, conv, { ...labels, today: '2026-10-30' }), plain);
  assert.equal(byLabel(plain, 'Clicks').backgroundColor, CLICKS_FILL);
  assert.equal(byLabel(plain, 'Tracked').segment, undefined);
  const opts = clicksConversionsOptions({
    clicksLabel: 'Clicks', axisLabel: 'Sales', dates: ['2026-10-21', '2026-10-22'], today: '2026-10-22', todayFormat: (l) => `${l} — today so far`,
  });
  assert.equal(opts.plugins.tooltip.callbacks.title([{ label: '22 Oct', dataIndex: 1 }]), '22 Oct — today so far');
  assert.equal(opts.plugins.tooltip.callbacks.title([{ label: '21 Oct', dataIndex: 0 }]), '21 Oct');
});

test('# mode: plain integer ticks on the right axis, its title and numbers in the line color', () => {
  const opts = clicksConversionsOptions({
    clicksLabel: 'Clicks', axisLabel: 'Orders', unit: 'orders', convMax: 6,
  });
  assert.equal(opts.scales.y1.ticks.callback, undefined);
  assert.deepEqual([opts.scales.y1.max, opts.scales.y1.title.color, opts.scales.y1.ticks.color], [6, CONV_COLOR, CONV_COLOR]);
});

test('session state: defaults $ and Matched shown; only an explicit false hides it; a pre-swap showTracked is ignored', () => {
  assert.deepEqual(normalizeConvState(null), { unit: 'usd', showMatched: true });
  assert.deepEqual(normalizeConvState({ unit: 'bogus', showMatched: 'no' }), { unit: 'usd', showMatched: true });
  assert.deepEqual(normalizeConvState({ unit: 'orders', showTracked: false }), { unit: 'orders', showMatched: true });
  const mem = new Map();
  const store = { getItem: (k) => mem.get(k) ?? null, setItem: (k, v) => mem.set(k, v) };
  assert.equal(saveConvState(store, { unit: 'orders', showMatched: false }), true);
  assert.deepEqual(JSON.parse(mem.get(CONV_STORAGE_KEY)), { unit: 'orders', showMatched: false });
  assert.deepEqual(loadConvState(store), { unit: 'orders', showMatched: false });
  const bad = { getItem: () => { throw new Error('blocked'); }, setItem: () => { throw new Error('blocked'); } };
  assert.deepEqual(loadConvState(bad), { unit: 'usd', showMatched: true });
  assert.equal(saveConvState(bad, {}), false);
  assert.equal(saveConvState(null, {}), false);
});
