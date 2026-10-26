// Fork (brand matrix, integrations listmonk/BRAND-MATRIX-SPEC I5-I8). Run: yarn test
// (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  DEFAULT_STATE, ENGAGEMENT_FIELDS, STORAGE_KEY, cellText, clickSort, defaultOrder, displayValue, initialPane, loadState, rateOf,
  SPAM_FLAGGED_HELP, SPAM_HELP, saveState, sessionStore, shapeRows, sortRows, spamCell, tableRows, totalsRow,
  BREACH_CLASS, breachesThreshold, displayedRate, engagementCell, firstThresholds,
} from './brandMatrix.mjs'; // eslint-disable-line import/extensions

const api = [
  {
    brand: 'ruze',
    subscribers: {
      total: 1000, active: 800, held: 100, unsubscribed: 60, blocked: 40,
    },
    sent: 10000,
    views: 1200,
    clicks: 300,
    hard_bounces: 50,
    blocks: 10,
    complaints: 2,
    health: { status: 'ok', spam_rate: 0.0012 },
  },
  {
    brand: 'thirstygirl',
    subscribers: {
      total: 200, active: 150, held: 20, unsubscribed: 20, blocked: 10,
    },
    sent: 1000,
    views: 200, // 20% -- the highest view rate, but not the highest count
    clicks: 60, // 6% -- the highest rate, but not the highest count
    hard_bounces: 5,
    blocks: 0,
    complaints: 1,
    health: { status: 'warn' },
  },
  {
    brand: 'joybelle',
    subscribers: {
      total: 50, active: 50, held: 0, unsubscribed: 0, blocked: 0,
    },
    sent: null, // no SES row in the window
    views: 20,
    clicks: 9,
    hard_bounces: 0,
    blocks: 0,
    complaints: 0,
    health: { status: 'unknown' }, // unlaunched
  },
  {
    brand: 'delta',
    subscribers: {
      total: 0, active: 0, held: 0, unsubscribed: 0, blocked: 0,
    },
    sent: 0,
    views: 0,
    clicks: 0,
    hard_bounces: 0,
    blocks: 0,
    complaints: 0,
    health: {}, // no health document: launched
  },
];
const brands = [
  { slug: 'ruze', display_name: 'RUZE' },
  { slug: 'thirstygirl', display_name: 'Thirsty Girl' },
  { slug: 'joybelle', display_name: 'JoyBelle' },
];
const order = (rows) => rows.map((r) => r.brand);

test('shapeRows: display names (slug fallback), absent sent and spam rate are null, no status is empty', () => {
  const rows = shapeRows(api, { brands });
  assert.deepEqual(rows.map((r) => r.label), ['RUZE', 'Thirsty Girl', 'JoyBelle', 'delta']);
  assert.equal(rows[2].sent, null);
  assert.equal(rows[1].spam_rate, null);
  assert.equal(rows[0].spam_rate, 0.0012);
  assert.equal(rows[3].status, '');
  assert.equal(rows[0].held, 100);
  assert.equal(shapeRows(undefined).length, 0);
});

test('D2: the launch filter reads the rows\' own health documents -- no document is launched', () => {
  assert.deepEqual(order(shapeRows(api, { brands, launch: 'launched' })), ['ruze', 'thirstygirl', 'delta']);
  assert.deepEqual(order(shapeRows(api, { brands, launch: 'unlaunched' })), ['joybelle']);
  assert.deepEqual(order(shapeRows(api, { brands, launch: 'bogus' })), order(api));
});

