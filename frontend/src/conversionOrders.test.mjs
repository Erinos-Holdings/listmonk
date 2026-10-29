// Fork (campaign conversions, the Conversions card's drill-down). Run: node --test
// src/conversionOrders.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  dayAfterSend, shapeOrderRows, arrivals, sortOrderRows, toCsv, csvFileName, DAY_BUCKETS,
} from './conversionOrders.mjs'; // eslint-disable-line import/extensions

const START = '2026-10-22T00:10:00Z';
const row = (id, at, extra = {}) => ({
  order_id: id,
  campaign_id: 217,
  campaign_name: 'Win-back',
  purchased_at: at,
  started_at: START,
  evergreen: false,
  tracked: true,
  matched: false,
  sales_usd: 10,
  ...extra,
});

test('day after send: whole days from the start; null for evergreens, no start, or before the start', () => {
  assert.equal(dayAfterSend(row('a', '2026-10-22T05:00:00Z')), 0);
  assert.equal(dayAfterSend(row('a', '2026-10-23T00:09:59Z')), 0);
  assert.equal(dayAfterSend(row('a', '2026-10-23T00:10:00Z')), 1);
  assert.equal(dayAfterSend(row('a', '2026-10-28T12:00:00Z')), 6);
  assert.equal(dayAfterSend(row('a', '2026-10-23T00:00:00Z', { evergreen: true })), null);
  assert.equal(dayAfterSend(row('a', '2026-10-23T00:00:00Z', { started_at: null })), null);
  assert.equal(dayAfterSend(row('a', '2026-10-21T00:00:00Z')), null);
});

test('arrivals: D0..D5 then D6+, evergreen orders undated, sales per bucket, the bar scale', () => {
  const rows = shapeOrderRows([
    row('a', '2026-10-22T05:00:00Z', { sales_usd: 5 }),
    row('b', '2026-10-22T06:00:00Z', { sales_usd: '7.5' }),
    row('c', '2026-10-24T01:00:00Z'),
    row('d', '2026-11-05T01:00:00Z'),
    row('e', '2026-10-24T01:00:00Z', { evergreen: true }),
  ]);
  const a = arrivals(rows);
  assert.deepEqual(a.buckets.map((b) => b.label), DAY_BUCKETS);
  assert.deepEqual(a.buckets.map((b) => b.orders), [2, 0, 1, 0, 0, 0, 1]);
  assert.equal(a.buckets[0].sales, 12.5);
  assert.equal(a.undated, 1);
  assert.equal(a.max, 2);
  assert.equal(arrivals([]).max, 0);
});

test('arrivals by measure: only the rows that measure credits, so the bars sum to its order count', () => {
  const rows = shapeOrderRows([
    row('a', '2026-10-22T05:00:00Z', { tracked: true, matched: true }),
    row('b', '2026-10-22T06:00:00Z', { tracked: true, matched: false }),
    row('c', '2026-10-24T01:00:00Z', { tracked: false, matched: true }),
    row('d', '2026-10-24T01:00:00Z', { tracked: false, matched: true, evergreen: true }),
  ]);
  const m = arrivals(rows, 'matched');
  assert.deepEqual(m.buckets.map((b) => b.orders), [1, 0, 1, 0, 0, 0, 0]);
  assert.equal(m.undated, 1);
  assert.deepEqual(arrivals(rows, 'tracked').buckets.map((b) => b.orders), [2, 0, 0, 0, 0, 0, 0]);
  assert.equal(arrivals(rows).buckets[0].orders + arrivals(rows).buckets[2].orders + arrivals(rows).undated, 4);
});

test('sort: by sales, by day (missing last both ways), default newest first, stable ties', () => {
  const rows = shapeOrderRows([
    row('a', '2026-10-22T05:00:00Z', { sales_usd: 30 }),
    row('b', '2026-10-25T05:00:00Z', { sales_usd: 10 }),
    row('c', '2026-10-23T05:00:00Z', { sales_usd: 20, evergreen: true }),
  ]);
  assert.deepEqual(sortOrderRows(rows).map((r) => r.order_id), ['b', 'c', 'a']);
  assert.deepEqual(sortOrderRows(rows, { field: 'sales_usd', order: 'desc' }).map((r) => r.order_id), ['a', 'c', 'b']);
  assert.deepEqual(sortOrderRows(rows, { field: 'day', order: 'asc' }).map((r) => r.order_id), ['a', 'b', 'c']);
  assert.deepEqual(sortOrderRows(rows, { field: 'day', order: 'desc' }).map((r) => r.order_id), ['b', 'a', 'c']);
  assert.deepEqual(sortOrderRows(rows, { field: 'bogus' }).map((r) => r.order_id), ['b', 'c', 'a']);
});

test('CSV: header, UTC times, cents, yes/no, quoting, formula-safe names, CRLF', () => {
  const rows = shapeOrderRows([
    row('a', '2026-10-22T05:00:00-00:00', { sales_usd: 12.345, matched: true, campaign_name: 'Say "hi", all' }),
    row('b', '2026-10-24T01:00:00Z', { evergreen: true, campaign_name: '=HYPERLINK("x")' }),
  ]);
  const lines = toCsv(rows).split('\r\n');
  assert.equal(lines[0], 'order_id,purchased_at_utc,campaign_id,campaign_name,day_after_send,sales_usd,tracked,matched');
  assert.equal(lines[1], 'a,2026-10-22T05:00:00.000Z,217,"Say ""hi"", all",0,12.35,yes,yes');
  assert.equal(lines[2], 'b,2026-10-24T01:00:00.000Z,217,"\'=HYPERLINK(""x"")",,10.00,yes,no');
  assert.equal(lines[3], '');
  // Every text column is formula-safe, the order id included.
  assert.equal(toCsv(shapeOrderRows([row('=cmd', '2026-10-22T05:00:00Z')])).split('\r\n')[1].split(',')[0], "'=cmd");
  assert.equal(toCsv([]), 'order_id,purchased_at_utc,campaign_id,campaign_name,day_after_send,sales_usd,tracked,matched\r\n');
});

test('file name names up to three campaigns and the date', () => {
  const d = new Date('2026-10-29T12:00:00Z');
  assert.equal(csvFileName([217], d), 'conversions-c217-2026-10-29.csv');
  assert.equal(csvFileName([1, 2, 3, 4, 5], d), 'conversions-c1-2-3+2-2026-10-29.csv');
  assert.equal(csvFileName([], d), 'conversions-cnone-2026-10-29.csv');
});
