// Fork (campaign matrix, integrations CAMPAIGN-MATRIX-SPEC I6) -- the pure half of the Campaign
// Matrix: the tier fold through CLIENT_TIERS only (never a summed tier figure), the %/$ and # cells,
// the totals row's covered denominators, '—' sorting last, the Date / journey sort, the launch
// filter, the GP send-day colour and the session state. Run: node --test src/campaignMatrix.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  COUNT_FIELDS, DEFAULT_STATE, MISSING, STORAGE_KEY, TIER_FIELDS, cellText, clickSort, conversionValue, dateCell,
  displayValue, filterByLaunch, foldTiers, gpClass, gpLevel, gpTip, loadState, saveState, shapeRow, shapeRows,
  sortRows, tableRows, totalsRow,
} from './campaignMatrix.mjs'; // eslint-disable-line import/extensions
import { CLIENT_TIERS, clientTier } from './clientRows.mjs'; // eslint-disable-line import/extensions
import { scopedCell } from './conversionCells.mjs'; // eslint-disable-line import/extensions

const api = [
  {
    id: 1,
    name: 'Spring',
    evergreen: false,
    send_day: '2026-10-20',
    brand: 'ruze',
    sent: 1000,
    views: 300,
    view_clients: {
      'outlook-windows': 100, 'apple-mail': 150, 'gmail-proxy': 20, automation: 5, other: 7, '': 40,
    },
    clicks: 50,
    hard_bounces: 10,
    soft_bounces: 4,
    complaints: 1,
    tracked: { orders: 3, sales_usd: 120.5 },
    matched: null,
    gp_rate: 0.0025,
    gp_thresholds: { warn: 0.001, issues: 0.003 },
    client_floor: false,
  },
  {
    id: 2,
    name: 'Autumn',
    evergreen: false,
    send_day: '2026-09-15',
    brand: 'thirstygirl',
    sent: 500,
    views: 100,
    view_clients: { 'browser-ios': 60 },
    clicks: 10,
    hard_bounces: 0,
    soft_bounces: 0,
    complaints: 0,
    tracked: null,
    matched: { orders: 1, sales_usd: 40 },
    gp_rate: null,
    gp_thresholds: null,
    client_floor: true,
  },
  {
    id: 3,
    name: 'Welcome',
    evergreen: true,
    last_sent: '2026-10-26',
    brand: 'ruze',
    sent: 200,
    views: 80,
    view_clients: { yahoo: 30 },
    clicks: 20,
    hard_bounces: 2,
    soft_bounces: 0,
    complaints: 0,
    tracked: { orders: 1, sales_usd: 10 },
    matched: { orders: 2, sales_usd: 30 },
    gp_rate: null,
    gp_thresholds: null,
    client_floor: false,
  },
  {
    id: 4,
    name: 'Two brands',
    evergreen: false,
    send_day: '2026-10-01',
    brand: null,
    sent: 0,
    views: 0,
    view_clients: {},
    clicks: 0,
    hard_bounces: 0,
    soft_bounces: 0,
    complaints: 0,
    tracked: null,
    matched: null,
    gp_rate: null,
    gp_thresholds: null,
    client_floor: false,
  },
];

test('I6: the tier fold goes through CLIENT_TIERS only -- Unknown and untiered tokens fall in no tier', () => {
  assert.deepEqual(foldTiers(api[0].view_clients), { client: 100, proxy: 170, machine: 5 });
  // Every token of the tier map lands in exactly its CLIENT_TIERS tier.
  Object.keys(CLIENT_TIERS).forEach((token) => {
    const f = foldTiers({ [token]: 1 });
    TIER_FIELDS.forEach((k) => assert.equal(f[k], k === clientTier(token) ? 1 : 0, `${token} -> ${k}`));
  });
  assert.deepEqual(foldTiers({
    other: 9, 'bare-mozilla': 3, '': 4, 'never-heard-of': 2,
  }), { client: 0, proxy: 0, machine: 0 });
  assert.deepEqual(foldTiers(null), { client: 0, proxy: 0, machine: 0 });
});

test('I6: no summed tier figure -- Total is the server count, never the tiers added up', () => {
  const r = shapeRow(api[0]);
  assert.equal(r.views, 300);
  assert.notEqual(r.views, r.client + r.proxy + r.machine);
  const keys = Object.keys(r);
  ['human', 'tiers', 'tierTotal', 'confident'].forEach((k) => assert.ok(!keys.includes(k), k));
});