// I5
test('I5: totals are summed numerators over summed sends, never an average of rates', () => {
  const rows = shapeRows(api, { brands });
  const t = totalsRow(rows);
  assert.equal(t.totals, true);
  assert.equal(t.total, 1250);
  assert.equal(t.active, 1000);
  assert.equal(t.held, 120);
  assert.equal(t.unsubscribed, 80);
  assert.equal(t.blocked, 50);
  assert.equal(t.sent, 11000);
  assert.equal(t.views, 1420);
  assert.equal(rateOf(t, 'views'), 1420 / 11000);
  assert.equal(t.clicks, 369);
  assert.equal(rateOf(t, 'clicks'), 369 / 11000);
  // The average of the two rates (3% and 6%) would be 4.5%.
  assert.notEqual(rateOf(t, 'clicks'), (0.03 + 0.06) / 2);
  assert.equal(rateOf(t, 'hard_bounces'), 55 / 11000);
  assert.equal(t.spam_rate, null);
  assert.equal(t.status, '');
});

test('D2: the totals row exists only with more than one row and is pinned last, outside the sort', () => {
  const rows = shapeRows(api, { brands });
  assert.equal(totalsRow(rows.slice(0, 1)), null);
  assert.equal(totalsRow([]), null);
  const asc = tableRows(rows, { field: 'brand', order: 'asc' }, 'pct');
  const desc = tableRows(rows, { field: 'brand', order: 'desc' }, 'pct');
  assert.equal(asc[asc.length - 1].totals, true);
  assert.equal(desc[desc.length - 1].totals, true);
  assert.equal(asc.length, rows.length + 1);
  assert.equal(tableRows(rows.slice(0, 1), DEFAULT_STATE.sort, 'pct').length, 1);
});

// I6
test('I6: a missing denominator renders a dash and ranks last in both orders', () => {
  const rows = shapeRows(api, { brands });
  const joy = rows.find((r) => r.brand === 'joybelle');
  const delta = rows.find((r) => r.brand === 'delta');
  ['views', 'clicks', 'hard_bounces', 'blocks', 'complaints'].forEach((f) => {
    assert.equal(rateOf(joy, f), null);
    assert.equal(rateOf(delta, f), null);
    assert.equal(cellText(joy, f, 'pct'), '—');
    assert.equal(cellText(delta, f, 'pct'), '—');
  });
  ['desc', 'asc'].forEach((o) => {
    const got = order(sortRows(rows, { field: 'clicks', order: o }, 'pct'));
    assert.deepEqual(got.slice(2).sort(), ['delta', 'joybelle'], `order ${o}: ${got}`);
  });
  assert.deepEqual(order(sortRows(rows, { field: 'clicks', order: 'asc' }, 'pct')).slice(0, 2), ['ruze', 'thirstygirl']);
  // A missing spam rate and missing sends rank last too.
  ['desc', 'asc'].forEach((o) => {
    const s = order(sortRows(rows, { field: 'spam_rate', order: o }, 'pct'));
    assert.equal(s[0], 'ruze');
    assert.equal(cellText(rows[1], 'spam_rate', 'pct'), '—');
    assert.equal(order(sortRows(rows, { field: 'sent', order: o }, 'pct')).pop(), 'joybelle');
  });
  // num mode: the counts exist, so nothing is missing.
  assert.equal(cellText(joy, 'clicks', 'num'), '9');
});

// I8
test('I8: default first-click directions -- brand ascending, every other column descending', () => {
  assert.equal(defaultOrder('brand'), 'asc');
  ['total', 'active', 'held', 'unsubscribed', 'blocked', 'sent', 'views', 'clicks', 'hard_bounces', 'blocks',
    'complaints', 'spam_rate'].forEach((f) => assert.equal(defaultOrder(f), 'desc', f));
  // A NEW column takes its default direction.
  assert.deepEqual(clickSort({ field: 'clicks', order: 'desc' }, 'held'), { field: 'held', order: 'desc' });
  assert.deepEqual(clickSort({ field: 'clicks', order: 'desc' }, 'brand'), { field: 'brand', order: 'asc' });
  // A repeat click on the current column toggles it, both ways.
  assert.deepEqual(clickSort({ field: 'held', order: 'desc' }, 'held'), { field: 'held', order: 'asc' });
  assert.deepEqual(clickSort({ field: 'held', order: 'asc' }, 'held'), { field: 'held', order: 'desc' });
  // An unknown/unsortable field keeps the current sort.
  assert.deepEqual(clickSort({ field: 'held', order: 'asc' }, 'nope'), { field: 'held', order: 'asc' });
  assert.deepEqual(clickSort(null, 'nope'), DEFAULT_STATE.sort);
});

