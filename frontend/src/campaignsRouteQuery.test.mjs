// Fork (brand usage, integrations BRAND-USAGE-V2-SPEC I8; DASHBOARDS-POLISH2-SPEC I2). Run: yarn test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  CAMPAIGN_STATUSES, LEGACY_SCOPE_PREF, ROUTE_SCOPE_OPTION, SCOPE_KINDS, SCOPE_KIND_PREF, afterBrandChange,
  afterScopeChange, campaignListOptions, listQualifies,
  campaignsRouteQuery, canSelectAll, deleteEvergreenParam, displayedRunning, evergreenOfKind, kindOfEvergreen,
  kindStatusParams, listParam, listSelectValue, pollsStats, preEnablesEvergreen, routeEvergreen, routeIdsAfter,
  routeIdsOf, routeStatusAfter, routeStatusOf, scopeKindPref, selectList, STATUS_CLEARING,
  CAMPAIGNS_LINK_KEYS, offeredCampaignListIds, restoreCampaignsPage, saveCampaignsPage,
} from './campaignsRouteQuery.mjs'; // eslint-disable-line import/extensions

test('I8: a numeric list_id string selects the list; garbage is ignored', () => {
  assert.equal(campaignsRouteQuery({ list_id: '12' }).listID, 12);
  assert.equal(campaignsRouteQuery({ list_id: '12' }).routeListIDs, null);
  ['', 'abc', '12abc', '-3', '0', '1.5', ' 7', '012', '99999999999999999999'].forEach((v) => {
    assert.equal(campaignsRouteQuery({ list_id: v }).listID, null, v);
    assert.equal(campaignsRouteQuery({ list_id: v }).routeListIDs, null, v);
  });
  assert.equal(campaignsRouteQuery({}).listID, null);
  assert.equal(campaignsRouteQuery(null).listID, null);
  assert.equal(campaignsRouteQuery({ list_id: 12 }).listID, null); // the router only hands strings
});

// DASHBOARDS-POLISH2-SPEC I2 (supersedes I8's "a repeated list_id takes its first element").
test('I2: a repeated list_id is the multi set -- order preserved, garbage dropped, all-garbage ignored', () => {
  assert.deepEqual(campaignsRouteQuery({ list_id: ['5', '9'] }), { listID: null, routeListIDs: [5, 9], evergreen: false });
  assert.deepEqual(campaignsRouteQuery({ list_id: ['9', '5', '3'] }).routeListIDs, [9, 5, 3]);
  assert.deepEqual(campaignsRouteQuery({ list_id: ['x', '9', '0', '4', '-1', 7] }).routeListIDs, [9, 4]);
  assert.deepEqual(campaignsRouteQuery({ list_id: ['5', '5', '9'] }).routeListIDs, [5, 9]);
  // ONE valid id (alone, or the only survivor) is the single filter, unchanged behavior.
  assert.deepEqual(campaignsRouteQuery({ list_id: ['7'] }), { listID: 7, routeListIDs: null, evergreen: false });
  assert.deepEqual(campaignsRouteQuery({ list_id: ['x', '9'] }), { listID: 9, routeListIDs: null, evergreen: false });
  assert.deepEqual(campaignsRouteQuery({ list_id: ['4', '4'] }), { listID: 4, routeListIDs: null, evergreen: false });
  // Nothing valid -> ignored ("All lists").
  assert.deepEqual(campaignsRouteQuery({ list_id: ['x', '', '0'] }), { listID: null, routeListIDs: null, evergreen: false });
  assert.deepEqual(campaignsRouteQuery({ list_id: [] }), { listID: null, routeListIDs: null, evergreen: false });
  // evergreen still reads its first element.
  assert.equal(campaignsRouteQuery({ evergreen: ['true', 'false'] }).evergreen, true);
});

