// Fork (campaign matrix, integrations CAMPAIGN-MATRIX-SPEC I6) -- the pure half of the Campaign
// Matrix: the %/$ and # cells, the totals row's covered denominators, '—' sorting last, the Date /
// journey sort, the launch filter, the GP send-day colour and the session state.
// Fork (matrix polish, integrations MATRIX-POLISH-SPEC I8) -- the shop-link cells (%/# and totals,
// '—' for a campaign with no shop link), the header texts (one Views column, the Shop Link
// Attribution group, Tracked / Matched), the Brand column hidden under a single-brand selection, the
// 25-character name with the full title. Run: node --test src/campaignMatrix.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  COUNT_FIELDS, DEFAULT_STATE, MISSING, NAME_MAX, SHOP_FIELDS, STORAGE_KEY, cellText, clickSort, conversionValue,
  dateCell, displayValue, filterByLaunch, gpClass, gpLevel, gpTip, groupHeaders, loadState, midColumns, nameCell,
  saveState, shapeRow, shapeRows, showBrandColumn, sortRows, tableRows, totalsRow,
} from './campaignMatrix.mjs'; // eslint-disable-line import/extensions
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
    clicks: 50,
    shop_founder: 5,
    shop_self: 3,
    shop_creator: 2,
    shop_corp: 0,
    hard_bounces: 10,
    soft_bounces: 4,
    complaints: 1,
    tracked: { orders: 3, sales_usd: 120.5 },
    matched: null,
    gp_rate: 0.0025,
    gp_thresholds: { warn: 0.001, issues: 0.003 },
  },
  {
    id: 2,
    name: 'Autumn',
    evergreen: false,
    send_day: '2026-09-15',
    brand: 'thirstygirl',
    sent: 500,
    views: 100,
    clicks: 10,
    // No shop link in the body: the four kinds are null.
    shop_founder: null,
    shop_self: null,
    shop_creator: null,
    shop_corp: null,
    hard_bounces: 0,
    soft_bounces: 0,
    complaints: 0,
    tracked: null,
    matched: { orders: 1, sales_usd: 40 },
    gp_rate: null,
    gp_thresholds: null,
  },
  {
    id: 3,
    name: 'Welcome',
    evergreen: true,
    last_sent: '2026-10-26',
    brand: 'ruze',
    sent: 200,
    views: 80,
    clicks: 20,
    shop_founder: 2,
    shop_self: 0,
    shop_creator: 0,
    shop_corp: 4,
    hard_bounces: 2,
    soft_bounces: 0,
    complaints: 0,
    tracked: { orders: 1, sales_usd: 10 },
    matched: { orders: 2, sales_usd: 30 },
    gp_rate: null,
    gp_thresholds: null,
  },
  {
    id: 4,
    name: 'Two brands',
    evergreen: false,
    send_day: '2026-10-01',
    brand: null,
    sent: 0,
    views: 0,
    clicks: 0,
    shop_founder: 0,
    shop_self: 0,
    shop_creator: 0,
    shop_corp: 0,
    hard_bounces: 0,
    soft_bounces: 0,
    complaints: 0,
    tracked: null,
    matched: null,
    gp_rate: null,
    gp_thresholds: null,
  },
];

test('MATRIX-POLISH I8: shop-link cells -- count / Sent in %/$, counts in #, — with no shop link', () => {
  assert.deepEqual(SHOP_FIELDS, ['shop_founder', 'shop_self', 'shop_creator', 'shop_corp']);
  const r = shapeRow(api[0]);
  assert.equal(cellText(r, 'shop_founder', 'pct'), '0.50%');
  assert.equal(cellText(r, 'shop_self', 'pct'), '0.30%');
  assert.equal(cellText(r, 'shop_corp', 'pct'), '0.00%', 'an unclicked kind is a real zero');
  assert.equal(cellText(r, 'shop_creator', 'num', String), '2');
  const none = shapeRow(api[1]);
  SHOP_FIELDS.forEach((k) => {
    assert.equal(none[k], null);
    assert.equal(cellText(none, k, 'pct'), MISSING, `${k} %`);
    assert.equal(cellText(none, k, 'num'), MISSING, `${k} #`);
  });
  // A row with a shop link but no clicks shows 0, not —.
  assert.equal(cellText(shapeRow(api[3]), 'shop_founder', 'num', String), '0');
});

test('MATRIX-POLISH I8: shop-link totals -- over the rows with a shop link, with those rows\' Sent', () => {
  const t = totalsRow(shapeRows(api), 'Total');
  // shop_founder: rows 1, 3, 4 carry a value (row 2 has no shop link) -> (5 + 2 + 0) / (1000 + 200 + 0).
  assert.equal(t.shop_founder, 7);
  assert.equal(displayValue(t, 'shop_founder', 'pct'), 7 / 1200);
  assert.equal(cellText(t, 'shop_corp', 'num', String), '4');
  // No row with a shop link -> the totals cell is —.
  const noShop = shapeRows([api[1], { ...api[1], id: 9 }]);
  assert.equal(cellText(totalsRow(noShop), 'shop_self', 'pct'), MISSING);
});

