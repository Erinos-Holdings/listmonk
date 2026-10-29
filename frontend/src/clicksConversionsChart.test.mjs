// Fork (campaign conversions, the Clicks + conversions chart). Run: node --test
// src/clicksConversionsChart.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  shapeClicksConversions, conversionsAxisMax, clicksConversionsOptions, tooltipLines, normalizeConvState,
  loadConvState, saveConvState, formatDollars, CLICKS_FILL, CONV_COLOR, TRACKED_FILL, CONV_STORAGE_KEY,
} from './clicksConversionsChart.mjs'; // eslint-disable-line import/extensions

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

test('default: Clicks bars on y, Matched dollars line on y1 drawn on top, no Tracked series', () => {
  const out = shapeClicksConversions(clicks, conv, labels);
  assert.deepEqual(out.labels, ['10-21', '10-22']);
  const b = byLabel(out, 'Clicks');
  const m = byLabel(out, 'Matched');
  assert.deepEqual([b.type, b.yAxisID, b.backgroundColor, b.data], ['bar', 'y', CLICKS_FILL, [207, 395]]);
  assert.deepEqual([m.type, m.yAxisID, m.borderColor, m.data], ['line', 'y1', CONV_COLOR, [120.5, 80]]);
  assert.equal(byLabel(out, 'Tracked'), undefined);
  assert.ok(m.order < b.order);
});

test('# switches the line to orders; Show Tracked adds a borderless fill of the same hue under the line', () => {
  const out = shapeClicksConversions(clicks, conv, { ...labels, unit: 'orders', showTracked: true });
  assert.deepEqual(byLabel(out, 'Matched').data, [3, 2]);
  const t = byLabel(out, 'Tracked');
  assert.deepEqual([t.fill, t.backgroundColor, t.borderWidth, t.pointRadius, t.data], ['origin', TRACKED_FILL, 0, 0, [1, 0]]);
  assert.ok(byLabel(out, 'Matched').order < t.order && t.order < byLabel(out, 'Clicks').order);
});

test('a day only clicks has is a gap (null) on the conversion line, never 0', () => {
  const out = shapeClicksConversions([...clicks, { date: '2026-10-23', count: 5 }], conv, labels);
  assert.deepEqual(byLabel(out, 'Matched').data, [120.5, 80, null]);
});

test('the right axis max takes headroom over what is drawn (Tracked only when shown)', () => {
  assert.equal(conversionsAxisMax(conv), 300); // 120.5 x 2 = 241 -> 300
  assert.equal(conversionsAxisMax(conv, { unit: 'orders' }), 6);
  const spike = [day('2026-10-21', [9, 900], [3, 120])];
  assert.equal(conversionsAxisMax(spike, { showTracked: false }), 300);
  assert.equal(conversionsAxisMax(spike, { showTracked: true }), 2000);
  assert.equal(conversionsAxisMax([day('2026-10-21', [0, 0], [0, 0])]), undefined);
});

test('the tooltip always lists all four values, whatever is toggled; whole dollars', () => {
  assert.deepEqual(
    tooltipLines(conv[0], { matchedLabel: 'Matched', trackedLabel: 'Tracked', ordersLabel: 'orders' }),
    ['Matched: $121 · 3 orders', 'Tracked: $40 · 1 orders'],
  );
  assert.deepEqual(tooltipLines(undefined, {}), []);
  const opts = clicksConversionsOptions({
    clicksLabel: 'Clicks',
    axisLabel: 'Sales',
    dates: ['2026-10-21'],
    byDate: new Map([['2026-10-21', conv[0]]]),
    matchedLabel: 'Matched',
    trackedLabel: 'Tracked',
    ordersLabel: 'orders',
  });
  assert.equal(opts.plugins.tooltip.callbacks.footer([{ dataIndex: 0 }]).length, 2);
  assert.equal(opts.plugins.tooltip.filter({ dataset: { type: 'line' } }), false);
  assert.equal(opts.plugins.tooltip.filter({ dataset: { type: 'bar' } }), true);
  assert.equal(opts.scales.y1.ticks.callback(12509.38), '$12,509');
  assert.equal(formatDollars(0), '$0');
});

test('# mode: plain integer ticks on the right axis, its title and numbers in the line color', () => {
  const opts = clicksConversionsOptions({
    clicksLabel: 'Clicks', axisLabel: 'Orders', unit: 'orders', convMax: 6,
  });
  assert.equal(opts.scales.y1.ticks.callback, undefined);
  assert.deepEqual([opts.scales.y1.max, opts.scales.y1.title.color, opts.scales.y1.ticks.color], [6, CONV_COLOR, CONV_COLOR]);
});

test('session state: defaults $ and Tracked off; bad input and a throwing store are the default', () => {
  assert.deepEqual(normalizeConvState(null), { unit: 'usd', showTracked: false });
  assert.deepEqual(normalizeConvState({ unit: 'bogus', showTracked: 'yes' }), { unit: 'usd', showTracked: false });
  const mem = new Map();
  const store = { getItem: (k) => mem.get(k) ?? null, setItem: (k, v) => mem.set(k, v) };
  assert.equal(saveConvState(store, { unit: 'orders', showTracked: true }), true);
  assert.deepEqual(JSON.parse(mem.get(CONV_STORAGE_KEY)), { unit: 'orders', showTracked: true });
  assert.deepEqual(loadConvState(store), { unit: 'orders', showTracked: true });
  const bad = { getItem: () => { throw new Error('blocked'); }, setItem: () => { throw new Error('blocked'); } };
  assert.deepEqual(loadConvState(bad), { unit: 'usd', showTracked: false });
  assert.equal(saveConvState(bad, {}), false);
  assert.equal(saveConvState(null, {}), false);
});