test('I8: evergreen=true selects Automations; otherwise the sticky pref stands', () => {
  assert.equal(campaignsRouteQuery({ evergreen: 'true' }, false).evergreen, true);
  assert.equal(campaignsRouteQuery({}, false).evergreen, false);
  assert.equal(campaignsRouteQuery({}, true).evergreen, true);
  assert.equal(campaignsRouteQuery({ evergreen: '1' }, false).evergreen, false);
  assert.equal(campaignsRouteQuery({ evergreen: 'TRUE' }, false).evergreen, false);
  assert.deepEqual(campaignsRouteQuery({ list_id: '7', evergreen: 'true' }, false), { listID: 7, routeListIDs: null, evergreen: true });
});

// DASHBOARDS-POLISH-SPEC I3 (supersedes V2's "only exactly true overrides").
test('I3: evergreen=false forces Broadcasts; "true" Automations; anything else keeps the pref', () => {
  assert.equal(campaignsRouteQuery({ evergreen: 'false' }, true).evergreen, false);
  assert.equal(campaignsRouteQuery({ evergreen: 'false' }, false).evergreen, false);
  assert.equal(campaignsRouteQuery({ evergreen: 'true' }, true).evergreen, true);
  ['FALSE', '0', 'no', '', ' false'].forEach((v) => {
    assert.equal(campaignsRouteQuery({ evergreen: v }, true).evergreen, true, v);
    assert.equal(campaignsRouteQuery({ evergreen: v }, false).evergreen, false, v);
  });
  assert.equal(campaignsRouteQuery({ evergreen: ['false', 'true'] }, true).evergreen, false);
  assert.deepEqual(campaignsRouteQuery({ list_id: '9', evergreen: 'false' }, true), { listID: 9, routeListIDs: null, evergreen: false });
  assert.deepEqual(
    campaignsRouteQuery({ list_id: ['3', '9'], evergreen: 'false' }, true),
    { listID: null, routeListIDs: [3, 9], evergreen: false },
  );
  // The parse never writes the pref: the same pref in gives the pref back with no override.
  const q = Object.freeze({ evergreen: 'false' });
  campaignsRouteQuery(q, true);
  assert.equal(campaignsRouteQuery({}, true).evergreen, true);
  assert.deepEqual(q, { evergreen: 'false' });
});

test('I8 / I2: the parse and every helper write nothing -- pure functions of their inputs', () => {
  const q = Object.freeze({ list_id: Object.freeze(['7', '3']), evergreen: 'true' });
  assert.deepEqual(campaignsRouteQuery(q, false), { listID: null, routeListIDs: [7, 3], evergreen: true });
  assert.deepEqual(q, { list_id: ['7', '3'], evergreen: 'true' });
  const s = Object.freeze({ listID: null, routeListIDs: Object.freeze([3, 7]) });
  listParam({ ...s, scopeListIds: [1] });
  listSelectValue(s);
  selectList(s, 4);
  afterBrandChange(s, [1]);
  canSelectAll({ ...s, total: 100, perPage: 20 });
  assert.deepEqual(s, { listID: null, routeListIDs: [3, 7] });
});

test('I2: precedence -- single filter, else the route set (same slot, beats the selector scope), else the scope', () => {
  assert.equal(listParam({ listID: 7, routeListIDs: null, scopeListIds: [1, 2] }), 7);
  assert.deepEqual(listParam({ listID: null, routeListIDs: [3, 9], scopeListIds: [1, 2] }), [3, 9]);
  assert.deepEqual(listParam({ listID: null, routeListIDs: [3, 9], scopeListIds: null }), [3, 9]);
  assert.deepEqual(listParam({ listID: null, routeListIDs: null, scopeListIds: [1, 2] }), [1, 2]);
  assert.equal(listParam({ listID: null, routeListIDs: null, scopeListIds: null }), undefined);
  assert.equal(listParam(), undefined);
  // An empty route set is no scope.
  assert.deepEqual(listParam({ listID: null, routeListIDs: [], scopeListIds: [1] }), [1]);
});

test('I2: the Lists select shows the placeholder while the route scope is active', () => {
  assert.equal(listSelectValue({ listID: null, routeListIDs: [3, 9] }), ROUTE_SCOPE_OPTION);
  assert.equal(listSelectValue({ listID: 7, routeListIDs: null }), 7);
  assert.equal(listSelectValue({ listID: null, routeListIDs: null }), null);
});

