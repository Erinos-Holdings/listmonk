// Fork (brand usage, integrations listmonk/BRAND-USAGE-SPEC I6/I7, D2/D7; BRAND-USAGE-V2-SPEC
// I1-I4, I7). Run: yarn test (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  CIRCLE_FIELDS, DEFAULT_STATE, MISSING, SCORE_PARTS, SECTIONS, SORT_FIELDS, SERIES_DAYS, STORAGE_KEY,
  boolCell, broadcastTip, cell, cellBadges, cellLink, circlesOf, circleWords, clickSort, columnMeta, defaultOrder,
  headerLayout, latestAsOf, loadState, mountainRunTime, WELCOME_FIELDS, POST_PURCHASE_FIELDS, seriesLeafOf, saveState, scoreTip, shapeRows,
  skippedLangOf, sortRows, sortValue, storedCounts, triggerTip, linkTip, brandListIds,
  GUIDE_MIN_COLUMN, GUIDE_SERIES, guideSplitX, parseStepText, seriesGuide, bodyColumns, circleLink,
} from './brandUsage.mjs'; // eslint-disable-line import/extensions
import { PANES, initialPane, sessionStore } from './brandMatrix.mjs'; // eslint-disable-line import/extensions

const t = (k, p) => (p ? `${k}${JSON.stringify(p)}` : k);

// LIFECYCLE-SERIES-SPEC S9: the parts (lib/brand-usage.ts SCORE_WEIGHTS) -- broadcasts 40, the two
// series 20 each (4 leaves x active 5), attribution 10, sunset 10.
const parts = (b, w, p, a, s) => ({
  broadcasts: { points: b, max: 40 },
  welcomeSeries: { points: w, max: 20 },
  postPurchaseSeries: { points: p, max: 20 },
  attribution: { points: a, max: 10 },
  sunset: { points: s, max: 10 },
});
const score = (total, p) => ({ total, parts: p });

// A stored series family: one {days, state, rows} per standard leaf (rows by day), plus legacy.
const series = (family, rowsByDay = {}, legacy = [], states = {}) => ({
  steps: SERIES_DAYS[family].map((days) => {
    const rows = rowsByDay[days] || [];
    const clean = (r) => !r.error && r.listId != null && r.evergreen === 'running';
    let state = 'none';
    if (rows.length) state = rows.every(clean) ? 'active' : 'idle';
    return { days, state: states[days] || state, rows };
  }),
  legacy,
});
const ppRow = (days, listId, evergreen, error) => ({
  slug: `x-post-purchase-${listId}`, listId, evergreen, days, ...(error ? { error } : {}),
});

const usage = (over = {}) => ({
  channel: 'listmonk',
  welcome: {
    state: 'active', listId: 7, campaigns: [{ id: 3, status: 'running' }], counts: { green: 1, amber: 0 },
  },
  series: { welcome: series('welcome', {}, [], { 0: 'active' }), postPurchase: series('postPurchase') },
  sunset: true,
  attribution: false,
  broadcasts: {
    lastAt: '2026-10-20T12:00:00.000Z', windowCount: 4, cadenceDays: 12.4, daysSince: 4.6, ratio: 0.37, state: 'ok',
  },
  score: score(55, parts(40, 5, 0, 0, 10)),
  ...over,
});

const api = [
  { brand: 'ruze', health: { status: 'ok' }, usage: usage() },
  {
    brand: 'thirstygirl',
    health: { status: 'warn' },
    usage: usage({
      broadcasts: {
        lastAt: null, windowCount: 0, cadenceDays: null, daysSince: null, ratio: null, state: 'none',
      },
      score: score(15, parts(0, 5, 0, 0, 10)),
    }),
  },
  {
    brand: 'lux',
    health: { status: 'ok' },
    usage: usage({
      broadcasts: {
        lastAt: '2026-08-01T00:00:00.000Z', windowCount: 1, cadenceDays: null, daysSince: 84.6, ratio: 2.82, state: 'stale',
      },
      score: score(25, parts(10, 5, 0, 0, 10)),
    }),
  },
  {
    brand: 'oldzma',
    health: { status: 'ok' },
    usage: usage({
      channel: 'zma',
      welcome: {
        state: 'none', listId: 9, campaigns: [], counts: { green: 0, amber: 0 },
      },
      series: {
        welcome: series('welcome'),
        postPurchase: series('postPurchase', { 0: [{ slug: 'oldzma-first-order', listId: null, evergreen: 'none' }] }),
      },
      journeys: { running: 0, idle: 0 },
      broadcasts: {
        lastAt: null, windowCount: 0, cadenceDays: null, daysSince: null, ratio: null, state: 'none',
      },
      score: null,
    }),
  },
  { brand: 'nodoc' }, // no health document, no usage (pre-deploy or unmapped)
  { brand: '' }, // never a row
];

// Integrations CAMPAIGN-MATRIX-SPEC D2/I7 -- the usage pane moved to the Brands page (paneState.mjs):
// the Dashboard pill is Overview | SES and a stored `usage` reads as overview.
test('D2: the Dashboard pill is Overview | SES; a stored usage pane reads as overview', () => {
  assert.deepEqual(PANES, ['overview', 'ses']);
  assert.equal(initialPane(true, 'usage', false), 'overview');
  assert.equal(initialPane(false, 'usage', false), 'overview');
});

test('D2: rows are the Brand Matrix rows -- the launch filter over the rows\' own health docs', () => {
  assert.deepEqual(shapeRows(api).map((r) => r.brand), ['ruze', 'thirstygirl', 'lux', 'oldzma', 'nodoc']);
  const brands = [{ slug: 'ruze', display_name: 'RUZE' }];
  const r = shapeRows(api, { brands })[0];
  assert.equal(r.brand, 'ruze');
  assert.equal(r.label, 'RUZE');
  assert.equal(r.zma, false);
  assert.equal(shapeRows(api).find((x) => x.brand === 'oldzma').zma, true);
  assert.equal(shapeRows(api).find((x) => x.brand === 'nodoc').usage, null);
});

