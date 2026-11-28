// Fork (campaign mode) -- integrations listmonk/CAMPAIGN-MODE-SPEC I1, I5, I11-I14, I16:
// campaignGroups.mjs -- the name key against the shared fixture (the server's NameKeySQL reads the
// same file), the toggle's value and its one writer, the applicability table over the router and the
// captions, the Matrix group rows (one aggregation rule with the totals row), the bucketing, the
// Campaigns page flattening and the picker's labels and fill.
// Run: node --test src/campaignGroups.test.mjs (yarn test:groups).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import {
  GROUP_ACTIONS, MODES, MODE_APPLICABILITY, MODE_PREF, bucketMatrix, flattenGroups, flattenTree, groupAnalyticsRoute,
  groupCellTip, groupRow, groupedTableRows, langCodes, memberRows, modeApplicability, modeNote, nameKey, nodeMembers,
  normalizeMode, pickerFill, pickerLabel, sortTree, statusSummary, statusSummaryText, statusTdAttrs, groupSendLine,
} from './campaignGroups.mjs'; // eslint-disable-line import/extensions
import {
  cellText, gpTip, rateOf, shapeRow, sumRows, totalsRow,
} from './campaignMatrix.mjs'; // eslint-disable-line import/extensions
import { gapOf } from './gmailGap.mjs'; // eslint-disable-line import/extensions

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');

// I1 -- the JS half of the name key against the fixture the Go DB test reads through Postgres.
test('nameKey fixture', () => {
  const fx = JSON.parse(read('./testdata/campaign-groups.json'));
  assert.ok(fx.cases.length >= 10);
  fx.cases.forEach((c) => assert.equal(nameKey(c.name), c.key, JSON.stringify(c)));
  // U+2003 is kept, also at the end; null/undefined fold to ''.
  assert.ok(fx.cases.some((c) => c.key.endsWith('\u2003')));
  assert.equal(nameKey(null), '');
  assert.equal(nameKey(undefined), '');
});