test('I2: a real list selection (or All lists) clears the route scope', () => {
  const routed = { listID: null, routeListIDs: [3, 9] };
  assert.deepEqual(selectList(routed, 4), { listID: 4, routeListIDs: null });
  assert.deepEqual(selectList(routed, null), { listID: null, routeListIDs: null });
  // The disabled placeholder, were it ever emitted, changes nothing.
  assert.deepEqual(selectList(routed, ROUTE_SCOPE_OPTION), routed);
  assert.deepEqual(selectList({ listID: 4, routeListIDs: null }, 5), { listID: 5, routeListIDs: null });
});

test('I2: ANY brand-selector change clears the route scope; a single filter outside the new scope resets', () => {
  assert.deepEqual(afterBrandChange({ listID: null, routeListIDs: [3, 9] }, null), { listID: null, routeListIDs: null });
  assert.deepEqual(afterBrandChange({ listID: null, routeListIDs: [3, 9] }, [3, 9]), { listID: null, routeListIDs: null });
  assert.deepEqual(afterBrandChange({ listID: 5, routeListIDs: null }, [1, 2]), { listID: null, routeListIDs: null });
  assert.deepEqual(afterBrandChange({ listID: 5, routeListIDs: null }, [5, 6]), { listID: 5, routeListIDs: null });
  assert.deepEqual(afterBrandChange({ listID: 5, routeListIDs: null }, null), { listID: 5, routeListIDs: null });
});

test('I2 (stage 2 F1): "select all N" is never offered under the route scope, the single filter or the selector scope', () => {
  const base = {
    bulkAll: false, total: 100, perPage: 20, listID: null, routeListIDs: null, scopeListIds: null,
  };
  assert.equal(canSelectAll(base), true);
  assert.equal(canSelectAll({ ...base, routeListIDs: [3, 9] }), false);
  assert.equal(canSelectAll({ ...base, listID: 3 }), false);
  assert.equal(canSelectAll({ ...base, scopeListIds: [1] }), false);
  assert.equal(canSelectAll({ ...base, bulkAll: true }), false);
  assert.equal(canSelectAll({ ...base, total: 20 }), false);
  // NAMING-STANDARD-SPEC D2 / review S4-1: the language toggle narrows the page but not the
  // by-query delete, so "select all N" is offered only under All.
  ['en', 'es', 'fr', 'de', 'it'].forEach((lang) => assert.equal(canSelectAll({ ...base, lang }), false, lang));
  assert.equal(canSelectAll({ ...base, lang: 'all' }), true);
  assert.equal(canSelectAll({ ...base, lang: '' }), true);
  // After the route scope clears, the affordance comes back.
  const cleared = selectList({ listID: null, routeListIDs: [3, 9] }, null);
  assert.equal(canSelectAll({ ...base, ...cleared }), true);
});

// ---------------------------------------------------------------------------------------------
// DASHBOARDS-POLISH3-SPEC I2 -- the route status scope
// ---------------------------------------------------------------------------------------------

test('POLISH3 I2: route status -- exactly one enum value, anything else ignored', () => {
  assert.deepEqual(CAMPAIGN_STATUSES, ['draft', 'scheduled', 'running', 'paused', 'finished', 'cancelled']);
  CAMPAIGN_STATUSES.forEach((s) => assert.equal(routeStatusOf({ status: s }), s));
  ['', 'FINISHED', 'done', 'finished ', 'canceled', 'all', 1, null].forEach((v) => assert.equal(routeStatusOf({ status: v }), null, String(v)));
  assert.equal(routeStatusOf({}), null);
  assert.equal(routeStatusOf(null), null);
  // A repeated param reads its first element (the evergreen rule).
  assert.equal(routeStatusOf({ status: ['finished', 'draft'] }), 'finished');
  assert.equal(routeStatusOf({ status: ['nope', 'draft'] }), null);
  // The rest of the route read is untouched by a status.
  assert.deepEqual(
    campaignsRouteQuery({ list_id: ['3', '9'], evergreen: 'false', status: 'finished' }, true),
    { listID: null, routeListIDs: [3, 9], evergreen: false },
  );
});

