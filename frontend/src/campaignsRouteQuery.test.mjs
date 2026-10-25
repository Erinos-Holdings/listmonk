// Fork (brand usage, integrations listmonk/BRAND-USAGE-V2-SPEC I8). Run: yarn test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { campaignsRouteQuery } from './campaignsRouteQuery.mjs'; // eslint-disable-line import/extensions

test('I8: a numeric list_id string selects the list; garbage is ignored', () => {
  assert.equal(campaignsRouteQuery({ list_id: '12' }).listID, 12);
  ['', 'abc', '12abc', '-3', '0', '1.5', ' 7', '012', '99999999999999999999'].forEach((v) => {
    assert.equal(campaignsRouteQuery({ list_id: v }).listID, null, v);
  });
  assert.equal(campaignsRouteQuery({}).listID, null);
  assert.equal(campaignsRouteQuery(null).listID, null);
  assert.equal(campaignsRouteQuery({ list_id: 12 }).listID, null); // the router only hands strings
});

test('I8: a repeated param takes its first element', () => {
  assert.equal(campaignsRouteQuery({ list_id: ['5', '9'] }).listID, 5);
  assert.equal(campaignsRouteQuery({ list_id: ['x', '9'] }).listID, null);
  assert.equal(campaignsRouteQuery({ evergreen: ['true', 'false'] }).evergreen, true);
});

test('I8: evergreen=true selects Automations; otherwise the sticky pref stands', () => {
  assert.equal(campaignsRouteQuery({ evergreen: 'true' }, false).evergreen, true);
  assert.equal(campaignsRouteQuery({}, false).evergreen, false);
  assert.equal(campaignsRouteQuery({}, true).evergreen, true);
  assert.equal(campaignsRouteQuery({ evergreen: '1' }, false).evergreen, false);
  assert.equal(campaignsRouteQuery({ evergreen: 'TRUE' }, false).evergreen, false);
  assert.deepEqual(campaignsRouteQuery({ list_id: '7', evergreen: 'true' }, false), { listID: 7, evergreen: true });
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
  assert.deepEqual(campaignsRouteQuery({ list_id: '9', evergreen: 'false' }, true), { listID: 9, evergreen: false });
  // The parse never writes the pref: the same pref in gives the pref back with no override.
  const q = Object.freeze({ evergreen: 'false' });
  campaignsRouteQuery(q, true);
  assert.equal(campaignsRouteQuery({}, true).evergreen, true);
  assert.deepEqual(q, { evergreen: 'false' });
});

test('I8: the parse writes nothing -- it is a pure function of its inputs', () => {
  const q = Object.freeze({ list_id: '7', evergreen: 'true' });
  assert.deepEqual(campaignsRouteQuery(q, false), { listID: 7, evergreen: true });
  assert.deepEqual(q, { list_id: '7', evergreen: 'true' });
});