// LIFECYCLE-SERIES-SPEC S9 (replacing POLISH3 D4's No purchase 3d | 7d | 15d sub-group and the
// plain Welcome / 1st purchase columns): Journeys is the No-purchase series group over
// Day 0 | 2d | 5d | 10d and the Post-purchase series group over Day 0 | 5d | 14d | 30d (the
// journeys count column retired 2026-10-30 -- the Score sorts by usage).
// The Languages section (Skipped over FR ES DE IT) still sits between Journeys and Configurations.
test('S9 / POLISH3 D5: columns -- Score, Brand, then Broadcasts / Journeys (two series groups) / Languages / Configurations', () => {
  assert.deepEqual(SERIES_DAYS, { welcome: [0, 2, 5, 10], postPurchase: [0, 5, 14, 30] });
  const W = ['welcome:0', 'welcome:2', 'welcome:5', 'welcome:10'];
  const P = ['postPurchase:0', 'postPurchase:5', 'postPurchase:14', 'postPurchase:30'];
  assert.deepEqual(WELCOME_FIELDS, W);
  assert.deepEqual(POST_PURCHASE_FIELDS, P);
  assert.deepEqual(SECTIONS.map((s) => [s.key, s.fields]), [
    ['broadcasts', ['cadence', 'lastBroadcast']],
    ['journeys', [...W, ...P, 'sku']],
    ['languages', ['skipped:fr', 'skipped:es', 'skipped:de', 'skipped:it']],
    ['configurations', ['attribution', 'sunset']],
  ]);
  assert.deepEqual(SORT_FIELDS, [
    'score', 'brand', 'cadence', 'lastBroadcast', ...W, ...P, 'sku',
    'skipped:fr', 'skipped:es', 'skipped:de', 'skipped:it', 'attribution', 'sunset',
  ]);
  assert.deepEqual(CIRCLE_FIELDS, [...W, ...P, 'sku']);
  // The header: plain leaves in row 2 (rowspan 2), the sub-group labels over their leaves in row 3.
  const h = headerLayout();
  assert.deepEqual(h.row2.map((c) => (c.kind === 'group' ? `[${c.key}:${c.span}]` : c.field)), [
    'cadence', 'lastBroadcast', '[welcome:4]', '[postPurchase:4]', 'sku', '[skipped:4]', 'attribution', 'sunset',
  ]);
  // G5 round 2: every group label takes the single-row headers' font size (small: false).
  assert.deepEqual(h.row2.filter((c) => c.kind === 'group').map((c) => [c.label, c.band, c.first, c.small]), [
    ['dashboard.usage.noPurchase', 'white', true, false], ['dashboard.usage.postPurchase', 'white', false, false],
    ['dashboard.usage.skipped', 'grey', true, false],
  ]);
  assert.deepEqual(h.row3.map((c) => [c.field, c.first, c.groupStart]), [
    ['welcome:0', true, true], ['welcome:2', false, false], ['welcome:5', false, false], ['welcome:10', false, false],
    ['postPurchase:0', false, true], ['postPurchase:5', false, false], ['postPurchase:14', false, false], ['postPurchase:30', false, false],
    ['skipped:fr', true, true], ['skipped:es', false, false], ['skipped:de', false, false], ['skipped:it', false, false],
  ]);
  // S9: each leaf's header hover is its D5 description.
  assert.deepEqual(columnMeta('welcome:5'), { label: 'dashboard.usage.npDays', params: { n: 5 }, help: 'series.welcomeDay5' });
  assert.deepEqual(columnMeta('welcome:0'), { label: 'dashboard.usage.day0', help: 'series.welcomeDay0' });
  assert.deepEqual(columnMeta('postPurchase:30'), { label: 'dashboard.usage.npDays', params: { n: 30 }, help: 'series.postPurchaseDay30' });
  assert.deepEqual(columnMeta('postPurchase:0'), { label: 'dashboard.usage.day0', help: 'series.postPurchaseDay0' });
  assert.deepEqual(columnMeta('skipped:de'), { text: 'DE', help: 'dashboard.usage.skippedHelp' });
  assert.equal(seriesLeafOf('welcome:15'), null); // off-standard: no leaf (a legacy chip)
  assert.equal(seriesLeafOf('noPurchase:3'), null); // the retired leaves
  assert.equal(skippedLangOf('skipped:en'), null); // EN has no column (recorded limitation)
});

// LIFECYCLE-SERIES-SPEC I16 -- every series leaf's hover resolves a D5 key present in en.json, and
// every label key the pane binds exists.
test('I16: every leaf help, group label and score part key is in en.json', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  [...WELCOME_FIELDS, ...POST_PURCHASE_FIELDS].forEach((f) => {
    const m = columnMeta(f);
    assert.equal(typeof en[m.help], 'string', `${f} help ${m.help}`);
    assert.equal(typeof en[m.label], 'string', `${f} label ${m.label}`);
  });
  headerLayout().row2.filter((c) => c.kind === 'group').forEach((c) => {
    assert.equal(typeof en[c.label], 'string', c.label);
    assert.equal(typeof en[c.help], 'string', c.help);
  });
  SCORE_PARTS.forEach((p) => assert.equal(typeof en[p.label], 'string', p.label));
  ['dashboard.usage.legacyChip', 'dashboard.usage.seriesMissing', 'series.welcomeLegacy', 'series.postPurchaseLegacy'].forEach((k) => assert.equal(typeof en[k], 'string', k));
  assert.equal(en['dashboard.usage.noPurchase'], 'No-purchase series');
  assert.equal(en['dashboard.usage.postPurchase'], 'Post-purchase series');
  assert.equal(en['dashboard.usage.partWelcomeSeries'], 'No-purchase series');
  assert.equal(en['dashboard.usage.partPostPurchaseSeries'], 'Post-purchase series');
});

test('V2 D1 / I1: the Score cell -- the stored total, the breakdown from parts verbatim in the FIXED order', () => {
  const [ruze] = shapeRows(api);
  const c = cell(ruze, 'score', t);
  assert.equal(c.text, '55');
  assert.deepEqual(SCORE_PARTS.map((p) => p.key), ['broadcasts', 'welcomeSeries', 'postPurchaseSeries', 'attribution', 'sunset']);
  assert.equal(
    c.tooltip,
    'dashboard.usage.partBroadcasts 40/40 · dashboard.usage.partWelcomeSeries 5/20 · dashboard.usage.partPostPurchaseSeries 0/20 · '
      + 'dashboard.usage.partAttribution 0/10 · dashboard.usage.partSunset 10/10',
  );
  // jsonb returns keys lexicographically -- the display order is ours, not the object's.
  const lexi = {
    attribution: { points: 1, max: 10 },
    broadcasts: { points: 2, max: 40 },
    postPurchaseSeries: { points: 4, max: 20 },
    sunset: { points: 5, max: 10 },
    welcomeSeries: { points: 6, max: 20 },
  };
  const id = (k) => k;
  assert.equal(scoreTip({ total: 18, parts: lexi }, id), [
    'dashboard.usage.partBroadcasts 2/40', 'dashboard.usage.partWelcomeSeries 6/20', 'dashboard.usage.partPostPurchaseSeries 4/20',
    'dashboard.usage.partAttribution 1/10', 'dashboard.usage.partSunset 5/10',
  ].join(' · '));
  // The numbers are rendered verbatim -- the fork computes no weight (a stored 7/99 shows as 7/99).
  assert.equal(scoreTip({ total: 7, parts: { welcomeSeries: { points: 7, max: 99 } } }, id), 'dashboard.usage.partWelcomeSeries 7/99');
  // S9 / F7: a doc written before the release keeps its old parts -- the unknown ones are skipped.
  const pre = {
    broadcasts: { points: 40, max: 40 },
    welcome: { points: 15, max: 15 },
    purchase: { points: 0, max: 15 },
    noPurchase: { points: 10, max: 10 },
    attribution: { points: 0, max: 10 },
    sunset: { points: 10, max: 10 },
  };
  assert.equal(scoreTip({ total: 75, parts: pre }, id), 'dashboard.usage.partBroadcasts 40/40 · dashboard.usage.partAttribution 0/10 · dashboard.usage.partSunset 10/10');
  // ZMA: null score -> '—' (the ZMA help); a pre-V2 doc without score -> '—' (scored next run).
  const zma = shapeRows(api).find((r) => r.brand === 'oldzma');
  assert.deepEqual([cell(zma, 'score', t).text, cell(zma, 'score', t).tooltip], [MISSING, 'dashboard.usage.zmaHelp']);
  const old = shapeRows([{ brand: 'old', usage: usage({ score: undefined }) }])[0];
  assert.deepEqual([cell(old, 'score', t).text, cell(old, 'score', t).tooltip], [MISSING, 'dashboard.usage.scoreMissing']);
});

test('V2 D3 / I3: circles -- one per state from the STORED numbers, green/amber omitted at 0, red 0 when both are 0', () => {
  assert.deepEqual(circlesOf({ green: 1, amber: 2 }), [{ color: 'green', n: 1 }, { color: 'amber', n: 2 }]);
  assert.deepEqual(circlesOf({ green: 3, amber: 0 }), [{ color: 'green', n: 3 }]);
  assert.deepEqual(circlesOf({ green: 0, amber: 2 }), [{ color: 'amber', n: 2 }]);
  assert.deepEqual(circlesOf({ green: 0, amber: 0 }), [{ color: 'red', n: 0 }]);
  assert.deepEqual(circlesOf(null), []);
  // The numbers are the stored ones: Welcome Day 0 from the welcome family's counts.
  const u = usage({
    welcome: {
      state: 'idle', listId: 7, campaigns: [], counts: { green: 4, amber: 5 },
    },
  });
  assert.equal(storedCounts(u, 'journeys'), null); // the journeys column retired 2026-10-30
  // The state is NOT consulted: idle with a stored green 4 renders 4 green (lib owns the predicate).
  assert.deepEqual(storedCounts(u, 'welcome'), { green: 4, amber: 5 });
  assert.deepEqual(cell(shapeRows([{ brand: 'u', usage: u }])[0], 'welcome:0', t).circles, [{ color: 'green', n: 4 }, { color: 'amber', n: 5 }]);
  assert.equal(storedCounts(u, 'sunset'), null);
  // A doc written before V2 (no counts) renders '—', never a derived circle.
  const old = shapeRows([{ brand: 'o', usage: usage({ welcome: { state: 'active', listId: 7, campaigns: [] } }) }])[0];
  assert.deepEqual([cell(old, 'welcome:0', t).text, cell(old, 'welcome:0', t).circles], [MISSING, []]);
});