test('POLISH3 I2: the status scope narrows the fetch; All omits evergreen', () => {
  assert.deepEqual(kindStatusParams({ evergreen: false, routeStatus: 'finished' }), { evergreen: false, status: 'finished' });
  assert.deepEqual(kindStatusParams({ evergreen: true, routeStatus: null }), { evergreen: true });
  assert.deepEqual(kindStatusParams({ evergreen: null, routeStatus: null }), {});
  assert.deepEqual(kindStatusParams({ evergreen: null, routeStatus: 'draft' }), { status: 'draft' });
  assert.deepEqual(kindStatusParams({ evergreen: false, routeStatus: 'bogus' }), { evergreen: false });
  assert.deepEqual(kindStatusParams(), { evergreen: false });
});

// CAMPAIGN-STATUS-FILTER-SPEC D6 amendment (I12) replaces POLISH3 I2's clearing rule: only the
// chip's dismissal clears the status; a list pick, a brand change and the kind pill keep it.
test('STATUS-FILTER I12: only dismissal clears the status filter; list/brand/scope and the rest keep it', () => {
  assert.equal(routeStatusAfter('finished', 'dismiss'), null);
  ['list', 'brand', 'scope', 'page', 'sort', 'search', undefined].forEach((i) => assert.equal(routeStatusAfter('finished', i), 'finished', String(i)));
  assert.equal(routeStatusAfter(null, 'page'), null);
  assert.equal(routeStatusAfter('bogus', 'page'), null);
});

test('POLISH3 I2: "select all N" is never offered under the route status scope', () => {
  const base = {
    bulkAll: false, total: 100, perPage: 20, listID: null, routeListIDs: null, scopeListIds: null, routeStatus: null,
  };
  assert.equal(canSelectAll(base), true);
  assert.equal(canSelectAll({ ...base, routeStatus: 'finished' }), false);
  assert.equal(canSelectAll({ ...base, routeStatus: routeStatusAfter('finished', 'dismiss') }), true);
});

test('POLISH3 I2/I3: the status and kind helpers write nothing -- no pref, no input mutation', () => {
  const q = Object.freeze({ status: 'finished', evergreen: 'all' });
  routeStatusOf(q);
  campaignsRouteQuery(q, false);
  assert.deepEqual(q, { status: 'finished', evergreen: 'all' });
  const s = Object.freeze({ evergreen: null, routeStatus: 'finished' });
  kindStatusParams(s);
  assert.deepEqual(s, { evergreen: null, routeStatus: 'finished' });
});

// ---------------------------------------------------------------------------------------------
// DASHBOARDS-POLISH3-SPEC I3 -- the All pill and the pref migration
// ---------------------------------------------------------------------------------------------

test('POLISH3 I3: campaigns.scopeKind is read; the legacy boolean only while it is unset', () => {
  assert.equal(SCOPE_KIND_PREF, 'campaigns.scopeKind');
  assert.equal(LEGACY_SCOPE_PREF, 'campaigns.scopeAutomations');
  assert.deepEqual(SCOPE_KINDS, ['broadcasts', 'automations', 'all']);
  SCOPE_KINDS.forEach((k) => {
    assert.equal(scopeKindPref(k, true), k);
    assert.equal(scopeKindPref(k, false), k);
    assert.equal(scopeKindPref(k, null), k);
  });
  // Unset (or garbage) string pref -> the legacy boolean decides.
  assert.equal(scopeKindPref(null, true), 'automations');
  assert.equal(scopeKindPref(null, false), 'broadcasts');
  assert.equal(scopeKindPref(null, null), 'broadcasts');
  assert.equal(scopeKindPref('ALL', true), 'automations');
  assert.equal(scopeKindPref(true, null), 'broadcasts');
});

// G5 feedback: a NEW browser session -- no scopeKind pref, no legacy pref (utils.getPref returns
// null for both, including when localStorage throws), no route override -- lands on Broadcasts,
// never All, although All is now the pill's FIRST button.
test('POLISH3 G5: a fresh session defaults to Broadcasts (evergreen false), not All', () => {
  assert.equal(scopeKindPref(null, null), 'broadcasts');
  const fresh = campaignsRouteQuery({}, evergreenOfKind(scopeKindPref(null, null)));
  assert.equal(fresh.evergreen, false);
  assert.equal(kindOfEvergreen(fresh.evergreen), 'broadcasts');
  assert.deepEqual(kindStatusParams({ evergreen: fresh.evergreen, routeStatus: routeStatusOf({}) }), { evergreen: false });
});