test('MATRIX-POLISH I8: header texts -- one Views column, Shop Link Attribution after Clicks, Tracked / Matched', () => {
  const cols = midColumns(false);
  assert.deepEqual(cols.map((c) => c.field), [
    'views', 'clicks', 'tracked', 'matched',
    'shop_founder', 'shop_self', 'shop_creator', 'shop_corp', 'hard_bounces', 'soft_bounces', 'complaints',
  ]);
  assert.deepEqual(cols.map((c) => c.label), [
    'analytics.matrix.views', 'dashboard.matrix.clicks',
    'analytics.conversions.tracked', 'analytics.conversions.matched',
    'analytics.matrix.founder', 'analytics.matrix.self', 'analytics.matrix.creator', 'analytics.matrix.corp',
    'analytics.matrix.hard', 'analytics.matrix.soft', 'analytics.matrix.fbl',
  ]);
  cols.filter((c) => c.field.startsWith('shop_')).forEach((c) => assert.equal(c.group, 'bm-shop'));
  assert.equal(midColumns(true).slice(-1)[0].field, 'gp_rate');
  // No tier columns, nowhere.
  ['client', 'proxy', 'machine'].forEach((f) => {
    assert.ok(!COUNT_FIELDS.includes(f), f);
    assert.ok(!midColumns(true).some((c) => c.field === f), f);
  });
  // The group row: nothing over Views/Clicks, then the four groups; spans match the leaves.
  const g = groupHeaders(false);
  assert.deepEqual(g.map((x) => x.label), [
    '', 'analytics.conversions.title', 'analytics.matrix.shopLinks', 'analytics.matrix.bounces', 'analytics.matrix.complaints',
  ]);
  assert.equal(g.reduce((t, x) => t + x.span, 0), midColumns(false).length);
  assert.equal(groupHeaders(true).reduce((t, x) => t + x.span, 0), midColumns(true).length);
  assert.ok(g[2].cls.includes('bm-shop'));
  // The key texts themselves.
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.equal(en['analytics.conversions.tracked'], 'Tracked');
  assert.equal(en['analytics.conversions.matched'], 'Matched');
  assert.equal(en['analytics.matrix.views'], 'Views');
  assert.equal(en['analytics.matrix.shopLinks'].toUpperCase(), 'SHOP LINK ATTRIBUTION');
  assert.deepEqual(['founder', 'self', 'creator', 'corp'].map((k) => en[`analytics.matrix.${k}`]), ['Founder', 'Self', 'Creator', 'Corp']);
  cols.concat(midColumns(true)).forEach((c) => assert.ok(en[c.label], c.label));
});

test('MATRIX-POLISH I8: the Brand column is hidden only under a single-brand selection', () => {
  assert.equal(showBrandColumn({ kind: 'all' }), true);
  assert.equal(showBrandColumn({ kind: 'none' }), true);
  assert.equal(showBrandColumn(null), true);
  assert.equal(showBrandColumn({ kind: 'brand', slug: 'ruze' }), false);
});

test('MATRIX-POLISH I8: the name shows 25 characters then …, the full name in the title', () => {
  assert.equal(NAME_MAX, 25);
  const exact = 'A'.repeat(25);
  assert.deepEqual(nameCell(exact), { text: exact, title: exact });
  const long = 'Thirsty Girl October Restock Weekend';
  assert.deepEqual(nameCell(long), { text: `${long.slice(0, 25)}…`, title: long });
  assert.deepEqual(nameCell('Spring'), { text: 'Spring', title: 'Spring' });
  assert.deepEqual(nameCell(undefined), { text: '', title: '' });
  // Characters, not UTF-16 units: an emoji is never split.
  const emoji = `${'x'.repeat(24)}🎉🎉`;
  assert.equal(nameCell(emoji).text, `${'x'.repeat(24)}🎉…`);
});

test('I6: %/$ cells are count / Sent (conversions in $); # cells are counts (conversions in orders)', () => {
  const r = shapeRow(api[0]);
  assert.equal(cellText(r, 'views', 'pct'), '30.00%');
  assert.equal(cellText(r, 'clicks', 'pct'), '5.00%');
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
  // Conversions sum the covered rows only; GP and date are blank.
  assert.deepEqual(t.tracked, { orders: 4, sales: 130.5 });
  assert.deepEqual(t.matched, { orders: 3, sales: 70 });
  assert.equal(t.gp_rate, null);
  assert.equal(gpClass(t), '');
  assert.deepEqual(dateCell(t), { text: '', tip: null });
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
  // shop_founder: row 2 has no shop link (null) -> last both ways; row 4 has Sent 0 -> null in pct.
  // Rates: row 1 5/1000, row 3 2/200.
  assert.deepEqual(ids('shop_founder', 'desc'), [3, 1, 2, 4]);
  assert.deepEqual(ids('shop_founder', 'asc'), [1, 3, 2, 4]);
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