test('I8: the initial sort is Clicks descending in pct mode', () => {
  assert.deepEqual(DEFAULT_STATE, { sort: { field: 'clicks', order: 'desc' }, mode: 'pct' });
  assert.deepEqual(loadState(null), { sort: { field: 'clicks', order: 'desc' }, mode: 'pct' });
});

test('Views: funnel order before Clicks, sortable, mode-following, no threshold ever', () => {
  // Column order flows from ENGAGEMENT_FIELDS; the default sort stays Clicks (asserted above).
  assert.deepEqual([...ENGAGEMENT_FIELDS], ['views', 'clicks', 'hard_bounces', 'blocks', 'complaints']);
  const rows = shapeRows(api, { brands }).slice(0, 2);
  assert.deepEqual(order(sortRows(rows, { field: 'views', order: 'desc' }, 'pct')), ['thirstygirl', 'ruze']);
  assert.deepEqual(order(sortRows(rows, { field: 'views', order: 'desc' }, 'num')), ['ruze', 'thirstygirl']);
  assert.equal(cellText(rows[0], 'views', 'pct'), '12.00%');
  assert.equal(cellText(rows[0], 'views', 'num'), '1200');
  assert.deepEqual(clickSort({ field: 'clicks', order: 'desc' }, 'views'), { field: 'views', order: 'desc' });
  // Context, not a verdict: no stored threshold applies to views, whatever a doc carries.
  assert.equal(breachesThreshold({ views: { warn: 0, minSent: 0 } }, 'views', 0.9, { sent: 100000 }), false);
  const r = shapeRows([{
    brand: 'b', subscribers: {}, sent: 1000, views: 900, clicks: 0, hard_bounces: 0, blocks: 0, complaints: 0, health: {}, thresholds: { views: { warn: 0.1, minSent: 1 } },
  }])[0];
  assert.deepEqual(engagementCell(r, 'views', 'pct'), { cls: '' });
});

test('I8: an engagement column sorts by the displayed quantity -- rate in pct, count in num', () => {
  const rows = shapeRows(api, { brands }).slice(0, 2);
  assert.deepEqual(order(sortRows(rows, { field: 'clicks', order: 'desc' }, 'pct')), ['thirstygirl', 'ruze']);
  assert.deepEqual(order(sortRows(rows, { field: 'clicks', order: 'desc' }, 'num')), ['ruze', 'thirstygirl']);
  assert.equal(displayValue(rows[0], 'clicks', 'pct'), 0.03);
  assert.equal(displayValue(rows[0], 'clicks', 'num'), 300);
  assert.equal(cellText(rows[0], 'clicks', 'pct'), '3.00%');
  assert.equal(cellText(rows[0], 'clicks', 'num', (n) => n.toLocaleString('en-US')), '300');
  // Subscriber columns are counts in both modes.
  assert.equal(cellText(rows[0], 'held', 'pct', (n) => n.toLocaleString('en-US')), '100');
  assert.equal(cellText(rows[0], 'spam_rate', 'num'), '0.12%');
  // Brand sorts by display name.
  const byBrand = order(sortRows(shapeRows(api, { brands }), { field: 'brand', order: 'asc' }, 'pct'));
  assert.deepEqual(byBrand, ['delta', 'joybelle', 'ruze', 'thirstygirl']);
});

test('D1: the pane pref -- brands:get gates every non-Overview pane; the legacy boolean only while unset', () => {
  assert.equal(initialPane(false, 'matrix', true), 'overview');
  assert.equal(initialPane(true, 'matrix', true), 'matrix');
  assert.equal(initialPane(true, 'overview', true), 'overview');
  assert.equal(initialPane(true, undefined, true), 'ses');
  assert.equal(initialPane(true, null, false), 'overview');
  assert.equal(initialPane(true, 'bogus', true), 'overview');
});

