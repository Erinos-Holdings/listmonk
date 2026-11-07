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
  COUNT_FIELDS, DEFAULT_STATE, GAP_FAILED, GAP_PENDING, MISSING, NAME_MAX, PENDING_MARK, SHOP_FIELDS, SORT_FIELDS,
  STATUS_ORDER, STORAGE_KEY, cellText, clickSort, conversionValue, dateCell, displayValue, filterByLaunch, filterByName,
  gapClass, gapRequestIds, gapTip, gpClass, gpLevel, gpTip, groupHeaders, leftColumns, loadState, mergeGapCounts,
  midColumns, nameCell, normalizeState, saveState, shapeRow, shapeRows, showBrandColumn,
  sortRows, statusCell, statusRank, stripBrand, tableRows, totalsRow,
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
    'gmail_open', 'yahoo_open', 'gmail_gap',
  ]);
  assert.deepEqual(cols.map((c) => c.label), [
    'analytics.matrix.views', 'dashboard.matrix.clicks',
    'analytics.conversions.tracked', 'analytics.conversions.matched',
    'analytics.matrix.founder', 'analytics.matrix.self', 'analytics.matrix.creator', 'analytics.matrix.corp',
    'analytics.matrix.hard', 'analytics.matrix.soft', 'analytics.matrix.fbl',
    'analytics.matrix.gmail', 'analytics.matrix.yahoo', 'analytics.matrix.gap',
  ]);
  cols.filter((c) => c.field.startsWith('shop_')).forEach((c) => assert.equal(c.group, 'bm-shop'));
  // GMAIL-GAP-SPEC D3: GP S% closes the Complaints group; the GMAIL GAP group is right-most.
  assert.deepEqual(midColumns(true).slice(-4).map((c) => c.field), ['gp_rate', 'gmail_open', 'yahoo_open', 'gmail_gap']);
  // No tier columns, nowhere.
  ['client', 'proxy', 'machine'].forEach((f) => {
    assert.ok(!COUNT_FIELDS.includes(f), f);
    assert.ok(!midColumns(true).some((c) => c.field === f), f);
  });
  // The group row: nothing over Views/Clicks, then the four groups; spans match the leaves.
  const g = groupHeaders(false);
  assert.deepEqual(g.map((x) => x.label), [
    '', 'analytics.conversions.title', 'analytics.matrix.shopLinks', 'analytics.matrix.bounces', 'analytics.matrix.complaints',
    'analytics.matrix.gmailGap',
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

test('MATRIX-POLISH I8 (widened 2026-11-06): the name shows 60 characters then …, the stored name in the title', () => {
  assert.equal(NAME_MAX, 60);
  const exact = 'A'.repeat(60);
  assert.deepEqual(nameCell(exact), { text: exact, title: exact });
  const long = `Thirsty Girl journey — Re-engagement series — Day 365: ${'x'.repeat(20)}`;
  assert.deepEqual(nameCell(long), { text: `${long.slice(0, 60)}…`, title: long });
  assert.deepEqual(nameCell('Spring'), { text: 'Spring', title: 'Spring' });
  assert.deepEqual(nameCell(undefined), { text: '', title: '' });
  // Characters, not UTF-16 units: an emoji is never split.
  const emoji = `${'x'.repeat(59)}🎉🎉`;
  assert.equal(nameCell(emoji).text, `${'x'.repeat(59)}🎉…`);
  // Under a single-brand selection the shown text is brand-stripped; the title is the stored name.
  const tg = { slug: 'thirstygirl', label: 'Thirsty Girl' };
  assert.deepEqual(nameCell('Thirsty Girl Is Live', tg), { text: 'Is Live', title: 'Thirsty Girl Is Live' });
  assert.deepEqual(nameCell('Thirsty Girl Is Live', null), { text: 'Thirsty Girl Is Live', title: 'Thirsty Girl Is Live' });
});

test('2026-11-06: stripBrand removes the brand name as a whole segment, with one separator run', () => {
  const yq = { slug: 'younique', label: 'Younique' };
  assert.equal(stripBrand('Younique Active - Nov 5th DE', yq), 'Active - Nov 5th DE');
  assert.equal(stripBrand('Younique - Active Oct 15th - DE', yq), 'Active Oct 15th - DE');
  assert.equal(stripBrand('Copy of Younique - Winback Oct 20th ES', yq), 'Copy of Winback Oct 20th ES');
  assert.equal(stripBrand('110526_JoyBelle_Festive Flame_IT', { slug: 'joybelle', label: 'JoyBelle' }), '110526_Festive Flame_IT');
  // Case-insensitive; the slug counts; a word-prefix of the display name (RUZE of "Ruze Pouches",
  // BNKD of "BNKD Wellness") only between hard separators.
  const ruze = { slug: 'ruze', label: 'Ruze Pouches' };
  assert.equal(stripBrand('102926_RUZE_Energy, Focus, Unwind._EN', ruze), '102926_Energy, Focus, Unwind._EN');
  assert.equal(stripBrand('Ruze_No purchase_15 days_EN', ruze), 'No purchase_15 days_EN');
  assert.equal(stripBrand('BNKD_Welcome_EN', { slug: 'bnkd', label: 'BNKD Wellness' }), 'Welcome_EN');
  assert.equal(stripBrand('High Five Friday', { slug: 'highcontrast', label: 'High Contrast Beauty' }), 'High Five Friday');
  // A hard separator before and a space after keeps the hard one.
  const tg = { slug: 'thirstygirl', label: 'Thirsty Girl' };
  assert.equal(stripBrand('101926_Thirsty Girl Is Live_EN', tg), '101926_Is Live_EN');
  assert.equal(stripBrand('Thirsty Girl journey — Day 0: Welcome', tg), 'journey — Day 0: Welcome');
  // Never inside a word, never to an empty name, never without a brand.
  assert.equal(stripBrand('Shalala party', { slug: 'shala', label: 'Shala' }), 'Shalala party');
  assert.equal(stripBrand('TG_WE02_EN', tg), 'TG_WE02_EN');
  assert.equal(stripBrand('Younique', yq), 'Younique');
  assert.equal(stripBrand('Younique Active', null), 'Younique Active');
  assert.equal(stripBrand('', yq), '');
});

test('2026-11-06: filterByName is a case-insensitive substring of the stored name; blank keeps all', () => {
  const rows = [{ name: 'Younique Active - Nov 5th' }, { name: 'Shala CNL Oct 28th' }];
  assert.deepEqual(filterByName(rows, ''), rows);
  assert.deepEqual(filterByName(rows, '  '), rows);
  assert.deepEqual(filterByName(rows, 'nov 5'), [rows[0]]);
  assert.deepEqual(filterByName(rows, 'CNL'), [rows[1]]);
  assert.deepEqual(filterByName(rows, 'zzz'), []);
  assert.deepEqual(filterByName(rows, undefined), rows);
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

// HOURLY-HEALTH-SPEC I2 -- the Status column.
test('HOURLY-HEALTH I2: Status sorts running, paused, scheduled, draft, finished, cancelled, unknown last', () => {
  assert.deepEqual([...STATUS_ORDER], ['running', 'paused', 'scheduled', 'draft', 'finished', 'cancelled']);
  assert.equal(statusRank('running'), 0);
  assert.equal(statusRank('cancelled'), 5);
  assert.equal(statusRank('weird'), 6);
  const statuses = ['finished', 'weird', 'draft', 'running', 'cancelled', 'scheduled', 'paused', ''];
  const rows = statuses.map((s, i) => shapeRow({
    ...api[0], id: i + 1, name: `C${i}`, status: s,
  }));
  const asc = sortRows(rows, { field: 'status', order: 'asc' }).map((r) => r.status);
  assert.deepEqual(asc.slice(0, 6), ['running', 'paused', 'scheduled', 'draft', 'finished', 'cancelled']);
  assert.deepEqual([...asc.slice(6)].sort(), ['', 'weird']);
  const desc = sortRows(rows, { field: 'status', order: 'desc' }).map((r) => r.status);
  assert.deepEqual(desc.slice(0, 6), ['cancelled', 'finished', 'draft', 'scheduled', 'paused', 'running']);
  assert.deepEqual([...desc.slice(6)].sort(), ['', 'weird']);
  // First click is ascending (still moving first).
  assert.deepEqual(clickSort({ field: 'date', order: 'desc' }, 'status'), { field: 'status', order: 'asc' });
});

test('HOURLY-HEALTH I2: the Status column after Date, before Sent; the header and cell texts; the totals cell empty', () => {
  // Integrations SMART-SENDING-SPEC D5: Skipped follows Sent.
  assert.deepEqual(leftColumns(true).map((c) => c.field), ['name', 'brand', 'date', 'status', 'sent', 'skipped']);
  assert.deepEqual(leftColumns(false).map((c) => c.field), ['name', 'date', 'status', 'sent', 'skipped']);
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.equal(en['analytics.matrix.status'], 'Status');
  leftColumns(true).forEach((c) => assert.ok(en[c.label], c.label));
  const row = shapeRow({ ...api[0], status: 'running' });
  assert.equal(row.status, 'running');
  assert.deepEqual(statusCell(row), { status: 'running', label: 'campaigns.status.running' });
  STATUS_ORDER.forEach((s) => assert.ok(en[`campaigns.status.${s}`], s));
  assert.equal(statusCell(totalsRow([row, row])), null);
  assert.equal(statusCell(shapeRow({ ...api[0] })), null);
});

test('HOURLY-HEALTH I2: status round-trips normalizeState', () => {
  const s = {
    sort: { field: 'status', order: 'asc' }, mode: 'num', kind: 'broadcasts',
  };
  assert.deepEqual(normalizeState(s), s);
  assert.deepEqual(normalizeState(JSON.parse(JSON.stringify(normalizeState(s)))), s);
});

// --- integrations GMAIL-GAP-SPEC I2 / I3 (fixture threshold values, never the live ones) ---

const GAP_T = { warn: 0.8, red: 0.4, minControl: 150 };
const gapApi = (id, name, counts, extra = {}) => ({
  ...api[0], id, name, gmail_proxy: counts[1], gmail: counts[0], yahoo: counts[2], yahoo_proxy: counts[3], ...extra,
});
const gapRows = (thresholds = GAP_T) => shapeRows([
  gapApi(11, 'Normal', [1000, 100, 400, 40]), // gap 1.00
  gapApi(12, 'Bulked', [2000, 4, 500, 100]), // 0.002 / 0.2 -> 0.01
  gapApi(13, 'Small Yahoo', [1000, 50, 149, 20]), // under the floor
  gapApi(14, 'No Yahoo opens', [1000, 50, 400, 0]),
  gapApi(15, 'Journey', [null, null, null, null], { evergreen: true, last_sent: '2026-10-26' }),
], { gapThresholds: thresholds });

test('GMAIL-GAP I2: Gmail / Yahoo are rates in %/$ and opener counts in #; Gap is the ratio in both', () => {
  const [normal, bulked] = gapRows();
  assert.equal(cellText(normal, 'gmail_open', 'pct'), '10.00%');
  assert.equal(cellText(normal, 'yahoo_open', 'pct'), '10.00%');
  assert.equal(cellText(normal, 'gmail_open', 'num', String), '100');
  assert.equal(cellText(normal, 'yahoo_open', 'num', String), '40');
  assert.equal(cellText(normal, 'gmail_gap', 'pct'), '1.00');
  assert.equal(cellText(normal, 'gmail_gap', 'num'), '1.00');
  assert.equal(cellText(bulked, 'gmail_gap', 'pct'), '0.01');
  assert.equal(displayValue(bulked, 'gmail_gap', 'num'), 0.002 / 0.2);
});

test('GMAIL-GAP I2: — exactly for a Yahoo base under the floor, no Yahoo opens, a 0 denominator, a journey', () => {
  const rows = gapRows();
  const small = rows.find((r) => r.id === 13);
  const noOpens = rows.find((r) => r.id === 14);
  const journey = rows.find((r) => r.id === 15);
  assert.equal(cellText(small, 'gmail_gap', 'pct'), MISSING);
  assert.equal(cellText(small, 'yahoo_open', 'pct'), '13.42%', 'the Yahoo rate itself is still shown');
  assert.equal(gapTip(small, 'gmail_gap').key, 'analytics.matrix.gapFloor');
  assert.equal(gapTip(small, 'gmail_gap', (n) => `#${n}`).params.floor, '#150');
  assert.equal(cellText(noOpens, 'gmail_gap', 'pct'), MISSING);
  assert.equal(gapTip(noOpens, 'gmail_gap').key, 'analytics.matrix.gapNoYahooOpens');
  ['gmail_open', 'yahoo_open', 'gmail_gap'].forEach((f) => {
    ['pct', 'num'].forEach((m) => assert.equal(cellText(journey, f, m), MISSING, `${f} ${m}`));
    assert.equal(gapTip(journey, f).key, 'analytics.matrix.gapJourney');
  });
  const zeroGmail = shapeRows([gapApi(16, 'No Gmail', [0, 0, 400, 40])], { gapThresholds: GAP_T })[0];
  assert.equal(cellText(zeroGmail, 'gmail_open', 'pct'), MISSING);
  assert.equal(cellText(zeroGmail, 'gmail_open', 'num', String), '0');
  assert.equal(cellText(zeroGmail, 'gmail_gap', 'pct'), MISSING);
  // The fraction hovers.
  const normal = rows[0];
  assert.deepEqual(gapTip(normal, 'gmail_open', String), { key: 'analytics.matrix.gmailTip', params: { n: '100', of: '1000' } });
  assert.deepEqual(gapTip(normal, 'yahoo_open', String), { key: 'analytics.matrix.yahooTip', params: { n: '40', of: '400' } });
  assert.equal(gapTip(normal, 'gmail_gap').key, 'analytics.matrix.gapHelp');
  // Without a stored line the floor is unknown: the small-Yahoo row's gap is shown, uncolored.
  const noLine = gapRows(null).find((r) => r.id === 13);
  assert.equal(cellText(noLine, 'gmail_gap', 'pct'), ((50 / 1000) / (20 / 149)).toFixed(2));
  assert.equal(gapClass(noLine), '');
});

test('GMAIL-GAP I2: the totals row pools the four counts over the broadcasts (journeys skipped)', () => {
  const t = totalsRow(gapRows(), 'Total');
  assert.deepEqual(t.gap, {
    gmail: 5000, gmail_proxy: 204, yahoo: 1449, yahoo_proxy: 160,
  });
  assert.equal(displayValue(t, 'gmail_open', 'pct'), 204 / 5000);
  assert.equal(cellText(t, 'yahoo_open', 'num', String), '160');
  assert.equal(cellText(t, 'gmail_gap', 'pct'), ((204 / 5000) / (160 / 1449)).toFixed(2));
  assert.equal(gapClass(t), 'has-text-danger', 'the pooled 0.37 is at or below the stored red');
  // Only journeys -> no counts -> —.
  const jt = totalsRow(gapRows().filter((r) => r.evergreen).concat(gapRows().filter((r) => r.evergreen)), 'Total');
  assert.equal(jt.gap, null);
  assert.equal(cellText(jt, 'gmail_gap', 'pct'), MISSING);
});

test('GMAIL-GAP I3: the Gap cell is colored from the stored line only, on the displayed value', () => {
  const rows = gapRows();
  assert.equal(gapClass(rows[0]), '', '1.00 is above warn');
  assert.equal(gapClass(rows[1]), 'has-text-danger', '0.01 at or below red');
  const amber = shapeRows([gapApi(17, 'Amber', [1000, 80, 400, 40])], { gapThresholds: GAP_T })[0];
  assert.equal(cellText(amber, 'gmail_gap', 'pct'), '0.80');
  assert.equal(gapClass(amber), 'has-text-warning-dark');
  assert.equal(gapClass(gapRows(null)[1]), '', 'no stored line -> no color');
  assert.equal(gapClass(rows.find((r) => r.evergreen)), '');
});

test('GMAIL-GAP I2: the gap columns sort with a missing value last in both orders', () => {
  const rows = gapRows();
  const ids = (s) => sortRows(rows, s, 'pct').map((r) => r.id);
  assert.deepEqual(ids({ field: 'gmail_gap', order: 'desc' }), [11, 12, 15, 14, 13]);
  assert.deepEqual(ids({ field: 'gmail_gap', order: 'asc' }), [12, 11, 15, 14, 13]);
  // # mode sorts by the opener counts (a tie falls back to the name); the journey is last.
  assert.deepEqual(sortRows(rows, { field: 'gmail_open', order: 'desc' }, 'num').map((r) => r.id), [11, 14, 13, 12, 15]);
  assert.deepEqual(sortRows(rows, { field: 'yahoo_open', order: 'asc' }, 'num').map((r) => r.id), [14, 13, 11, 12, 15]);
  assert.ok(SORT_FIELDS.includes('gmail_gap'));
  assert.deepEqual(clickSort(DEFAULT_STATE.sort, 'gmail_gap'), { field: 'gmail_gap', order: 'desc' });
  assert.deepEqual(normalizeState({ sort: { field: 'yahoo_open', order: 'asc' } }).sort, { field: 'yahoo_open', order: 'asc' });
  // The i18n keys the group reads exist.
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  ['gmailGap', 'gmail', 'yahoo', 'gap', 'gmailHelp', 'yahooHelp', 'gapHelp', 'gmailTip', 'yahooTip', 'gapJourney',
    'gapFloor', 'gapNoYahoo', 'gapNoYahooOpens', 'gapNoGmail', 'gapLoading'].forEach((k) => assert.ok(en[`analytics.matrix.${k}`], k));
  assert.equal(en['analytics.matrix.gapLoading'], 'Loading the Gmail gap…');
});

// --- the Matrix's second request (gmail_gap) -- merge and the pending / failed cell states ---

const bareApi = () => [
  { ...api[0], id: 21, name: 'B1' },
  { ...api[0], id: 22, name: 'B2' },
  {
    ...api[2], id: 23, name: 'J', evergreen: true,
  },
];
const gapRow = (id, gmail, gp, yahoo, yp) => ({
  campaign_id: id, gmail, gmail_proxy: gp, yahoo, yahoo_proxy: yp,
});

test('mergeGapCounts: by id, unknown ids ignored, journeys untouched, thresholds returned', () => {
  const rows = bareApi();
  assert.deepEqual(gapRequestIds(rows), [21, 22], 'journeys are never asked for');
  const t = mergeGapCounts(rows, {
    rows: [gapRow(21, 1000, 100, 400, 40), gapRow(99, 5, 5, 5, 5), gapRow(23, 7, 7, 7, 7)],
    gap_thresholds: GAP_T,
  });
  assert.deepEqual(t, GAP_T);
  assert.deepEqual([rows[0].gmail, rows[0].gmail_proxy, rows[0].yahoo, rows[0].yahoo_proxy], [1000, 100, 400, 40]);
  assert.equal(rows[1].gmail, undefined, 'a row the response did not carry stays without counts');
  assert.equal(rows[2].gmail, undefined, 'a journey is never written');
  assert.equal(rows.length, 3);
  assert.equal(mergeGapCounts(bareApi(), { rows: [] }), null, 'no stored line -> null');
  assert.equal(mergeGapCounts(bareApi(), null), null);
  // After the merge the cells, totals and coloring compute from the counts.
  const shaped = shapeRows(rows, { gapThresholds: t });
  assert.equal(cellText(shaped[0], 'gmail_gap', 'pct'), '1.00');
  assert.equal(cellText(totalsRow(shaped), 'gmail_gap', 'pct'), '1.00');
  assert.equal(gapClass(shaped[0]), '');
});

test('the pending state: broadcast gap cells read …, hover the loading text, sort as missing', () => {
  const rows = shapeRows(bareApi(), { gapState: GAP_PENDING });
  const [b1, , j] = rows;
  ['gmail_open', 'yahoo_open', 'gmail_gap'].forEach((f) => {
    ['pct', 'num'].forEach((m) => assert.equal(cellText(b1, f, m), PENDING_MARK, `${f} ${m}`));
    assert.deepEqual(gapTip(b1, f), { key: 'analytics.matrix.gapLoading', params: {} });
    // A journey never waits.
    assert.equal(cellText(j, f, 'pct'), MISSING);
    assert.equal(gapTip(j, f).key, 'analytics.matrix.gapJourney');
  });
  const t = totalsRow(rows);
  assert.equal(cellText(t, 'gmail_gap', 'pct'), PENDING_MARK);
  assert.equal(gapTip(t, 'gmail_gap').key, 'analytics.matrix.gapLoading');
  assert.equal(displayValue(b1, 'gmail_gap', 'pct'), null, 'sorts as missing');
  assert.equal(gapClass(b1), '');
  // Pending rows sort by the tie rule (name), missing last in both orders.
  assert.deepEqual(sortRows(rows, { field: 'gmail_gap', order: 'desc' }, 'pct').map((r) => r.id), [21, 22, 23]);
});

test('the failed state: broadcast gap cells read — with the unavailable wording, never the journey one', () => {
  const rows = shapeRows(bareApi(), { gapState: GAP_FAILED });
  const [b1, , j] = rows;
  ['gmail_open', 'yahoo_open', 'gmail_gap'].forEach((f) => {
    assert.equal(cellText(b1, f, 'pct'), MISSING);
    assert.deepEqual(gapTip(b1, f), { key: 'analytics.gmailGapUnavailable', params: {} });
    assert.equal(gapTip(j, f).key, 'analytics.matrix.gapJourney');
  });
  assert.equal(gapTip(totalsRow(rows), 'gmail_gap').key, 'analytics.gmailGapUnavailable');
  assert.equal(gapClass(b1), '');
  // An unknown state is ready.
  assert.equal(shapeRows(bareApi(), { gapState: 'x' })[0].gapState, 'ready');
});

// --- integrations SMART-SENDING-SPEC I8 ---

test('SMART-SENDING I8: Skipped shows the count, "off" for a bypass, — for a journey; sortable, totals summed', () => {
  const rows = shapeRows([
    {
      ...api[0], id: 31, name: 'Ruled', skipped: 12, smart_off: false,
    },
    {
      ...api[0], id: 32, name: 'Launch', skipped: 0, smart_off: true,
    },
    {
      ...api[0], id: 33, name: 'Welcome', evergreen: true, skipped: null, smart_off: false, last_sent: '2026-10-26',
    },
    {
      ...api[0], id: 34, name: 'Quiet', skipped: 0, smart_off: false,
    },
  ]);
  const [ruled, launch, journey, quiet] = rows;
  const fmt = (n) => String(n);
  assert.equal(cellText(ruled, 'skipped', 'pct', fmt), '12', 'a count in %/$ mode, never a rate');
  assert.equal(cellText(ruled, 'skipped', 'num', fmt), '12');
  assert.equal(cellText(launch, 'skipped', 'pct', fmt), 'off');
  assert.equal(cellText(journey, 'skipped', 'pct', fmt), MISSING);
  assert.equal(cellText(quiet, 'skipped', 'num', fmt), '0');
  // A journey's smart_off is never set, even if the server sent one.
  assert.equal(shapeRow({ ...api[0], evergreen: true, smart_off: true }).smartOff, false);
  assert.ok(SORT_FIELDS.includes('skipped'));
  assert.deepEqual(sortRows(rows, { field: 'skipped', order: 'desc' }).map((r) => r.id), [31, 32, 34, 33]);
  assert.deepEqual(sortRows(rows, { field: 'skipped', order: 'asc' }).map((r) => r.id), [32, 34, 31, 33]);
  const t = totalsRow(rows);
  assert.equal(cellText(t, 'skipped', 'pct', fmt), '12');
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  ['analytics.matrix.skipped', 'analytics.matrix.skippedHelp'].forEach((k) => assert.ok(en[k], k));
});