test('V2 I3: every circle carries a words tooltip; red is the digit 0', () => {
  assert.equal(circleWords({ green: 1, amber: 2 }, t), 'dashboard.usage.circleRunning{"n":1}, dashboard.usage.circleIdle{"n":2}');
  assert.equal(circleWords({ green: 0, amber: 0 }, t), 'dashboard.usage.circleNone');
  const [ruze] = shapeRows(api);
  // Welcome Day 0: the D5 description, the words, then the stored campaigns (S9).
  assert.equal(cell(ruze, 'welcome:0', t).tooltip, 'series.welcomeDay0\ndashboard.usage.circleRunning{"n":1}\n#3 running');
  const p = cell(ruze, 'postPurchase:0', t);
  assert.deepEqual(p.circles, [{ color: 'red', n: 0 }]);
  assert.equal(p.tooltip, 'series.postPurchaseDay0\ndashboard.usage.circleNone');
  CIRCLE_FIELDS.forEach((f) => {
    const c = cell(ruze, f, t);
    assert.ok(c.tooltip.length > 0, f);
    c.circles.filter((x) => x.color === 'red').forEach((x) => assert.equal(x.n, 0, f));
  });
});

test('D7: the trigger tooltip lists each stored row with its error or its list', () => {
  const tip = triggerTip({
    state: 'idle',
    rows: [
      {
        slug: 'a', listId: 4, error: 'trigger is missing', evergreen: 'none',
      },
      { slug: 'b', listId: null, evergreen: 'none' },
      { slug: 'c', listId: 5, evergreen: 'idle' },
    ],
  }, t);
  assert.equal(tip, 'a: trigger is missing\nb: dashboard.usage.noList\nc: dashboard.usage.listEvergreen{"list":5,"state":"idle"}');
  assert.equal(broadcastTip({ cadenceDays: null, ratio: 2.8216, windowCount: 1 }, t), 'dashboard.usage.broadcastHelp{"cadence":"—","ratio":"2.82","count":1}');
  const row = shapeRows([{
    brand: 'x',
    usage: usage({
      series: {
        welcome: series('welcome'),
        postPurchase: series('postPurchase', {
          0: [{ slug: 'x-first-order', listId: 4, evergreen: 'running' }, { slug: 'x-first-order-b', listId: 5, evergreen: 'idle' }],
        }),
      },
    }),
  }])[0];
  assert.equal(
    cell(row, 'postPurchase:0', t).tooltip,
    'series.postPurchaseDay0\ndashboard.usage.circleRunning{"n":1}, dashboard.usage.circleIdle{"n":1}\n'
      + 'x-first-order: dashboard.usage.listEvergreen{"list":4,"state":"running"}\n'
      + 'x-first-order-b: dashboard.usage.listEvergreen{"list":5,"state":"idle"}',
  );
});

test('V2 D4 / S9: links per cell -- Welcome Day 0 its master list, a series leaf only with exactly one row; Automations scope', () => {
  const [ruze] = shapeRows(api);
  assert.deepEqual(cellLink(ruze, 'welcome:0'), { list_id: '7', evergreen: 'true' });
  assert.equal(cellLink(ruze, 'postPurchase:0'), null); // no rows
  assert.equal(cellLink(ruze, 'sunset'), null);
  assert.equal(cellLink(ruze, 'attribution'), null);
  assert.equal(cellLink(ruze, 'score'), null);
  const one = shapeRows([{
    brand: 'x',
    usage: usage({
      series: {
        welcome: series('welcome', { 5: [ppRow(5, 12, 'running')] }),
        postPurchase: series('postPurchase', {
          0: [{ slug: 'a', listId: 4, evergreen: 'running' }, { slug: 'b', listId: 5, evergreen: 'idle' }],
        }),
      },
    }),
  }])[0];
  assert.deepEqual(cellLink(one, 'welcome:5'), { list_id: '12', evergreen: 'true' }); // (a step row; the slug is immaterial)
  assert.equal(cellLink(one, 'postPurchase:0'), null); // several rows = several lists
  const noList = shapeRows([{
    brand: 'y',
    usage: usage({ series: { welcome: series('welcome'), postPurchase: series('postPurchase', { 0: [{ slug: 'a', listId: null, evergreen: 'none' }] }) } }),
  }])[0];
  assert.equal(cellLink(noList, 'postPurchase:0'), null);
  // A ZMA row's dimmed Welcome Day 0 does not link; no usage, no link.
  assert.equal(cellLink(shapeRows(api).find((r) => r.brand === 'oldzma'), 'welcome:0'), null);
  assert.equal(cellLink(shapeRows(api).find((r) => r.brand === 'nodoc'), 'welcome:0'), null);
});

// DASHBOARDS-POLISH2-SPEC I3 (supersedes POLISH I4's master-list / no-list landings): the
// Journeys circles and the broadcast cells link the BRAND's active lists.
const lists = [
  { id: 12, brand: 'ruze', status: 'active' },
  { id: 7, brand: 'ruze', status: 'active' },
  { id: 3, brand: 'ruze' },
  { id: 30, brand: 'ruze', status: 'archived' },
  { id: 20, brand: 'thirstygirl', status: 'active' },
  { id: 21, brand: 'lux', status: 'active' },
  { id: 9, brand: 'oldzma', status: 'active' },
  { id: 5, brand: 'other', status: 'active' },
  { id: 6, brand: '', status: 'active' },
];

test('I3: the brand list set -- the brand\'s active lists, ascending; store shapes; none -> []', () => {
  assert.deepEqual(brandListIds(lists, 'ruze'), [3, 7, 12]); // numeric, not lexicographic; archived out
  assert.deepEqual(brandListIds({ results: lists }, 'lux'), [21]);
  assert.deepEqual(brandListIds(lists, 'nobody'), []);
  assert.deepEqual(brandListIds(lists, ''), []); // a brandless list never joins a brand's set
  assert.deepEqual(brandListIds(undefined, 'ruze'), []);
  assert.deepEqual(shapeRows(api, { lists }).map((r) => [r.brand, r.listIds]), [
    ['ruze', [3, 7, 12]], ['thirstygirl', [20]], ['lux', [21]], ['oldzma', [9]], ['nodoc', []],
  ]);
  // No lists given (the store not loaded) -> every set empty.
  assert.deepEqual(shapeRows(api)[0].listIds, []);
});