// I7
test('I7: a throwing sessionStorage leaves the defaults and never throws', () => {
  const throwing = {
    getItem() { throw new Error('SecurityError'); },
    setItem() { throw new Error('QuotaExceededError'); },
  };
  assert.deepEqual(loadState(throwing), { sort: { field: 'clicks', order: 'desc' }, mode: 'pct' });
  assert.equal(saveState(throwing, { sort: { field: 'held', order: 'asc' }, mode: 'num' }), false);
  assert.equal(saveState(undefined, DEFAULT_STATE), false);
  // The property access itself throwing (blocked site data).
  const win = Object.defineProperty({}, 'sessionStorage', { get() { throw new Error('SecurityError'); } });
  assert.equal(sessionStore(win), null);
  assert.deepEqual(loadState(sessionStore(win)), DEFAULT_STATE);
  assert.equal(sessionStore(undefined), null);
});

test('I7: state round-trips through a working store; garbage reads as defaults', () => {
  const m = new Map();
  const store = { getItem: (k) => (m.has(k) ? m.get(k) : null), setItem: (k, v) => m.set(k, v) };
  assert.equal(saveState(store, { sort: { field: 'held', order: 'asc' }, mode: 'num' }), true);
  assert.deepEqual(JSON.parse(m.get(STORAGE_KEY)), { sort: { field: 'held', order: 'asc' }, mode: 'num' });
  assert.deepEqual(loadState(store), { sort: { field: 'held', order: 'asc' }, mode: 'num' });
  m.set(STORAGE_KEY, '{not json');
  assert.deepEqual(loadState(store), DEFAULT_STATE);
  m.set(STORAGE_KEY, JSON.stringify({ sort: { field: 'evil', order: 'sideways' }, mode: 'x' }));
  assert.deepEqual(loadState(store), DEFAULT_STATE);
});

// GP-SPAM-VISIBILITY-SPEC I3: the GP Spam % cell's class/tooltip from the STORED status and flag.
test('I3: spamCell colors by the stored spam_status; a quiet flag gets the dotted tooltip; color wins', () => {
  const row = (health) => shapeRows([{ brand: 'b', subscribers: {}, health }])[0];
  const plain = { cls: '', style: '', tooltip: SPAM_HELP };
  const quiet = { cls: '', style: 'text-decoration: underline dotted;', tooltip: SPAM_FLAGGED_HELP };

  assert.equal(SPAM_HELP, 'dashboard.matrix.spamHelp');
  assert.equal(SPAM_FLAGGED_HELP, 'dashboard.matrix.spamFlaggedHelp');

  // Color by status alone.
  assert.deepEqual(spamCell(row({ spam_status: 'issues', spam_rate: 0.015 })), { ...plain, cls: 'has-text-danger' });
  assert.deepEqual(spamCell(row({ spam_status: 'warn', spam_rate: 0.002 })), { ...plain, cls: 'has-text-warning-dark' });
  assert.deepEqual(spamCell(row({ spam_status: 'ok', spam_rate: 0 })), plain);
  assert.deepEqual(spamCell(row({ spam_status: 'unknown' })), plain);
  assert.deepEqual(spamCell(row({})), plain);
  assert.deepEqual(spamCell(row({ spam_status: 'bogus' })), plain);
  // The color keys off the stored status, never the value (no threshold in the fork).
  assert.deepEqual(spamCell(row({ spam_status: 'ok', spam_rate: 0.5 })), plain);

  // Flagged but quiet (ok / unknown / absent status): dotted underline + the flagged tooltip.
  assert.deepEqual(spamCell(row({ spam_status: 'ok', spam_flagged: true, spam_rate: 0 })), quiet);
  assert.deepEqual(spamCell(row({ spam_status: 'unknown', spam_flagged: true })), quiet);
  assert.deepEqual(spamCell(row({ spam_flagged: true })), quiet);

  // Flagged while warn/issues: the color wins, no tooltip duplication.
  assert.deepEqual(spamCell(row({ spam_status: 'issues', spam_flagged: true })), { ...plain, cls: 'has-text-danger' });
  assert.deepEqual(spamCell(row({ spam_status: 'warn', spam_flagged: true })), { ...plain, cls: 'has-text-warning-dark' });

  // Only a literal true flags.
  assert.deepEqual(spamCell(row({ spam_status: 'ok', spam_flagged: 'true' })), plain);

  // The totals row carries no spam signal; a missing row is plain.
  const t = totalsRow([row({ spam_status: 'issues', spam_flagged: true }), row({ spam_flagged: true })]);
  assert.equal(t.spam_status, '');
  assert.equal(t.spam_flagged, false);
  assert.deepEqual(spamCell(t), plain);
  assert.deepEqual(spamCell({ totals: true, spam_status: 'issues', spam_flagged: true }), plain);
  assert.deepEqual(spamCell(undefined), plain);
});

