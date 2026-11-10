// Fork (brand usage, integrations listmonk/BRAND-USAGE-SPEC I6/I7, D2/D7; BRAND-USAGE-V2-SPEC
// I1-I4, I7). Run: yarn test (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  CIRCLE_FIELDS, DEFAULT_STATE, MISSING, SCORE_PARTS, SECTIONS, SORT_FIELDS, SERIES_DAYS, STORAGE_KEY,
  boolCell, broadcastTip, cell, cellBadges, cellLink, circlesOf, circleWords, clickSort, columnMeta, defaultOrder,
  computedAtText, headerLayout, loadState, WELCOME_FIELDS, POST_PURCHASE_FIELDS, seriesLeafOf, saveState, scoreTip, shapeRows,
  skippedLangOf, sortRows, sortValue, storedCounts, triggerTip, linkTip, brandListIds,
  GUIDE_COLUMN_REM, GUIDE_MIN_COLUMN, GUIDE_SERIES, guideColumnsFit, linePad, pageWidth, parseStepText, seriesGuide, bodyColumns, circleLink, circleName,
  REENGAGEMENT_DAYS, REENGAGEMENT_FIELDS, reengagementLeafOf, reCircleName, approxClock, hoverText, splitStepTitle, stepDetail,
} from './brandUsage.mjs'; // eslint-disable-line import/extensions
import { seriesStepKey } from './listTrigger.mjs'; // eslint-disable-line import/extensions
import { PANES, initialPane, sessionStore } from './brandMatrix.mjs'; // eslint-disable-line import/extensions

const t = (k, p) => (p ? `${k}${JSON.stringify(p)}` : k);

// LIFECYCLE-SERIES-SPEC S9: the parts (lib/brand-usage.ts SCORE_WEIGHTS) -- broadcasts 40, the two
// series 20 each (4 leaves x active 5), attribution 10, sunset 10. (Historical fixture values --
// the fork renders parts verbatim, so the live maxima, e.g. welcome 15 since USAGE-DAY10-SPEC D3,
// are not pinned here.)
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
// Day 0 | 2d | 5d (10d retired, USAGE-DAY10-SPEC) and the Post-purchase series group over Day 0 | 5d | 14d | 30d (the
// journeys count column retired 2026-10-30 -- the Score sorts by usage).
// The Languages section (Skipped over FR ES DE IT) still sits between Journeys and Configurations.
test('S9 / POLISH3 D5: columns -- Score, Brand, then Broadcasts / Journeys (two series groups) / Languages / Configurations', () => {
  // USAGE-DAY10-SPEC I5: the No-purchase series is Day 0 | 2d | 5d (day 10 retired).
  assert.deepEqual(SERIES_DAYS, { welcome: [0, 2, 5], postPurchase: [0, 5, 14, 30] });
  const W = ['welcome:0', 'welcome:2', 'welcome:5'];
  const P = ['postPurchase:0', 'postPurchase:5', 'postPurchase:14', 'postPurchase:30'];
  assert.deepEqual(WELCOME_FIELDS, W);
  assert.deepEqual(POST_PURCHASE_FIELDS, P);
  // USAGE-REENGAGEMENT-SPEC F1 / SUNSET-V4 D7: the Re-engagement group (180, 270, 360, 365) after
  // Post-purchase, before sku.
  const R = ['reengagement:180', 'reengagement:270', 'reengagement:360', 'reengagement:365'];
  assert.deepEqual(REENGAGEMENT_FIELDS, R);
  assert.deepEqual(SECTIONS.map((s) => [s.key, s.fields]), [
    ['broadcasts', ['cadence', 'lastBroadcast']],
    ['journeys', [...W, ...P, ...R, 'sku']],
    ['languages', ['skipped:fr', 'skipped:es', 'skipped:de', 'skipped:it']],
    ['configurations', ['attribution', 'sunset']],
  ]);
  assert.deepEqual(SORT_FIELDS, [
    'score', 'brand', 'cadence', 'lastBroadcast', ...W, ...P, ...R, 'sku',
    'skipped:fr', 'skipped:es', 'skipped:de', 'skipped:it', 'attribution', 'sunset',
  ]);
  assert.deepEqual(CIRCLE_FIELDS, [...W, ...P, ...R, 'sku']);
  // The header: plain leaves in row 2 (rowspan 2), the sub-group labels over their leaves in row 3.
  const h = headerLayout();
  assert.deepEqual(h.row2.map((c) => (c.kind === 'group' ? `[${c.key}:${c.span}]` : c.field)), [
    'cadence', 'lastBroadcast', '[welcome:3]', '[postPurchase:4]', '[reengagement:4]', 'sku', '[skipped:4]', 'attribution', 'sunset',
  ]);
  // G5 round 2: every group label takes the single-row headers' font size (small: false).
  assert.deepEqual(h.row2.filter((c) => c.kind === 'group').map((c) => [c.label, c.band, c.first, c.small]), [
    ['dashboard.usage.noPurchase', 'white', true, false], ['dashboard.usage.postPurchase', 'white', false, false],
    ['dashboard.usage.reengagement', 'white', false, false],
    ['dashboard.usage.skipped', 'grey', true, false],
  ]);
  assert.deepEqual(h.row3.map((c) => [c.field, c.first, c.groupStart]), [
    ['welcome:0', true, true], ['welcome:2', false, false], ['welcome:5', false, false],
    ['postPurchase:0', false, true], ['postPurchase:5', false, false], ['postPurchase:14', false, false], ['postPurchase:30', false, false],
    ['reengagement:180', false, true], ['reengagement:270', false, false], ['reengagement:360', false, false], ['reengagement:365', false, false],
    ['skipped:fr', true, true], ['skipped:es', false, false], ['skipped:de', false, false], ['skipped:it', false, false],
  ]);
  // 2026-10-31 polish 2: the line-pad sides, the same on a header cell and its column's body cells
  // -- the side opening a rule and the side facing the next column's rule; a group takes its first
  // leaf's left and its last leaf's right; the section row pads every section's left and all but
  // the last's right. SUNSET-V4 D7: no leaf carries an alignment (the hugged pair is gone) -- every
  // leaf is centred.
  const pads = (c) => `${c.padLeft ? 'L' : '-'}${c.padRight ? 'R' : '-'}`;
  assert.deepEqual(h.row1.map((c) => [c.key, c.span, c.band, pads(c)]), [
    ['broadcasts', 2, 'grey', 'LR'], ['journeys', 12, 'white', 'LR'], ['languages', 4, 'grey', 'LR'], ['configurations', 2, 'white', 'L-'],
  ]);
  assert.deepEqual(h.row2.map((c) => [c.kind === 'group' ? `[${c.key}]` : c.field, pads(c)]), [
    ['cadence', 'L-'], ['lastBroadcast', '-R'], ['[welcome]', 'LR'], ['[postPurchase]', 'LR'], ['[reengagement]', 'LR'], ['sku', 'LR'],
    ['[skipped]', 'LR'], ['attribution', 'L-'], ['sunset', '--'],
  ]);
  assert.deepEqual(h.row3.map((c) => [c.field, pads(c)]), [
    ['welcome:0', 'L-'], ['welcome:2', '--'], ['welcome:5', '-R'],
    ['postPurchase:0', 'L-'], ['postPurchase:5', '--'], ['postPurchase:14', '--'], ['postPurchase:30', '-R'],
    ['reengagement:180', 'L-'], ['reengagement:270', '--'], ['reengagement:360', '--'], ['reengagement:365', '-R'],
    ['skipped:fr', 'L-'], ['skipped:es', '--'], ['skipped:de', '--'], ['skipped:it', '-R'],
  ]);
  h.row3.forEach((c) => assert.equal('align' in c, false, c.field));
  const body = bodyColumns();
  const bodyLeaf = (f) => body.find((c) => c.field === f);
  [...h.row2.filter((c) => c.kind === 'leaf'), ...h.row3].forEach((c) => {
    const b = bodyLeaf(c.field);
    assert.deepEqual([b.padLeft, b.padRight], [!!c.padLeft, !!c.padRight], c.field);
    assert.equal('align' in b, false, c.field);
  });
  REENGAGEMENT_DAYS.forEach((n) => {
    assert.deepEqual(columnMeta(`reengagement:${n}`), { label: 'dashboard.usage.npDays', params: { n }, help: `series.reengagementDay${n}` });
  });
  // S9: each leaf's header hover is its D5 description.
  assert.deepEqual(columnMeta('welcome:5'), { label: 'dashboard.usage.npDays', params: { n: 5 }, help: 'series.welcomeDay5' });
  assert.deepEqual(columnMeta('welcome:0'), { label: 'dashboard.usage.day0', help: 'series.welcomeDay0' });
  assert.deepEqual(columnMeta('welcome:2'), { label: 'dashboard.usage.npDays', params: { n: 2 }, help: 'series.welcomeDay2' });
  assert.deepEqual(columnMeta('postPurchase:0'), { label: 'dashboard.usage.day0', help: 'series.postPurchaseDay0' });
  // LIFECYCLE-DOCS-SPEC D21 / I11: a Post-purchase step after day 0 reads "~Nd" and its hover adds
  // the real clock -- {n, offset}, plus the reorder sentence on day 30.
  const clock = (n, offset) => ({ key: 'dashboard.usage.approxClock', params: { n, offset } });
  assert.deepEqual(columnMeta('postPurchase:5'), {
    label: 'dashboard.usage.approxDays', params: { n: 5 }, help: 'series.postPurchaseDay5', helpExtra: [clock(5, 0)],
  });
  assert.deepEqual(columnMeta('postPurchase:14'), {
    label: 'dashboard.usage.approxDays', params: { n: 14 }, help: 'series.postPurchaseDay14', helpExtra: [clock(14, 9)],
  });
  assert.deepEqual(columnMeta('postPurchase:30'), {
    label: 'dashboard.usage.approxDays',
    params: { n: 30 },
    help: 'series.postPurchaseDay30',
    helpExtra: [clock(30, 25), { key: 'dashboard.usage.approxClockReorder', params: {} }],
  });
  // The hover: help, a newline, the clock sentence; every other leaf's hover is its help alone.
  assert.equal(hoverText(columnMeta('postPurchase:30'), t), 'series.postPurchaseDay30\ndashboard.usage.approxClock{"n":30,"offset":25} dashboard.usage.approxClockReorder{}');
  assert.equal(hoverText(columnMeta('postPurchase:0'), t), 'series.postPurchaseDay0');
  assert.equal(hoverText(columnMeta('welcome:5'), t), 'series.welcomeDay5');
  assert.equal(hoverText(columnMeta('reengagement:180'), t), 'series.reengagementDay180');
  assert.deepEqual(columnMeta('skipped:de'), { text: 'DE', help: 'dashboard.usage.skippedHelp' });
  assert.equal(seriesLeafOf('welcome:15'), null); // off-standard: no leaf (a legacy chip)
  assert.equal(seriesLeafOf('noPurchase:3'), null); // the retired leaves
  assert.equal(skippedLangOf('skipped:en'), null); // EN has no column (recorded limitation)
});