test('POLISH3 I3: kind <-> evergreen -- All is null', () => {
  assert.equal(evergreenOfKind('broadcasts'), false);
  assert.equal(evergreenOfKind('automations'), true);
  assert.equal(evergreenOfKind('all'), null);
  assert.equal(evergreenOfKind('bogus'), false);
  assert.equal(kindOfEvergreen(false), 'broadcasts');
  assert.equal(kindOfEvergreen(true), 'automations');
  assert.equal(kindOfEvergreen(null), 'all');
  SCOPE_KINDS.forEach((k) => assert.equal(kindOfEvergreen(evergreenOfKind(k)), k));
});

test('POLISH3 I3: route evergreen all/true/false override the pref for the load; anything else keeps it', () => {
  [true, false, null].forEach((pref) => {
    assert.equal(campaignsRouteQuery({ evergreen: 'all' }, pref).evergreen, null);
    assert.equal(campaignsRouteQuery({ evergreen: 'true' }, pref).evergreen, true);
    assert.equal(campaignsRouteQuery({ evergreen: 'false' }, pref).evergreen, false);
    ['ALL', 'All', '', 'both'].forEach((v) => assert.equal(campaignsRouteQuery({ evergreen: v }, pref).evergreen, pref, v));
    assert.equal(campaignsRouteQuery({}, pref).evergreen, pref);
  });
  // The pref read through the migration: a standing All pref is null.
  assert.equal(campaignsRouteQuery({}, evergreenOfKind(scopeKindPref('all', true))).evergreen, null);
  assert.deepEqual(
    campaignsRouteQuery({ list_id: ['4', '8'], evergreen: 'all' }, false),
    { listID: null, routeListIDs: [4, 8], evergreen: null },
  );
});

test('POLISH3 D3 (stage 2 F2): the three boolean consumers of evergreen are pinned', () => {
  // By-query delete: omits evergreen under All.
  assert.deepEqual(deleteEvergreenParam(null), {});
  assert.deepEqual(deleteEvergreenParam(false), { evergreen: false });
  assert.deepEqual(deleteEvergreenParam(true), { evergreen: true });
  // Progress polling: whenever broadcasts are displayed (All included).
  assert.equal(pollsStats(false), true);
  assert.equal(pollsStats(null), true);
  assert.equal(pollsStats(true), false);
  // New pre-enables the evergreen toggle only from Automations.
  assert.equal(preEnablesEvergreen(true), true);
  assert.equal(preEnablesEvergreen(false), false);
  assert.equal(preEnablesEvergreen(null), false);
});

test('POLISH3 D3: the poll follows displayed running BROADCASTS only (a running evergreen never ends it under All)', () => {
  assert.equal(displayedRunning([{ status: 'running', evergreen: false }]), true);
  assert.equal(displayedRunning([{ status: 'running', evergreen: true }]), false);
  assert.equal(displayedRunning([{ status: 'running', evergreen: true }, { status: 'running' }]), true);
  assert.equal(displayedRunning([{ status: 'finished', evergreen: false }]), false);
  assert.equal(displayedRunning(null), false);
});

