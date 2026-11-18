// Fork (integrations listmonk/CAMPAIGN-STATUS-FILTER-SPEC I7-I10). Run: yarn test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  campaignFilterParams, pausedAtOf, statusCountParams, statusOptions,
} from './campaignStatusFilter.mjs'; // eslint-disable-line import/extensions
import { CAMPAIGN_STATUSES, pickStatus } from './campaignsRouteQuery.mjs'; // eslint-disable-line import/extensions

const TS = '2026-11-17T10:00:00Z';

test('I7: the Paused row shows only for a paused row with paused_at set', () => {
  assert.equal(pausedAtOf({ status: 'paused', pausedAt: TS }), TS);
  assert.equal(pausedAtOf({ status: 'paused', pausedAt: null }), null);
  assert.equal(pausedAtOf({ status: 'paused' }), null);
  // A resumed campaign keeps its stored paused_at; it is not displayed.
  assert.equal(pausedAtOf({ status: 'running', pausedAt: TS }), null);
  ['draft', 'scheduled', 'finished', 'cancelled'].forEach((s) => assert.equal(pausedAtOf({ status: s, pausedAt: TS }), null, s));
  assert.equal(pausedAtOf(null), null);
});

test('I8: All first, then the present statuses in CAMPAIGN_STATUSES order, plus the selected at 0', () => {
  // The server's order (enum declaration) is not the display order.
  const counts = [{ status: 'draft', count: 3 }, { status: 'running', count: 1 }, { status: 'paused', count: 7 },
    { status: 'finished', count: 12 }];
  assert.deepEqual(statusOptions(counts, null), [
    { value: null, count: null },
    { value: 'draft', count: 3 },
    { value: 'running', count: 1 },
    { value: 'paused', count: 7 },
    { value: 'finished', count: 12 },
  ]);
  // The display order follows CAMPAIGN_STATUSES (scheduled before running).
  const order = statusOptions(CAMPAIGN_STATUSES.slice().reverse().map((status) => ({ status, count: 1 })), null)
    .slice(1).map((o) => o.value);
  assert.deepEqual(order, CAMPAIGN_STATUSES);
  // The selected status stays visible (and reversible) at 0, in its place.
  assert.deepEqual(statusOptions(counts, 'cancelled').map((o) => o.value), [null, 'draft', 'running', 'paused', 'finished', 'cancelled']);
  assert.deepEqual(statusOptions(counts, 'cancelled').at(-1), { value: 'cancelled', count: 0 });
  assert.deepEqual(statusOptions([], 'scheduled'), [{ value: null, count: null }, { value: 'scheduled', count: 0 }]);
  // A selected status that IS present is listed once, with its count.
  assert.equal(statusOptions(counts, 'paused').filter((o) => o.value === 'paused').length, 1);
  // Zero / unknown / malformed rows are not present statuses.
  const junk = [{ status: 'draft', count: 0 }, { status: 'bogus', count: 4 }, null, { status: 'paused' }];
  assert.deepEqual(statusOptions(junk, null), [{ value: null, count: null }]);
  // No counts (a failed request): All plus the six statuses uncounted.
  assert.deepEqual(statusOptions(null, 'paused'), [{ value: null, count: null }, ...CAMPAIGN_STATUSES.map((value) => ({ value, count: null }))]);
});

test('I9: the statuses request carries the list request\'s filter params minus status, never paging/order', () => {
  const base = {
    query: '', evergreen: false, routeStatus: null, routeIDs: null, listID: null, routeListIDs: null, scopeListIds: null, langSelection: 'all',
  };
  const cases = [
    ['no filters', {}, { query: '', evergreen: false }],
    ['a list', { listID: 4 }, { query: '', evergreen: false, list_id: 4 }],
    ['a route list scope', { routeListIDs: [5, 9] }, { query: '', evergreen: false, list_id: [5, 9] }],
    ['the brand scope', { scopeListIds: [2, 3] }, { query: '', evergreen: false, list_id: [2, 3] }],
    ['a route id scope (kind All)', { routeIDs: [11, 12], evergreen: null }, { query: '', id: [11, 12] }],
    ['a language', { langSelection: 'fr' }, { query: '', evergreen: false, lang: 'fr' }],
    ['a search (sanitised)', { query: 'spring: sale!' }, { query: 'spring  sale ', evergreen: false }],
    ['Automations', { evergreen: true }, { query: '', evergreen: true }],
    ['a set status', { routeStatus: 'paused' }, { query: '', evergreen: false }],
  ];
  cases.forEach(([name, over, wantCounts]) => {
    const state = { ...base, ...over };
    const list = campaignFilterParams(state);
    const counts = statusCountParams(state);
    assert.deepEqual(counts, wantCounts, name);
    // Exactly the list's filter params minus status.
    const { status, ...rest } = list;
    assert.deepEqual(counts, rest, name);
    assert.equal(status, state.routeStatus === 'paused' ? 'paused' : undefined, name);
    ['page', 'per_page', 'order_by', 'order', 'no_body', 'status'].forEach((k) => assert.equal(k in counts, false, `${name}: ${k}`));
    ['page', 'per_page', 'order_by', 'order', 'no_body'].forEach((k) => assert.equal(k in list, false, `${name}: list ${k}`));
  });
  // The single filter beats the route list scope, which beats the brand scope (listParam's precedence).
  const all3 = {
    ...base, listID: 4, routeListIDs: [5, 9], scopeListIds: [2],
  };
  assert.equal(statusCountParams(all3).list_id, 4);
  assert.deepEqual(statusCountParams({ ...base, routeListIDs: [5, 9], scopeListIds: [2] }).list_id, [5, 9]);
});

test('I10: a status pick sets the one status state, page 1, and clears the selection; the id scope is kept', () => {
  const state = {
    page: 4, routeStatus: 'draft', routeIDs: [7, 8], bulk: { checked: [{ id: 1 }, { id: 2 }], all: true },
  };
  const picked = pickStatus(state, 'paused');
  assert.deepEqual(picked, {
    routeStatus: 'paused', routeIDs: [7, 8], page: 1, bulk: { checked: [], all: false },
  });
  const all = pickStatus(state, null);
  assert.deepEqual(all, {
    routeStatus: null, routeIDs: [7, 8], page: 1, bulk: { checked: [], all: false },
  });
  // Anything not in the enum reads as All; no id scope stays none.
  assert.equal(pickStatus({ routeIDs: null }, 'bogus').routeStatus, null);
  assert.equal(pickStatus({}, 'running').routeIDs, null);
  // The input is not mutated.
  assert.equal(state.bulk.all, true);
  assert.equal(state.bulk.checked.length, 2);
});
