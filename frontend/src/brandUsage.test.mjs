// Fork (brand usage, integrations listmonk/BRAND-USAGE-SPEC I6/I7, D2/D7; BRAND-USAGE-V2-SPEC
// I1-I4, I7). Run: yarn test (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  CIRCLE_FIELDS, DEFAULT_STATE, MISSING, SCORE_PARTS, SECTIONS, SORT_FIELDS, STANDARD_NO_PURCHASE_DAYS, STORAGE_KEY,
  boolCell, broadcastTip, cell, cellBadges, cellLink, circlesOf, circleWords, clickSort, columnMeta, defaultOrder,
  headerLayout, latestAsOf, loadState, mountainRunTime, NO_PURCHASE_FIELDS as NO_PURCHASE_LEAVES, npDaysOf, saveState, scoreTip, shapeRows, skippedLangOf, sortRows,
  sortValue, storedCounts, triggerTip, linkTip, brandListIds,
} from './brandUsage.mjs'; // eslint-disable-line import/extensions
import { PANES, initialPane, sessionStore } from './brandMatrix.mjs'; // eslint-disable-line import/extensions

const t = (k, p) => (p ? `${k}${JSON.stringify(p)}` : k);

const parts = (b, w, p, n, a, s) => ({
  broadcasts: { points: b, max: 40 },
  welcome: { points: w, max: 15 },
  purchase: { points: p, max: 15 },
  noPurchase: { points: n, max: 10 },
  attribution: { points: a, max: 10 },
  sunset: { points: s, max: 10 },
});
const score = (total, p) => ({ total, parts: p });