// Fork (matrix polish, integrations MATRIX-POLISH-SPEC I5) -- the route id scope: route id -> query
// state, kind All for the view with the pref untouched, select-all suppressed, cleared by the
// STATUS_CLEARING interactions.
test('MATRIX-POLISH I5: route id -> the id scope (one or repeated; garbage dropped, order kept)', () => {
  assert.deepEqual(routeIdsOf({ id: '41' }), [41]);
  assert.deepEqual(routeIdsOf({ id: ['57', '41', '57'] }), [57, 41]);
  assert.deepEqual(routeIdsOf({ id: ['x', '0', '-3', '12'] }), [12]);
  assert.equal(routeIdsOf({ id: ['x', ''] }), null);
  assert.equal(routeIdsOf({}), null);
  assert.equal(routeIdsOf(null), null);
  // The Brand Usage link's whole query: id + evergreen=all; the list parse is untouched by id.
  const q = { id: ['41', '57'], evergreen: 'all' };
  assert.deepEqual(campaignsRouteQuery(q, false), { listID: null, routeListIDs: null, evergreen: null });
  assert.deepEqual(kindStatusParams({ evergreen: null, routeStatus: null, routeIDs: routeIdsOf(q) }), { id: [41, 57] });
  assert.deepEqual(kindStatusParams({ evergreen: false, routeStatus: null, routeIDs: null }), { evergreen: false });
});

test('MATRIX-POLISH I5: the kind is All for the id view, whatever the route/pref says; the pref is untouched', () => {
  const pref = Object.freeze({ kind: 'broadcasts' });
  assert.equal(routeEvergreen(false, [41]), null);
  assert.equal(routeEvergreen(true, [41]), null);
  assert.equal(routeEvergreen(false, null), false);
  assert.equal(routeEvergreen(true, []), true);
  // An id link with no evergreen param on a Broadcasts pref still lands on All.
  const q = { id: '41' };
  assert.equal(routeEvergreen(campaignsRouteQuery(q, false).evergreen, routeIdsOf(q)), null);
  assert.equal(kindOfEvergreen(routeEvergreen(false, [41])), 'all');
  assert.deepEqual(pref, { kind: 'broadcasts' });
});

test('MATRIX-POLISH I5: the id scope is a route scope -- "select all N" is never offered under it', () => {
  const base = { total: 100, perPage: 20 };
  assert.equal(canSelectAll(base), true);
  assert.equal(canSelectAll({ ...base, routeIDs: [41, 57] }), false);
  assert.equal(canSelectAll({ ...base, routeIDs: [] }), true);
  assert.equal(canSelectAll({ ...base, routeIDs: routeIdsAfter([41], 'dismiss') }), true);
});

test('MATRIX-POLISH I5: cleared by exactly the STATUS_CLEARING interactions; others keep it', () => {
  assert.deepEqual(STATUS_CLEARING, ['dismiss', 'list', 'brand', 'scope']);
  STATUS_CLEARING.forEach((i) => assert.equal(routeIdsAfter([41, 57], i), null, i));
  ['page', 'sort', 'search', undefined].forEach((i) => assert.deepEqual(routeIdsAfter([41, 57], i), [41, 57], String(i)));
  assert.equal(routeIdsAfter(null, 'page'), null);
  assert.equal(routeIdsAfter([], 'page'), null);
  // The helpers write nothing.
  const ids = Object.freeze([41]);
  routeIdsAfter(ids, 'scope');
  kindStatusParams({ routeIDs: ids });
  canSelectAll({ total: 100, perPage: 20, routeIDs: ids });
  assert.deepEqual(ids, [41]);
});

// SUBSCRIBERS-FILTERS-2-SPEC I11 -- the Campaigns kind narrowing: Broadcasts offers only
// has_broadcasts, Automations only has_automations, All everything; the selected list is always
// present; a scope change resets a non-qualifying listID to null and leaves routeListIDs alone.
const KIND_LISTS = {
  results: [
    {
      id: 1, name: 'Ruze Emails', brand: 'ruze', hasBroadcasts: true, hasAutomations: true,
    },
    {
      id: 2, name: 'Ruze winback', brand: 'ruze', has_broadcasts: true, has_automations: false,
    },
    {
      id: 3, name: 'RUZE First order', brand: 'ruze', journeyLocked: true, hasBroadcasts: false, hasAutomations: true,
    },
    {
      id: 4, name: 'Ruze empty', brand: 'ruze', hasBroadcasts: false, hasAutomations: false,
    },
    {
      id: 5, name: 'Bnkd Emails', brand: 'bnkd', hasBroadcasts: true, hasAutomations: true,
    },
  ],
};
const optIds = (g) => [...g.ordinary.map((l) => l.id), '|', ...g.system.map((l) => l.id)];

