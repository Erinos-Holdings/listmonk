// Fork (brand matrix, integrations listmonk/BRAND-MATRIX-SPEC I5-I8). Run: yarn test
// (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  DEFAULT_STATE, STORAGE_KEY, cellText, defaultOrder, displayValue, initialPane, loadState, rateOf, resolveSort,
  SPAM_FLAGGED_HELP, SPAM_HELP, saveState, sessionStore, shapeRows, sortRows, spamCell, tableRows, totalsRow,
} from './brandMatrix.mjs'; // eslint-disable-line import/extensions

const api = [
  {
    brand: 'ruze',
    subscribers: {
      total: 1000, active: 800, held: 100, unsubscribed: 60, blocked: 40,
    },
    sent: 10000,
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
  ['clicks', 'hard_bounces', 'blocks', 'complaints'].forEach((f) => {
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
  ['total', 'active', 'held', 'unsubscribed', 'blocked', 'sent', 'clicks', 'hard_bounces', 'blocks',
    'complaints', 'spam_rate'].forEach((f) => assert.equal(defaultOrder(f), 'desc', f));
  // A NEW column takes its default whatever Buefy's first-click direction says.
  assert.deepEqual(resolveSort({ field: 'clicks', order: 'desc' }, 'held', 'asc'), { field: 'held', order: 'desc' });
  assert.deepEqual(resolveSort({ field: 'clicks', order: 'desc' }, 'brand', 'asc'), { field: 'brand', order: 'asc' });
  // The current column toggles with Buefy.
  assert.deepEqual(resolveSort({ field: 'held', order: 'desc' }, 'held', 'asc'), { field: 'held', order: 'asc' });
  assert.deepEqual(resolveSort({ field: 'held', order: 'asc' }, 'held', 'desc'), { field: 'held', order: 'desc' });
  // An unknown field keeps the current sort.
  assert.deepEqual(resolveSort({ field: 'held', order: 'asc' }, 'nope', 'desc'), { field: 'held', order: 'asc' });
});

test('I8: the initial sort is Clicks descending in pct mode', () => {
  assert.deepEqual(DEFAULT_STATE, { sort: { field: 'clicks', order: 'desc' }, mode: 'pct' });
  assert.deepEqual(loadState(null), { sort: { field: 'clicks', order: 'desc' }, mode: 'pct' });
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
