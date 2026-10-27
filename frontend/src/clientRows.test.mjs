// Fork (client stats, integrations CLIENT-STATS-SPEC I6). Run: npm run test:clients
// (node --test src/clientRows.test.mjs src/clientRoster.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  CLIENT_LABELS,
  CLIENT_TIERS,
  CLIENT_TOKENS,
  DEFAULT_SORT,
  TIERS,
  UNTIERED_TOKENS,
  clientLabel,
  clientTier,
  clientTierSubtotals,
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

// CLIENT-VOCABULARY-SPEC J8: the LITERAL token list, in the classifier's first-match order. The
// hand-edit this forces on every vocabulary change is the acknowledgment ritual, not an accident.
test('I6 the vocabulary is the D2 token set, each with a label', () => {
  assert.deepEqual([...CLIENT_TOKENS], [
    'gmail-proxy', 'yahoo', 'outlook-windows', 'outlook-mac', 'thunderbird', 'outlook-mobile',
    'unfurler', 'automation',
    'apple-mail', 'browser-ios', 'browser-android', 'browser-windows', 'browser-mac', 'browser-linux',
    'webview-ios', 'bare-mozilla',
    'other',
  ]);
  CLIENT_TOKENS.forEach((t) => assert.equal(typeof CLIENT_LABELS[t], 'string'));
  assert.equal(clientLabel('apple-mail'), 'Apple Mail (incl. privacy proxy)');
  assert.equal(clientLabel('webview-ios'), 'In-app browser (iOS)');
  assert.equal(clientLabel('bare-mozilla'), 'Unnamed client (bare Mozilla)');
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

// --- integrations CLIENT-VOCABULARY-SPEC ---

const vocabApi = [
  { client: '', views: 900, clicks: 400 },
  { client: 'other', views: 500, clicks: 300 },
  { client: 'bare-mozilla', views: 700, clicks: 2 },
  { client: 'webview-ios', views: 40, clicks: 60 },
  { client: 'automation', views: 30, clicks: 1 },
  { client: 'unfurler', views: 20, clicks: 4 },
  { client: 'browser-windows', views: 11, clicks: 9 },
  { client: 'apple-mail', views: 50, clicks: 3 },
  { client: 'gmail-proxy', views: 10, clicks: 5 },
];

test('J10 the new tokens sort as ordinary rows: Unknown last, other next-to-last on the default sort', () => {
  const rows = shapeClientRows(vocabApi);
  // bare-mozilla's 700 views outrank every real client yet it is an ordinary row; only `other`
  // and Unknown are pinned to the bottom.
  assert.deepEqual(tokens(rows), [
    'bare-mozilla', 'apple-mail', 'webview-ios', 'automation', 'unfurler', 'browser-windows',
    'gmail-proxy', 'other', '',
  ]);
  // A column sort keeps Unknown last in both directions; `other` is pinned on the default only.
  assert.equal(tokens(sortClientRows(rows, 'clicks', 'desc')).at(-1), '');
  assert.equal(tokens(sortClientRows(rows, 'clicks', 'desc'))[0], 'other');
  assert.equal(tokens(sortClientRows(rows, 'name', 'asc')).at(-1), '');
});

test('J9 every token is tiered or explicitly untiered, and the tier map adds no token', () => {
  const vocab = new Set(CLIENT_TOKENS);
  Object.keys(CLIENT_TIERS).forEach((t) => assert.ok(vocab.has(t), `tier key ${t} is not a vocabulary token`));
  Object.values(CLIENT_TIERS).forEach((tier) => assert.ok(TIERS.includes(tier), `unknown tier ${tier}`));
  const untiered = new Set(UNTIERED_TOKENS);
  CLIENT_TOKENS.forEach((t) => {
    const tiered = Object.prototype.hasOwnProperty.call(CLIENT_TIERS, t);
    assert.ok(tiered !== untiered.has(t), `${t} must be either tiered or explicitly untiered, not both/neither`);
  });
  // D8's assignment, spelled out: `apple-mail` is proxy (the conservative choice), the declared
  // robots are machine, `other`/`bare-mozilla`/Unknown claim nothing.
  assert.equal(clientTier('apple-mail'), 'proxy');
  assert.equal(clientTier('webview-ios'), 'client');
  assert.equal(clientTier('automation'), 'machine');
  assert.equal(clientTier('unfurler'), 'machine');
  assert.equal(clientTier('other'), 'untiered');
  assert.equal(clientTier('bare-mozilla'), 'untiered');
  assert.equal(clientTier(''), 'untiered');
  assert.equal(clientTier('constructor'), 'untiered');
  assert.equal(clientTier('future-token'), 'untiered');
});

test('J9 the subtotals are the per-tier sums over Views and Clicks, with no combined total key', () => {
  const sub = clientTierSubtotals(shapeClientRows(vocabApi));
  // Shape first: exactly the four keys, no `total`/`confident` figure is ever published.
  assert.deepEqual(Object.keys(sub).sort(), ['client', 'machine', 'proxy', 'untiered']);
  assert.deepEqual(Object.keys(sub.client).sort(), ['clicks', 'views']);
  // Hand-computed: client = webview-ios 40/60 + browser-windows 11/9; proxy = apple-mail 50/3 +
  // gmail-proxy 10/5; machine = automation 30/1 + unfurler 20/4; untiered = other 500/300 +
  // bare-mozilla 700/2 + Unknown 900/400.
  assert.deepEqual(sub, {
    client: { views: 51, clicks: 69 },
    proxy: { views: 60, clicks: 8 },
    machine: { views: 50, clicks: 5 },
    untiered: { views: 2100, clicks: 702 },
  });
  // Drift and junk fall to untiered rather than inventing a bucket; the input may be raw API rows.
  assert.deepEqual(clientTierSubtotals([
    { client: 'future-token', views: 3, clicks: 1 },
    { client: '  ', views: 2, clicks: null },
    { client: 'browser-mac', views: 1, clicks: 1 },
  ]), {
    client: { views: 1, clicks: 1 },
    proxy: { views: 0, clicks: 0 },
    machine: { views: 0, clicks: 0 },
    untiered: { views: 5, clicks: 1 },
  });
  assert.deepEqual(clientTierSubtotals(null), {
    client: { views: 0, clicks: 0 },
    proxy: { views: 0, clicks: 0 },
    machine: { views: 0, clicks: 0 },
    untiered: { views: 0, clicks: 0 },
  });
});