// DASHBOARDS-POLISH3-SPEC D2 (amending POLISH2 I3's broadcast link): the broadcast cells also
// carry status=finished -- the landing shows the broadcasts the cadence counts, never drafts.
test('I3: broadcast cells link the brand set in the Broadcasts scope', () => {
  const [ruze, tg, lux] = shapeRows(api, { lists });
  const set = ['3', '7', '12'];
  const bcast = { evergreen: 'false', status: 'finished', list_id: set };
  assert.deepEqual(cellLink(ruze, 'cadence'), bcast);
  assert.deepEqual(cellLink(ruze, 'lastBroadcast'), bcast);
  assert.equal(cellLink(ruze, 'journeys'), null); // the journeys column retired 2026-10-30
  // A one-list brand links its one list (the Campaigns page reads it as the single filter).
  assert.equal(cell(tg, 'lastBroadcast', t).text, 'dashboard.usage.never'); // "never" is shown and links
  assert.deepEqual(cellLink(tg, 'lastBroadcast'), { evergreen: 'false', status: 'finished', list_id: ['20'] });
  assert.equal(cell(tg, 'cadence', t).text, MISSING); // a '—' cadence does not
  assert.equal(cellLink(tg, 'cadence'), null);
  assert.equal(cell(lux, 'cadence', t).text, MISSING);
  assert.equal(cellLink(lux, 'cadence'), null);
  assert.deepEqual(cellLink(lux, 'lastBroadcast'), { evergreen: 'false', status: 'finished', list_id: ['21'] });
  // A blank Last broadcast (no stored daysSince, not "never") does not link.
  const blank = shapeRows([{
    brand: 'ruze',
    usage: usage({
      broadcasts: {
        lastAt: null, windowCount: 0, cadenceDays: 5, daysSince: null, ratio: null, state: 'ok',
      },
    }),
  }], { lists })[0];
  assert.equal(cell(blank, 'lastBroadcast', t).text, '');
  assert.equal(cellLink(blank, 'lastBroadcast'), null);
  assert.deepEqual(cellLink(blank, 'cadence'), bcast);
  // The master list no longer gates the broadcast link -- the brand set does.
  const noMaster = shapeRows([{
    brand: 'ruze',
    usage: usage({
      welcome: {
        state: 'none', listId: null, campaigns: [], counts: { green: 0, amber: 0 },
      },
    }),
  }], { lists })[0];
  assert.deepEqual(cellLink(noMaster, 'cadence'), bcast);
  // An EMPTY set -> no link on either (the brand has no active list in the store).
  const noLists = shapeRows([{ brand: 'nolists', usage: usage() }], { lists })[0];
  ['cadence', 'lastBroadcast'].forEach((f) => assert.equal(cellLink(noLists, f), null, f));
  const unloaded = shapeRows(api)[0]; // ruze, shaped before the lists store loaded
  ['cadence', 'lastBroadcast'].forEach((f) => assert.equal(cellLink(unloaded, f), null, f));
  // ZMA-dimmed cells ('—') never link, even with lists.
  const zma = shapeRows(api, { lists }).find((r) => r.brand === 'oldzma');
  ['cadence', 'lastBroadcast'].forEach((f) => {
    assert.equal(cell(zma, f, t).text, MISSING, f);
    assert.equal(cellLink(zma, f), null, f);
  });
  // No usage, no link.
  const nodoc = shapeRows([{ brand: 'ruze' }], { lists })[0];
  ['cadence', 'lastBroadcast'].forEach((f) => assert.equal(cellLink(nodoc, f), null, f));
  // The Welcome Day 0 / series links are unchanged by the brand set (Automations, their own single list).
  assert.deepEqual(cellLink(ruze, 'welcome:0'), { list_id: '7', evergreen: 'true' });
  assert.equal(cellLink(ruze, 'postPurchase:0'), null);
});

// Fork (matrix polish, integrations MATRIX-POLISH-SPEC I6) -- Last broadcast opens the newest send
// event's campaigns ({ id, evergreen: 'all' }) when the doc carries broadcasts.lastIds; without it
// (a doc written before D10.1, or a malformed one) the old brand-lists link. Cadence is unchanged.
test('MATRIX-POLISH I6: cellLink(row, lastBroadcast) -> { id } with lastIds, the old link without', () => {
  const [ruze] = shapeRows(api, { lists });
  const withIds = { ...ruze, usage: { ...ruze.usage, broadcasts: { ...ruze.usage.broadcasts, lastIds: [41, 57] } } };
  assert.deepEqual(cellLink(withIds, 'lastBroadcast'), { id: ['41', '57'], evergreen: 'all' });
  assert.equal(linkTip(withIds, 'lastBroadcast', t), 'dashboard.usage.lastBroadcastLinkHelp');
  // Cadence keeps its brand-lists link and note.
  assert.deepEqual(cellLink(withIds, 'cadence'), { evergreen: 'false', status: 'finished', list_id: ['3', '7', '12'] });
  assert.equal(linkTip(withIds, 'cadence', t), 'dashboard.usage.broadcastLinkHelp');
  // No lastIds (an older doc), an empty one or a malformed one: the old link.
  const old = { evergreen: 'false', status: 'finished', list_id: ['3', '7', '12'] };
  [undefined, [], [0], [3, 'x'], [1.5], 'nope'].forEach((v) => {
    const r = { ...ruze, usage: { ...ruze.usage, broadcasts: { ...ruze.usage.broadcasts, lastIds: v } } };
    assert.deepEqual(cellLink(r, 'lastBroadcast'), old, JSON.stringify(v));
    assert.equal(linkTip(r, 'lastBroadcast', t), 'dashboard.usage.broadcastLinkHelp', JSON.stringify(v));
  });
  // A ZMA-dimmed row never links, lastIds or not.
  const zma = shapeRows(api).find((r) => r.brand === 'oldzma');
  const zmaIds = { ...zma, usage: { ...zma.usage, broadcasts: { ...(zma.usage.broadcasts || {}), lastIds: [5] } } };
  assert.equal(cellLink(zmaIds, 'lastBroadcast'), null);
});

test('I3: a linked broadcast cell carries the brand-broadcasts note; nothing else does', () => {
  const [ruze, tg] = shapeRows(api, { lists });
  assert.equal(linkTip(ruze, 'cadence', t), 'dashboard.usage.broadcastLinkHelp');
  assert.equal(linkTip(ruze, 'lastBroadcast', t), 'dashboard.usage.broadcastLinkHelp');
  assert.equal(linkTip(tg, 'cadence', t), ''); // '—', unlinked
  assert.equal(linkTip(ruze, 'journeys', t), '');
  assert.equal(linkTip(ruze, 'welcome:0', t), '');
  assert.equal(linkTip(shapeRows(api).find((r) => r.brand === 'oldzma'), 'lastBroadcast', t), '');
});

test('D7: cells -- Nd ago, never, ~Nd, —, Last broadcast colored on the TEXT; booleans ✓/✗', () => {
  const [ruze, tg, lux] = shapeRows(api);
  assert.deepEqual(
    [cell(ruze, 'lastBroadcast', t).text, cell(ruze, 'lastBroadcast', t).cls],
    ['dashboard.usage.daysAgo{"n":4}', 'has-text-success'],
  );
  assert.equal(cell(ruze, 'cadence', t).text, 'dashboard.usage.cadenceDays{"n":12}');
  assert.deepEqual([cell(tg, 'lastBroadcast', t).text, cell(tg, 'lastBroadcast', t).cls], ['dashboard.usage.never', 'has-text-danger']);
  assert.equal(cell(tg, 'cadence', t).text, MISSING);
  assert.equal(cell(lux, 'lastBroadcast', t).cls, 'has-text-danger');
  const due = shapeRows([{ brand: 'x', usage: usage({ broadcasts: { ...usage().broadcasts, state: 'due' } }) }])[0];
  assert.equal(cell(due, 'lastBroadcast', t).cls, 'has-text-warning-dark');
  assert.equal(cell(ruze, 'sunset', t).text, '✓');
  assert.equal(cell(ruze, 'attribution', t).cls, 'has-text-danger');
  assert.deepEqual(boolCell(true), { glyph: '✓', cls: 'has-text-success' });
  assert.deepEqual(boolCell(false), { glyph: '✗', cls: 'has-text-danger' });
  assert.deepEqual(boolCell(undefined), { glyph: '', cls: '' });
});

test('I6: a ZMA row never renders a red broadcast or welcome cell; registry facts still render', () => {
  const zma = shapeRows(api).find((r) => r.brand === 'oldzma');
  ['score', 'welcome:0', 'lastBroadcast', 'cadence'].forEach((f) => {
    const c = cell(zma, f, t);
    assert.equal(c.text, MISSING, f);
    assert.notEqual(c.cls, 'has-text-danger', f);
    assert.equal(c.cls, '', f);
    assert.deepEqual(c.circles, [], f);
  });
  assert.deepEqual(cell(zma, 'postPurchase:0', t).circles, [{ color: 'amber', n: 1 }]);
  assert.equal(cell(zma, 'sunset', t).text, '✓');
});

