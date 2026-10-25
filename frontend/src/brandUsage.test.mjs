// Fork (brand usage, integrations listmonk/BRAND-USAGE-SPEC I6/I7, D2/D7). Run: yarn test
// (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  DEFAULT_STATE, MISSING, STORAGE_KEY, boolCell, broadcastTip, cell, loadState, saveState, shapeRows, sortRows,
  defaultOrder, latestAsOf, onSortClick, sortValue, stateCell, triggerTip,
} from './brandUsage.mjs'; // eslint-disable-line import/extensions
import { PANES, initialPane, sessionStore } from './brandMatrix.mjs'; // eslint-disable-line import/extensions

const t = (k, p) => (p ? `${k}${JSON.stringify(p)}` : k);

const usage = (over = {}) => ({
  channel: 'listmonk',
  welcome: { state: 'active', listId: 7, campaigns: [{ id: 3, status: 'running' }] },
  purchase: { state: 'none', rows: [] },
  noPurchase: { state: 'none', rows: [] },
  sunset: true,
  attribution: false,
  journeys: { running: 2, idle: 1 },
  broadcasts: {
    lastAt: '2026-10-20T12:00:00.000Z', windowCount: 4, cadenceDays: 12.4, daysSince: 4.6, ratio: 0.37, state: 'ok',
  },
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
    }),
  },
  {
    brand: 'lux',
    health: { status: 'ok' },
    usage: usage({
      broadcasts: {
        lastAt: '2026-08-01T00:00:00.000Z', windowCount: 1, cadenceDays: null, daysSince: 84.6, ratio: 2.82, state: 'stale',
      },
    }),
  },
  {
    brand: 'oldzma',
    health: { status: 'ok' },
    usage: usage({
      channel: 'zma',
      welcome: { state: 'none', listId: 9, campaigns: [] },
      purchase: { state: 'idle', rows: [{ slug: 'oldzma-first-order', listId: null, evergreen: 'none' }] },
      journeys: { running: 0, idle: 0 },
      broadcasts: {
        lastAt: null, windowCount: 0, cadenceDays: null, daysSince: null, ratio: null, state: 'none',
      },
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

test('I7: glyph mapping -- ✓ green active/running, ⚠ amber idle, ✗ red none; booleans ✓/✗; unknown blank', () => {
  assert.deepEqual(stateCell('active'), { glyph: '✓', cls: 'has-text-success' });
  assert.deepEqual(stateCell('running'), { glyph: '✓', cls: 'has-text-success' });
  assert.deepEqual(stateCell('idle'), { glyph: '⚠', cls: 'has-text-warning-dark' });
  assert.deepEqual(stateCell('none'), { glyph: '✗', cls: 'has-text-danger' });
  assert.deepEqual(stateCell('bogus'), { glyph: '', cls: '' });
  assert.deepEqual(boolCell(true), { glyph: '✓', cls: 'has-text-success' });
  assert.deepEqual(boolCell(false), { glyph: '✗', cls: 'has-text-danger' });
  assert.deepEqual(boolCell(undefined), { glyph: '', cls: '' });
});

test('D7: cells -- journeys n running (idle in tooltip), Nd ago, never, ~Nd, —', () => {
  const [ruze, tg, lux] = shapeRows(api);
  assert.equal(cell(ruze, 'journeys', t).text, 'dashboard.usage.running{"n":2}');
  assert.equal(cell(ruze, 'journeys', t).tooltip, 'dashboard.usage.idleCount{"n":1}');
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
  assert.equal(cell(ruze, 'welcome', t).tooltip, '#3 running');
});

test('D7: the ⚠ tooltip lists each row with its error or its not-running list', () => {
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
});

test('I6: a ZMA row never renders a red broadcast or welcome cell; registry facts still render', () => {
  const zma = shapeRows(api).find((r) => r.brand === 'oldzma');
  ['welcome', 'journeys', 'lastBroadcast', 'cadence'].forEach((f) => {
    const c = cell(zma, f, t);
    assert.equal(c.text, MISSING, f);
    assert.notEqual(c.cls, 'has-text-danger', f);
    assert.equal(c.cls, '', f);
  });
  assert.equal(cell(zma, 'purchase', t).text, '⚠');
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

test('D7: a row without usage renders blanks', () => {
  const nodoc = shapeRows(api).find((r) => r.brand === 'nodoc');
  ['welcome', 'purchase', 'noPurchase', 'sunset', 'attribution', 'journeys', 'lastBroadcast', 'cadence'].forEach((f) => {
    assert.deepEqual(cell(nodoc, f, t), { text: '', cls: '', tooltip: '' }, f);
  });
});

test('I7: default sort -- Last broadcast worst first: never, then daysSince descending; missing last', () => {
  assert.deepEqual(DEFAULT_STATE.sort, { field: 'lastBroadcast', order: 'desc' });
  const rows = shapeRows(api);
  // oldzma is ZMA (dimmed: missing) and nodoc has no usage -- both last, by name.
  assert.deepEqual(sortRows(rows).map((r) => r.brand), ['thirstygirl', 'lux', 'ruze', 'nodoc', 'oldzma']);
  // Ascending flips the numbers but keeps missing last.
  assert.deepEqual(sortRows(rows, { field: 'lastBroadcast', order: 'asc' }).map((r) => r.brand), ['ruze', 'lux', 'thirstygirl', 'nodoc', 'oldzma']);
  assert.deepEqual(sortRows(rows, { field: 'brand', order: 'asc' }).map((r) => r.brand), ['lux', 'nodoc', 'oldzma', 'ruze', 'thirstygirl']);
  // An unknown sort field falls back to the default.
  assert.deepEqual(sortRows(rows, { field: 'bogus', order: 'desc' }).map((r) => r.brand), sortRows(rows).map((r) => r.brand));
  // Every column sorts; states by badness (desc = worst first).
  assert.equal(sortValue(rows[0], 'welcome'), 0);
  assert.equal(sortValue(rows[3], 'purchase'), 1);
  assert.equal(sortValue(rows[3], 'welcome'), null);
});

test('D7 / stage 4 F1: header clicks -- first click on Brand asc, on any other column desc; repeat flips', () => {
  assert.equal(defaultOrder('brand'), 'asc');
  const start = { field: 'lastBroadcast', order: 'desc' };
  // Buefy emits 'asc' on a click that changes the column.
  assert.deepEqual(onSortClick(start, 'brand', 'asc'), { sort: { field: 'brand', order: 'asc' }, flipArrow: false });
  ['welcome', 'purchase', 'noPurchase', 'sunset', 'attribution', 'journeys', 'cadence'].forEach((f) => {
    assert.deepEqual(onSortClick(start, f, 'asc'), { sort: { field: f, order: 'desc' }, flipArrow: true }, f);
  });
  const brandStart = { field: 'brand', order: 'asc' };
  assert.deepEqual(onSortClick(brandStart, 'lastBroadcast', 'asc'), { sort: { field: 'lastBroadcast', order: 'desc' }, flipArrow: true });
  // A repeat click is Buefy's toggle, passed through.
  assert.deepEqual(onSortClick(start, 'lastBroadcast', 'asc'), { sort: { field: 'lastBroadcast', order: 'asc' }, flipArrow: false });
  assert.deepEqual(onSortClick({ field: 'lastBroadcast', order: 'asc' }, 'lastBroadcast', 'desc'), { sort: { field: 'lastBroadcast', order: 'desc' }, flipArrow: false });
  assert.deepEqual(onSortClick(brandStart, 'brand', 'desc'), { sort: { field: 'brand', order: 'desc' }, flipArrow: false });
  // An unsortable field leaves the sort unchanged.
  assert.deepEqual(onSortClick(start, 'bogus', 'asc'), { sort: start, flipArrow: false });
});

test('I7: session persistence -- a throwing sessionStorage yields the defaults and still renders', () => {
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
  assert.equal(sortRows(shapeRows(api), loadState(store).sort)[0].brand, 'thirstygirl');

  // A working store round-trips under the pane's own key.
  const mem = new Map();
  const ok = { getItem: (k) => (mem.has(k) ? mem.get(k) : null), setItem: (k, v) => mem.set(k, v) };
  assert.equal(saveState(ok, { sort: { field: 'cadence', order: 'asc' } }), true);
  assert.ok(mem.has(STORAGE_KEY));
  assert.equal(STORAGE_KEY, 'dashboard.usage');
  assert.deepEqual(loadState(ok), { sort: { field: 'cadence', order: 'asc' } });
  assert.deepEqual(loadState({ getItem: () => JSON.stringify({ sort: { field: 'nope', order: 'up' } }) }), { sort: { ...DEFAULT_STATE.sort } });
});