test('I6: a client_floor row shows its tiers missing, Total unaffected', () => {
  const r = shapeRow(api[1]);
  TIER_FIELDS.forEach((k) => assert.equal(r[k], null));
  assert.equal(r.views, 100);
  assert.equal(cellText(r, 'client', 'pct'), MISSING);
  assert.equal(cellText(r, 'client', 'num'), MISSING);
  assert.equal(cellText(r, 'views', 'num', String), '100');
});

test('I6: %/$ cells are count / Sent (conversions in $); # cells are counts (conversions in orders)', () => {
  const r = shapeRow(api[0]);
  assert.equal(cellText(r, 'views', 'pct'), '30.00%');
  assert.equal(cellText(r, 'proxy', 'pct'), '17.00%');
  assert.equal(cellText(r, 'hard_bounces', 'pct'), '1.00%');
  assert.equal(cellText(r, 'views', 'num', String), '300');
  assert.equal(cellText(r, 'sent', 'pct', String), '1000');
  assert.equal(cellText(r, 'tracked', 'pct', String, (n) => `$${n}`), '$120.5');
  assert.equal(cellText(r, 'tracked', 'num', String), '3');
  assert.equal(cellText(r, 'matched', 'pct'), MISSING);
  // GP is a rate in both modes.
  assert.equal(cellText(r, 'gp_rate', 'pct'), '0.25%');
  assert.equal(cellText(r, 'gp_rate', 'num'), '0.25%');
  // A zero Sent has no rate.
  assert.equal(cellText(shapeRow(api[3]), 'views', 'pct'), MISSING);
  // D9: the conversion cell reuses conversionCells.mjs::scopedCell over the server-shaped value.
  const meta = { as_of: '2026-10-28' };
  assert.deepEqual(scopedCell(conversionValue(r, 'tracked'), meta, String), { orders: '3', sales: '$121' });
  assert.deepEqual(scopedCell(conversionValue(r, 'matched'), meta, String), { orders: MISSING, sales: '' });
});

test('I6: the totals row -- per column over the rows carrying a value, with those rows\' Sent', () => {
  const rows = shapeRows(api);
  const t = totalsRow(rows, 'Total');
  assert.equal(t.totals, true);
  assert.equal(t.sent, 1700);
  assert.equal(t.views, 480);
  assert.equal(cellText(t, 'views', 'pct'), '28.24%'); // 480 / 1700
  // client: rows 1, 3, 4 carry a value (row 2 is before the floor) -> (100 + 0 + 0) / (1000 + 200 + 0).
  assert.equal(t.client, 100);
  assert.equal(displayValue(t, 'client', 'pct'), 100 / 1200);
  // Conversions sum the covered rows only; GP and date are blank.
  assert.deepEqual(t.tracked, { orders: 4, sales: 130.5 });
  assert.deepEqual(t.matched, { orders: 3, sales: 70 });
  assert.equal(t.gp_rate, null);
  assert.equal(gpClass(t), '');
  assert.deepEqual(dateCell(t), { text: '', tip: null });
  // A column no row carries is '—'.
  const floorOnly = shapeRows([api[1], { ...api[1], id: 9 }]);
  assert.equal(cellText(totalsRow(floorOnly), 'client', 'pct'), MISSING);
  // One row: no totals row (the Brand Matrix's rule).
  assert.equal(totalsRow(shapeRows([api[0]])), null);
  // tableRows pins it last.
  const tr = tableRows(rows, DEFAULT_STATE.sort, 'pct');
  assert.equal(tr[tr.length - 1].totals, true);
});

test('I6: Date sort -- broadcasts by send date, journeys after them by last sent, in both orders', () => {
  const rows = shapeRows([...api, {
    ...api[2], id: 5, name: 'Old journey', last_sent: '2026-10-01',
  }]);
  const ids = (s) => sortRows(rows, s, 'pct').map((r) => r.id);
  assert.deepEqual(ids({ field: 'date', order: 'desc' }), [1, 4, 2, 3, 5]);
  assert.deepEqual(ids({ field: 'date', order: 'asc' }), [2, 4, 1, 5, 3]);
  assert.deepEqual(dateCell(rows.find((r) => r.id === 3)), {
    text: MISSING, tip: { key: 'analytics.matrix.journeyLastSent', params: { date: '2026-10-26' } },
  });
  assert.deepEqual(dateCell(rows[0]), { text: '2026-10-20', tip: null });
});