const usage = (over = {}) => ({
  channel: 'listmonk',
  welcome: {
    state: 'active', listId: 7, campaigns: [{ id: 3, status: 'running' }], counts: { green: 1, amber: 0 },
  },
  purchase: { state: 'none', rows: [], counts: { green: 0, amber: 0 } },
  noPurchase: { state: 'none', rows: [], counts: { green: 0, amber: 0 } },
  sunset: true,
  attribution: false,
  journeys: { running: 2, idle: 1 },
  broadcasts: {
    lastAt: '2026-10-20T12:00:00.000Z', windowCount: 4, cadenceDays: 12.4, daysSince: 4.6, ratio: 0.37, state: 'ok',
  },
  score: score(65, parts(40, 15, 0, 0, 0, 10)),
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
      score: score(25, parts(0, 15, 0, 0, 0, 10)),
    }),
  },
  {
    brand: 'lux',
    health: { status: 'ok' },
    usage: usage({
      broadcasts: {
        lastAt: '2026-08-01T00:00:00.000Z', windowCount: 1, cadenceDays: null, daysSince: 84.6, ratio: 2.82, state: 'stale',
      },
      score: score(35, parts(10, 15, 0, 0, 0, 10)),
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
      purchase: { state: 'idle', rows: [{ slug: 'oldzma-first-order', listId: null, evergreen: 'none' }], counts: { green: 0, amber: 1 } },
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

// DASHBOARDS-POLISH3-SPEC D4/D5 (replacing V2 D2's single 'noPurchase' column and the
// three-section layout): No purchase is a sub-group over 3d | 7d | 15d, and the Languages section
// (Skipped over FR ES DE IT) sits between Journeys and Configurations.
test('V2 D2 / POLISH3 D4-D5: columns -- Score, Brand, then Broadcasts / Journeys / Languages / Configurations', () => {
  assert.deepEqual(STANDARD_NO_PURCHASE_DAYS, [3, 7, 15]);
  assert.deepEqual(SECTIONS.map((s) => [s.key, s.fields]), [
    ['broadcasts', ['cadence', 'lastBroadcast']],
    ['journeys', ['journeys', 'welcome', 'purchase', 'noPurchase:3', 'noPurchase:7', 'noPurchase:15']],
    ['languages', ['skipped:fr', 'skipped:es', 'skipped:de', 'skipped:it']],
    ['configurations', ['attribution', 'sunset']],
  ]);
  assert.deepEqual(SORT_FIELDS, [
    'score', 'brand', 'cadence', 'lastBroadcast', 'journeys', 'welcome', 'purchase', 'noPurchase:3', 'noPurchase:7',
    'noPurchase:15', 'skipped:fr', 'skipped:es', 'skipped:de', 'skipped:it', 'attribution', 'sunset',
  ]);
  assert.deepEqual(CIRCLE_FIELDS, ['journeys', 'welcome', 'purchase', 'noPurchase:3', 'noPurchase:7', 'noPurchase:15']);
  // The header: plain leaves in row 2 (rowspan 2), the sub-group labels over their leaves in row 3.
  const h = headerLayout();
  assert.deepEqual(h.row2.map((c) => (c.kind === 'group' ? `[${c.key}:${c.span}]` : c.field)), [
    'cadence', 'lastBroadcast', 'journeys', 'welcome', 'purchase', '[noPurchase:3]', '[skipped:4]', 'attribution', 'sunset',
  ]);
  // G5 round 2: BOTH group labels take the single-row headers' font size (small: false); only the
  // leaf row (row3) renders small.
  assert.deepEqual(h.row2.filter((c) => c.kind === 'group').map((c) => [c.label, c.band, c.first, c.small]), [
    ['dashboard.usage.noPurchase', 'white', false, false], ['dashboard.usage.skipped', 'grey', true, false],
  ]);
  assert.deepEqual(h.row3.map((c) => [c.field, c.first, c.groupStart]), [
    ['noPurchase:3', false, true], ['noPurchase:7', false, false], ['noPurchase:15', false, false],
    ['skipped:fr', true, true], ['skipped:es', false, false], ['skipped:de', false, false], ['skipped:it', false, false],
  ]);
  assert.deepEqual(columnMeta('noPurchase:7'), { label: 'dashboard.usage.npDays', params: { n: 7 }, help: 'dashboard.usage.noPurchaseHelp' });
  assert.deepEqual(columnMeta('skipped:de'), { text: 'DE', help: 'dashboard.usage.skippedHelp' });
  assert.equal(columnMeta('welcome').label, 'dashboard.usage.welcome');
  assert.equal(npDaysOf('noPurchase:5'), null); // off-standard: no leaf
  assert.equal(skippedLangOf('skipped:en'), null); // EN has no column (recorded limitation)
});

test('V2 D1 / I1: the Score cell -- the stored total, the breakdown from parts verbatim in the FIXED order', () => {
  const [ruze] = shapeRows(api);
  const c = cell(ruze, 'score', t);
  assert.equal(c.text, '65');
  assert.deepEqual(SCORE_PARTS.map((p) => p.key), ['broadcasts', 'welcome', 'purchase', 'noPurchase', 'attribution', 'sunset']);
  assert.equal(
    c.tooltip,
    'dashboard.usage.partBroadcasts 40/40 · dashboard.usage.partWelcome 15/15 · dashboard.usage.partPurchase 0/15 · '
      + 'dashboard.usage.partNoPurchase 0/10 · dashboard.usage.partAttribution 0/10 · dashboard.usage.partSunset 10/10',
  );
  // jsonb returns keys lexicographically -- the display order is ours, not the object's.
  const lexi = {
    attribution: { points: 1, max: 10 },
    broadcasts: { points: 2, max: 40 },
    noPurchase: { points: 3, max: 10 },
    purchase: { points: 4, max: 15 },
    sunset: { points: 5, max: 10 },
    welcome: { points: 6, max: 15 },
  };
  const id = (k) => k;
  assert.equal(scoreTip({ total: 21, parts: lexi }, id), [
    'dashboard.usage.partBroadcasts 2/40', 'dashboard.usage.partWelcome 6/15', 'dashboard.usage.partPurchase 4/15',
    'dashboard.usage.partNoPurchase 3/10', 'dashboard.usage.partAttribution 1/10', 'dashboard.usage.partSunset 5/10',
  ].join(' · '));
  // The numbers are rendered verbatim -- the fork computes no weight (a stored 7/99 shows as 7/99).
  assert.equal(scoreTip({ total: 7, parts: { welcome: { points: 7, max: 99 } } }, id), 'dashboard.usage.partWelcome 7/99');
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
  // The numbers are the stored ones: Journeys from journeys.{running, idle}, the rest from counts.
  const u = usage({
    welcome: {
      state: 'idle', listId: 7, campaigns: [], counts: { green: 4, amber: 5 },
    },
    purchase: { state: 'idle', rows: [], counts: { green: 1, amber: 2 } },
  });
  assert.deepEqual(storedCounts(u, 'journeys'), { green: 2, amber: 1 });
  // The state is NOT consulted: idle with a stored green 4 renders 4 green (lib owns the predicate).
  assert.deepEqual(storedCounts(u, 'welcome'), { green: 4, amber: 5 });
  assert.deepEqual(storedCounts(u, 'purchase'), { green: 1, amber: 2 });
  assert.deepEqual(storedCounts(u, 'noPurchase'), { green: 0, amber: 0 });
  assert.equal(storedCounts(u, 'sunset'), null);
  // A doc written before V2 (no counts) renders '—', never a derived circle.
  const old = shapeRows([{ brand: 'o', usage: usage({ welcome: { state: 'active', listId: 7, campaigns: [] } }) }])[0];
  assert.deepEqual([cell(old, 'welcome', t).text, cell(old, 'welcome', t).circles], [MISSING, []]);
  assert.equal(storedCounts(usage({ journeys: { running: 1 } }), 'journeys'), null);
});

test('V2 I3: every circle carries a words tooltip; red is the digit 0', () => {
  assert.equal(circleWords({ green: 1, amber: 2 }, t), 'dashboard.usage.circleRunning{"n":1}, dashboard.usage.circleIdle{"n":2}');
  assert.equal(circleWords({ green: 0, amber: 0 }, t), 'dashboard.usage.circleNone');
  const [ruze] = shapeRows(api);
  const j = cell(ruze, 'journeys', t);
  assert.deepEqual(j.circles, [{ color: 'green', n: 2 }, { color: 'amber', n: 1 }]);
  assert.equal(j.tooltip, 'dashboard.usage.circleRunning{"n":2}, dashboard.usage.circleIdle{"n":1}');
  // Welcome: the words, then the stored campaigns.
  assert.equal(cell(ruze, 'welcome', t).tooltip, 'dashboard.usage.circleRunning{"n":1}\n#3 running');
  const p = cell(ruze, 'purchase', t);
  assert.deepEqual(p.circles, [{ color: 'red', n: 0 }]);
  assert.equal(p.tooltip, 'dashboard.usage.circleNone');
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
      purchase: {
        state: 'idle',
        rows: [{ slug: 'x-first-order', listId: 4, evergreen: 'running' }, { slug: 'x-skus', listId: 5, evergreen: 'idle' }],
        counts: { green: 1, amber: 1 },
      },
    }),
  }])[0];
  assert.equal(
    cell(row, 'purchase', t).tooltip,
    'dashboard.usage.circleRunning{"n":1}, dashboard.usage.circleIdle{"n":1}\n'
      + 'x-first-order: dashboard.usage.listEvergreen{"list":4,"state":"running"}\n'
      + 'x-skus: dashboard.usage.listEvergreen{"list":5,"state":"idle"}',
  );
});

