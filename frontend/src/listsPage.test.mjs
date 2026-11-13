// Fork (campaign lists, integrations CAMPAIGN-LISTS-SPEC I9). Run: node --test src/listsPage.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';

import { SHOW_SYSTEM_PREF, showSystemLists, systemListsParams } from './listsPage.mjs'; // eslint-disable-line import/extensions

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