// LIFECYCLE-DOCS-SPEC D20 / I10 / I13: the ONE "~" rule -- null for every leaf but a Post-purchase
// step after day 0; the offset from delivery (ASSUMED_DELIVERY_DAYS 5) and the day-30 reorder skip.
test('LIFECYCLE-DOCS I10: approxClock -- Post-purchase steps after day 0 only', () => {
  assert.equal(approxClock('postPurchase', 0), null);
  [0, 2, 5].forEach((n) => assert.equal(approxClock('welcome', n), null, `welcome ${n}`));
  REENGAGEMENT_DAYS.forEach((n) => assert.equal(approxClock('reengagement', n), null, `reengagement ${n}`));
  assert.deepEqual(approxClock('postPurchase', 5), { offset: 0, reorder: false });
  assert.deepEqual(approxClock('postPurchase', 14), { offset: 9, reorder: false });
  assert.deepEqual(approxClock('postPurchase', 30), { offset: 25, reorder: true });
  // I13: the new keys, verbatim D23.
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.equal(en['dashboard.usage.approxDays'], '~{n}d');
  assert.equal(en['dashboard.usage.approxDay'], '~{n}');
  assert.equal(en['dashboard.usage.approxClock'], [
    'Sent at the first daily run (15:10 UTC — 8:10 am Mountain in winter, 9:10 am in summer) on or after delivery day + {offset}, as Aftership',
    ' reports the first order delivered. A tracked package still in transit waits up to 10 days after the order, then',
    ' — like an order with no tracking — sends at order date + {n} days.',
  ].join(''));
  assert.equal(en['dashboard.usage.approxClockReorder'], 'Skipped for anyone who has ordered again since their first order.');
  // D20: no literal "~" day label in the templates -- every one goes through approxClock.
  ['./components/BrandUsage.vue', './views/JourneyPreview.vue'].forEach((p) => {
    const vue = readFileSync(new URL(p, import.meta.url), 'utf8');
    const template = vue.slice(0, vue.indexOf('<script'));
    assert.ok(!/~\{\{|>~|'~|"~/.test(template), `${p} carries a literal ~ day label`);
    assert.ok(!vue.includes('v-html'), `${p} uses v-html`);
  });
});

// LIFECYCLE-SERIES-SPEC I16 -- every series leaf's hover resolves a D5 key present in en.json, and
// every label key the pane binds exists.
test('I16: every leaf help, group label and score part key is in en.json', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  [...WELCOME_FIELDS, ...POST_PURCHASE_FIELDS, ...REENGAGEMENT_FIELDS].forEach((f) => {
    const m = columnMeta(f);
    assert.equal(typeof en[m.help], 'string', `${f} help ${m.help}`);
    assert.equal(typeof en[m.label], 'string', `${f} label ${m.label}`);
    (m.helpExtra || []).forEach((p) => assert.equal(typeof en[p.key], 'string', `${f} helpExtra ${p.key}`));
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
  assert.deepEqual(SCORE_PARTS.map((p) => p.key), ['broadcasts', 'welcomeSeries', 'postPurchaseSeries', 'reengagement', 'attribution', 'sunset']);
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
  // A doc written before V2 (no counts) renders the "none" circle (a journey cell is always a
  // circle -- user 2026-11-02), with the countsMissing note; never a derived count.
  const old = shapeRows([{ brand: 'o', usage: usage({ welcome: { state: 'active', listId: 7, campaigns: [] } }) }])[0];
  assert.deepEqual([cell(old, 'welcome:0', t).text, cell(old, 'welcome:0', t).circles], ['', [{ color: 'red', n: 0 }]]);
  assert.match(cell(old, 'welcome:0', t).tooltip, /\ndashboard\.usage\.countsMissing$/);
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

// 2026-10-30: the circle cells link per CIRCLE (circleLink), never per cell; cellLink serves the
// text cells only.
test('V2 D4 / S9 / §10 A9: circle cells link per circle -- Day 0 its own campaigns, a leaf its own rows\' lists', () => {
  const [ruze] = shapeRows(api);
  assert.equal(cellLink(ruze, 'welcome:0'), null);
  assert.deepEqual(circleLink(ruze, 'welcome:0', 'green'), { id: ['3'], evergreen: 'all' });
  assert.equal(circleLink(ruze, 'postPurchase:0', 'green'), null); // no rows
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
  assert.deepEqual(circleLink(one, 'welcome:5', 'green'), { list_id: ['12'], evergreen: 'true' }); // (a step row; the slug is immaterial)
  // Several rows: each circle its own rows' lists.
  assert.deepEqual(circleLink(one, 'postPurchase:0', 'green'), { list_id: ['4'], evergreen: 'true' });
  assert.deepEqual(circleLink(one, 'postPurchase:0', 'amber'), { list_id: ['5'], evergreen: 'true' });
  const noList = shapeRows([{
    brand: 'y',
    usage: usage({ series: { welcome: series('welcome'), postPurchase: series('postPurchase', { 0: [{ slug: 'a', listId: null, evergreen: 'none' }] }) } }),
  }])[0];
  // A list-less (invalid) row is counted amber but cannot be linked (§10 A9).
  assert.deepEqual(cell(noList, 'postPurchase:0', t).circles, [{ color: 'amber', n: 1 }]);
  assert.equal(circleLink(noList, 'postPurchase:0', 'amber'), null);
  // A ZMA row's dimmed Welcome Day 0 does not link; no usage, no link.
  assert.equal(circleLink(shapeRows(api).find((r) => r.brand === 'oldzma'), 'welcome:0', 'green'), null);
  assert.equal(circleLink(shapeRows(api).find((r) => r.brand === 'nodoc'), 'welcome:0', 'green'), null);
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
  assert.deepEqual(circleLink(ruze, 'welcome:0', 'green'), { id: ['3'], evergreen: 'all' });
  assert.equal(circleLink(ruze, 'postPurchase:0', 'green'), null);
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
  ['score', 'lastBroadcast', 'cadence'].forEach((f) => {
    const c = cell(zma, f, t);
    assert.equal(c.text, MISSING, f);
    assert.notEqual(c.cls, 'has-text-danger', f);
    assert.equal(c.cls, '', f);
    assert.deepEqual(c.circles, [], f);
  });
  // A journey cell is always a circle (user 2026-11-02): welcome Day 0 and the Re-engagement leaves
  // render their stored circles on a ZMA row too (dimmed with the row), never '—'.
  assert.equal(cell(zma, 'welcome:0', t).text, '');
  assert.ok(cell(zma, 'welcome:0', t).circles.length >= 1);
  assert.deepEqual(cell(zma, 'postPurchase:0', t).circles, [{ color: 'amber', n: 1 }]);
  assert.equal(cell(zma, 'sunset', t).text, '✓');
});

test('HOURLY-HEALTH D5 / I5: the note time is the newest stored computedAt, in Mountain time (date + time)', () => {
  const rows = shapeRows([
    { brand: 'a', computedAt: '2026-11-06T20:00:21Z', usage: usage() },
    { brand: 'b', computedAt: '2026-11-06T21:00:19Z', usage: usage() },
    { brand: 'c', computedAt: 'garbage', usage: usage() },
    { brand: 'd' },
  ]);
  assert.equal(rows[1].computedAt, '2026-11-06T21:00:19Z');
  // 21:00Z on Nov 6 is 2:00 PM MST; 15:00Z on Jul 15 is 9:00 AM MDT -- DST-correct, no zone text.
  assert.equal(computedAtText(rows), 'Nov 6, 2026, 2:00 PM');
  assert.equal(computedAtText(shapeRows([{ brand: 'x', computedAt: '2026-07-15T15:00:00Z' }])), 'Jul 15, 2026, 9:00 AM');
  assert.equal(computedAtText(shapeRows([{ brand: 'd' }, { brand: 'e', usage: usage() }])), null);
  assert.equal(computedAtText([]), null);
});

test('HOURLY-HEALTH I5: the fixed run-time literal is gone from the fork frontend', () => {
  const src = readFileSync(new URL('./brandUsage.mjs', import.meta.url), 'utf8');
  // Assembled so that the I5 grep over frontend/src for the old constant's name stays empty.
  assert.ok(!src.includes(['RUN', 'TIME', 'UTC'].join('_')));
  assert.ok(!src.includes(['mountain', 'RunTime'].join('')));
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
  assert.equal(sortValue(rows[3], 'welcome:0'), 2); // a ZMA row sorts by its stored state like any row (2026-11-02; was null while dimmed to '—')
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
  // USAGE-DAY10-SPEC: the No-purchase leaves are 0/2/5 -- the chips sit after 5d.
  const r = wsRow([npStep(2, 40, 'running'), npStep(5, 41, 'idle')], [npStep(15, 42, 'running')]);
  assert.deepEqual(cell(r, 'welcome:2', t).circles, [{ color: 'green', n: 1 }]);
  assert.deepEqual(cell(r, 'welcome:5', t).circles, [{ color: 'amber', n: 1 }]);
  assert.equal(WELCOME_FIELDS.includes('welcome:10'), false); // no 10d leaf
  assert.equal(seriesLeafOf('welcome:10'), null);
  // The 15-day legacy step appears in no leaf -- it is the grey chip after 5d.
  WELCOME_FIELDS.forEach((f) => assert.ok(!cell(r, f, t).tooltip.includes('x-no-purchase-42'), f));
  assert.deepEqual(cellBadges(r, 'welcome:5', t), [{
    kind: 'legacy',
    glyph: 'dashboard.usage.legacyChip{"n":15}',
    tooltip: 'series.welcomeLegacy\nx-no-purchase-42: dashboard.usage.listEvergreen{"list":42,"state":"running"}',
  }]);
  // The tooltip: the D5 description, the words, then the step's row.
  assert.equal(
    cell(r, 'welcome:2', t).tooltip,
    'series.welcomeDay2\ndashboard.usage.circleRunning{"n":1}\nx-no-purchase-40: dashboard.usage.listEvergreen{"list":40,"state":"running"}',
  );
  // An absent step: red 0.
  const gap = wsRow([npStep(5, 41, 'idle')]);
  assert.deepEqual(cell(gap, 'welcome:2', t).circles, [{ color: 'red', n: 0 }]);
  assert.equal(cell(gap, 'welcome:2', t).tooltip, 'series.welcomeDay2\ndashboard.usage.circleNone');
  // USAGE-DAY10-SPEC D2: a stored day-10 step is a legacy chip, never a leaf.
  const ten = wsRow([], [npStep(10, 47, 'running')]);
  assert.deepEqual(cellBadges(ten, 'welcome:5', t).map((b) => [b.kind, b.glyph]), [['legacy', 'dashboard.usage.legacyChip{"n":10}']]);
  // An invalid row WITH a standard day sits in its leaf as amber, its error in the tooltip.
  const bad = wsRow([npStep(2, 43, 'running', 'sinceAt "x" is not an ISO timestamp')]);
  assert.deepEqual(cell(bad, 'welcome:2', t).circles, [{ color: 'amber', n: 1 }]);
  assert.match(cell(bad, 'welcome:2', t).tooltip, /sinceAt "x" is not an ISO timestamp/);
  // A step without a list is amber.
  assert.deepEqual(cell(wsRow([npStep(5, null, 'none')]), 'welcome:5', t).circles, [{ color: 'amber', n: 1 }]);
  // The Ruze shape (G7): welcome running, only the grandfathered 15d step -> Day 0 green, red 0 x2,
  // one legacy chip.
  const ruze = wsRow([], [npStep(15, 56, 'running')], 'ruze');
  assert.deepEqual(WELCOME_FIELDS.map((f) => cell(ruze, f, t).circles), [
    [{ color: 'green', n: 1 }], [{ color: 'red', n: 0 }], [{ color: 'red', n: 0 }],
  ]);
  assert.deepEqual(cellBadges(ruze, 'welcome:5', t).map((b) => b.kind), ['legacy']);
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

// Fork (usage decisions, integrations USAGE-DECISIONS-SPEC I12 / D7): the legacy chip's glyph is
// "{n}d" (no "legacy"), and it links the row's STORED campaigns -- { id, evergreen: 'all' }, the Day 0
// circle's shape, ids ascending as strings -- with no link when the row stores none or predates
// the field. The warn badge is unchanged (no link).
test('I12: the legacy chip reads "{n}d" and links the row\'s stored campaigns; none when empty or absent', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.equal(en['dashboard.usage.legacyChip'], '{n}d');
  const tEn = (k, p) => (en[k] || k).replace(/\{(\w+)\}/g, (m, name) => (p && name in p ? String(p[name]) : m));
  const stored = { ...npStep(15, 56, 'running'), campaigns: [{ id: 231, status: 'paused' }, { id: 229, status: 'running' }, { id: 229, status: 'running' }] };
  const ruze = wsRow([], [stored], 'ruze');
  const [chip] = cellBadges(ruze, 'welcome:5', tEn);
  assert.equal(chip.kind, 'legacy');
  assert.equal(chip.glyph, '15d');
  assert.deepEqual(chip.link, { id: ['229', '231'], evergreen: 'all' });
  // Empty, absent, or malformed ids -> no link key at all.
  [[], undefined, [{ id: 0 }, { id: 'x' }, null]].forEach((campaigns) => {
    const row = wsRow([], [{ ...npStep(15, 56, 'running'), ...(campaigns === undefined ? {} : { campaigns }) }], 'ruze');
    const [b] = cellBadges(row, 'welcome:5', tEn);
    assert.equal(b.glyph, '15d');
    assert.equal('link' in b, false, JSON.stringify(campaigns));
  });
  // The warn badge carries no link even when the row stores campaigns.
  const warn = wsRow([], [{ ...npStep(null, 44, 'running', 'bad days'), campaigns: [{ id: 9, status: 'running' }] }]);
  const [w] = cellBadges(warn, 'welcome:5', tEn);
  assert.equal(w.kind, 'warn');
  assert.equal('link' in w, false);
});

test('I15 (POLISH3 stage 2 F5): a no-days row is an amber ⚠ badge with its parser error; a pre-release doc is — throughout', () => {
  const r = wsRow([npStep(5, 41, 'running')], [npStep(null, 44, 'none', 'days "x" is not an integer ≥ 1'), npStep(null, 45, 'none', 'days "0" is not an integer ≥ 1')]);
  assert.deepEqual(cellBadges(r, 'welcome:5', t), [
    { kind: 'warn', glyph: '⚠', tooltip: 'dashboard.usage.npInvalid{"slug":"x-no-purchase-44","error":"days \\"x\\" is not an integer ≥ 1"}' },
    { kind: 'warn', glyph: '⚠', tooltip: 'dashboard.usage.npInvalid{"slug":"x-no-purchase-45","error":"days \\"0\\" is not an integer ≥ 1"}' },
  ]);
  // Only after the group's LAST leaf (5d since USAGE-DAY10-SPEC); nowhere else.
  ['welcome:0', 'welcome:2', 'postPurchase:0', 'postPurchase:30', 'skipped:fr'].forEach((f) => assert.deepEqual(cellBadges(r, f, t), [], f));
  assert.deepEqual(cell(r, 'welcome:5', t).circles, [{ color: 'green', n: 1 }]);
  // A doc written before the release (no `series`; it still carries noPurchase) -> every series
  // leaf the "none" circle (a journey cell is always a circle -- user 2026-11-02; before that '—'),
  // with the seriesMissing note; no link, no sort value, no chip (S9 / Stage 2 F7).
  const preUsage = usage({ noPurchase: { state: 'active', rows: [npStep(15, 9, 'running')], counts: { green: 1, amber: 0 } } });
  delete preUsage.series;
  const pre = shapeRows([{ brand: 'p', usage: preUsage }], { lists })[0];
  [...WELCOME_FIELDS, ...POST_PURCHASE_FIELDS].forEach((f) => {
    const c = cell(pre, f, t);
    assert.equal(c.text, '', f);
    assert.deepEqual(c.circles, [{ color: 'red', n: 0 }], f);
    assert.match(c.tooltip, /\ndashboard\.usage\.seriesMissing$/, f);
    assert.equal(cellLink(pre, f), null, f);
    assert.equal(sortValue(pre, f), null, f);
  });
  assert.deepEqual(cellBadges(pre, 'welcome:5', t), []);
});

test('I15 / §10 A9: a leaf\'s circles link their own steps\' lists -- green the running, amber the rest', () => {
  const r = wsRow([npStep(2, 40, 'running'), npStep(5, 41, 'idle'), npStep(5, 46, 'running')]);
  assert.deepEqual(circleLink(r, 'welcome:2', 'green'), { list_id: ['40'], evergreen: 'true' });
  // Two steps at 5d: one circle each, each linking its own.
  assert.deepEqual(cell(r, 'welcome:5', t).circles, [{ color: 'green', n: 1 }, { color: 'amber', n: 1 }]);
  assert.deepEqual(circleLink(r, 'welcome:5', 'green'), { list_id: ['46'], evergreen: 'true' });
  assert.deepEqual(circleLink(r, 'welcome:5', 'amber'), { list_id: ['41'], evergreen: 'true' });
  assert.equal(circleLink(r, 'welcome:2', 'amber'), null); // absent
  assert.equal(circleLink(wsRow([]), 'welcome:2', 'red'), null); // red never links
  assert.equal(circleLink(wsRow([npStep(2, null, 'none')]), 'welcome:2', 'amber'), null); // no list
  assert.equal(circleLink(shapeRows(api).find((x) => x.brand === 'nodoc'), 'welcome:2', 'green'), null);
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
// stored counts), each circle linked to its own rows' lists, sorted by how many there are.
test('SKU column: stored counts as circles, rows in the tooltip, per-circle links, sorted by count', () => {
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
  // Per circle (§10 A9): green the running row's list, amber the others' (the list-less invalid
  // row is counted but not linkable); no per-cell link.
  assert.deepEqual(circleLink(r, 'sku', 'green'), { list_id: ['41'], evergreen: 'true' });
  assert.deepEqual(circleLink(r, 'sku', 'amber'), { list_id: ['40'], evergreen: 'true' });
  assert.equal(cellLink(r, 'sku'), null);
  assert.equal(linkTip(r, 'sku', t), ''); // like the series leaves: no link hint (circle cells show the tooltip only)
  assert.equal(sortValue(r, 'sku'), 3);
  assert.deepEqual(columnMeta('sku'), { label: 'dashboard.usage.sku', help: 'dashboard.usage.skuHelp' });
  // None: '—' (optional, never a red 0 -- user 2026-10-30), no link, sort 0.
  const [none] = shapeRows([{ brand: 'ruze', usage: usage({ sku: { rows: [], counts: { green: 0, amber: 0 } } }) }]);
  assert.deepEqual(cell(none, 'sku', t).circles, []);
  assert.equal(cell(none, 'sku', t).text, MISSING);
  assert.equal(cell(none, 'sku', t).tooltip, 'dashboard.usage.skuNone');
  assert.equal(circleLink(none, 'sku', 'red'), null);
  assert.equal(sortValue(none, 'sku'), 0);
  // A doc written before the column (no `sku`): '—', no link, sorts last (null).
  const [old] = shapeRows([{ brand: 'ruze', usage: usage() }]);
  assert.equal(cell(old, 'sku', t).text, MISSING);
  assert.equal(circleLink(old, 'sku', 'green'), null);
  assert.equal(sortValue(old, 'sku'), null);
  // The strings exist.
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  ['dashboard.usage.sku', 'dashboard.usage.skuHelp', 'dashboard.usage.skuNone'].forEach((k) => assert.equal(typeof en[k], 'string', k));
});

// 2026-10-30: the in-section rules -- No-purchase | Post-purchase and Post-purchase | SKU, the same
// on the header and every body column; no rule between plain leaves (Cadence | Last broadcast).
test('dividers: header and body carry the same in-section rules', () => {
  const h = headerLayout();
  // USAGE-REENGAGEMENT-SPEC: the Re-engagement group opens its own rule, like Post-purchase.
  assert.deepEqual(h.row2.filter((c) => c.divide).map((c) => (c.kind === 'group' ? `[${c.key}]` : c.field)), ['[postPurchase]', '[reengagement]', 'sku']);
  assert.deepEqual(h.row3.filter((c) => c.divide).map((c) => c.field), ['postPurchase:0', 'reengagement:180']);
  const cols = bodyColumns();
  assert.deepEqual(cols.map((c) => c.field), SORT_FIELDS.slice(2));
  assert.deepEqual(cols.filter((c) => c.divide).map((c) => c.field), ['postPurchase:0', 'reengagement:180', 'sku']);
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
  assert.deepEqual(circleLink(r, 'welcome:0', 'green'), { id: ['67'], evergreen: 'all' });
  assert.deepEqual(circleLink(r, 'welcome:0', 'amber'), { id: ['70', '71'], evergreen: 'all' });
  // Each circle's own accessible name (§10 review F1), never the whole cell's tooltip.
  assert.equal(circleName('green', 1, t), 'dashboard.usage.circleRunning{"n":1}');
  assert.equal(circleName('amber', 2, t), 'dashboard.usage.circleIdle{"n":2}');
  assert.equal(circleName('red', 0, t), 'dashboard.usage.circleNone');
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
  assert.deepEqual(parseStepText('Day 2 Product Value / Why Choose Us: "Still exploring? Here\'s why [Brand] is different." Key product benefits.'), {
    title: 'Product Value / Why Choose Us', quote: 'Still exploring? Here\'s why [Brand] is different.', body: 'Key product benefits.',
  });
  // A title ending in "?" and a body containing ": " -- split at the FIRST ": ".
  assert.deepEqual(parseStepText('Day 14 Ready for more?: Beyond the basics: more tips and routine ideas.'), {
    title: 'Ready for more?', quote: '', body: 'Beyond the basics: more tips and routine ideas.',
  });
  assert.deepEqual(parseStepText('Day 5 Getting started: Around delivery: how to use it.'), {
    title: 'Getting started', quote: '', body: 'Around delivery: how to use it.',
  });
  assert.deepEqual(parseStepText('no colon here'), { title: '', quote: '', body: 'no colon here' });
  assert.deepEqual(parseStepText(undefined), { title: '', quote: '', body: '' });
});

test('guide: seriesGuide over the real en.json -- Welcome then New Customer, every standard day, every step titled', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  const tt = (k, p) => (en[k] === undefined ? k : en[k].replace(/\{(\w+)\}/g, (m, n) => (p && n in p ? String(p[n]) : m)));
  const g = seriesGuide(tt);
  // USAGE-REENGAGEMENT-SPEC F3: three series -- the third titled "Re-engagement series" (not the
  // group label), sublined "Sunset flow", its days REENGAGEMENT_DAYS.
  assert.deepEqual(g.map((s) => [s.family, tt(s.title), tt(s.subtitle)]), [
    ['welcome', 'No-purchase series', 'Welcome series'],
    ['postPurchase', 'Post-purchase series', 'New Customer series'],
    ['reengagement', 'Re-engagement series', 'Sunset flow'],
  ]);
  assert.deepEqual(g.map((s) => s.steps.map((x) => x.days)), [SERIES_DAYS.welcome, SERIES_DAYS.postPurchase, REENGAGEMENT_DAYS]);
  // SUNSET-V4 D7: four steps.
  // 2026-11-09 (the documentation audit): Kaylinn's step names, the whole pair, split into the
  // primary and the grey secondary (splitStepTitle); the day-365 step is outside her brief.
  assert.deepEqual(g[2].steps.map((x) => [x.title, x.quote]), [
    ['Reconnect With the Brand', 'Still want to hear from us?'], ['Another Reason to Reconnect', 'Your emails are paused. Here\'s what you\'ve been missing.'],
    ['Final Opportunity to Stay Connected', 'Should we stop emailing you?'], ['Inactivity Suppression', 'We\'ve stopped sending you marketing emails.'],
  ]);
  assert.equal(SERIES_DAYS.reengagement, undefined); // never a series family
  g.forEach((s) => s.steps.forEach((x) => {
    assert.ok(x.title && x.body && x.primary, `day ${x.days}`);
    // The at-a-glance body: two lines, three at most, in a 32rem column (~65 characters a line).
    assert.ok(x.body.length <= 200, `${s.family} day ${x.days} body ${x.body.length} chars`);
    // Every standard step carries a detail for the Day badge's hover; the clock sentence follows it.
    const detail = en[`${seriesStepKey(s.family, x.days)}Detail`];
    assert.ok(typeof detail === 'string' && detail.length > 0 && x.hover.startsWith(detail), `${s.family} day ${x.days} hover`);
  }));
  assert.deepEqual(g[0].steps.map((x) => [x.primary, x.secondary]), [
    ['Welcome', 'Brand Introduction'], ['Product Value', 'Why Choose Us'], ['Build Confidence', 'Product Discovery'],
  ]);
  assert.deepEqual(g[1].steps.map((x) => [x.primary, x.secondary]), [
    ['First-Time Customer Welcome', 'Thank You'], ['Getting Started', 'Delivery-Based'], ['Get More From Your Purchase', ''], ['Your Next Favorite', 'Second Purchase'],
  ]);
  assert.deepEqual(splitStepTitle('Product Value / Why Choose Us'), { primary: 'Product Value', secondary: 'Why Choose Us' });
  assert.deepEqual(splitStepTitle('Thank you'), { primary: 'Thank you', secondary: '' });
  assert.deepEqual(splitStepTitle(undefined), { primary: '', secondary: '' });
  // A hover is detail + clock on an approximate Post-purchase step, detail alone elsewhere.
  assert.equal(g[1].steps[1].hover, `${en['series.postPurchaseDay5Detail']}\n${g[1].steps[1].clock}`);
  assert.equal(g[0].steps[0].hover, en['series.welcomeDay0Detail']);
  assert.equal(stepDetail('welcome', 99, (k) => k), '');
  assert.equal(stepDetail('welcome', 2, (k) => k), ''); // no translation: the key comes back, so no detail
  // USAGE-DAY10-SPEC D1/D4 / I5: day 10 retired; day 14 is "Get More From Your Purchase"
  // (parseStepText splits on the FIRST ": "); its body is the concise form of LIFECYCLE-DOCS-SPEC D19.
  assert.deepEqual(g[0].steps.map((x) => x.title), ['Welcome / Brand Introduction', 'Product Value / Why Choose Us', 'Build Confidence / Product Discovery']);
  assert.deepEqual(g[1].steps.map((x) => x.title), [
    'First-Time Customer Welcome / Thank You', 'Getting Started / Delivery-Based', 'Get More From Your Purchase', 'Your Next Favorite / Second Purchase',
  ]);
  assert.equal(g[1].steps[2].body, 'Beyond the basics of Getting Started: more tips, techniques, routine ideas or ways to get more value from the product. '
    + 'Genuinely helpful, never a review request.');
  assert.equal(en['series.postPurchaseDay14'], `Day 14 Get More From Your Purchase: ${g[1].steps[2].body}`);
  // LIFECYCLE-DOCS-SPEC D22 / I12: the Post-purchase steps after day 0 carry "~N" and the real
  // clock; day 0 and every No-purchase / Re-engagement step the plain number and no clock.
  assert.deepEqual(g[1].steps.map((x) => x.label), ['0', '~5', '~14', '~30']);
  g[1].steps.slice(1).forEach((x) => {
    assert.ok(x.clock.startsWith('Sent at the first daily run') && !x.clock.includes('{'), `day ${x.days}: ${x.clock}`);
  });
  assert.match(g[1].steps[1].clock, /after delivery day \+ 0, .* order date \+ 5 days\.$/);
  assert.match(g[1].steps[2].clock, /after delivery day \+ 9, .* order date \+ 14 days\.$/);
  assert.ok(g[1].steps[3].clock.endsWith(`order date + 30 days. ${en['dashboard.usage.approxClockReorder']}`));
  assert.equal(g[1].steps[0].clock, '');
  [...g[0].steps, ...g[2].steps].forEach((x) => {
    assert.deepEqual([x.label, x.clock], [String(x.days), ''], `${x.days}`);
  });
  assert.equal(g[0].steps[1].quote, 'Still exploring? Here\'s why [Brand] is different.');
  assert.equal(g[1].steps[0].quote, '');
  assert.equal(GUIDE_SERIES.length, 3);
  assert.equal(typeof en['dashboard.usage.guideDay'], 'string');
});

test('guide: guideColumnsFit -- side by side when every series keeps the minimum, else stacked', () => {
  // 2026-10-31 polish: three left-justified columns, no measured line.
  assert.equal(guideColumnsFit(1400), true);
  assert.equal(guideColumnsFit(GUIDE_MIN_COLUMN * 3), true);
  assert.equal(guideColumnsFit(GUIDE_MIN_COLUMN * 3 - 1), false);
  assert.equal(guideColumnsFit(GUIDE_MIN_COLUMN * 2, GUIDE_MIN_COLUMN, 2), true);
  assert.equal(guideColumnsFit(NaN), false);
  assert.equal(guideColumnsFit(undefined), false);
});

test('table: pageWidth (2026-11-02) -- the wider of the natural table and three standard columns capped by the container', () => {
  assert.equal(GUIDE_COLUMN_REM, 32);
  assert.equal(pageWidth(1400, 1536, 1800), 1536); // the standard wins: the table is padded out to it
  assert.equal(pageWidth(1600, 1536, 1800), 1600); // a wider table wins: the documentation follows it
  assert.equal(pageWidth(1400, 1536, 1500), 1500); // the standard is capped by the container; the table never is
  assert.equal(pageWidth(1600, 1536, 1500), 1600);
  assert.equal(pageWidth(1400, 1536, NaN), 1536); // no container measure: the standard stands
  assert.equal(pageWidth(NaN, 1536, 1800), 1536);
  assert.equal(pageWidth(1400, NaN, 1800), 1400);
});

test('table: linePad -- the per-side pad that widens the table to the guide, from its unpadded width', () => {
  assert.equal(linePad(1536, 1136, 0, 5), 40); // 400 px over 10 sides
  assert.equal(linePad(1536, 1536, 40, 5), 40); // already fitted: stable
  assert.equal(linePad(1536, 1500, 40, 5), 43); // a header absorbed part of the pad: grows
  assert.equal(linePad(1536, 1137, 0, 5), 39); // floored, never over the target
  assert.equal(linePad(1000, 1200, 0, 5), 0); // already wider: no pad
  assert.equal(linePad(1536, 1136, 0, 0), 0); // no lines
  assert.equal(linePad(NaN, 1136, 0, 5), 0);
  assert.equal(linePad(1536, undefined, 0, 5), 0);
});

// USAGE-REENGAGEMENT-SPEC F2 -- the Re-engagement leaves: the stored circle verbatim, the tooltip
// lines, '—' without the field, the per-circle link, ZMA dimmed, sort by state, and never a series leaf.
test('F2: Re-engagement cells render the stored circle, tooltip, link and sort; not series leaves', () => {
  const step = (days, queue, listId, state, color, n, extra = {}) => ({
    days, queue, listId, state, circle: { color, n }, campaigns: [], waiting: 0, byLang: {}, uncovered: [], ...extra,
  });
  const reengagement = {
    steps: [
      step(180, 1, 49, 'idle', 'amber', 1, { waiting: 120, byLang: { en: 100, fr: 20 }, uncovered: ['fr', 'es'] }),
      step(270, 2, 50, 'active', 'green', 2, { waiting: 3, byLang: { en: 3 } }),
    ],
  };
  const [r] = shapeRows([{ brand: 'shala', usage: usage({ reengagement }) }]);
  const c180 = cell(r, 'reengagement:180', t);
  assert.deepEqual(c180.circles, [{ color: 'amber', n: 1, name: 'dashboard.usage.reCircleAttention' }]);
  assert.equal(c180.tooltip, [
    'series.reengagementDay180', 'dashboard.usage.reCircleAttention',
    'dashboard.usage.reWaiting{"n":120}', 'dashboard.usage.reUncovered{"langs":"FR, ES"}',
  ].join('\n'));
  const c270 = cell(r, 'reengagement:270', t);
  assert.deepEqual(c270.circles, [{ color: 'green', n: 2, name: 'dashboard.usage.reCircleRunning{"n":2}' }]);
  assert.equal(c270.tooltip, ['series.reengagementDay270', 'dashboard.usage.reCircleRunning{"n":2}', 'dashboard.usage.reWaiting{"n":3}'].join('\n'));
  // An amber with n 0 (zero evergreens on an active list) renders verbatim -- never circlesOf's red.
  const zeroSteps = [step(180, 1, 49, 'idle', 'amber', 0), step(270, 2, null, 'none', 'red', 0, { waiting: null })];
  const [idle0] = shapeRows([{ brand: 'x', usage: usage({ reengagement: { steps: zeroSteps } }) }]);
  assert.deepEqual(cell(idle0, 'reengagement:180', t).circles, [{ color: 'amber', n: 0, name: 'dashboard.usage.reCircleAttention' }]);
  assert.deepEqual(cell(idle0, 'reengagement:270', t).circles, [{ color: 'red', n: 0, name: 'dashboard.usage.reCircleNone' }]);
  assert.equal(cell(idle0, 'reengagement:270', t).tooltip, 'series.reengagementDay270\ndashboard.usage.reCircleNone'); // waiting null: no line
  // circleLink (2026-10-31 polish): every stored circle links to its campaigns -- green the running
  // automations, amber every automation, red every campaign on the queue list; with no stored
  // campaign ids, the list's automations. Another color than the stored one, or no list, never.
  assert.deepEqual(circleLink(r, 'reengagement:180', 'amber'), { list_id: ['49'], evergreen: 'true' });
  assert.equal(circleLink(r, 'reengagement:180', 'green'), null);
  assert.deepEqual(circleLink(r, 'reengagement:270', 'green'), { list_id: ['50'], evergreen: 'true' });
  assert.deepEqual(circleLink(idle0, 'reengagement:180', 'amber'), { list_id: ['49'], evergreen: 'true' });
  assert.equal(circleLink(idle0, 'reengagement:270', 'red'), null); // red with no list
  const camps = [{ id: 91, status: 'paused', lang: null }, { id: 90, status: 'running', lang: 'en' }];
  const linkSteps = [
    step(180, 1, 49, 'none', 'red', 0),
    step(270, 2, 50, 'active', 'green', 1, { campaigns: camps }),
  ];
  const [lk] = shapeRows([{ brand: 'y', usage: usage({ reengagement: { steps: linkSteps } }) }]);
  assert.deepEqual(circleLink(lk, 'reengagement:180', 'red'), { list_id: ['49'], evergreen: 'all' });
  assert.deepEqual(circleLink(lk, 'reengagement:270', 'green'), { id: ['90'], evergreen: 'all' });
  const amberCamps = [step(180, 1, 49, 'idle', 'amber', 0, { campaigns: camps }), step(270, 2, 50, 'none', 'red', 0)];
  const [am] = shapeRows([{ brand: 'z', usage: usage({ reengagement: { steps: amberCamps } }) }]);
  assert.deepEqual(circleLink(am, 'reengagement:180', 'amber'), { id: ['90', '91'], evergreen: 'all' });
  // sortValue by the stored state's rank (active 0, idle 1, none 2).
  assert.equal(sortValue(r, 'reengagement:270'), 0);
  assert.equal(sortValue(r, 'reengagement:180'), 1);
  assert.equal(sortValue(idle0, 'reengagement:270'), 2);
  // A doc without `reengagement` (or without this step -- a doc older than a new leaf): the "none"
  // circle with seriesMissing (a journey cell is always a circle -- user 2026-11-02), no link,
  // sorts null.
  const [old] = shapeRows([{ brand: 'ruze', usage: usage() }]);
  const m = cell(old, 'reengagement:180', t);
  assert.equal(m.text, '');
  assert.deepEqual(m.circles, [{ color: 'red', n: 0, name: 'dashboard.usage.reCircleNone' }]);
  assert.equal(m.tooltip, 'series.reengagementDay180\ndashboard.usage.reCircleNone\ndashboard.usage.seriesMissing');
  const [twoSteps] = shapeRows([{ brand: 'ruze', usage: usage({ reengagement }) }]);
  assert.deepEqual(cell(twoSteps, 'reengagement:365', t).circles, [{ color: 'red', n: 0, name: 'dashboard.usage.reCircleNone' }]);
  assert.equal(circleLink(old, 'reengagement:180', 'amber'), null);
  assert.equal(sortValue(old, 'reengagement:180'), null);
  // A ZMA brand (user 2026-11-02): the circle is shown like every journey leaf (dimmed with the
  // row), never linked; sorts by its stored state like any row.
  const [zma] = shapeRows([{ brand: 'z', usage: usage({ channel: 'zma', reengagement }) }]);
  assert.deepEqual(cell(zma, 'reengagement:180', t).circles, cell(r, 'reengagement:180', t).circles);
  assert.equal(circleLink(zma, 'reengagement:180', 'amber'), null);
  assert.equal(sortValue(zma, 'reengagement:180'), sortValue(r, 'reengagement:180'));
  // Not series leaves (Stage 2 F4).
  REENGAGEMENT_FIELDS.forEach((f) => assert.equal(seriesLeafOf(f), null, f));
  assert.deepEqual(reengagementLeafOf('reengagement:270'), { days: 270 });
  assert.equal(reengagementLeafOf('reengagement:120'), null);
  assert.equal(reengagementLeafOf('welcome:5'), null);
  // D3: each circle's accessible name.
  assert.equal(reCircleName('green', 3, t), 'dashboard.usage.reCircleRunning{"n":3}');
  assert.equal(reCircleName('amber', 0, t), 'dashboard.usage.reCircleAttention');
  assert.equal(reCircleName('red', 0, t), 'dashboard.usage.reCircleNone');
  // The score part renders after Post-purchase.
  const reParts = { sunset: { points: 0, max: 10 }, reengagement: { points: 4, max: 10 }, postPurchaseSeries: { points: 0, max: 20 } };
  assert.equal(
    scoreTip({ total: 1, parts: reParts }, (k) => k),
    'dashboard.usage.partPostPurchaseSeries 0/20 · dashboard.usage.partReengagement 4/10 · dashboard.usage.partSunset 0/10',
  );
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  ['dashboard.usage.reengagement', 'dashboard.usage.reengagementHelp', 'dashboard.usage.reCircleRunning', 'dashboard.usage.reCircleAttention',
    'dashboard.usage.reCircleNone', 'dashboard.usage.reWaiting', 'dashboard.usage.reUncovered', 'dashboard.usage.partReengagement',
    'dashboard.usage.guideReengagement', 'dashboard.usage.guideSunset'].forEach((k) => assert.equal(typeof en[k], 'string', k));
  assert.equal(en['dashboard.usage.reengagement'], 'Re-engagement');
});

// Integrations DOC-ALIGNMENT-SPEC D9 / I18: the three-key counts render verbatim on every leaf
// family (green running, amber paused, grey draft/scheduled, red 0 when nothing counted); a grey
// circle links like an amber one; an old-shape document renders through the pre-D9 path.
test('D9 / I18: three-key counts on every leaf family; grey links; an old-shape doc falls back', () => {
  const C = (green, amber, grey) => ({ green, amber, grey });
  const row = (days, listId, campaigns, error) => ({
    slug: `x-${listId}`, listId, evergreen: 'idle', days, campaigns, ...(error ? { error } : {}),
  });
  const camp = (id, status) => ({ id, status });
  const steps3 = (family, byDay) => SERIES_DAYS[family].map((days) => ({
    days, state: 'none', counts: C(0, 0, 0), rows: [], ...(byDay[days] || {}),
  }));
  const u3 = usage({
    welcome: {
      state: 'active', listId: 7, campaigns: [camp(70, 'running'), camp(71, 'paused'), camp(72, 'draft'), camp(73, 'finished')], counts: { green: 1, amber: 3 },
    },
    series: {
      welcome: {
        steps: steps3('welcome', {
          0: { state: 'active', counts: C(1, 1, 1) },
          5: {
            state: 'idle',
            counts: C(0, 0, 1),
            rows: [row(5, 50, [camp(500, 'draft')]), row(5, 51, [camp(510, 'scheduled')], 'bad row')],
          },
        }),
        legacy: [],
      },
      postPurchase: {
        steps: steps3('postPurchase', {
          0: {
            state: 'active', counts: C(1, 1, 0), rows: [row(0, 80, [camp(800, 'running')]), row(0, 81, [camp(810, 'paused'), camp(811, 'cancelled')])],
          },
        }),
        legacy: [],
      },
    },
    sku: {
      rows: [row(null, 90, [camp(900, 'scheduled')])], counts: { green: 0, amber: 1 }, leaf: { state: 'idle', counts: C(0, 0, 1) },
    },
    reengagement: {
      steps: [180, 270, 360, 365].map((days, i) => ({
        days,
        queue: i + 1,
        listId: 48 + i,
        state: i === 0 ? 'active' : 'none',
        counts: i === 0 ? C(1, 0, 1) : C(0, 0, 0),
        circle: i === 0 ? { color: 'amber', n: 1 } : { color: 'red', n: 0 },
        campaigns: i === 0 ? [{ id: 480, status: 'running', lang: 'en' }, { id: 481, status: 'draft', lang: null }] : [],
        waiting: i === 0 ? 7 : null,
        byLang: i === 0 ? { en: 5, fr: 2 } : {},
        uncovered: i === 0 ? ['fr'] : [],
      })),
    },
  });
  const [r] = shapeRows([{ brand: 'ruze', usage: u3 }]);
  const colors = (f) => cell(r, f, t).circles.map((c) => [c.color, c.n]);
  // Day 0: the step's three-key counts, never the legacy two-key welcome.counts.
  assert.deepEqual(colors('welcome:0'), [['green', 1], ['amber', 1], ['grey', 1]]);
  assert.deepEqual(colors('welcome:2'), [['red', 0]]);
  assert.deepEqual(colors('welcome:5'), [['grey', 1]]);
  assert.deepEqual(colors('postPurchase:0'), [['green', 1], ['amber', 1]]);
  assert.deepEqual(colors('sku'), [['grey', 1]]);
  // Re-engagement: the counts, not the legacy circle; "Not covered" stays in the hover only.
  assert.deepEqual(colors('reengagement:180'), [['green', 1], ['grey', 1]]);
  assert.deepEqual(colors('reengagement:270'), [['red', 0]]);
  assert.match(cell(r, 'reengagement:180', t).tooltip, /reUncovered/);
  assert.deepEqual(cell(r, 'reengagement:180', t).circles.map((c) => c.name), ['dashboard.usage.circleRunning{"n":1}', 'dashboard.usage.circleDraft{"n":1}']);
  assert.match(cell(r, 'welcome:5', t).tooltip, /circleDraft/);
  assert.equal(circleName('grey', 2, t), 'dashboard.usage.circleDraft{"n":2}');
  assert.equal(circleWords(C(1, 2, 3), t), 'dashboard.usage.circleRunning{"n":1}, dashboard.usage.circleIdle{"n":2}, dashboard.usage.circleDraft{"n":3}');
  // (e) Links: each circle opens the automations of its own colour; grey like amber.
  assert.deepEqual(circleLink(r, 'welcome:0', 'green'), { id: ['70'], evergreen: 'all' });
  assert.deepEqual(circleLink(r, 'welcome:0', 'amber'), { id: ['71'], evergreen: 'all' });
  assert.deepEqual(circleLink(r, 'welcome:0', 'grey'), { id: ['72'], evergreen: 'all' });
  assert.deepEqual(circleLink(r, 'welcome:5', 'grey'), { list_id: ['50'], evergreen: 'true' }); // the invalid row's list never
  assert.equal(circleLink(r, 'welcome:5', 'amber'), null); // no amber circle shown
  assert.deepEqual(circleLink(r, 'postPurchase:0', 'amber'), { list_id: ['81'], evergreen: 'true' });
  assert.deepEqual(circleLink(r, 'sku', 'grey'), { list_id: ['90'], evergreen: 'true' });
  assert.deepEqual(circleLink(r, 'reengagement:180', 'grey'), { id: ['481'], evergreen: 'all' });
  assert.deepEqual(circleLink(r, 'reengagement:270', 'red'), { list_id: ['49'], evergreen: 'all' });
  assert.equal(circleLink(r, 'welcome:2', 'red'), null);
  // Sort by the stored leaf state.
  assert.equal(sortValue(r, 'welcome:5'), 1);
  assert.equal(sortValue(r, 'welcome:2'), 2);
  assert.equal(sortValue(r, 'welcome:0'), 0);
  // The fallback: an old-shape doc (no three-key counts) renders exactly as before D9.
  const [old] = shapeRows([{ brand: 'ruze', usage: usage() }]);
  assert.deepEqual(cell(old, 'welcome:0', t).circles, [{ color: 'green', n: 1 }]);
  assert.deepEqual(cell(old, 'welcome:2', t).circles, [{ color: 'red', n: 0 }]);
  const oldStep = {
    days: 180, queue: 1, listId: 48, state: 'idle', circle: { color: 'amber', n: 1 }, campaigns: [], waiting: null, byLang: {}, uncovered: ['fr'],
  };
  const oldRe = shapeRows([{ brand: 'ruze', usage: usage({ reengagement: { steps: [oldStep] } }) }])[0];
  assert.deepEqual(cell(oldRe, 'reengagement:180', t).circles, [{ color: 'amber', n: 1, name: 'dashboard.usage.reCircleAttention' }]);
  // The new strings exist.
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.equal(en['dashboard.usage.circleIdle'], '{n} paused');
  ['dashboard.usage.circleDraft', 'dashboard.usage.legendDraft'].forEach((k) => assert.equal(typeof en[k], 'string', k));
});

// Integrations DOC-ALIGNMENT-SPEC stage 4b W3 / W6: an invalid row on a standard leaf (or in SKU)
// shows its ⚠ on that leaf whatever the colour; a red 0 on a row-backed leaf with a valid row that
// has a list opens those lists (every kind), and a red with no such row does not link.
test('D9 / W3 / W6: the ⚠ for an invalid row on a leaf; a red 0 links its trigger rows\' lists', () => {
  const C = (green, amber, grey) => ({ green, amber, grey });
  const camp = (id, status) => ({ id, status });
  const empty = (days) => ({
    days, state: 'none', counts: C(0, 0, 0), rows: [],
  });
  const r = (listId, campaigns, error) => ({
    slug: `x-${listId}`, listId, evergreen: 'running', days: 5, campaigns, ...(error ? { error } : {}),
  });
  const shaped = (step5, sku = { rows: [], counts: { green: 0, amber: 0 }, leaf: { state: 'none', counts: C(0, 0, 0) } }) => shapeRows([{
    brand: 'ruze',
    usage: usage({
      series: {
        welcome: { steps: SERIES_DAYS.welcome.map((days) => (days === 5 ? { days, ...step5 } : empty(days))), legacy: [] },
        postPurchase: { steps: SERIES_DAYS.postPurchase.map(empty), legacy: [] },
      },
      sku,
    }),
  }])[0];
  const warns = (row, f) => cellBadges(row, f, t).filter((b) => b.kind === 'warn' && b.glyph === '⚠');
  // An invalid row at day 5 with a running automation: a red 0 AND a ⚠ carrying its error.
  const lone = shaped({ state: 'none', counts: C(0, 0, 0), rows: [r(55, [camp(1, 'running')], 'sinceAt is not a date')] });
  assert.deepEqual(cell(lone, 'welcome:5', t).circles.map((c) => c.color), ['red']);
  assert.equal(warns(lone, 'welcome:5').length, 1);
  assert.match(warns(lone, 'welcome:5')[0].tooltip, /npInvalid.*sinceAt is not a date/);
  assert.equal(circleLink(lone, 'welcome:5', 'red'), null, 'no valid row: nothing set up, no link');
  // A valid + invalid pair: green AND a ⚠.
  const pair = shaped({ state: 'active', counts: C(1, 0, 0), rows: [r(56, [camp(2, 'running')]), r(57, [camp(3, 'running')], 'bad row')] });
  assert.deepEqual(cell(pair, 'welcome:5', t).circles.map((c) => c.color), ['green']);
  assert.equal(warns(pair, 'welcome:5').length, 1);
  // No ⚠ on a leaf whose rows are all valid, nor on another leaf.
  assert.deepEqual(warns(pair, 'welcome:2'), []);
  // The SKU column: its invalid row shows the ⚠ too.
  const sku = shaped({ state: 'none', counts: C(0, 0, 0), rows: [] }, {
    rows: [{ ...r(90, [camp(9, 'running')]), days: null }, { ...r(91, [], 'skus empty'), days: null }], counts: { green: 1, amber: 1 }, leaf: { state: 'active', counts: C(1, 0, 0) },
  });
  assert.equal(warns(sku, 'sku').length, 1);
  // W6: a valid row with a list and no automation counted -> the red 0 opens that list (all kinds).
  const unstarted = shaped({ state: 'none', counts: C(0, 0, 0), rows: [r(58, [camp(4, 'finished')]), r(59, [], 'bad')] });
  assert.deepEqual(cell(unstarted, 'welcome:5', t).circles.map((c) => c.color), ['red']);
  assert.deepEqual(circleLink(unstarted, 'welcome:5', 'red'), { list_id: ['58'], evergreen: 'all' });
});
