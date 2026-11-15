// Fork (integrations SUBSCRIBERS-FILTERS-2-SPEC I10) -- listGroups.mjs: the one grouping of a Lists
// dropdown, ordinary and System (journey_locked) lists each sorted by name, used by both pages.
// Run: node --test src/listGroups.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { groupLists } from './listGroups.mjs'; // eslint-disable-line import/extensions

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');

test('groupLists: ordinary then system, each by name', () => {
  const g = groupLists([
    { id: 1, name: 'Zeta' },
    { id: 2, name: 'alpha' },
    { id: 3, name: 'RUZE No-purchase D2', journeyLocked: true },
    { id: 4, name: 'RUZE First order', journey_locked: true },
    { id: 5, name: 'Beta', journey_locked: false },
    null,
  ]);
  assert.deepEqual(g.ordinary, [{ id: 2, name: 'alpha' }, { id: 5, name: 'Beta' }, { id: 1, name: 'Zeta' }]);
  assert.deepEqual(g.system, [{ id: 4, name: 'RUZE First order' }, { id: 3, name: 'RUZE No-purchase D2' }]);
  assert.deepEqual(groupLists(null), { ordinary: [], system: [] });
  assert.deepEqual(groupLists([{ id: 7 }]), { ordinary: [{ id: 7, name: '' }], system: [] });
});

test('both pages render the two groups', () => {
  const subs = read('./views/Subscribers.vue');
  const camps = read('./views/Campaigns.vue');
  assert.match(subs, /<optgroup v-if="listOpts\.system\.length" :label="\$t\('subscribers\.systemLists'\)">/);
  assert.match(camps, /<option v-for="l in filterLists\.ordinary" :key="l\.id" :value="l\.id">/);
  assert.match(camps, /<optgroup v-if="filterLists\.system\.length" :label="\$t\('subscribers\.systemLists'\)">/);
  assert.match(camps, /<option v-for="l in filterLists\.system" :key="l\.id" :value="l\.id">/);
  // The Campaigns "n linked lists" route-scope option keeps its place, first.
  const sel = camps.slice(camps.indexOf('data-cy="filter-list"'));
  assert.ok(sel.indexOf('filter-list-route') < sel.indexOf('campaigns.allLists'));
  assert.ok(sel.indexOf('campaigns.allLists') < sel.indexOf('filterLists.ordinary'));
});
