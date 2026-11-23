// Fork (system health, 2026-11-22) -- the Dashboard SES pane's reputation charts.
// Run: node --test src/sesRateChart.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  rateStatus, rateLines, rateAxisMax, shapeSesRate, sesRateOptions, pctText, refLines,
  STATUS_FILL, LINE_COLOR, RATE_HEADROOM, LINE_HEADROOM,
} from './sesRateChart.mjs'; // eslint-disable-line import/extensions

const DOC = {
  thresholds: { bounce: { warn: 0.05, issues: 0.1 }, complaint: { warn: 0.001, issues: 0.005 } },
  campaignThresholds: { complaint: { warn: 0.001, red: 0.003 } },
};
const opts = (kind) => ({
  kind, doc: DOC, labelOf: (d) => d.slice(5), label: 'Rate',
});

test('status at the document\'s lines', () => {
  const t = DOC.thresholds.complaint;
  assert.equal(rateStatus(0.0002, t), 'ok');
  assert.equal(rateStatus(0.001, t), 'warn');
  assert.equal(rateStatus(0.005, t), 'issues');
  assert.equal(rateStatus(null, t), 'unknown');
  assert.equal(rateStatus(0.01, {}), 'unknown');
});

test('lines come from the document only: bounce review + pause; complaint adds the Gmail/Yahoo line', () => {
  assert.deepEqual(rateLines('bounce', DOC), [{ key: 'review', value: 0.05 }, { key: 'pause', value: 0.1 }]);
  assert.deepEqual(rateLines('complaint', DOC).map((l) => l.key), ['review', 'gmail', 'pause']);
  assert.deepEqual(rateLines('complaint', { thresholds: DOC.thresholds }).map((l) => l.key), ['review', 'pause']);
  assert.deepEqual(rateLines('bounce', {}), []);
});

test('axis: the highest line with headroom, or headroom over the busiest day when higher', () => {
  const lines = rateLines('complaint', DOC);
  assert.equal(rateAxisMax([0.0002, 0.0003], lines), 0.005 * LINE_HEADROOM);
  assert.equal(rateAxisMax([0.006], lines), 0.006 * RATE_HEADROOM);
  assert.equal(rateAxisMax([], []), undefined);
});

test('shape: per-bar status colors; every line is always drawn', () => {
  const series = [
    { day: '2026-11-20', complaintRate: 0.0002 },
    { day: '2026-11-21', complaintRate: 0.0012, bounceRate: 0 },
  ];
  const out = shapeSesRate(series, 'complaintRate', opts('complaint'));
  assert.deepEqual(out.labels, ['11-20', '11-21']);
  assert.deepEqual(out.datasets[0].backgroundColor, [STATUS_FILL.ok, STATUS_FILL.warn]);
  assert.deepEqual(out.lines.map((l) => l.key), ['review', 'gmail', 'pause']);
  const hot = shapeSesRate([{ day: '2026-11-21', complaintRate: 0.006 }], 'complaintRate', opts('complaint'));
  assert.deepEqual(hot.lines.map((l) => l.key), ['review', 'gmail', 'pause']);
  assert.deepEqual(shapeSesRate([], 'complaintRate', opts('complaint')), {});
});

test('a missing day value is a gap (null), never 0', () => {
  const out = shapeSesRate([{ day: '2026-11-21', bounceRate: 0.01 }], 'complaintRate', opts('complaint'));
  assert.deepEqual(out.datasets[0].data, [null]);
});

test('options: percent ticks, the axis max, labelled lines in their colors', () => {
  const o = sesRateOptions({
    max: 0.006,
    lines: rateLines('complaint', DOC),
    digits: 3,
    valueLabel: 'Complaint rate',
    lineLabel: (k, v) => `${k} ${v}`,
  });
  assert.equal(o.scales.y.max, 0.006);
  assert.equal(o.scales.y.ticks.callback(0.003), '0.300%');
  assert.equal(o.plugins.tooltip.callbacks.label({ raw: 0.00032 }), 'Complaint rate 0.032%');
  assert.deepEqual(o.plugins.refLines.lines.map((l) => [l.text, l.color]), [
    ['review 0.100%', LINE_COLOR.review], ['gmail 0.300%', LINE_COLOR.gmail], ['pause 0.500%', LINE_COLOR.pause],
  ]);
  assert.equal(sesRateOptions({
    lines: [], digits: 2, valueLabel: '', lineLabel: () => '',
  }).scales.y.max, undefined);
});

test('pctText', () => {
  assert.equal(pctText(0.00024, 3), '0.024%');
  assert.equal(pctText(undefined, 2), '—');
});

test('refLines draws only lines inside the chart area', () => {
  const calls = [];
  const ctx = new Proxy({}, {
    get: (o, k) => (k in o ? o[k] : (...a) => calls.push([k, ...a])),
    set: (o, k, v) => Reflect.set(o, k, v),
  });
  const chart = {
    ctx,
    chartArea: {
      top: 0, bottom: 100, left: 0, right: 200,
    },
    scales: { y: { getPixelForValue: (v) => 100 - v * 10000 } },
  };
  refLines.afterDatasetsDraw(chart, {}, { lines: [{ value: 0.005, color: '#000', text: 'in' }, { value: 0.02, color: '#000', text: 'out' }] });
  const texts = calls.filter((c) => c[0] === 'fillText').map((c) => c[1]);
  assert.deepEqual(texts, ['in']);
});