test('SUBSCRIBERS-FILTERS-2 I11: the Lists dropdown narrows to the scope pill\'s kind', () => {
  // Broadcasts / Automations / All over every list (All brands: scopeListIds null).
  assert.deepEqual(optIds(campaignListOptions(KIND_LISTS, null, false)), [5, 1, 2, '|']);
  assert.deepEqual(optIds(campaignListOptions(KIND_LISTS, null, true)), [5, 1, '|', 3]);
  assert.deepEqual(optIds(campaignListOptions(KIND_LISTS, null, null)), [5, 1, 4, 2, '|', 3]);
  // Within a brand scope.
  assert.deepEqual(optIds(campaignListOptions(KIND_LISTS, [1, 2, 3, 4], true)), [1, '|', 3]);
  // The selected list is always an option, grouped by its kind of list; an unknown one is #<id>.
  assert.deepEqual(optIds(campaignListOptions(KIND_LISTS, [1, 2, 3, 4], false, 3)), [1, 2, '|', 3]);
  assert.deepEqual(optIds(campaignListOptions(KIND_LISTS, [1, 2, 3, 4], true, 4)), [1, 4, '|', 3]);
  assert.deepEqual(campaignListOptions(KIND_LISTS, null, false, 99).ordinary.find((l) => l.id === 99), { id: 99, name: '#99' });
  // The flag must be the server's true.
  assert.equal(listQualifies({ hasBroadcasts: 'true' }, false), false);
  assert.equal(listQualifies({}, true), false);
  assert.equal(listQualifies({}, null), true);
});

