// Fork (dashboard sends, integrations DASHBOARD-SENDS-SPEC follow-up: one Sends-bars + Clicks-line
// chart). Run: node --test src/sendsClicksChart.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  shapeSendsClicks, sendsClicksOptions, clicksAxisMax, SENDS_COLOR, SENDS_FILL, CLICKS_COLOR, CLICKS_HEADROOM,
} from './sendsClicksChart.mjs'; // eslint-disable-line import/extensions

const opts = { labelOf: (d) => d.slice(5), sendsLabel: 'Sends', clicksLabel: 'Clicks' };
const byLabel = (out, label) => out.datasets.find((d) => d.label === label);

test('Sends are bars on y (left), Clicks a line on y1 (right), aligned by date, the line drawn on top', () => {
  const out = shapeSendsClicks(
    [{ date: '2026-10-21', count: 8109 }, { date: '2026-10-22', count: 10999 }],
    [{ date: '2026-10-21', count: 207 }, { date: '2026-10-22', count: 395 }],
    opts,
  );
  assert.deepEqual(out.labels, ['10-21', '10-22']);
  const s = byLabel(out, 'Sends');
  const c = byLabel(out, 'Clicks');
  assert.deepEqual([s.type, s.yAxisID, s.backgroundColor, s.borderColor], ['bar', 'y', SENDS_FILL, SENDS_COLOR]);
  assert.deepEqual([c.type, c.yAxisID, c.borderColor], ['line', 'y1', CLICKS_COLOR]);
  assert.deepEqual(s.data, [8109, 10999]);
  assert.deepEqual(c.data, [207, 395]);
  assert.ok(c.order < s.order, 'chart.js draws the lowest order on top');
});

test('a day only one series has is a gap (null) in the other, never 0 -- the pre-read morning', () => {
  const out = shapeSendsClicks(
    [{ date: '2026-10-21', count: 5 }],
    [{ date: '2026-10-21', count: 1 }, { date: '2026-10-22', count: 0 }],
    opts,
  );
  assert.deepEqual(out.labels, ['10-21', '10-22']);
  assert.deepEqual(byLabel(out, 'Sends').data, [5, null]);
  assert.deepEqual(byLabel(out, 'Clicks').data, [1, 0]);
  assert.equal(byLabel(out, 'Clicks').spanGaps, false);
});

test('dates are sorted whatever the input order; both empty -> {}', () => {
  const out = shapeSendsClicks([], [{ date: '2026-10-22', count: 2 }, { date: '2026-10-20', count: 1 }], opts);
  assert.deepEqual(out.labels, ['10-20', '10-22']);
  assert.deepEqual(byLabel(out, 'Sends').data, [null, null]);
  assert.deepEqual(shapeSendsClicks([], [], opts), {});
  assert.deepEqual(shapeSendsClicks(null, undefined, opts), {});
});

test('the Clicks axis maximum is the headroom multiple of the busiest day (line peaks at half height); none without clicks', () => {
  assert.equal(CLICKS_HEADROOM, 2);
  assert.equal(clicksAxisMax([{ count: 120 }, { count: 395 }, { count: 0 }]), 790);
  assert.equal(clicksAxisMax([{ count: 0 }]), undefined);
  assert.equal(clicksAxisMax([]), undefined);
  assert.equal(clicksAxisMax(null), undefined);
});

test('options: Sends axis left, Clicks axis right with its max, titled; no legend; each axis in its series color; index tooltip', () => {
  const o = sendsClicksOptions({ sendsLabel: 'Sends', clicksLabel: 'Clicks', clicksMax: 790 });
  assert.equal(o.scales.y.position, 'left');
  assert.equal(o.scales.y1.position, 'right');
  assert.equal(o.scales.y1.max, 790);
  assert.equal(o.scales.y.max, undefined);
  assert.equal(o.scales.y.title.text, 'Sends');
  assert.equal(o.scales.y1.title.text, 'Clicks');
  assert.equal(o.plugins.legend.display, false);
  assert.deepEqual([o.scales.y.title.color, o.scales.y.ticks.color], [SENDS_COLOR, SENDS_COLOR]);
  assert.deepEqual([o.scales.y1.title.color, o.scales.y1.ticks.color], [CLICKS_COLOR, CLICKS_COLOR]);
  assert.equal(o.interaction.mode, 'index');
  assert.equal(o.scales.x.offset, true);
  assert.equal(sendsClicksOptions({ sendsLabel: 'S', clicksLabel: 'C' }).scales.y1.max, undefined);
});