test('I3: shapeRows carries spam_status and spam_flagged; sorting is untouched', () => {
  const rows = shapeRows([
    { brand: 'a', subscribers: {}, health: { spam_rate: 0.01, spam_status: 'issues', spam_flagged: true } },
    { brand: 'b', subscribers: {}, health: { spam_rate: 0.02, spam_status: 'ok' } },
  ]);
  assert.equal(rows[0].spam_status, 'issues');
  assert.equal(rows[0].spam_flagged, true);
  assert.equal(rows[1].spam_flagged, false);
  // Still by the numeric value: b (0.02) before a (0.01), whatever the status.
  assert.deepEqual(order(sortRows(rows, { field: 'spam_rate', order: 'desc' }, 'pct')), ['b', 'a']);
});

// DASHBOARDS-POLISH-SPEC I2: the engagement coloring reads ONLY the stored thresholds and floors.
// The fixture values are deliberately NOT production's, so a hard-coded number in the fork fails.
const TH = Object.freeze({
  clicks: { warnBelow: 0.05, minSent: 500 },
  hardBounces: { warn: 0.03, minSent: 400 },
  complaints: { warn: 0.004, minSent: 2000, minCount: 3 },
});
const mrow = (o) => shapeRows([{
  brand: 'b', subscribers: {}, clicks: 0, hard_bounces: 0, blocks: 0, complaints: 0, health: {}, ...o,
}])[0];

test('I2: breachesThreshold compares the displayed rate against the stored values; floors gate it', () => {
  // clicks: below warnBelow is red, at/above is not; only at sent >= minSent.
  assert.equal(breachesThreshold(TH, 'clicks', 0.049, { sent: 500 }), true);
  assert.equal(breachesThreshold(TH, 'clicks', 0.05, { sent: 500 }), false);
  assert.equal(breachesThreshold(TH, 'clicks', 0.049, { sent: 499 }), false);
  // hard bounces: at/above warn is red; floor minSent.
  assert.equal(breachesThreshold(TH, 'hard_bounces', 0.03, { sent: 400 }), true);
  assert.equal(breachesThreshold(TH, 'hard_bounces', 0.029, { sent: 400 }), false);
  assert.equal(breachesThreshold(TH, 'hard_bounces', 0.5, { sent: 399 }), false);
  // complaints: floor is sent >= minSent OR complaints >= minCount.
  assert.equal(breachesThreshold(TH, 'complaints', 0.004, { sent: 2000, complaints: 8 }), true);
  assert.equal(breachesThreshold(TH, 'complaints', 0.01, { sent: 300, complaints: 3 }), true);
  assert.equal(breachesThreshold(TH, 'complaints', 0.01, { sent: 300, complaints: 2 }), false);
  assert.equal(breachesThreshold(TH, 'complaints', 0.0039, { sent: 5000, complaints: 19 }), false);
  // Blocks (and anything else) has no threshold.
  assert.equal(breachesThreshold(TH, 'blocks', 0.9, { sent: 100000 }), false);
  assert.equal(breachesThreshold({ ...TH, blocks: { warn: 0, minSent: 0 } }, 'blocks', 0.9, { sent: 100000 }), false);
  // No object, a malformed entry, a non-number rate: nothing.
  assert.equal(breachesThreshold(null, 'clicks', 0, { sent: 100000 }), false);
  assert.equal(breachesThreshold(undefined, 'hard_bounces', 1, { sent: 100000 }), false);
  assert.equal(breachesThreshold({ clicks: { warnBelow: '0.05', minSent: 500 } }, 'clicks', 0, { sent: 1000 }), false);
  assert.equal(breachesThreshold({ clicks: { warnBelow: 0.05 } }, 'clicks', 0, { sent: 1000 }), false);
  assert.equal(breachesThreshold(TH, 'clicks', null, { sent: 1000 }), false);
});

