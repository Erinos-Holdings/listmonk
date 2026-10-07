// Fork (client stats, integrations CLIENT-STATS-SPEC I6). Run: npm run test:clients
// (node --test src/clientRows.test.mjs src/clientRoster.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  CLIENT_LABELS,
  CLIENT_TOKENS,
  DEFAULT_SORT,
  clientLabel,
  clientTotals,
  shapeClientRows,
  sortClientRows,
} from './clientRows.mjs'; // eslint-disable-line import/extensions

const api = [
  { client: '', views: 900, clicks: 400 },
  { client: 'other', views: 500, clicks: 300 },
  { client: 'apple-mail', views: 50, clicks: 3 },
  { client: 'gmail-proxy', views: 10, clicks: 5 },
  { client: 'outlook-windows', views: 10, clicks: 5 },
  { client: 'yahoo', views: 10, clicks: 1 },
  { client: 'browser-ios', views: 0, clicks: 7 },
];
const tokens = (rows) => rows.map((r) => r.client);

test('I6 the vocabulary is the D2 token set, each with a label', () => {
  assert.deepEqual([...CLIENT_TOKENS], [
    'gmail-proxy', 'yahoo', 'outlook-windows', 'outlook-mac', 'thunderbird', 'outlook-mobile',
    'apple-mail', 'browser-ios', 'browser-android', 'browser-windows', 'browser-mac', 'browser-linux',
    'other',
  ]);
  CLIENT_TOKENS.forEach((t) => assert.equal(typeof CLIENT_LABELS[t], 'string'));
  assert.equal(clientLabel('apple-mail'), 'Apple Mail (incl. privacy proxy)');
});

test('I6 label fallback is the token; Unknown is labelled and flagged; combined = views + clicks', () => {
  assert.equal(clientLabel('samsung-mail'), 'samsung-mail');
  assert.equal(clientLabel('constructor'), 'constructor');
  const rows = shapeClientRows([
    { client: '', views: 2, clicks: null },
    { client: 'future-token', views: '3', clicks: 4 },
    { client: 'thunderbird', views: 5, clicks: 6 },
  ], { unknownLabel: 'Unknown' });
  assert.deepEqual(rows, [
    {
      client: 'thunderbird', name: 'Thunderbird', views: 5, clicks: 6, combined: 11, unknown: false, other: false,
    },
    {
      client: 'future-token', name: 'future-token', views: 0, clicks: 4, combined: 4, unknown: false, other: false,
    },
    {
      client: '', name: 'Unknown', views: 2, clicks: 0, combined: 2, unknown: true, other: false,
    },
  ]);
  assert.deepEqual(shapeClientRows(null), []);
  assert.deepEqual(clientTotals(rows), { views: 7, clicks: 10, combined: 17 });
});

test('I6 default sort: Views desc, Clicks desc, label; other next-to-last and Unknown last despite the largest counts', () => {
  assert.deepEqual(DEFAULT_SORT, { field: 'views', order: 'desc' });
  const rows = shapeClientRows(api);
  // apple-mail 50; the 10-view tie: Gmail / Outlook (5 clicks, label A-Z) before Yahoo (1 click);
  // browser-ios 0 views; then other, then Unknown.
  assert.deepEqual(tokens(rows), ['apple-mail', 'gmail-proxy', 'outlook-windows', 'yahoo', 'browser-ios', 'other', '']);
});

test('I6 column sorts keep Unknown last in both directions and tie-break by the default order', () => {
  const rows = shapeClientRows(api);
  assert.deepEqual(tokens(sortClientRows(rows, 'views', 'asc')), ['browser-ios', 'gmail-proxy', 'outlook-windows', 'yahoo', 'apple-mail', 'other', '']);
  assert.deepEqual(tokens(sortClientRows(rows, 'clicks', 'desc')), ['other', 'browser-ios', 'gmail-proxy', 'outlook-windows', 'apple-mail', 'yahoo', '']);
  assert.deepEqual(tokens(sortClientRows(rows, 'combined', 'desc')), ['other', 'apple-mail', 'gmail-proxy', 'outlook-windows', 'yahoo', 'browser-ios', '']);
  // Labels: Apple Mail, Browser (iOS), Gmail, Other, Outlook desktop, Yahoo Mail.
  assert.deepEqual(tokens(sortClientRows(rows, 'name', 'asc')), ['apple-mail', 'browser-ios', 'gmail-proxy', 'other', 'outlook-windows', 'yahoo', '']);
  assert.deepEqual(tokens(sortClientRows(rows, 'name', 'desc')), ['yahoo', 'outlook-windows', 'other', 'gmail-proxy', 'browser-ios', 'apple-mail', '']);
  // Sorting never mutates its input.
  assert.deepEqual(tokens(rows), ['apple-mail', 'gmail-proxy', 'outlook-windows', 'yahoo', 'browser-ios', 'other', '']);
});