test('D1: the note date is the newest asOf among the rendered rows; none carries one -> null', () => {
  const rows = shapeRows([
    { brand: 'a', usage: usage({ asOf: '2026-10-23' }) },
    { brand: 'b', usage: usage({ asOf: '2026-10-24' }) },
    { brand: 'c', usage: usage({ asOf: 'garbage' }) },
    { brand: 'd' },
  ]);
  assert.equal(latestAsOf(rows), '2026-10-24');
  assert.equal(latestAsOf(shapeRows([{ brand: 'd' }, { brand: 'e', usage: usage() }])), null);
  assert.equal(latestAsOf([]), null);
});

test('V2 D5 / I4: the run time in Mountain time via Intl America/Denver -- 8:30 MST in winter, 9:30 MDT in summer', () => {
  const winter = mountainRunTime('2026-01-15');
  const summer = mountainRunTime('2026-07-15');
  assert.match(winter, /Jan 15, 2026/);
  assert.match(winter, /8:30\s?AM/u);
  assert.match(winter, /MST/);
  assert.match(summer, /Jul 15, 2026/);
  assert.match(summer, /9:30\s?AM/u);
  assert.match(summer, /MDT/);
  assert.equal(mountainRunTime('garbage'), null);
  assert.equal(mountainRunTime(null), null);
});

// POLISH3 D5 (stage 2 F7) narrows this: the Skipped leaves render from the row's live `skipped`,
// not from usage -- here (no `skipped` on the row either) they are '—'.
test('D7: a row without usage renders blanks', () => {
  const nodoc = shapeRows(api).find((r) => r.brand === 'nodoc');
  SORT_FIELDS.filter((f) => f !== 'brand' && skippedLangOf(f) === null).forEach((f) => {
    assert.deepEqual(cell(nodoc, f, t), {
      text: '', cls: '', tooltip: '', circles: [],
    }, f);
  });
  SORT_FIELDS.filter((f) => skippedLangOf(f) !== null).forEach((f) => assert.equal(cell(nodoc, f, t).text, MISSING, f));
});

test('V2 I2: a fresh session sorts by Score descending; missing score last in BOTH orders', () => {
  assert.deepEqual(DEFAULT_STATE.sort, { field: 'score', order: 'desc' });
  const rows = shapeRows(api);
  // oldzma is ZMA (null score) and nodoc has no usage -- both last, by name.
  assert.deepEqual(sortRows(rows).map((r) => r.brand), ['ruze', 'lux', 'thirstygirl', 'nodoc', 'oldzma']);
  assert.deepEqual(sortRows(rows, { field: 'score', order: 'asc' }).map((r) => r.brand), ['thirstygirl', 'lux', 'ruze', 'nodoc', 'oldzma']);
  // A pre-V2 doc (usage, no score) ranks with the missing.
  const withOld = [...rows, ...shapeRows([{ brand: 'aaold', usage: usage({ score: undefined }) }])];
  assert.deepEqual(sortRows(withOld).map((r) => r.brand).slice(-3), ['aaold', 'nodoc', 'oldzma']);
  assert.equal(sortValue(rows[0], 'score'), 55);
  assert.equal(sortValue(rows[3], 'score'), null);
  // An unknown sort field falls back to the default.
  assert.deepEqual(sortRows(rows, { field: 'bogus', order: 'desc' }).map((r) => r.brand), sortRows(rows).map((r) => r.brand));
});

test('I7: the v1 comparators stand -- Last broadcast worst first; states and booleans by badness', () => {
  const rows = shapeRows(api);
  assert.deepEqual(sortRows(rows, { field: 'lastBroadcast', order: 'desc' }).map((r) => r.brand), ['thirstygirl', 'lux', 'ruze', 'nodoc', 'oldzma']);
  assert.deepEqual(sortRows(rows, { field: 'lastBroadcast', order: 'asc' }).map((r) => r.brand), ['ruze', 'lux', 'thirstygirl', 'nodoc', 'oldzma']);
  assert.deepEqual(sortRows(rows, { field: 'brand', order: 'asc' }).map((r) => r.brand), ['lux', 'nodoc', 'oldzma', 'ruze', 'thirstygirl']);
  assert.equal(sortValue(rows[0], 'welcome:0'), 0);
  assert.equal(sortValue(rows[3], 'postPurchase:0'), 1);
  assert.equal(sortValue(rows[3], 'welcome:0'), null);
  assert.equal(sortValue(rows[0], 'attribution'), 1);
  assert.equal(sortValue(rows[0], 'sunset'), 0);
});

test('V2 D2 / I2: header clicks (clickSort) -- first click Score desc, Brand asc, every other desc; repeat flips', () => {
  assert.equal(defaultOrder('brand'), 'asc');
  assert.equal(defaultOrder('score'), 'desc');
  const start = { field: 'lastBroadcast', order: 'desc' };
  assert.deepEqual(clickSort(start, 'brand'), { field: 'brand', order: 'asc' });
  assert.deepEqual(clickSort(start, 'score'), { field: 'score', order: 'desc' });
  ['welcome:0', 'postPurchase:0', 'welcome:2', 'postPurchase:30', 'skipped:fr', 'sunset', 'attribution', 'cadence'].forEach((f) => {
    assert.deepEqual(clickSort(start, f), { field: f, order: 'desc' }, f);
  });
  assert.deepEqual(clickSort({ field: 'brand', order: 'asc' }, 'lastBroadcast'), { field: 'lastBroadcast', order: 'desc' });
  // A repeat click toggles.
  assert.deepEqual(clickSort(start, 'lastBroadcast'), { field: 'lastBroadcast', order: 'asc' });
  assert.deepEqual(clickSort({ field: 'score', order: 'desc' }, 'score'), { field: 'score', order: 'asc' });
  assert.deepEqual(clickSort({ field: 'brand', order: 'asc' }, 'brand'), { field: 'brand', order: 'desc' });
  // An unsortable field leaves the sort unchanged; no current sort is the default.
  assert.deepEqual(clickSort(start, 'bogus'), start);
  assert.deepEqual(clickSort(start, 'journeys'), start); // retired 2026-10-30: no longer a column
  assert.deepEqual(clickSort(null, 'score'), { field: 'score', order: 'asc' });
});

test('I7 / V2 I2: session persistence -- a stored sort wins; a throwing sessionStorage yields the defaults and still renders', () => {
  const throwingWindow = {};
  Object.defineProperty(throwingWindow, 'sessionStorage', { get() { throw new Error('SecurityError'); } });
  const store = sessionStore(throwingWindow);
  assert.equal(store, null);
  assert.deepEqual(loadState(store), { sort: { ...DEFAULT_STATE.sort } });
  assert.equal(saveState(store, { sort: { field: 'brand', order: 'asc' } }), false);
  // A store whose methods throw, and bad JSON, are defaults too.
  const bad = {
    getItem() { throw new Error('x'); },
    setItem() { throw new Error('x'); },
  };
  assert.deepEqual(loadState(bad), { sort: { ...DEFAULT_STATE.sort } });
  assert.equal(saveState(bad, {}), false);
  assert.deepEqual(loadState({ getItem: () => '{not json' }), { sort: { ...DEFAULT_STATE.sort } });
  // Rendering is unaffected by the storage failure.
  assert.equal(sortRows(shapeRows(api), loadState(store).sort)[0].brand, 'ruze');

  // A working store round-trips under the pane's own key, and the stored sort wins.
  const mem = new Map();
  const ok = { getItem: (k) => (mem.has(k) ? mem.get(k) : null), setItem: (k, v) => mem.set(k, v) };
  assert.equal(saveState(ok, { sort: { field: 'cadence', order: 'asc' } }), true);
  assert.ok(mem.has(STORAGE_KEY));
  assert.equal(STORAGE_KEY, 'dashboard.usage');
  assert.deepEqual(loadState(ok), { sort: { field: 'cadence', order: 'asc' } });
  // A v1 session's stored sort (lastBroadcast) still loads -- it wins over the new default.
  assert.deepEqual(loadState({ getItem: () => JSON.stringify({ sort: { field: 'lastBroadcast', order: 'desc' } }) }), { sort: { field: 'lastBroadcast', order: 'desc' } });
  assert.deepEqual(loadState({ getItem: () => JSON.stringify({ sort: { field: 'nope', order: 'up' } }) }), { sort: { ...DEFAULT_STATE.sort } });
  // POLISH3 D4 / S9: a session sorted by a retired column (the single 'noPurchase', a 3d | 7d | 15d
  // leaf, the plain Welcome / 1st purchase) falls back to the default.
  ['noPurchase', 'noPurchase:3', 'welcome', 'purchase'].forEach((field) => {
    assert.deepEqual(loadState({ getItem: () => JSON.stringify({ sort: { field, order: 'desc' } }) }), { sort: { ...DEFAULT_STATE.sort } }, field);
  });
});