test('SUBSCRIBERS-FILTERS-2 I11: a scope change resets a non-qualifying list, nothing else', () => {
  assert.equal(afterScopeChange(2, KIND_LISTS, true), null); // broadcasts only
  assert.equal(afterScopeChange(2, KIND_LISTS, false), 2);
  assert.equal(afterScopeChange(3, KIND_LISTS, false), null);
  assert.equal(afterScopeChange(3, KIND_LISTS, true), 3);
  assert.equal(afterScopeChange(4, KIND_LISTS, null), 4); // All: no narrowing
  assert.equal(afterScopeChange(99, KIND_LISTS, null), 99);
  assert.equal(afterScopeChange(99, KIND_LISTS, true), null); // unknown: cannot qualify
  assert.equal(afterScopeChange(null, KIND_LISTS, true), null);
  // The page's wiring: on the pill change only (not on mount), before the page/bulk reset; the
  // route list scope is not touched; the store lists are refreshed on mount.
  const vue = readFileSync(new URL('./views/Campaigns.vue', import.meta.url), 'utf8');
  const scope = vue.slice(vue.indexOf('onScopeChange() {'), vue.indexOf('this.refreshCampaigns();', vue.indexOf('onScopeChange() {')));
  assert.match(scope, /this\.queryParams\.listID = afterScopeChange\(this\.queryParams\.listID, this\.lists, this\.queryParams\.evergreen\);/);
  assert.match(scope, /this\.queryParams\.page = 1;/);
  assert.match(scope, /this\.bulk\.checked = \[\];/);
  assert.doesNotMatch(scope, /queryParams\.routeListIDs/);
  assert.equal((vue.match(/afterScopeChange\(/g) || []).length, 1);
  assert.match(vue, /return campaignListOptions\(this\.lists, this\.scopeListIds, this\.queryParams\.evergreen, this\.queryParams\.listID\);/);
  const mounted = vue.slice(vue.indexOf('  mounted() {'));
  assert.match(mounted, /this\.\$api\.getLists\(\{ minimal: true, per_page: 'all', status: 'active' \}\)/);
});

// Integrations PAGE-STATE-SPEC I3/I4/I5 -- the Campaigns page's session state.
const PAGE_LISTS = [
  { id: 5, name: 'A', has_broadcasts: true },
  { id: 6, name: 'B', has_automations: true },
  { id: 7, name: 'C', has_broadcasts: true },
];

test('PAGE-STATE I3: only the visible page-set fields round-trip -- never a link scope, selection or modal', () => {
  const shown = {
    query: 'bfcm', listID: 5, routeStatus: 'finished', orderBy: 'name', order: 'asc', page: 3,
  };
  const scopes = {
    routeIDs: [1, 2], routeListIDs: [5, 7], evergreen: true, bulk: { checked: [{ id: 1 }], all: true }, previewItem: { id: 1 },
  };
  const others = { evergreen: true, bulk: { checked: [{ id: 1 }], all: true }, previewItem: { id: 1 } };
  const saved = saveCampaignsPage({ ...shown, ...others });
  assert.deepEqual(Object.keys(saved).sort(), ['listID', 'order', 'orderBy', 'page', 'query', 'routeStatus']);
  assert.deepEqual(restoreCampaignsPage(JSON.parse(JSON.stringify(saved)), {}, [5, 7]), {
    query: 'bfcm', listID: 5, routeStatus: 'finished', orderBy: 'name', order: 'asc', page: 3,
  });
  // Even a stored object carrying them never restores a scope or a selection.
  const r = restoreCampaignsPage({ ...saved, ...scopes }, {}, [5]);
  ['routeIDs', 'routeListIDs', 'bulk', 'evergreen', 'previewItem'].forEach((k) => assert.equal(k in r, false, k));
  assert.equal(r.page, 3);
});

test('Review R1(c): a fetch under a route id or route list scope saves page 1', () => {
  assert.equal(saveCampaignsPage({ page: 4, routeIDs: [1, 2] }).page, 1);
  assert.equal(saveCampaignsPage({ page: 4, routeListIDs: [5, 7] }).page, 1);
  assert.equal(saveCampaignsPage({ page: 4, routeIDs: null, routeListIDs: null }).page, 4);
  assert.equal(saveCampaignsPage({ page: 4, routeIDs: [] }).page, 4);
});

test('PAGE-STATE I4: any of the page\'s query parameters and the link wins', () => {
  const stored = saveCampaignsPage({ query: 'bfcm', listID: 5, page: 2 });
  assert.deepEqual([...CAMPAIGNS_LINK_KEYS], ['list_id', 'evergreen', 'status', 'id']);
  CAMPAIGNS_LINK_KEYS.forEach((k) => {
    assert.equal(restoreCampaignsPage(stored, { [k]: '1' }, [5]), null, k);
  });
  assert.equal(restoreCampaignsPage(stored, { list_id: ['5', '7'] }, [5]), null);
  assert.equal(restoreCampaignsPage(null, {}, [5]), null);
  // A bare entry restores.
  assert.equal(restoreCampaignsPage(stored, {}, [5]).query, 'bfcm');
  assert.equal(restoreCampaignsPage(stored, undefined, [5]).listID, 5);
});

test('PAGE-STATE I5: a stored list outside the offered set is All lists; unknown enums and bad pages default', () => {
  const offered = offeredCampaignListIds(PAGE_LISTS, [5, 6], false);
  assert.deepEqual(offered, [5]);
  assert.deepEqual(offeredCampaignListIds(PAGE_LISTS, null, null), [5, 6, 7]);
  assert.equal(restoreCampaignsPage({ listID: 7 }, {}, offered).listID, null);
  assert.equal(restoreCampaignsPage({ listID: 6 }, {}, offered).listID, null);
  assert.equal(restoreCampaignsPage({ listID: 5 }, {}, offered).listID, 5);
  // Undefined offered set: the id passes through for the caller's later keptListId.
  assert.equal(restoreCampaignsPage({ listID: 7 }, {}).listID, 7);
  assert.equal(restoreCampaignsPage({ listID: '7' }, {}).listID, null);
  const bad = restoreCampaignsPage({ routeStatus: 'exploded', orderBy: 'views', order: 'up' }, {}, []);
  assert.equal(bad.routeStatus, null);
  assert.equal(bad.orderBy, 'created_at');
  assert.equal(bad.order, 'desc');
  [0, -2, 'x', 1.5].forEach((p) => assert.equal(restoreCampaignsPage({ page: p }, {}, []).page, 1, String(p)));
  assert.equal(restoreCampaignsPage({ query: 42 }, {}, []).query, '');
});