test('I6: a missing value sorts last in BOTH orders', () => {
  const rows = shapeRows(api);
  const ids = (field, order, mode = 'pct') => sortRows(rows, { field, order }, mode).map((r) => r.id);
  // client: row 2 is before the floor (null) -> last both ways; row 4 has Sent 0 -> null in pct.
  assert.deepEqual(ids('client', 'desc'), [1, 3, 2, 4]);
  assert.deepEqual(ids('client', 'asc'), [3, 1, 2, 4]);
  // gp_rate: only row 1 has one.
  assert.equal(ids('gp_rate', 'asc')[0], 1);
  assert.equal(ids('gp_rate', 'desc')[0], 1);
  // matched in # mode: rows 1 and 4 are uncovered -> last.
  assert.deepEqual(ids('matched', 'asc', 'num'), [2, 3, 1, 4]);
  assert.deepEqual(ids('matched', 'desc', 'num'), [3, 2, 1, 4]);
  // Brand: the two-brand row has none -> last both ways.
  assert.equal(ids('brand', 'asc')[3], 4);
  assert.equal(ids('brand', 'desc')[3], 4);
});

test('I6: clickSort -- a new column takes its default direction, a repeat toggles', () => {
  assert.deepEqual(clickSort(DEFAULT_STATE.sort, 'name'), { field: 'name', order: 'asc' });
  assert.deepEqual(clickSort(DEFAULT_STATE.sort, 'views'), { field: 'views', order: 'desc' });
  assert.deepEqual(clickSort({ field: 'views', order: 'desc' }, 'views'), { field: 'views', order: 'asc' });
  assert.deepEqual(clickSort(DEFAULT_STATE.sort, 'bogus'), DEFAULT_STATE.sort);
  COUNT_FIELDS.forEach((f) => assert.equal(clickSort(DEFAULT_STATE.sort, f).order, 'desc'));
});

test('D6: the launch filter narrows by the campaign brand; an unbranded row survives only All', () => {
  const rows = shapeRows(api);
  const docs = [{ brand: 'thirstygirl', status: 'unknown' }];
  assert.deepEqual(filterByLaunch(rows, 'all', docs).map((r) => r.id), [1, 2, 3, 4]);
  assert.deepEqual(filterByLaunch(rows, 'launched', docs).map((r) => r.id), [1, 3]);
  assert.deepEqual(filterByLaunch(rows, 'unlaunched', docs).map((r) => r.id), [2]);
  assert.deepEqual(shapeRows(api, { launch: 'launched', docs }).map((r) => r.id), [1, 3]);
});

test('D7: the GP colour compares the send-day rate against the STORED thresholds', () => {
  const t = { warn: 0.001, issues: 0.003 };
  assert.equal(gpLevel(0.003, t), 'issues');
  assert.equal(gpLevel(0.0025, t), 'warn');
  assert.equal(gpLevel(0.001, t), 'warn');
  assert.equal(gpLevel(0.0009, t), '');
  assert.equal(gpLevel(0.5, null), '');
  assert.equal(gpLevel(null, t), '');
  assert.equal(gpClass(shapeRow(api[0])), 'has-text-warning-dark');
  assert.equal(gpClass(shapeRow({ ...api[0], gp_rate: 0.004 })), 'has-text-danger');
  // Hovers: journey, no brand, no point (names brand and day).
  assert.equal(gpTip(shapeRow(api[2])).key, 'analytics.matrix.gpJourney');
  assert.equal(gpTip(shapeRow(api[3])).key, 'analytics.matrix.gpNoBrand');
  assert.deepEqual(gpTip(shapeRow(api[1], () => 'Thirsty Girl')), {
    key: 'analytics.matrix.gpNone', params: { brand: 'Thirsty Girl', date: '2026-09-15' },
  });
});

test('state: the session state round-trips and anything unreadable is the default', () => {
  const mem = new Map();
  const store = { getItem: (k) => (mem.has(k) ? mem.get(k) : null), setItem: (k, v) => mem.set(k, v) };
  assert.deepEqual(loadState(store), { sort: { field: 'date', order: 'desc' }, mode: 'pct', kind: 'all' });
  assert.equal(saveState(store, { sort: { field: 'views', order: 'asc' }, mode: 'num', kind: 'automations' }), true);
  assert.deepEqual(JSON.parse(mem.get(STORAGE_KEY)), { sort: { field: 'views', order: 'asc' }, mode: 'num', kind: 'automations' });
  assert.deepEqual(loadState(store).kind, 'automations');
  mem.set(STORAGE_KEY, '{bad');
  assert.deepEqual(loadState(store), loadState(null));
  const throwing = { getItem: () => { throw new Error('x'); }, setItem: () => { throw new Error('x'); } };
  assert.deepEqual(loadState(throwing).mode, 'pct');
  assert.equal(saveState(throwing, {}), false);
});