// ---------------------------------------------------------------------------------------------
// LIFECYCLE-SERIES-SPEC I15 (fork half; rewriting POLISH3 I4's No purchase 3d | 7d | 15d mapping)
// -- the series leaves, the legacy chip and the pre-release doc
// ---------------------------------------------------------------------------------------------

const npStep = (days, listId, evergreen, error) => ({
  slug: `x-no-purchase-${listId}`, listId, evergreen, days, ...(error ? { error } : {}),
});
// A row whose No-purchase series carries `rows` at their days (standard days only) and `legacy`.
const wsRow = (rows, legacy = [], brand = 'x') => {
  const byDay = {};
  rows.forEach((r) => { byDay[r.days] = [...(byDay[r.days] || []), r]; });
  return shapeRows([{
    brand,
    usage: usage({ series: { welcome: series('welcome', byDay, legacy, { 0: 'active' }), postPurchase: series('postPurchase') } }),
  }])[0];
};

test('I15: each leaf maps the stored step at EXACTLY that day -- green running, amber idle, red 0 absent; legacy only as a chip', () => {
  const r = wsRow([npStep(10, 40, 'running'), npStep(5, 41, 'idle')], [npStep(15, 42, 'running')]);
  assert.deepEqual(cell(r, 'welcome:2', t).circles, [{ color: 'red', n: 0 }]);
  assert.deepEqual(cell(r, 'welcome:5', t).circles, [{ color: 'amber', n: 1 }]);
  assert.deepEqual(cell(r, 'welcome:10', t).circles, [{ color: 'green', n: 1 }]);
  // The 15-day legacy step appears in no leaf -- it is the grey chip after 10d.
  WELCOME_FIELDS.forEach((f) => assert.ok(!cell(r, f, t).tooltip.includes('x-no-purchase-42'), f));
  assert.deepEqual(cellBadges(r, 'welcome:10', t), [{
    kind: 'legacy',
    glyph: 'dashboard.usage.legacyChip{"n":15}',
    tooltip: 'series.welcomeLegacy\nx-no-purchase-42: dashboard.usage.listEvergreen{"list":42,"state":"running"}',
  }]);
  // The tooltip: the D5 description, the words, then the step's row.
  assert.equal(
    cell(r, 'welcome:10', t).tooltip,
    'series.welcomeDay10\ndashboard.usage.circleRunning{"n":1}\nx-no-purchase-40: dashboard.usage.listEvergreen{"list":40,"state":"running"}',
  );
  assert.equal(cell(r, 'welcome:2', t).tooltip, 'series.welcomeDay2\ndashboard.usage.circleNone');
  // An invalid row WITH a standard day sits in its leaf as amber, its error in the tooltip.
  const bad = wsRow([npStep(2, 43, 'running', 'sinceAt "x" is not an ISO timestamp')]);
  assert.deepEqual(cell(bad, 'welcome:2', t).circles, [{ color: 'amber', n: 1 }]);
  assert.match(cell(bad, 'welcome:2', t).tooltip, /sinceAt "x" is not an ISO timestamp/);
  // A step without a list is amber.
  assert.deepEqual(cell(wsRow([npStep(5, null, 'none')]), 'welcome:5', t).circles, [{ color: 'amber', n: 1 }]);
  // The Ruze shape (G7): welcome running, only the grandfathered 15d step -> Day 0 green, red 0 x3,
  // one legacy chip.
  const ruze = wsRow([], [npStep(15, 56, 'running')], 'ruze');
  assert.deepEqual(WELCOME_FIELDS.map((f) => cell(ruze, f, t).circles), [
    [{ color: 'green', n: 1 }], [{ color: 'red', n: 0 }], [{ color: 'red', n: 0 }], [{ color: 'red', n: 0 }],
  ]);
  assert.deepEqual(cellBadges(ruze, 'welcome:10', t).map((b) => b.kind), ['legacy']);
  // The Post-purchase group maps the same way, its chips after 30d.
  const pp = shapeRows([{
    brand: 'p',
    usage: usage({
      series: {
        welcome: series('welcome'),
        postPurchase: series('postPurchase', { 0: [{ slug: 'p-first-order', listId: 51, evergreen: 'running' }], 14: [ppRow(14, 80, 'idle')] }),
      },
    }),
  }])[0];
  assert.deepEqual(POST_PURCHASE_FIELDS.map((f) => cell(pp, f, t).circles), [
    [{ color: 'green', n: 1 }], [{ color: 'red', n: 0 }], [{ color: 'amber', n: 1 }], [{ color: 'red', n: 0 }],
  ]);
  assert.match(cell(pp, 'postPurchase:14', t).tooltip, /^series\.postPurchaseDay14\n/);
});

test('I15 (POLISH3 stage 2 F5): a no-days row is an amber ⚠ badge with its parser error; a pre-release doc is — throughout', () => {
  const r = wsRow([npStep(5, 41, 'running')], [npStep(null, 44, 'none', 'days "x" is not an integer ≥ 1'), npStep(null, 45, 'none', 'days "0" is not an integer ≥ 1')]);
  assert.deepEqual(cellBadges(r, 'welcome:10', t), [
    { kind: 'warn', glyph: '⚠', tooltip: 'dashboard.usage.npInvalid{"slug":"x-no-purchase-44","error":"days \\"x\\" is not an integer ≥ 1"}' },
    { kind: 'warn', glyph: '⚠', tooltip: 'dashboard.usage.npInvalid{"slug":"x-no-purchase-45","error":"days \\"0\\" is not an integer ≥ 1"}' },
  ]);
  // Only after the group's LAST leaf; nowhere else.
  ['welcome:0', 'welcome:2', 'welcome:5', 'postPurchase:0', 'postPurchase:30', 'skipped:fr'].forEach((f) => assert.deepEqual(cellBadges(r, f, t), [], f));
  assert.deepEqual(cell(r, 'welcome:5', t).circles, [{ color: 'green', n: 1 }]);
  // A doc written before the release (no `series`; it still carries noPurchase) -> every series
  // leaf '—', never a red 0; no link, no sort value, no chip (S9 / Stage 2 F7).
  const preUsage = usage({ noPurchase: { state: 'active', rows: [npStep(15, 9, 'running')], counts: { green: 1, amber: 0 } } });
  delete preUsage.series;
  const pre = shapeRows([{ brand: 'p', usage: preUsage }], { lists })[0];
  [...WELCOME_FIELDS, ...POST_PURCHASE_FIELDS].forEach((f) => {
    const c = cell(pre, f, t);
    assert.equal(c.text, MISSING, f);
    assert.deepEqual(c.circles, [], f);
    assert.match(c.tooltip, /\ndashboard\.usage\.seriesMissing$/, f);
    assert.equal(cellLink(pre, f), null, f);
    assert.equal(sortValue(pre, f), null, f);
  });
  assert.deepEqual(cellBadges(pre, 'welcome:10', t), []);
});

