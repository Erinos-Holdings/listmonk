// Fork (brand usage, integrations BRAND-USAGE-V2-SPEC I8; DASHBOARDS-POLISH2-SPEC I2). Run: yarn test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  ROUTE_SCOPE_OPTION, afterBrandChange, campaignsRouteQuery, canSelectAll, listParam, listSelectValue, selectList,
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
  // After the route scope clears, the affordance comes back.
  const cleared = selectList({ listID: null, routeListIDs: [3, 9] }, null);
  assert.equal(canSelectAll({ ...base, ...cleared }), true);
});