test('V2 D4: links per cell -- Welcome its master list, a trigger family only with exactly one row; Automations scope', () => {
  const [ruze] = shapeRows(api);
  assert.deepEqual(cellLink(ruze, 'welcome'), { list_id: '7', evergreen: 'true' });
  assert.equal(cellLink(ruze, 'purchase'), null); // no rows
  assert.equal(cellLink(ruze, 'sunset'), null);
  assert.equal(cellLink(ruze, 'attribution'), null);
  assert.equal(cellLink(ruze, 'score'), null);
  const one = shapeRows([{
    brand: 'x',
    usage: usage({
      noPurchase: { state: 'active', rows: [{ slug: 'x-no-purchase-12', listId: 12, evergreen: 'running' }], counts: { green: 1, amber: 0 } },
      purchase: {
        state: 'idle',
        rows: [{ slug: 'a', listId: 4, evergreen: 'running' }, { slug: 'b', listId: 5, evergreen: 'idle' }],
        counts: { green: 1, amber: 1 },
      },
    }),
  }])[0];
  // (The plain 'noPurchase' column's link retired with the 3d/7d/15d leaves — POLISH3 stage 4 F2;
  // the leaf link rule is pinned in the D4 tests.)
  assert.equal(cellLink(one, 'noPurchase'), null);
  assert.equal(cellLink(one, 'purchase'), null); // several rows = several lists
  const noList = shapeRows([{
    brand: 'y',
    usage: usage({ purchase: { state: 'idle', rows: [{ slug: 'a', listId: null, evergreen: 'none' }], counts: { green: 0, amber: 1 } } }),
  }])[0];
  assert.equal(cellLink(noList, 'purchase'), null);
  // A ZMA row's dimmed Welcome does not link; no usage, no link.
  assert.equal(cellLink(shapeRows(api).find((r) => r.brand === 'oldzma'), 'welcome'), null);
  assert.equal(cellLink(shapeRows(api).find((r) => r.brand === 'nodoc'), 'welcome'), null);
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
test('I3: broadcast cells link the brand set in the Broadcasts scope; Journeys the brand set in Automations', () => {
  const [ruze, tg, lux] = shapeRows(api, { lists });
  const set = ['3', '7', '12'];
  const bcast = { evergreen: 'false', status: 'finished', list_id: set };
  assert.deepEqual(cellLink(ruze, 'cadence'), bcast);
  assert.deepEqual(cellLink(ruze, 'lastBroadcast'), bcast);
  assert.deepEqual(cellLink(ruze, 'journeys'), { evergreen: 'true', list_id: set });
  // A one-list brand links its one list (the Campaigns page reads it as the single filter).
  assert.equal(cell(tg, 'lastBroadcast', t).text, 'dashboard.usage.never'); // "never" is shown and links
  assert.deepEqual(cellLink(tg, 'lastBroadcast'), { evergreen: 'false', status: 'finished', list_id: ['20'] });
  assert.equal(cell(tg, 'cadence', t).text, MISSING); // a '—' cadence does not
  assert.equal(cellLink(tg, 'cadence'), null);
  assert.equal(cell(lux, 'cadence', t).text, MISSING);
  assert.equal(cellLink(lux, 'cadence'), null);
  assert.deepEqual(cellLink(lux, 'lastBroadcast'), { evergreen: 'false', status: 'finished', list_id: ['21'] });
  assert.deepEqual(cellLink(lux, 'journeys'), { evergreen: 'true', list_id: ['21'] }); // journeys: no status
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
  assert.deepEqual(cellLink(noMaster, 'journeys'), { evergreen: 'true', list_id: set });
  // An EMPTY set -> no link on any of the three (the brand has no active list in the store).
  const noLists = shapeRows([{ brand: 'nolists', usage: usage() }], { lists })[0];
  ['cadence', 'lastBroadcast', 'journeys'].forEach((f) => assert.equal(cellLink(noLists, f), null, f));
  const unloaded = shapeRows(api)[0]; // ruze, shaped before the lists store loaded
  ['cadence', 'lastBroadcast', 'journeys'].forEach((f) => assert.equal(cellLink(unloaded, f), null, f));
  // ZMA-dimmed cells ('—') never link, even with lists.
  const zma = shapeRows(api, { lists }).find((r) => r.brand === 'oldzma');
  ['cadence', 'lastBroadcast', 'journeys'].forEach((f) => {
    assert.equal(cell(zma, f, t).text, MISSING, f);
    assert.equal(cellLink(zma, f), null, f);
  });
  // Journeys without stored counts ('—', a pre-V2 doc) does not link; no usage, no link.
  const old = shapeRows([{ brand: 'ruze', usage: usage({ journeys: { running: 1 } }) }], { lists })[0];
  assert.equal(cell(old, 'journeys', t).text, MISSING);
  assert.equal(cellLink(old, 'journeys'), null);
  const nodoc = shapeRows([{ brand: 'ruze' }], { lists })[0];
  ['cadence', 'lastBroadcast', 'journeys'].forEach((f) => assert.equal(cellLink(nodoc, f), null, f));
  // The Welcome / trigger links are unchanged (Automations, their own single list).
  assert.deepEqual(cellLink(ruze, 'welcome'), { list_id: '7', evergreen: 'true' });
  assert.equal(cellLink(ruze, 'purchase'), null);
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
  assert.equal(linkTip(ruze, 'welcome', t), '');
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
  ['score', 'welcome', 'journeys', 'lastBroadcast', 'cadence'].forEach((f) => {
    const c = cell(zma, f, t);
    assert.equal(c.text, MISSING, f);
    assert.notEqual(c.cls, 'has-text-danger', f);
    assert.equal(c.cls, '', f);
    assert.deepEqual(c.circles, [], f);
  });
  assert.deepEqual(cell(zma, 'purchase', t).circles, [{ color: 'amber', n: 1 }]);
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
  assert.equal(sortValue(rows[0], 'score'), 65);
  assert.equal(sortValue(rows[3], 'score'), null);
  // An unknown sort field falls back to the default.
  assert.deepEqual(sortRows(rows, { field: 'bogus', order: 'desc' }).map((r) => r.brand), sortRows(rows).map((r) => r.brand));
});

test('I7: the v1 comparators stand -- Last broadcast worst first; states and booleans by badness', () => {
  const rows = shapeRows(api);
  assert.deepEqual(sortRows(rows, { field: 'lastBroadcast', order: 'desc' }).map((r) => r.brand), ['thirstygirl', 'lux', 'ruze', 'nodoc', 'oldzma']);
  assert.deepEqual(sortRows(rows, { field: 'lastBroadcast', order: 'asc' }).map((r) => r.brand), ['ruze', 'lux', 'thirstygirl', 'nodoc', 'oldzma']);
  assert.deepEqual(sortRows(rows, { field: 'brand', order: 'asc' }).map((r) => r.brand), ['lux', 'nodoc', 'oldzma', 'ruze', 'thirstygirl']);
  assert.equal(sortValue(rows[0], 'welcome'), 0);
  assert.equal(sortValue(rows[3], 'purchase'), 1);
  assert.equal(sortValue(rows[3], 'welcome'), null);
  assert.equal(sortValue(rows[0], 'attribution'), 1);
  assert.equal(sortValue(rows[0], 'sunset'), 0);
  assert.equal(sortValue(rows[0], 'journeys'), 2);
});

test('V2 D2 / I2: header clicks (clickSort) -- first click Score desc, Brand asc, every other desc; repeat flips', () => {
  assert.equal(defaultOrder('brand'), 'asc');
  assert.equal(defaultOrder('score'), 'desc');
  const start = { field: 'lastBroadcast', order: 'desc' };
  assert.deepEqual(clickSort(start, 'brand'), { field: 'brand', order: 'asc' });
  assert.deepEqual(clickSort(start, 'score'), { field: 'score', order: 'desc' });
  ['welcome', 'purchase', 'noPurchase:3', 'noPurchase:15', 'skipped:fr', 'sunset', 'attribution', 'journeys', 'cadence'].forEach((f) => {
    assert.deepEqual(clickSort(start, f), { field: f, order: 'desc' }, f);
  });
  assert.deepEqual(clickSort({ field: 'brand', order: 'asc' }, 'lastBroadcast'), { field: 'lastBroadcast', order: 'desc' });
  // A repeat click toggles.
  assert.deepEqual(clickSort(start, 'lastBroadcast'), { field: 'lastBroadcast', order: 'asc' });
  assert.deepEqual(clickSort({ field: 'score', order: 'desc' }, 'score'), { field: 'score', order: 'asc' });
  assert.deepEqual(clickSort({ field: 'brand', order: 'asc' }, 'brand'), { field: 'brand', order: 'desc' });
  // An unsortable field leaves the sort unchanged; no current sort is the default.
  assert.deepEqual(clickSort(start, 'bogus'), start);
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
  // POLISH3 D4: a session sorted by the retired single 'noPurchase' column falls back to the default.
  assert.deepEqual(loadState({ getItem: () => JSON.stringify({ sort: { field: 'noPurchase', order: 'desc' } }) }), { sort: { ...DEFAULT_STATE.sort } });
});

// ---------------------------------------------------------------------------------------------
// DASHBOARDS-POLISH3-SPEC I4 -- the No purchase 3d | 7d | 15d mapping
// ---------------------------------------------------------------------------------------------

const npStep = (days, listId, evergreen, error) => ({
  slug: `x-no-purchase-${listId}`, listId, evergreen, days, ...(error ? { error } : {}),
});
const npRow = (rows, brand = 'x') => shapeRows([{
  brand,
  usage: usage({ noPurchase: { state: 'idle', rows, counts: { green: 0, amber: rows.length } } }),
}])[0];

test('POLISH3 I4: each leaf maps the step at EXACTLY that day -- green running, amber idle, red 0 absent; off-standard not shown', () => {
  const r = npRow([npStep(15, 40, 'running'), npStep(7, 41, 'idle'), npStep(5, 42, 'running')]);
  assert.deepEqual(cell(r, 'noPurchase:3', t).circles, [{ color: 'red', n: 0 }]);
  assert.deepEqual(cell(r, 'noPurchase:7', t).circles, [{ color: 'amber', n: 1 }]);
  assert.deepEqual(cell(r, 'noPurchase:15', t).circles, [{ color: 'green', n: 1 }]);
  // The 5-day step appears in no leaf (deliberately unprovisioned), and is no badge.
  NO_PURCHASE_LEAVES.forEach((f) => assert.ok(!cell(r, f, t).tooltip.includes('x-no-purchase-42'), f));
  assert.deepEqual(cellBadges(r, 'noPurchase:15', t), []);
  // The tooltip: the words, then the step's row.
  assert.equal(
    cell(r, 'noPurchase:15', t).tooltip,
    'dashboard.usage.circleRunning{"n":1}\nx-no-purchase-40: dashboard.usage.listEvergreen{"list":40,"state":"running"}',
  );
  assert.equal(cell(r, 'noPurchase:3', t).tooltip, 'dashboard.usage.circleNone');
  // An invalid row WITH a parseable day sits in its leaf as amber, its error in the tooltip.
  const bad = npRow([npStep(3, 43, 'running', 'sinceAt "x" is not an ISO timestamp')]);
  assert.deepEqual(cell(bad, 'noPurchase:3', t).circles, [{ color: 'amber', n: 1 }]);
  assert.match(cell(bad, 'noPurchase:3', t).tooltip, /sinceAt "x" is not an ISO timestamp/);
  // A step without a list is amber.
  assert.deepEqual(cell(npRow([npStep(7, null, 'none')]), 'noPurchase:7', t).circles, [{ color: 'amber', n: 1 }]);
  // The Ruze demo shape (G4): only a 15d step running -> red 0 | red 0 | green 1.
  const ruze = npRow([npStep(15, 50, 'running')], 'ruze');
  assert.deepEqual(NO_PURCHASE_LEAVES.map((f) => cell(ruze, f, t).circles), [
    [{ color: 'red', n: 0 }], [{ color: 'red', n: 0 }], [{ color: 'green', n: 1 }],
  ]);
});

test('POLISH3 I4 (stage 2 F5): a no-days row is an amber ⚠ badge with its parser error; a pre-days doc is — throughout', () => {
  const r = npRow([npStep(7, 41, 'running'), npStep(null, 44, 'none', 'days "x" is not an integer ≥ 1'), npStep(null, 45, 'none', 'days "0" is not an integer ≥ 1')]);
  assert.deepEqual(cellBadges(r, 'noPurchase:15', t), [
    { glyph: '⚠', tooltip: 'dashboard.usage.npInvalid{"slug":"x-no-purchase-44","error":"days \\"x\\" is not an integer ≥ 1"}' },
    { glyph: '⚠', tooltip: 'dashboard.usage.npInvalid{"slug":"x-no-purchase-45","error":"days \\"0\\" is not an integer ≥ 1"}' },
  ]);
  // Only after the sub-group's LAST leaf; nowhere else.
  ['noPurchase:3', 'noPurchase:7', 'welcome', 'skipped:fr'].forEach((f) => assert.deepEqual(cellBadges(r, f, t), [], f));
  assert.deepEqual(cell(r, 'noPurchase:7', t).circles, [{ color: 'green', n: 1 }]);
  // A pre-`days` doc (valid rows with no days key) -> the whole sub-group '—', never red 0.
  const pre = shapeRows([{
    brand: 'p',
    usage: usage({ noPurchase: { state: 'active', rows: [{ slug: 'p-no-purchase-9', listId: 9, evergreen: 'running' }], counts: { green: 1, amber: 0 } } }),
  }], { lists })[0];
  NO_PURCHASE_LEAVES.forEach((f) => {
    const c = cell(pre, f, t);
    assert.equal(c.text, MISSING, f);
    assert.deepEqual(c.circles, [], f);
    assert.match(c.tooltip, /^dashboard\.usage\.countsMissing/, f);
    assert.equal(cellLink(pre, f), null, f);
    assert.equal(sortValue(pre, f), null, f);
  });
  assert.deepEqual(cellBadges(pre, 'noPurchase:15', t), []);
  // A doc without the family at all is '—' too.
  const none = shapeRows([{ brand: 'n', usage: usage({ noPurchase: undefined }) }])[0];
  assert.equal(cell(none, 'noPurchase:3', t).text, MISSING);
});

test('POLISH3 I4: a leaf links its step list only when exactly one step sits at that day', () => {
  const r = npRow([npStep(15, 40, 'running'), npStep(7, 41, 'idle'), npStep(7, 46, 'running')]);
  assert.deepEqual(cellLink(r, 'noPurchase:15'), { list_id: '40', evergreen: 'true' });
  assert.equal(cellLink(r, 'noPurchase:7'), null); // two steps at 7d
  assert.deepEqual(cell(r, 'noPurchase:7', t).circles, [{ color: 'green', n: 1 }, { color: 'amber', n: 1 }]);
  assert.equal(cellLink(r, 'noPurchase:3'), null); // absent
  assert.equal(cellLink(npRow([npStep(3, null, 'none')]), 'noPurchase:3'), null); // no list
  assert.equal(cellLink(shapeRows(api).find((x) => x.brand === 'nodoc'), 'noPurchase:3'), null);
});

test('POLISH3 I4: the leaves sort by badness -- absent worst, then amber, then green; — last', () => {
  const rows = [
    npRow([npStep(3, 40, 'running')], 'green'),
    npRow([npStep(3, 41, 'idle')], 'amber'),
    npRow([], 'absent'),
    shapeRows([{ brand: 'pre', usage: usage({ noPurchase: { state: 'idle', rows: [{ slug: 's', listId: 1, evergreen: 'idle' }], counts: { green: 0, amber: 1 } } }) }])[0],
  ];
  assert.deepEqual(rows.map((r) => sortValue(r, 'noPurchase:3')), [0, 1, 2, null]);
  assert.deepEqual(sortRows(rows, { field: 'noPurchase:3', order: 'desc' }).map((r) => r.brand), ['absent', 'amber', 'green', 'pre']);
  assert.deepEqual(sortRows(rows, { field: 'noPurchase:3', order: 'asc' }).map((r) => r.brand), ['green', 'amber', 'absent', 'pre']);
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
  // 0 -> the green ✓ (G5 feedback, replacing the plain '0'), unlinked (covered or no activity --
  // the tooltip owns the wording).
  assert.deepEqual([cell(ruze, 'skipped:de', t).text, cell(ruze, 'skipped:de', t).cls], ['✓', 'has-text-success']);
  assert.equal(sortValue(ruze, 'skipped:de'), 0); // the sort key stays the count
  assert.equal(cell(ruze, 'skipped:de', t).tooltip, 'dashboard.usage.skippedZero');
  assert.equal(cellLink(ruze, 'skipped:de'), null);
  assert.equal(linkTip(ruze, 'skipped:de', t), '');
  // No usage: the Skipped counts still render (and link); the other cells stay blank.
  assert.equal(nousage.usage, null);
  assert.deepEqual([cell(nousage, 'skipped:de', t).text, cell(nousage, 'skipped:de', t).cls], ['5', 'has-text-danger']);
  assert.deepEqual(cellLink(nousage, 'skipped:de'), { list_id: ['21'], evergreen: 'all' });
  assert.equal(cell(nousage, 'skipped:fr', t).text, '✓'); // G5: was '0'
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
