// Fork (campaign lists, integrations CAMPAIGN-LISTS-SPEC I9). Run: node --test src/listsPage.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';

import {
  LISTS_LINK_KEYS, SHOW_SYSTEM_PREF, restoreListsPage, saveListsPage, showSystemLists, systemListsParams,
} from './listsPage.mjs'; // eslint-disable-line import/extensions

// Integrations PAGE-STATE-SPEC I3/I4/I5 -- the Lists page's session state.
test('PAGE-STATE I3: search, sort and page round-trip; never the status (R5), a selection or a form', () => {
  const shown = {
    query: 'ruze', status: 'active', orderBy: 'held_count', order: 'asc', page: 2,
  };
  const saved = saveListsPage({
    ...shown, bulk: { all: true }, isFormVisible: true, expanded: { 1: true },
  });
  assert.deepEqual(restoreListsPage(JSON.parse(JSON.stringify({ ...saved, status: 'archived', bulk: { all: true } })), {}), {
    query: 'ruze', orderBy: 'held_count', order: 'asc', page: 2,
  });
});

test('Review R1(c): a fetch of the archived view (a link) saves page 1', () => {
  assert.equal(saveListsPage({ status: 'archived', page: 3 }).page, 1);
  assert.equal(saveListsPage({ status: 'active', page: 3 }).page, 3);
  assert.equal('status' in saveListsPage({ status: 'archived' }), false);
});

test('PAGE-STATE I4: a route ?status= wins', () => {
  assert.deepEqual([...LISTS_LINK_KEYS], ['status']);
  const stored = saveListsPage({ query: 'ruze', page: 3 });
  assert.equal(restoreListsPage(stored, { status: 'active' }), null);
  assert.equal(restoreListsPage(stored, { status: 'archived' }), null);
  assert.equal(restoreListsPage(null, {}), null);
  assert.equal(restoreListsPage(stored, {}).page, 3);
});

test('PAGE-STATE I5: unknown enums and bad pages default', () => {
  const r = restoreListsPage({ orderBy: 'secret', order: 'up' }, {});
  assert.deepEqual([r.orderBy, r.order], ['created_at', 'desc']);
  [0, -2, 'x', 1.5].forEach((p) => assert.equal(restoreListsPage({ page: p }, {}).page, 1, String(p)));
});

test('I9: the toggle defaults off; only exactly true shows the system lists', () => {
  assert.equal(SHOW_SYSTEM_PREF, 'lists.showSystem');
  assert.equal(showSystemLists(null), false);
  assert.equal(showSystemLists(undefined), false);
  assert.equal(showSystemLists(false), false);
  assert.equal(showSystemLists('true'), false);
  assert.equal(showSystemLists(1), false);
  assert.equal(showSystemLists({}), false);
  assert.equal(showSystemLists(true), true);
});

test('I9: hide_system=true is sent exactly when the toggle is off', () => {
  assert.deepEqual(systemListsParams(false), { hide_system: true });
  assert.deepEqual(systemListsParams(null), { hide_system: true });
  assert.deepEqual(systemListsParams(showSystemLists(null)), { hide_system: true });
  assert.deepEqual(systemListsParams(true), {});
});