test('I15: a leaf links its step list only when exactly one step sits at that day', () => {
  const r = wsRow([npStep(10, 40, 'running'), npStep(5, 41, 'idle'), npStep(5, 46, 'running')]);
  assert.deepEqual(cellLink(r, 'welcome:10'), { list_id: '40', evergreen: 'true' });
  assert.equal(cellLink(r, 'welcome:5'), null); // two steps at 5d
  assert.deepEqual(cell(r, 'welcome:5', t).circles, [{ color: 'green', n: 1 }, { color: 'amber', n: 1 }]);
  assert.equal(cellLink(r, 'welcome:2'), null); // absent
  assert.equal(cellLink(wsRow([npStep(2, null, 'none')]), 'welcome:2'), null); // no list
  assert.equal(cellLink(shapeRows(api).find((x) => x.brand === 'nodoc'), 'welcome:2'), null);
});

test('I15: the leaves sort by badness -- absent worst, then amber, then green; — last', () => {
  const preUsage = usage();
  delete preUsage.series;
  const rows = [
    wsRow([npStep(2, 40, 'running')], [], 'green'),
    wsRow([npStep(2, 41, 'idle')], [], 'amber'),
    wsRow([], [], 'absent'),
    shapeRows([{ brand: 'pre', usage: preUsage }])[0],
  ];
  assert.deepEqual(rows.map((r) => sortValue(r, 'welcome:2')), [0, 1, 2, null]);
  assert.deepEqual(sortRows(rows, { field: 'welcome:2', order: 'desc' }).map((r) => r.brand), ['absent', 'amber', 'green', 'pre']);
  assert.deepEqual(sortRows(rows, { field: 'welcome:2', order: 'asc' }).map((r) => r.brand), ['green', 'amber', 'absent', 'pre']);
});

// ---------------------------------------------------------------------------------------------
// DASHBOARDS-POLISH3-SPEC D5 -- the Languages / Skipped leaves
// ---------------------------------------------------------------------------------------------

const zero = { count: 0, lists: [] };
const skipped = (over = {}) => ({
  fr: zero, es: zero, de: zero, it: zero, ...over,
});

test('POLISH3 D5: Skipped -- red linked count > 0, plain 0 otherwise, — on ZMA rows; independent of usage', () => {
  const rows = shapeRows([
    { brand: 'ruze', usage: usage(), skipped: skipped({ fr: { count: 12, lists: [7, 12] }, es: { count: 3, lists: [12] } }) },
    { brand: 'nousage', skipped: skipped({ de: { count: 5, lists: [21] } }) },
    { brand: 'oldzma', usage: usage({ channel: 'zma' }), skipped: skipped({ fr: { count: 9, lists: [9] } }) },
  ], { lists });
  const [ruze, nousage, zma] = rows;
  assert.deepEqual([cell(ruze, 'skipped:fr', t).text, cell(ruze, 'skipped:fr', t).cls], ['12', 'has-text-danger']);
  assert.equal(cell(ruze, 'skipped:fr', t).tooltip, 'dashboard.usage.skippedCell{"n":12,"lists":"7, 12"}');
  assert.deepEqual(cellLink(ruze, 'skipped:fr'), { list_id: ['7', '12'], evergreen: 'all' });
  assert.deepEqual(cellLink(ruze, 'skipped:es'), { list_id: ['12'], evergreen: 'all' });
  assert.equal(linkTip(ruze, 'skipped:fr', t), 'dashboard.usage.skippedLinkHelp');
  // 0 -> a plain grey '0' (2026-10-30, replacing G5's green ✓: a count, not a coverage claim),
  // unlinked; the tooltip names the causes.
  assert.deepEqual([cell(ruze, 'skipped:de', t).text, cell(ruze, 'skipped:de', t).cls], ['0', 'has-text-grey']);
  assert.equal(sortValue(ruze, 'skipped:de'), 0); // the sort key stays the count
  assert.equal(cell(ruze, 'skipped:de', t).tooltip, 'dashboard.usage.skippedZero');
  assert.equal(cellLink(ruze, 'skipped:de'), null);
  assert.equal(linkTip(ruze, 'skipped:de', t), '');
  // No usage: the Skipped counts still render (and link); the other cells stay blank.
  assert.equal(nousage.usage, null);
  assert.deepEqual([cell(nousage, 'skipped:de', t).text, cell(nousage, 'skipped:de', t).cls], ['5', 'has-text-danger']);
  assert.deepEqual(cellLink(nousage, 'skipped:de'), { list_id: ['21'], evergreen: 'all' });
  assert.equal(cell(nousage, 'skipped:fr', t).text, '0'); // a count again (2026-10-30; G5's ✓ retired)
  assert.equal(cell(nousage, 'welcome', t).text, '');
  // ZMA dimming (keyed off usage.channel): '—', unlinked, sorts last.
  assert.equal(cell(zma, 'skipped:fr', t).text, MISSING);
  assert.equal(cellLink(zma, 'skipped:fr'), null);
  assert.equal(sortValue(zma, 'skipped:fr'), null);
  // A malformed entry is '—'.
  const bad = shapeRows([{ brand: 'b', skipped: { fr: { count: -1, lists: [] }, es: 'x' } }])[0];
  ['skipped:fr', 'skipped:es', 'skipped:it'].forEach((f) => assert.equal(cell(bad, f, t).text, MISSING, f));
});

test('POLISH3 D5: Skipped leaves sort descending by count, missing/— last', () => {
  const rows = shapeRows([
    { brand: 'a', skipped: skipped({ fr: { count: 2, lists: [1] } }) },
    { brand: 'b', skipped: skipped({ fr: { count: 40, lists: [2] } }) },
    { brand: 'c', skipped: skipped() },
    { brand: 'd' },
    { brand: 'z', usage: usage({ channel: 'zma' }), skipped: skipped({ fr: { count: 99, lists: [3] } }) },
  ]);
  assert.equal(defaultOrder('skipped:fr'), 'desc');
  assert.deepEqual(sortRows(rows, { field: 'skipped:fr', order: 'desc' }).map((r) => r.brand), ['b', 'a', 'c', 'd', 'z']);
  assert.deepEqual(sortRows(rows, { field: 'skipped:fr', order: 'asc' }).map((r) => r.brand), ['c', 'a', 'b', 'd', 'z']);
});

// 2026-10-30: the SKU column -- the lib's stored `usage.sku` rendered verbatim (circles from the
// stored counts), linked to the automations of every SKU-trigger list, sorted by how many there are.
test('SKU column: stored counts as circles, rows in the tooltip, linked to every SKU list, sorted by count', () => {
  const sku = {
    rows: [
      {
        slug: 'ruze-skus-41', listId: 41, evergreen: 'running', days: null,
      },
      {
        slug: 'ruze-skus-40', listId: 40, evergreen: 'idle', days: null,
      },
      {
        slug: 'ruze-skus-42', listId: null, error: 'trigger.skus must be a non-empty array', evergreen: 'none', days: null,
      },
    ],
    counts: { green: 1, amber: 2 },
  };
  const [r] = shapeRows([{ brand: 'ruze', usage: usage({ sku }) }]);
  const c = cell(r, 'sku', t);
  assert.deepEqual(c.circles, [{ color: 'green', n: 1 }, { color: 'amber', n: 2 }]);
  assert.equal(c.tooltip, [
    'dashboard.usage.circleRunning{"n":1}, dashboard.usage.circleIdle{"n":2}',
    'ruze-skus-41: dashboard.usage.listEvergreen{"list":41,"state":"running"}',
    'ruze-skus-40: dashboard.usage.listEvergreen{"list":40,"state":"idle"}',
    'ruze-skus-42: trigger.skus must be a non-empty array',
  ].join('\n'));
  // The filtered set: every SKU row's list (ascending, de-duplicated), in the Automations scope.
  assert.deepEqual(cellLink(r, 'sku'), { evergreen: 'true', list_id: ['40', '41'] });
  assert.equal(linkTip(r, 'sku', t), ''); // like the series leaves: no link hint (circle cells show the tooltip only)
  assert.equal(sortValue(r, 'sku'), 3);
  assert.deepEqual(columnMeta('sku'), { label: 'dashboard.usage.sku', help: 'dashboard.usage.skuHelp' });
  // None: a red 0, no link, sort 0.
  const [none] = shapeRows([{ brand: 'ruze', usage: usage({ sku: { rows: [], counts: { green: 0, amber: 0 } } }) }]);
  assert.deepEqual(cell(none, 'sku', t).circles, [{ color: 'red', n: 0 }]);
  assert.equal(cellLink(none, 'sku'), null);
  assert.equal(sortValue(none, 'sku'), 0);
  // A doc written before the column (no `sku`): '—', no link, sorts last (null).
  const [old] = shapeRows([{ brand: 'ruze', usage: usage() }]);
  assert.equal(cell(old, 'sku', t).text, MISSING);
  assert.equal(cellLink(old, 'sku'), null);
  assert.equal(sortValue(old, 'sku'), null);
  // The strings exist.
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  ['dashboard.usage.sku', 'dashboard.usage.skuHelp'].forEach((k) => assert.equal(typeof en[k], 'string', k));
});

