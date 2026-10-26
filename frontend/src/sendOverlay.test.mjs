// Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC I4). Run:
// node --test src/sendOverlay.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  SEND_BAR_ALPHA, bucketLabel, metricAxisMax, sendOverlayOptions, shapeSendOverlay, withAlpha,
} from './sendOverlay.mjs'; // eslint-disable-line import/extensions

const palette = ['#0055d4', '#FFB50D', '#41AC9C'];
const campaigns = [{ id: 1, name: 'A' }, { id: 2, name: 'B' }];
const sendsLabel = (n) => `${n} sent`;
const row = (campaignId, timestamp, count) => ({ campaignId, timestamp, count });
const shape = (metric, sends, camps = campaigns) => shapeSendOverlay({
  campaigns: camps, metric, sends, palette, sendsLabel,
});
const ds = (out, label) => out.datasets.find((d) => d.label === label);

test('same color per campaign for its line and its bars; ~30% alpha on the bars only', () => {
  const out = shape(
    [row(1, '2026-10-21T18:00:00', 3), row(2, '2026-10-21T18:00:00', 1)],
    [row(1, '2026-10-21T18:00:00', 100), row(2, '2026-10-21T18:00:00', 50)],
  );
  assert.equal(SEND_BAR_ALPHA, 0.3);
  assert.equal(ds(out, 'A').borderColor, '#0055d4');
  assert.equal(ds(out, 'A sent').backgroundColor, 'rgba(0, 85, 212, 0.3)');
  assert.equal(ds(out, 'B').borderColor, '#FFB50D');
  assert.equal(ds(out, 'B sent').backgroundColor, 'rgba(255, 181, 13, 0.3)');
  // The line is opaque.
  assert.equal(ds(out, 'A').backgroundColor, '#0055d4');
  // Lines on the right axis, drawn on top; bars on the left.
  assert.deepEqual([ds(out, 'A').type, ds(out, 'A').yAxisID], ['line', 'y1']);
  assert.deepEqual([ds(out, 'A sent').type, ds(out, 'A sent').yAxisID], ['bar', 'y']);
  assert.ok(ds(out, 'A').order < ds(out, 'A sent').order);
});

test('the color index wraps the palette by the campaign position', () => {
  const camps = [{ id: 1, name: 'A' }, { id: 2, name: 'B' }, { id: 3, name: 'C' }, { id: 4, name: 'D' }];
  const out = shape([], [row(4, '2026-10-21T18:00:00', 1)], camps);
  assert.equal(ds(out, 'D').borderColor, palette[0]);
  assert.equal(ds(out, 'D sent').backgroundColor, withAlpha(palette[0], 0.3));
});

test('missing buckets in the sends series are null, never 0; the x-axis is the sorted union', () => {
  const out = shape(
    [row(1, '2026-10-22T09:00:00', 2), row(1, '2026-10-21T18:00:00', 1)],
    [row(1, '2026-10-21T18:00:00', 100)],
  );
  assert.deepEqual(out.labels, ['2026-10-21 18:00', '2026-10-22 09:00']);
  assert.deepEqual(ds(out, 'A sent').data, [100, null]);
  assert.deepEqual(ds(out, 'A').data, [1, 2]);
  // A real zero send stays 0 (it is a value, not a missing bucket).
  const z = shape([], [row(1, '2026-10-21T18:00:00', 0), row(1, '2026-10-21T19:00:00', 4)]);
  assert.deepEqual(ds(z, 'A sent').data, [0, 4]);
});

test('an empty sends series gives no bar dataset (per campaign and overall)', () => {
  const out = shape([row(1, '2026-10-21T18:00:00', 1)], [row(1, '2026-10-21T18:00:00', 9)]);
  assert.ok(ds(out, 'A sent'));
  assert.equal(ds(out, 'B sent'), undefined);
  const none = shape([row(1, '2026-10-21T18:00:00', 1)], []);
  assert.equal(none.datasets.filter((d) => d.type === 'bar').length, 0);
  assert.equal(none.datasets.filter((d) => d.type === 'line').length, 2);
  const nothing = shape([], null);
  assert.deepEqual(nothing.labels, []);
  assert.equal(nothing.datasets.filter((d) => d.type === 'bar').length, 0);
});

test('axis max = 2x the busiest bucket, rounded up to its leading power of ten; undefined with no data', () => {
  assert.equal(metricAxisMax([row(1, 't', 425), row(2, 't', 100)]), 900);
  assert.equal(metricAxisMax([row(1, 't', 395)]), 800);
  assert.equal(metricAxisMax([row(1, 't', 825)]), 2000);
  assert.equal(metricAxisMax([row(1, 't', 3)]), 6);
  assert.equal(metricAxisMax([]), undefined);
  assert.equal(metricAxisMax([row(1, 't', 0)]), undefined);
  const o = sendOverlayOptions({ sendsTitle: 'Sent', metricTitle: 'Views', metricMax: 900 });
  assert.equal(o.scales.y1.max, 900);
  assert.equal(o.scales.y1.position, 'right');
  assert.equal(o.scales.y.position, 'left');
  assert.equal(o.scales.y.max, undefined);
  assert.equal(sendOverlayOptions({ sendsTitle: 'S', metricTitle: 'M', metricMax: undefined }).scales.y1.max, undefined);
});

test('buckets are labelled verbatim -- the naive Mountain digits, never shifted, no zone text', () => {
  assert.equal(bucketLabel('2026-10-21T23:00:00'), '2026-10-21 23:00');
  // Even a value that did carry an offset is not converted (the digits are the contract).
  assert.equal(bucketLabel('2026-10-21T23:00:00Z'), '2026-10-21 23:00');
  const out = shape([row(1, '2026-10-21T23:00:00', 1)], []);
  assert.ok(!out.labels.some((l) => /UTC|Z|[+-]\d\d:\d\d$/.test(l)));
});

test('withAlpha handles short hex and leaves non-hex alone', () => {
  assert.equal(withAlpha('#fff', 0.3), 'rgba(255, 255, 255, 0.3)');
  assert.equal(withAlpha('rgb(1,2,3)', 0.3), 'rgb(1,2,3)');
});
