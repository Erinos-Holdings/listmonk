// Fork (brand usage, integrations listmonk/BRAND-USAGE-SPEC I6/I7, D2/D7; BRAND-USAGE-V2-SPEC
// I1-I4, I7). Run: yarn test (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  CIRCLE_FIELDS, DEFAULT_STATE, MISSING, SCORE_PARTS, SECTIONS, SORT_FIELDS, STORAGE_KEY, boolCell, broadcastTip, cell,
  cellLink, circlesOf, circleWords, clickSort, defaultOrder, latestAsOf, loadState, mountainRunTime, saveState,
  scoreTip, shapeRows, sortRows, sortValue, storedCounts, triggerTip, linkTip,
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

test('D2: the pane pill sits between matrix and ses and the sticky pref keeps it', () => {
  assert.deepEqual(PANES, ['overview', 'matrix', 'usage', 'ses']);
  assert.equal(initialPane(true, 'usage', false), 'usage');
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

test('V2 D2: columns -- Score, Brand, then Broadcasts / Journeys / Configurations sections', () => {
  assert.deepEqual(SECTIONS.map((s) => [s.key, s.fields]), [
    ['broadcasts', ['cadence', 'lastBroadcast']],
    ['journeys', ['journeys', 'welcome', 'purchase', 'noPurchase']],
    ['configurations', ['attribution', 'sunset']],
  ]);
  assert.deepEqual(SORT_FIELDS, ['score', 'brand', 'cadence', 'lastBroadcast', 'journeys', 'welcome', 'purchase', 'noPurchase', 'attribution', 'sunset']);
  assert.deepEqual(CIRCLE_FIELDS, ['journeys', 'welcome', 'purchase', 'noPurchase']);
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
  assert.deepEqual(cellLink(one, 'noPurchase'), { list_id: '12', evergreen: 'true' });
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

// DASHBOARDS-POLISH-SPEC I4: the broadcast cells and the Journeys circles.
test('I4: broadcast cells link the MASTER list in the Broadcasts scope; Journeys the Automations scope, no list', () => {
  const [ruze, tg, lux] = shapeRows(api);
  const bcast = { list_id: '7', evergreen: 'false' };
  assert.deepEqual(cellLink(ruze, 'cadence'), bcast);
  assert.deepEqual(cellLink(ruze, 'lastBroadcast'), bcast);
  assert.deepEqual(cellLink(ruze, 'journeys'), { evergreen: 'true' });
  assert.equal(Object.prototype.hasOwnProperty.call(cellLink(ruze, 'journeys'), 'list_id'), false);
  // "never" is a shown value and links; a '—' cadence does not.
  assert.equal(cell(tg, 'lastBroadcast', t).text, 'dashboard.usage.never');
  assert.deepEqual(cellLink(tg, 'lastBroadcast'), bcast);
  assert.equal(cell(tg, 'cadence', t).text, MISSING);
  assert.equal(cellLink(tg, 'cadence'), null);
  assert.equal(cell(lux, 'cadence', t).text, MISSING);
  assert.equal(cellLink(lux, 'cadence'), null);
  assert.deepEqual(cellLink(lux, 'lastBroadcast'), bcast);
  // A blank Last broadcast (no stored daysSince, not "never") does not link.
  const blank = shapeRows([{
    brand: 'b',
    usage: usage({
      broadcasts: {
        lastAt: null, windowCount: 0, cadenceDays: 5, daysSince: null, ratio: null, state: 'ok',
      },
    }),
  }])[0];
  assert.equal(cell(blank, 'lastBroadcast', t).text, '');
  assert.equal(cellLink(blank, 'lastBroadcast'), null);
  assert.deepEqual(cellLink(blank, 'cadence'), { list_id: '7', evergreen: 'false' });
  // No master list -> the broadcast cells do not link (Journeys still does).
  const noMaster = shapeRows([{
    brand: 'n',
    usage: usage({
      welcome: {
        state: 'none', listId: null, campaigns: [], counts: { green: 0, amber: 0 },
      },
    }),
  }])[0];
  assert.equal(cellLink(noMaster, 'cadence'), null);
  assert.equal(cellLink(noMaster, 'lastBroadcast'), null);
  assert.deepEqual(cellLink(noMaster, 'journeys'), { evergreen: 'true' });
  // ZMA-dimmed cells ('—') never link.
  const zma = shapeRows(api).find((r) => r.brand === 'oldzma');
  ['cadence', 'lastBroadcast', 'journeys'].forEach((f) => {
    assert.equal(cell(zma, f, t).text, MISSING, f);
    assert.equal(cellLink(zma, f), null, f);
  });
  // Journeys without stored counts ('—', a pre-V2 doc) does not link; no usage, no link.
  const old = shapeRows([{ brand: 'o', usage: usage({ journeys: { running: 1 } }) }])[0];
  assert.equal(cell(old, 'journeys', t).text, MISSING);
  assert.equal(cellLink(old, 'journeys'), null);
  const nodoc = shapeRows(api).find((r) => r.brand === 'nodoc');
  ['cadence', 'lastBroadcast', 'journeys'].forEach((f) => assert.equal(cellLink(nodoc, f), null, f));
  // The Welcome / trigger links are unchanged (Automations, their own list).
  assert.deepEqual(cellLink(ruze, 'welcome'), { list_id: '7', evergreen: 'true' });
});

test('I4: a linked broadcast cell carries the master-list note; nothing else does', () => {
  const [ruze, tg] = shapeRows(api);
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

test('D7: a row without usage renders blanks', () => {
  const nodoc = shapeRows(api).find((r) => r.brand === 'nodoc');
  SORT_FIELDS.filter((f) => f !== 'brand').forEach((f) => {
    assert.deepEqual(cell(nodoc, f, t), {
      text: '', cls: '', tooltip: '', circles: [],
    }, f);
  });
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
  ['welcome', 'purchase', 'noPurchase', 'sunset', 'attribution', 'journeys', 'cadence'].forEach((f) => {
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
});