// 2026-10-30: the in-section rules -- No-purchase | Post-purchase and Post-purchase | SKU, the same
// on the header and every body column; no rule between plain leaves (Cadence | Last broadcast).
test('dividers: header and body carry the same in-section rules', () => {
  const h = headerLayout();
  assert.deepEqual(h.row2.filter((c) => c.divide).map((c) => (c.kind === 'group' ? `[${c.key}]` : c.field)), ['[postPurchase]', 'sku']);
  assert.deepEqual(h.row3.filter((c) => c.divide).map((c) => c.field), ['postPurchase:0']);
  const cols = bodyColumns();
  assert.deepEqual(cols.map((c) => c.field), SORT_FIELDS.slice(2));
  assert.deepEqual(cols.filter((c) => c.divide).map((c) => c.field), ['postPurchase:0', 'sku']);
  assert.deepEqual(cols.filter((c) => c.first).map((c) => c.field), ['cadence', 'welcome:0', 'skipped:fr', 'attribution']);
});

// 2026-10-30: each circle links only what it counts.
test('circleLink: Day 0 green/amber open their own campaigns; step and SKU circles their own lists; red never', () => {
  const welcome = {
    state: 'idle',
    listId: 6,
    campaigns: [{ id: 67, status: 'running' }, { id: 70, status: 'paused' }, { id: 71, status: 'draft' }],
    counts: { green: 1, amber: 2 },
  };
  const step = (listId, evergreen, extra = {}) => ({
    slug: `x-${listId}`, listId, evergreen, days: 5, ...extra,
  });
  const postPurchase = series('postPurchase');
  postPurchase.steps = postPurchase.steps.map((s) => (s.days === 5 ? { ...s, rows: [step(80, 'running'), step(81, 'idle'), step(82, 'none', { error: 'bad' })] } : s));
  const sku = { rows: [step(90, 'running'), step(91, 'idle')], counts: { green: 1, amber: 1 } };
  const [r] = shapeRows([{
    brand: 'thirstygirl',
    usage: usage({
      welcome,
      series: { welcome: series('welcome', {}, [], { 0: 'idle' }), postPurchase },
      sku,
    }),
  }]);
  assert.deepEqual(circleLink(r, 'welcome:0', 'green'), { id: ['67'], evergreen: 'true' });
  assert.deepEqual(circleLink(r, 'welcome:0', 'amber'), { id: ['70', '71'], evergreen: 'true' });
  assert.equal(circleLink(r, 'welcome:0', 'red'), null);
  assert.deepEqual(circleLink(r, 'postPurchase:5', 'green'), { list_id: ['80'], evergreen: 'true' });
  assert.deepEqual(circleLink(r, 'postPurchase:5', 'amber'), { list_id: ['81', '82'], evergreen: 'true' });
  assert.equal(circleLink(r, 'postPurchase:14', 'green'), null); // no rows at that day
  assert.deepEqual(circleLink(r, 'sku', 'green'), { list_id: ['90'], evergreen: 'true' });
  assert.deepEqual(circleLink(r, 'sku', 'amber'), { list_id: ['91'], evergreen: 'true' });
  // No usage, or a ZMA-dimmed Day 0: no link.
  assert.equal(circleLink(shapeRows([{ brand: 'n' }])[0], 'welcome:0', 'green'), null);
  const zma = shapeRows(api).find((x) => x.brand === 'oldzma');
  assert.equal(circleLink(zma, 'welcome:0', 'green'), null);
});

// 2026-10-30: the journey guide below the table.
test('guide: parseStepText splits title / quoted subject line / guidance', () => {
  assert.deepEqual(parseStepText('Day 2 Gentle conversion reminder: "Still exploring? Here\'s why customers love [Brand]." Highlight bestsellers.'), {
    title: 'Gentle conversion reminder', quote: 'Still exploring? Here\'s why customers love [Brand].', body: 'Highlight bestsellers.',
  });
  // A title ending in "?" and a body containing ": " -- split at the FIRST ": ".
  assert.deepEqual(parseStepText('Day 14 How\'s it going?: About a week after delivery: ask for a review.'), {
    title: 'How\'s it going?', quote: '', body: 'About a week after delivery: ask for a review.',
  });
  assert.deepEqual(parseStepText('Day 5 Getting started: Around delivery: how to use it.'), {
    title: 'Getting started', quote: '', body: 'Around delivery: how to use it.',
  });
  assert.deepEqual(parseStepText('no colon here'), { title: '', quote: '', body: 'no colon here' });
  assert.deepEqual(parseStepText(undefined), { title: '', quote: '', body: '' });
});

test('guide: seriesGuide over the real en.json -- Welcome then New Customer, every standard day, every step titled', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  const tt = (k) => en[k];
  const g = seriesGuide(tt);
  assert.deepEqual(g.map((s) => [s.family, tt(s.title), tt(s.subtitle)]), [
    ['welcome', 'No-purchase series', 'Welcome series'],
    ['postPurchase', 'Post-purchase series', 'New Customer series'],
  ]);
  assert.deepEqual(g.map((s) => s.steps.map((x) => x.days)), [SERIES_DAYS.welcome, SERIES_DAYS.postPurchase]);
  g.flatMap((s) => s.steps).forEach((x) => {
    assert.ok(x.title && x.body, `day ${x.days}`);
  });
  assert.deepEqual(g[0].steps.map((x) => x.title), ['Welcome', 'Gentle conversion reminder', 'Address purchase hesitation', 'Final conversion nudge']);
  assert.deepEqual(g[1].steps.map((x) => x.title), ['Thank you', 'Getting started', 'How\'s it going?', 'Your next favorite']);
  assert.equal(g[0].steps[1].quote, 'Still exploring? Here\'s why customers love [Brand].');
  assert.equal(g[1].steps[0].quote, '');
  assert.equal(GUIDE_SERIES.length, 2);
  assert.equal(typeof en['dashboard.usage.guideDay'], 'string');
});

test('guide: guideSplitX -- the measured line when both sides keep the minimum, else stacked (null)', () => {
  assert.equal(guideSplitX(640.4, 1400), 640);
  assert.equal(guideSplitX(GUIDE_MIN_COLUMN, GUIDE_MIN_COLUMN * 2), GUIDE_MIN_COLUMN);
  assert.equal(guideSplitX(GUIDE_MIN_COLUMN - 1, 1400), null); // too little room on the left
  assert.equal(guideSplitX(1200, 1400), null); // too little room on the right
  assert.equal(guideSplitX(-50, 1400), null); // the table scrolled the line off to the left
  assert.equal(guideSplitX(NaN, 1400), null);
  assert.equal(guideSplitX(600, undefined), null);
});