test('I2 / POLISH3 I1: engagementCell -- stored values only, both modes alike, no thresholds colors nothing', () => {
  const red = { cls: BREACH_CLASS };
  const plain = { cls: '' };
  assert.equal(BREACH_CLASS, 'has-text-danger'); // the GP Spam % issues treatment
  const r = mrow({
    sent: 1000, clicks: 10, hard_bounces: 40, blocks: 30, complaints: 5, thresholds: TH,
  });
  assert.deepEqual(engagementCell(r, 'clicks', 'pct'), red); // 1% < 5%
  assert.deepEqual(engagementCell(r, 'hard_bounces', 'pct'), red); // 4% >= 3%
  assert.deepEqual(engagementCell(r, 'complaints', 'pct'), red); // 0.5% >= 0.4%, 5 >= minCount
  assert.deepEqual(engagementCell(r, 'blocks', 'pct'), plain);
  // DASHBOARDS-POLISH3-SPEC I1 (replacing POLISH I2's "Count (#) mode never colors"): the breach is
  // rate-based and identical in both modes -- same inputs, both cells colored (or both plain).
  ['clicks', 'hard_bounces', 'complaints', 'blocks'].forEach((f) => assert.deepEqual(engagementCell(r, f, 'num'), engagementCell(r, f, 'pct')));
  ['clicks', 'hard_bounces', 'complaints'].forEach((f) => assert.deepEqual(engagementCell(r, f, 'num'), red));
  assert.deepEqual(engagementCell(r, 'blocks', 'num'), plain);
  // A row without thresholds (an older document) colors nothing.
  const old = mrow({
    sent: 1000, clicks: 0, hard_bounces: 900, complaints: 900,
  });
  assert.equal(old.thresholds, null);
  ['clicks', 'hard_bounces', 'complaints'].forEach((f) => assert.deepEqual(engagementCell(old, f, 'pct'), plain));
  // A non-object thresholds value is no thresholds.
  assert.equal(mrow({ thresholds: [1] }).thresholds, null);
  assert.equal(mrow({ thresholds: 'x' }).thresholds, null);
  // Below-floor rates color nothing; no sends (no rate) color nothing.
  const small = mrow({
    sent: 300, clicks: 0, hard_bounces: 200, complaints: 1, thresholds: TH,
  });
  ['clicks', 'hard_bounces', 'complaints'].forEach((f) => assert.deepEqual(engagementCell(small, f, 'pct'), plain));
  const none = mrow({
    sent: null, clicks: 0, hard_bounces: 5, complaints: 5, thresholds: TH,
  });
  ['clicks', 'hard_bounces', 'complaints'].forEach((f) => assert.deepEqual(engagementCell(none, f, 'pct'), plain));
  // A healthy brand is uncolored.
  const clean = mrow({
    sent: 10000, clicks: 900, hard_bounces: 10, complaints: 1, thresholds: TH,
  });
  ['clicks', 'hard_bounces', 'complaints'].forEach((f) => assert.deepEqual(engagementCell(clean, f, 'pct'), plain));
  assert.deepEqual(engagementCell(undefined, 'clicks', 'pct'), plain);
});