// I14 -- the pref: an unknown stored value reads off; selectCampaignMode is the one writer and only
// the toggle commits it.
test('normalizeMode and the one writer', () => {
  assert.deepEqual([...MODES], ['off', 'groups']);
  assert.equal(MODE_PREF, 'campaigns.mode');
  ['off', 'groups'].forEach((v) => assert.equal(normalizeMode(v), v));
  [null, undefined, '', 'on', 'GROUPS', 1, {}].forEach((v) => assert.equal(normalizeMode(v), 'off'));

  const files = [];
  const walk = (dir) => readdirSync(dir).forEach((f) => {
    const p = `${dir}/${f}`;
    if (statSync(p).isDirectory()) {
      walk(p);
    } else if (/\.(vue|js|mjs)$/.test(f) && !/\.test\.mjs$/.test(f)) {
      files.push(p);
    }
  });
  const root = new URL('.', import.meta.url).pathname.replace(/\/$/, '');
  walk(root);
  const writers = files.filter((p) => /setPref\(\s*(MODE_PREF|'campaigns\.mode')/.test(readFileSync(p, 'utf8')));
  assert.deepEqual(writers.map((p) => p.slice(root.length + 1)), ['store/index.js']);
  const store = read('./store/index.js');
  const mutation = store.slice(store.indexOf('selectCampaignMode(state'), store.indexOf('selectCampaignMode(state') + 300);
  assert.match(mutation, /prefs\.setPref\(MODE_PREF, v\)/);
  assert.equal((store.match(/setPref\(MODE_PREF/g) || []).length, 1);
  const committers = files.filter((p) => /commit\(\s*'selectCampaignMode'/.test(readFileSync(p, 'utf8')));
  assert.deepEqual(committers.map((p) => p.slice(root.length + 1)), ['components/CampaignModeToggle.vue']);
  // The getter normalises.
  assert.match(store, /campaignMode: \(state\) => normalizeMode\(state\.campaignModeStored\)/);
});

// I13 -- the table covers every named route; the captions per D13.
test('MODE_APPLICABILITY covers the router', () => {
  const src = read('./router/index.js');
  const names = [...src.matchAll(/^\s*name:\s*'([^']+)'/gm)].map((m) => m[1]).sort();
  assert.ok(names.length >= 20);
  assert.deepEqual(Object.keys(MODE_APPLICABILITY).sort(), names);
  assert.equal(modeApplicability('campaigns'), 'all');
  assert.equal(modeApplicability('campaignAnalytics'), 'all');
  assert.equal(modeApplicability('campaignAnalytics', 'analytics:matrix'), 'all');
  assert.equal(modeApplicability('campaignAnalytics', 'analytics:campaign'), 'all');
  names.filter((n) => n !== 'campaigns' && n !== 'campaignAnalytics').forEach((n) => assert.equal(modeApplicability(n), 'none', n));
  assert.equal(modeApplicability('nope'), 'none');
});

test('modeNote captions', () => {
  const all = { langEffective: 'all' };
  const fr = { langEffective: 'fr' };
  assert.equal(modeNote('off', 'campaigns', all), null);
  assert.equal(modeNote('off', 'dashboard', fr), null);
  // G5 amendment: the Campaigns page groups by send -- under one language nothing groups there.
  assert.equal(modeNote('groups', 'campaigns', all), 'naming.modeApp.sends');
  assert.equal(modeNote('groups', 'campaigns', fr), 'naming.modeApp.sendsOneLang');
  const en = JSON.parse(read('../../i18n/en.json'));
  ['naming.modeApp.sends', 'naming.modeApp.sendsOneLang'].forEach((k) => assert.ok(en[k], k));
  assert.equal(modeNote('groups', 'campaignAnalytics', { ...fr, pane: 'analytics:matrix' }), 'naming.modeApp.seriesOnly');
  // The picker pane is not language-filtered: the groups line under a single language.
  assert.equal(modeNote('groups', 'campaignAnalytics', { ...fr, pane: 'analytics:campaign' }), 'naming.modeApp.groups');
  // Languages off, or a subscriber-only selection: the effective selection is All.
  assert.equal(modeNote('groups', 'campaigns', { langEffective: 'all' }), 'naming.modeApp.sends');
  assert.equal(modeNote('groups', 'campaigns', {}), 'naming.modeApp.sends');
  assert.equal(modeNote('groups', 'campaignAnalytics', { ...all, pane: 'analytics:matrix' }), 'naming.modeApp.groups');
  assert.equal(modeNote('groups', 'dashboard', all), 'naming.modeApp.none');
  assert.equal(modeNote('groups', 'templates', fr), 'naming.modeApp.none');
});

// Matrix rows in the server's shape.
const api = (over) => ({
  id: 1,
  name: 'Spring Sale',
  status: 'finished',
  evergreen: false,
  started_at: '2026-11-01T18:00:00Z',
  send_day: '2026-11-01',
  brand: 'alpha',
  sent: 100,
  views: 10,
  clicks: 4,
  hard_bounces: 1,
  soft_bounces: 0,
  complaints: 0,
  shop_founder: null,
  shop_self: null,
  shop_creator: null,
  shop_corp: null,
  tracked: { orders: 1, sales_usd: 10 },
  matched: null,
  gp_rate: 0.001,
  gp_thresholds: { warn: 0.001, issues: 0.003 },
  skipped: 2,
  skipped_lookback: 2,
  skipped_lookahead: 0,
  skipped_silent: 0,
  skipped_purchased: 0,
  lang: 'en',
  set_key: 'alpha\u001f\u001fspring sale',
  series_key: null,
  ...over,
});
const gap = (g, gp, y, yp) => ({
  gmail: g, gmail_proxy: gp, yahoo: y, yahoo_proxy: yp,
});

// I5 -- D8 aggregation.
test('groupRow aggregation', () => {
  const a = shapeRow({ ...api({ id: 1, ...gap(100, 50, 400, 200) }) });
  const b = shapeRow(api({
    id: 2,
    lang: 'fr',
    sent: 300,
    views: 30,
    clicks: 30,
    hard_bounces: 3,
    tracked: { orders: 2, sales_usd: 5 },
    matched: { orders: 1, sales_usd: 7 },
    shop_founder: 2,
    shop_self: 1,
    shop_creator: 0,
    shop_corp: 0,
    status: 'running',
    started_at: '2026-10-30T10:00:00Z',
    send_day: '2026-10-30',
    ...gap(300, 30, 600, 300),
  }));
  const g = groupRow([a, b], { kind: 'set', key: 'e:x' });
  // Counts sum; rates over each column's own Sent (shop kinds only b carries).
  assert.equal(g.sent, 400);
  assert.equal(g.views, 40);
  assert.equal(rateOf(g, 'views'), 40 / 400);
  assert.equal(rateOf(g, 'clicks'), 34 / 400);
  assert.equal(g.shop_founder, 2);
  assert.equal(rateOf(g, 'shop_founder'), 2 / 300);
  assert.equal(g.skipped, 4);
  // Conversions sum (covered rows only).
  assert.deepEqual(g.tracked, { orders: 3, sales: 15 });
  assert.deepEqual(g.matched, { orders: 1, sales: 7 });
  // The gap pooled, never averaged.
  assert.deepEqual(g.gap, gap(400, 80, 1000, 500));
  assert.equal(gapOf(g.gap, null).gap, (80 / 400) / (500 / 1000));
  // GP: two broadcasts on two send days -> '—' with the mixed hover.
  assert.equal(g.gp_rate, null);
  assert.equal(g.gpMixed, true);
  assert.deepEqual(gpTip(g), { key: 'analytics.matrix.gpMixedDays', params: {} });
  assert.equal(cellText(g, 'gp_rate', 'pct'), '—');
  assert.deepEqual(groupCellTip(g, 'gp_rate'), { key: 'analytics.matrix.gpMixedDays' });
  // One send day -> shown.
  const one = groupRow([a, shapeRow(api({ id: 3, lang: 'de' }))], { kind: 'set' });
  assert.equal(one.gp_rate, 0.001);
  assert.equal(one.gpMixed, false);
  assert.deepEqual(one.gpThresholds, { warn: 0.001, issues: 0.003 });
  // Review S3 -- several send days and no GP value: still the mixed-days hover, never gpNone's
  // empty date.
  const noGp = groupRow([
    shapeRow(api({ id: 10, gp_rate: null, send_day: '2026-11-01' })),
    shapeRow(api({
      id: 11, gp_rate: null, send_day: '2026-11-02', lang: 'fr',
    })),
    shapeRow(api({
      id: 12, gp_rate: null, send_day: '2026-11-02', lang: 'de',
    })),
  ], { kind: 'set' });
  assert.equal(noGp.gp_rate, null);
  assert.equal(noGp.gpMixed, true);
  assert.deepEqual(gpTip(noGp), { key: 'analytics.matrix.gpMixedDays', params: {} });
  // One send day and no GP value: gpNone with that day.
  const oneNoGp = groupRow([shapeRow(api({ id: 13, gp_rate: null })), shapeRow(api({ id: 14, gp_rate: null, lang: 'fr' }))], { kind: 'set' });
  assert.equal(oneNoGp.gpMixed, false);
  assert.equal(gpTip(oneNoGp).key, 'analytics.matrix.gpNone');
  assert.equal(gpTip(oneNoGp).params.date, '2026-11-01');
  // The per-campaign hover on every group row's Views and Clicks.
  [g, one, groupRow([a], { kind: 'series' })].forEach((r) => {
    assert.deepEqual(groupCellTip(r, 'views'), { key: 'analytics.matrix.perCampaign' });
    assert.deepEqual(groupCellTip(r, 'clicks'), { key: 'analytics.matrix.perCampaign' });
    assert.equal(groupCellTip(r, 'sent'), null);
  });
  assert.equal(groupCellTip(a, 'views'), null);
  // Status: summary in STATUS_ORDER; sorts as the most active member.
  assert.deepEqual(g.statusSummary, [{ status: 'running', n: 1 }, { status: 'finished', n: 1 }]);
  assert.equal(g.status, 'running');
  // Started: the earliest; Lang codes in LANG_VALUES order.
  assert.equal(g.startedAt, '2026-10-30T10:00:00Z');
  assert.equal(g.lang, 'en, fr');
  // Code / Step: a series shows its Code, a set its agreed Step.
  const s1 = shapeRow(api({ id: 4, promo_code: 'BF', promo_step: 1 }));
  const s1b = shapeRow(api({
    id: 5, promo_code: 'BF', promo_step: 1, lang: 'fr',
  }));
  const s2 = shapeRow(api({ id: 6, promo_code: 'BF', promo_step: 2 }));
  assert.equal(groupRow([s1, s1b], { kind: 'set' }).promo_step, 1);
  assert.equal(groupRow([s1, s2], { kind: 'set' }).promo_step, null);
  assert.equal(groupRow([s1, s2], { kind: 'series' }).promo_code, 'BF');
  assert.equal(groupRow([s1, s2], { kind: 'series' }).promo_step, null);
  assert.equal(groupRow([s1, s1b], { kind: 'set' }).promo_code, '');
  // The name is the newest (max id) member's.
  assert.equal(groupRow([s2, s1], { kind: 'series' }).name, 'Spring Sale');
  // Never-started only when none started.
  const ns = groupRow([shapeRow(api({ id: 7, started_at: null })), a], { kind: 'set' });
  assert.equal(ns.startedAt, a.startedAt);
  assert.equal(groupRow([shapeRow(api({ id: 7, started_at: null }))], { kind: 'set' }).startedAt, null);
});

test('statusSummary and langCodes', () => {
  const rows = ['finished', 'draft', 'finished', 'finished', 'finished', 'weird'].map((status) => ({ status }));
  const s = statusSummary(rows);
  assert.deepEqual(s, [{ status: 'draft', n: 1 }, { status: 'finished', n: 4 }, { status: 'weird', n: 1 }]);
  assert.equal(statusSummaryText(statusSummary(rows.slice(0, 5)), (x) => ({ draft: 'Draft', finished: 'Finished' }[x])), '1 draft, 4 finished');
  assert.deepEqual(langCodes(['fr', 'en', '', 'it', 'fr', 'xx', 'DE']), ['en', 'fr', 'de', 'it', 'xx']);
});

// I5 -- the totals row still reads through sumRows (one rule for Totals and groups).
test('totalsRow is sumRows plus the fixed fields', () => {
  const rows = [shapeRow(api({ id: 1 })), shapeRow(api({ id: 2, sent: 50, views: 5 }))];
  const t = totalsRow(rows, 'Total');
  const s = sumRows(rows);
  Object.keys(s).forEach((k) => assert.deepEqual(t[k], s[k], k));
  assert.equal(t.totals, true);
  // A group row of the same members has the same numbers.
  const g = groupRow(rows);
  Object.keys(s).forEach((k) => assert.deepEqual(g[k], s[k], k));
  assert.equal(rateOf(g, 'views'), rateOf(t, 'views'));
  assert.ok(read('./campaignMatrix.mjs').includes('...sumRows(r),'));
});

// I11 -- the Matrix bucketing.
test('bucketMatrix', () => {
  const rows = [
    api({ id: 1, lang: 'en' }),
    api({ id: 2, lang: 'fr' }),
    api({ id: 3, name: 'Alone', set_key: 'alpha\u001f\u001falone' }),
    api({
      id: 4, name: 'No brand', brand: null, set_key: null,
    }),
    api({
      id: 5, name: 'Promo', promo_code: 'BF', promo_step: 1, set_key: 'alpha\u001fbf\u001fpromo', series_key: 'alpha\u001fbf',
    }),
    api({
      id: 6, name: 'Promo', lang: 'fr', promo_code: 'BF', promo_step: 1, set_key: 'alpha\u001fbf\u001fpromo', series_key: 'alpha\u001fbf',
    }),
    api({
      id: 7, name: 'Last call', promo_code: 'BF', promo_step: 2, set_key: 'alpha\u001fbf\u001flast call', series_key: 'alpha\u001fbf',
    }),
    api({
      id: 8, name: 'One-step', promo_code: 'CM', set_key: 'alpha\u001fcm\u001fone-step', series_key: 'alpha\u001fcm',
    }),
  ].map((r) => shapeRow(r));
  const nodes = bucketMatrix(rows);
  assert.equal(nodes.length, 5);
  const [set, alone, nobrand, series, single] = nodes;
  assert.equal(set.row.group, true);
  assert.equal(set.row.kind, 'set');
  assert.deepEqual(set.children.map((c) => c.row.id), [1, 2]);
  // A set of one renders as the campaign row.
  assert.equal(alone.row.group, undefined);
  assert.equal(alone.row.id, 3);
  assert.equal(nobrand.row.id, 4);
  // A series is always a group row; its children are its sets (a set of one is the campaign row).
  assert.equal(series.row.kind, 'series');
  assert.equal(series.row.promo_code, 'BF');
  assert.deepEqual(series.children.map((c) => (c.row.group ? c.row.kind : c.row.id)), ['set', 7]);
  assert.deepEqual(series.children[0].children.map((c) => c.row.id), [5, 6]);
  assert.equal(single.row.kind, 'series');
  assert.deepEqual(nodeMembers(single).map((r) => r.id), [8]);

  // Name filter: any member passing filterByName keeps the whole node.
  const hit = bucketMatrix(rows, 'last call');
  assert.equal(hit.length, 1);
  assert.deepEqual(nodeMembers(hit[0]).map((r) => r.id), [5, 6, 7]);
  assert.deepEqual(bucketMatrix(rows, 'zzz'), []);
  // Bucketing comes after the language filter (the caller's), so a filtered member never joins.
  const fr = bucketMatrix(rows.filter((r) => r.lang !== 'en'));
  assert.ok(fr.every((n) => !nodeMembers(n).some((r) => r.lang === 'en')));

  // Flattening: children only when expanded; members are copies, never the shaped rows mutated.
  const sorted = sortTree(nodes, { field: 'name', order: 'asc' }, 'pct');
  assert.equal(flattenTree(sorted).length, 5);
  const open = flattenTree(sorted, new Set([series.row.key]));
  assert.equal(open.length, 7);
  const expandedSeries = open.find((r) => r.rowKey === series.row.key);
  assert.equal(expandedSeries.expanded, true);
  assert.equal(rows[0].depth, undefined);
  // Children sorted within their parent by the same sort.
  const inner = flattenTree(sorted, new Set([series.row.key, series.children[0].row.key]));
  const iSeries = inner.findIndex((r) => r.rowKey === series.row.key);
  assert.deepEqual(inner.slice(iSeries + 1, iSeries + 3).map((r) => r.name), ['Last call', 'Promo']);
  // Totals over every member that passed the filters, expanded or not.
  const table = groupedTableRows(rows, { sort: { field: 'date', order: 'desc' }, mode: 'pct' });
  const tot = table[table.length - 1];
  assert.equal(tot.totals, true);
  assert.equal(tot.sent, rows.reduce((t, r) => t + r.sent, 0));
  assert.equal(table.length, 6);
});

// I12 -- the Campaigns page flattening.
const member = (id, over) => ({
  id,
  name: `C${id}`,
  status: 'finished',
  evergreen: false,
  sent: 10,
  views: 2,
  clicks: 1,
  hardBounces: 0,
  complaints: 0,
  lists: [{ id: 1, name: 'L1' }],
  attribs: { lang: 'en' },
  startedAt: `2026-11-0${id}T00:00:00Z`,
  ...over,
});
test('flattenGroups and memberRows', () => {
  const running = member(3, {
    status: 'running', attribs: { lang: 'fr' }, lists: [{ id: 2, name: 'L2' }, { id: 1, name: 'L1' }], clicks: 5, hardBounces: 2, complaints: 1,
  });
  const results = [
    {
      kind: 'set', brand: 'alpha', name: 'C3', ids: [3, 1], campaigns: [running, member(1)],
    },
    {
      kind: 'campaign', brand: null, name: 'C2', ids: [2], campaigns: [member(2)],
    },
    {
      kind: 'series', brand: 'alpha', code: 'BF', name: 'C5', ids: [5, 4], sets: [{ name: 'C5', step: 1, ids: [5, 4] }], campaigns: [member(5), member(4)],
    },
  ];
  const flat = flattenGroups(results);
  assert.equal(flat.length, 3);
  assert.equal(flat[0].group, true);
  assert.equal(flat[1], results[1].campaigns[0]); // a singleton is the unmodified row
  assert.equal(flat[2].group, true);
  // Members follow their group row only when expanded, as the unmodified server rows.
  const open = flattenGroups(results, new Set([flat[0].key]));
  assert.equal(open.length, 5);
  assert.equal(open[1], running);
  assert.equal(open[2], results[0].campaigns[1]);
  // The group row's view.
  const v = flat[0];
  assert.equal(v.brand, 'alpha');
  assert.deepEqual(v.memberIds, [3, 1]);
  assert.deepEqual(v.langs, ['en', 'fr']);
  assert.deepEqual(v.lists.map((l) => l.id), [2, 1]);
  assert.equal(v.startedAt, '2026-11-01T00:00:00Z');
  assert.equal(v.sent, 20);
  assert.equal(v.views, 4);
  // Review S6 -- every summed Stats field.
  assert.equal(v.clicks, 6);
  assert.equal(v.hardBounces, 2);
  assert.equal(v.complaints, 1);
  assert.deepEqual(v.statusSummary, [{ status: 'running', n: 1 }, { status: 'finished', n: 1 }]);
  assert.equal(flat[2].code, 'BF');
  // G5 amendment: a send row (set) carrying a Code shows it with its Step.
  const stepRow = flattenGroups([{
    kind: 'set', brand: 'alpha', code: 'BF', step: 2, name: 'C7', ids: [7, 6], campaigns: [member(7), member(6)],
  }])[0];
  assert.equal(stepRow.code, 'BF');
  assert.equal(stepRow.step, 2);
  assert.deepEqual(groupAnalyticsRoute(v), { name: 'campaignAnalytics', query: { id: [3, 1] } });
  // Group rows carry only the analytics action.
  assert.deepEqual(GROUP_ACTIONS.filter((a) => a.enabled).map((a) => a.action), ['analytics']);
  GROUP_ACTIONS.filter((a) => !a.enabled).forEach((a) => {
    const want = ['clone', 'preview'].includes(a.action) ? 'campaigns.group.notForGroups' : 'campaigns.group.phase2';
    assert.equal(a.tip, want, a.action);
  });
  assert.deepEqual(
    GROUP_ACTIONS.map((a) => a.action).sort(),
    ['analytics', 'cancel', 'clone', 'delete', 'inspect', 'pause', 'preview', 'resume', 'schedule', 'start'],
  );

  // memberRows: every member, collapsed or not; a running member of a collapsed group is displayed-running.
  assert.deepEqual(memberRows(results).map((c) => c.id), [3, 1, 2, 5, 4]);
  assert.ok(memberRows(results).some((c) => c.status === 'running' && !c.evergreen));
  // Ungrouped: the rows themselves.
  const plain = [member(9), member(8)];
  assert.equal(memberRows(plain), plain);
  assert.equal(flattenGroups(plain), plain);
  assert.deepEqual(memberRows(null), []);
  // The Status cell's attributes: a group row has no id (upstream tdID threw on it -- G5, "Nothing
  // here"); a member row keeps upstream's data-id.
  assert.deepEqual(statusTdAttrs(flat[0]), {});
  assert.deepEqual(statusTdAttrs(open[1]), { 'data-id': '3' });
  // G5 amendment -- the group send line sums what each member row shows.
  const ts = (c) => (c.audience === undefined ? null : { count: c.audience - (c.skip || 0) });
  const st = (c) => (c.pending ? 'pending' : null);
  const fin = (id, sent) => ({ id, sent, toSend: sent });
  const draft = (id, audience, extra = {}) => ({
    id, sent: 0, toSend: 0, audience, ...extra,
  });
  const live = { 3: { sent: 40, toSend: 100 } };
  const line = (rows) => {
    const l = groupSendLine(rows, { toSendOf: ts, stateOf: st, statsOf: (c) => live[c.id] || c });
    return [l.label, l.sent, l.total, l.state];
  };
  assert.deepEqual(line([fin(1, 342), fin(2, 2260)]), ['sent', 2602, 2602, null]);
  assert.deepEqual(line([draft(1, 337), draft(2, 2010, { skip: 1 })]), ['toSend', 0, 2346, null]);
  // Mixed: the drafts' To send joins the denominator; a running member reads the live poll.
  const running3 = { id: 3, sent: 0, toSend: 0 };
  assert.deepEqual(line([fin(1, 10), draft(2, 50), running3]), ['sent', 50, 160, null]);
  assert.deepEqual(line([draft(1, 5, { pending: true }), fin(2, 9)]), ['sent', 9, 9, 'pending']);
  assert.deepEqual(line([draft(1, 5, { pending: true })]), ['toSend', 0, 0, 'pending']);
  // Evergreen members: Sent alone, never a denominator.
  const ever = {
    id: 1, evergreen: true, sent: 7, toSend: 99, audience: 3,
  };
  assert.deepEqual(line([ever]), ['sent', 7, null, null]);
  // The page renders the guarded cell, never upstream's tdID on a group-capable table.
  // The page reads memberRows for the poll and the clone names.
  const page = read('./views/Campaigns.vue');
  assert.ok(!page.includes(':td-attrs="$utils.tdID"'));
  assert.match(page, /:td-attrs="statusTdAttrs"/);
  // G5 amendment -- the group toggle is the standard +/- box (the icon subset has no chevron-down,
  // so an expanded group drew nothing), and the switch shows no caption line beside it.
  assert.match(page, /<button type="button" class="expand-btn"[^>]*data-cy="group-toggle"/);
  assert.ok(!page.includes('chevron-down'));
  assert.match(page, /:set="line = groupSendLine\(props\.row\)"/);
  // G5 Step 4 -- a group row's disabled glyph keeps pointer events (upstream's [data-disabled] sets
  // pointer-events: none, which suppressed its hover).
  const groupActions = page.slice(page.indexOf('data-cy="group-actions"'), page.indexOf('<div v-else>', page.indexOf('data-cy="group-actions"')));
  assert.ok(!groupActions.includes('data-disabled'));
  assert.match(groupActions, /class="group-disabled"/);
  const css = read('./assets/style.scss');
  const rule = /\.actions a\.group-disabled \{([^}]*)\}/.exec(css);
  assert.ok(rule && !rule[1].includes('pointer-events'));
  // G5 amendment -- Campaign mode skips the audience count and fills To send after the rows land.
  assert.match(page, /getCampaignGroups\(\{ \.\.\.params, no_audience: true \}\)\.then\(\(\) => \{\s*this\.fillAudiences\(\);/);
  const toggle = read('./components/CampaignModeToggle.vue');
  assert.ok(!toggle.includes('campaign-mode-note'));
  assert.match(toggle, /:title="noteText \|\| /);
  assert.match(page, /displayedRunning\(memberRows\(this\.campaigns\.results\)\)/);
  assert.match(page, /const stale = memberRows\(this\.campaigns\.results\)\.some\(/);
  assert.match(page, /let names = memberRows\(/);
  // Review S6 -- the page's group Views and Clicks carry the per-campaign hover (groupRateTip).
  const tip = page.slice(page.indexOf('groupRateTip(row, f) {'), page.indexOf('groupRateTip(row, f) {') + 300);
  assert.match(tip, /f\.field === 'views' \|\| f\.field === 'clicks'/);
  assert.match(tip, /this\.\$t\('analytics\.matrix\.perCampaign'\)/);
  assert.match(page, /:title="groupRateTip\(props\.row, f\)"/);
});

// I16 -- the picker's fill and labels.
test('pickerFill and pickerLabel', () => {
  const pick = (id, over) => ({
    id, name: `N${id}`, promoCode: null, promoStep: null, ...over,
  });
  const set = {
    kind: 'set', brand: 'ruze', name: 'Spring Sale', campaigns: [pick(3), pick(1)],
  };
  const series = {
    kind: 'series', brand: 'ruze', code: 'BF', name: 'Promo', sets: [{}, {}], campaigns: [pick(5, { promoCode: 'BF', promoStep: 1 }), pick(4)],
  };
  const single = {
    kind: 'campaign', brand: 'ruze', name: 'N7', campaigns: [pick(7, { promoCode: 'CM', promoStep: 2 })],
  };
  const labels = { nSteps: (n) => `${n} steps`, nCampaigns: (n) => `${n} campaigns` };
  // Review S4 -- a group row leads with its brand slug; a single campaign keeps its own label.
  assert.equal(pickerLabel(set, labels), 'ruze · Spring Sale · 2 campaigns');
  assert.equal(pickerLabel(series, labels), 'ruze · BF · 2 steps');
  assert.equal(pickerLabel({ ...series, brand: 'bnkd' }, labels), 'bnkd · BF · 2 steps');
  assert.equal(pickerLabel(single, labels), '#7: N7 · CM S2');
  assert.ok(!/language/i.test(pickerLabel(set, labels)));

  const existing = [{ id: 1, name: '#1: N1' }, { id: 9, name: '#9: N9' }];
  const out = pickerFill(existing, set);
  assert.deepEqual(out.map((c) => c.id), [1, 9, 3]);
  assert.equal(out[0], existing[0]); // existing chips keep their place and object
  assert.equal(out[2].name, '#3: N3');
  assert.deepEqual(existing.map((c) => c.id), [1, 9]); // not mutated
  assert.deepEqual(pickerFill([], series).map((c) => [c.id, c.name]), [[5, '#5: N5 · BF S1'], [4, '#4: N4']]);
  assert.deepEqual(pickerFill(out, set).map((c) => c.id), [1, 9, 3]);
});