test('I2: the compared rate is the DISPLAYED one (rounded as cellText shows it)', () => {
  // 499.8 / 10000 = 4.998% displays 5.00% -- not below a 5% target.
  const edge = mrow({ sent: 10000, clicks: 499.8, thresholds: TH });
  assert.equal(cellText(edge, 'clicks', 'pct'), '5.00%');
  assert.equal(displayedRate(edge, 'clicks'), 0.05);
  assert.deepEqual(engagementCell(edge, 'clicks', 'pct'), { cls: '' });
  // 39.96 / 10000 = 0.3996% displays 0.40% -- at the 0.4% complaint target.
  const c = mrow({ sent: 10000, complaints: 39.96, thresholds: TH });
  assert.equal(cellText(c, 'complaints', 'pct'), '0.40%');
  assert.deepEqual(engagementCell(c, 'complaints', 'pct'), { cls: BREACH_CLASS });
  assert.equal(displayedRate(mrow({ sent: null }), 'clicks'), null);
});

test('I2: the totals row colors from the FIRST row (brand order) carrying thresholds; none → nothing', () => {
  const other = { ...TH, clicks: { warnBelow: 0.5, minSent: 1 } }; // a different carrier's values
  const ar = (brand, sent, o) => ({
    brand, subscribers: {}, sent, ...o,
  });
  const rows = shapeRows([
    ar('zeta', 1000, { clicks: 10, thresholds: other }),
    ar('alpha', 1000, { clicks: 30 }), // no thresholds (an older doc)
    ar('mid', 1000, { clicks: 20, thresholds: TH }),
  ]);
  assert.equal(firstThresholds(rows), rows[2].thresholds); // 'mid' is the first carrier by brand
  const t = totalsRow(rows);
  assert.equal(t.thresholds, rows[2].thresholds);
  // 60 / 3000 = 2% < TH's 5%, 3000 >= TH's 500-send floor -> red.
  assert.deepEqual(engagementCell(t, 'clicks', 'pct'), { cls: BREACH_CLASS });
  // The two orders below differ only in which carrier is first by brand; 400 summed sends at 0.5%
  // is red under `other` (floor 1, below 50%) and uncolored under TH (floor 500 unmet).
  const t2 = totalsRow(shapeRows([ar('b', 200, { clicks: 1, thresholds: TH }), ar('a', 200, { clicks: 1, thresholds: other })]));
  assert.deepEqual(engagementCell(t2, 'clicks', 'pct'), { cls: BREACH_CLASS });
  const t3 = totalsRow(shapeRows([ar('a', 200, { clicks: 1, thresholds: TH }), ar('b', 200, { clicks: 1, thresholds: other })]));
  assert.deepEqual(engagementCell(t3, 'clicks', 'pct'), { cls: '' });
  // No row carries thresholds -> the totals row colors nothing.
  const bare = totalsRow(shapeRows([ar('a', 1000, { clicks: 0, hard_bounces: 900 }), ar('b', 1000, { clicks: 0, hard_bounces: 900 })]));
  assert.equal(bare.thresholds, null);
  ['clicks', 'hard_bounces', 'complaints'].forEach((f) => assert.deepEqual(engagementCell(bare, f, 'pct'), { cls: '' }));
  assert.equal(firstThresholds(undefined), null);
});

test('I2: the fork source carries no numeric threshold literal (code lines of brandMatrix.mjs)', () => {
  const src = readFileSync(new URL('./brandMatrix.mjs', import.meta.url), 'utf8');
  const code = src.split('\n').filter((l) => !/^\s*\/\//.test(l)).map((l) => l.replace(/\/\/.*$/, '')).join('\n');
  assert.equal(/\b0\.\d+/.test(code), false, 'a fractional literal in brandMatrix.mjs code');
  assert.equal(/\b(200|1000)\b/.test(code), false, 'a floor-like literal in brandMatrix.mjs code');
});
