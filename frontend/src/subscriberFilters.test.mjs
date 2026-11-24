// Fork (Subscribers filters) -- integrations listmonk/SUBSCRIBERS-FILTERS-SPEC I5, I7, I8, I9, I11
// and I12: the router guard's route fix, the Lists dropdown, the list/status navigation, the status
// gate, the page's controls and its count formatting.
// Run: node --test src/subscriberFilters.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  LANG_CELL_EMPTY, SEGMENTS, langCell, langColumnShown, listOptions, listRoute, routeListIdOf, statusEnabled,
  statusQuery, subscribersRouteFix,
  isBareSubscribersRoute, offeredSubscriberListIds, restoreSubscribersPage, saveSubscribersPage, subscribersRestoreRoute,
} from './subscriberFilters.mjs'; // eslint-disable-line import/extensions
import { RESTORE_PARAM } from './pageState.mjs'; // eslint-disable-line import/extensions
import { LANG_VALUES, SUBSCRIBER_ROUTES } from './langScope.mjs'; // eslint-disable-line import/extensions

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');
const to = (name, query = {}, params = {}) => ({
  name, query, params, path: '/x', hash: '',
});
const LIST = (query = {}) => to('subscribers_list', query, { listID: '5' });

// I5 -- D3.1-D3.6 and their combinations.
test('route fix', () => {
  // D3.6: agreeing values, nothing to do.
  assert.deepEqual(subscribersRouteFix(to('subscribers'), 'all', true), {});
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'fr' }), 'fr', true), {});
  assert.deepEqual(subscribersRouteFix(LIST({ segment: 'held', lang: 'other' }), 'other', true), {});
  // Not a Subscribers route: never.
  ['lists', 'campaigns', 'dashboard', undefined].forEach((n) => {
    assert.deepEqual(subscribersRouteFix(to(n, { lang: 'pt', segment: 'x' }), 'fr', true), {});
  });
  assert.deepEqual(subscribersRouteFix(null, 'fr', true), {});

  // D3.1: toggle hidden -- any route lang dropped, nothing added, nothing committed.
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'fr', search: 'a' }), 'fr', false), { query: { search: 'a' } });
  assert.deepEqual(subscribersRouteFix(to('subscribers'), 'fr', false), {});
  assert.deepEqual(subscribersRouteFix(LIST({ lang: 'none', segment: 'held' }), 'all', false), { query: { segment: 'held' } });

  // D3.2: a value the toggle cannot hold is dropped (then D3.4 applies under a selection).
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'pt' }), 'all', true), { query: {} });
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'all' }), 'all', true), { query: {} });
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: ['fr', 'es'] }), 'all', true), { query: {} });
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'pt' }), 'es', true), { query: { lang: 'es' } });

  // D3.3: a toggle value different from the selection is committed; the query is unchanged.
  LANG_VALUES.filter((v) => v !== 'all').forEach((v) => {
    assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: v }), 'all', true), { commitLang: v }, v);
  });
  assert.deepEqual(subscribersRouteFix(LIST({ lang: 'other', segment: 'active' }), 'fr', true), { commitLang: 'other' });
  // A stored en_only selection (erinos.457) normalises to All: the route's en is committed.
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'en' }), 'en_only', true), { commitLang: 'en' });

  // SUBSCRIBERS-FILTERS-2-SPEC I2 -- a route lang=en_only or lang=none (an erinos.457 link or
  // bookmark) is dropped and, under a non-All selection, replaced by lang=<selection>; it never
  // commits a toggle value.
  ['en_only', 'none'].forEach((v) => {
    assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: v }), 'all', true), { query: {} }, v);
    assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: v, search: 's' }), 'fr', true), { query: { search: 's', lang: 'fr' } }, v);
    assert.deepEqual(subscribersRouteFix(LIST({ lang: v, segment: 'active' }), 'en', true), { query: { segment: 'active', lang: 'en' } }, v);
    assert.deepEqual(subscribersRouteFix(LIST({ lang: v }), 'other', true), { query: { lang: 'other' } }, v);
    // A stale selection of the same value is All: dropped, nothing added, nothing committed.
    assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: v }), v, true), { query: {} }, v);
    SUBSCRIBER_ROUTES.forEach((r) => {
      const fix = subscribersRouteFix(to(r, { lang: v }, { listID: '5', id: '9' }), 'de', true);
      assert.equal(fix.commitLang, undefined, `${r} ${v}`);
      assert.equal(fix.query.lang, 'de', `${r} ${v}`);
    });
  });

  // D3.4: no route lang, a selection -- the raw value is added (segment links included).
  assert.deepEqual(subscribersRouteFix(to('subscribers', { search: 'x' }), 'es', true), { query: { search: 'x', lang: 'es' } });
  assert.deepEqual(subscribersRouteFix(LIST({ segment: 'held' }), 'other', true), { query: { segment: 'held', lang: 'other' } });
  SUBSCRIBER_ROUTES.forEach((r) => {
    assert.equal(subscribersRouteFix(to(r, {}, { listID: '5', id: '9' }), 'other', true).query.lang, 'other', r);
  });

  // D3.5: a segment without a route list, or outside the five, is dropped.
  assert.deepEqual(subscribersRouteFix(to('subscribers', { segment: 'held' }), 'all', true), { query: {} });
  assert.deepEqual(subscribersRouteFix(to('subscriber', { segment: 'held' }, { id: '3' }), 'all', true), { query: {} });
  assert.deepEqual(subscribersRouteFix(LIST({ segment: 'bogus' }), 'all', true), { query: {} });
  assert.deepEqual(subscribersRouteFix(to('subscribers_list', { segment: 'held' }, { listID: 'abc' }), 'all', true), { query: {} });
  // A list route keeps each of the five.
  SEGMENTS.forEach((s) => assert.deepEqual(subscribersRouteFix(LIST({ segment: s }), 'all', true), {}, s));

  // Combinations: hidden + stray segment; invalid lang + stray segment under a selection; commit
  // plus a dropped segment.
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'fr', segment: 'held' }), 'fr', false), { query: {} });
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'pt', segment: 'held' }), 'de', true), { query: { lang: 'de' } });
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'other', segment: 'held' }), 'fr', true), { commitLang: 'other', query: { lang: 'other' } });
  assert.deepEqual(subscribersRouteFix(to('subscribers', { lang: 'none', segment: 'held' }), 'fr', true), { query: { lang: 'fr' } });
  // The input query is never edited in place.
  const q = { lang: 'pt', segment: 'held' };
  subscribersRouteFix(to('subscribers', q), 'fr', true);
  assert.deepEqual(q, { lang: 'pt', segment: 'held' });

  assert.equal(routeListIdOf({ listID: '12' }), 12);
  assert.equal(routeListIdOf({ listID: 12 }), 12);
  assert.equal(routeListIdOf({ listID: '12x' }), null);
  assert.equal(routeListIdOf({ listID: '0' }), null);
  assert.equal(routeListIdOf({}), null);
});

const LISTS = {
  results: [
    { id: 1, name: 'Zeta', brand: 'ruze' },
    { id: 2, name: 'alpha', brand: 'ruze' },
    {
      id: 3, name: 'RUZE No-purchase D2', brand: 'ruze', journeyLocked: true,
    },
    {
      id: 4, name: 'RUZE First order', brand: 'ruze', journey_locked: true,
    },
    { id: 5, name: 'Shala', brand: 'shala' },
    { id: 6, name: 'Unbranded', brand: '' },
  ],
};
const RUZE = { kind: 'brand', slug: 'ruze' };
const ids = (xs) => xs.map((x) => x.id);

// I7 -- options and enablement per D4.
test('list options', () => {
  // A brand: its lists, ordinary by name, system lists grouped after.
  const r = listOptions(LISTS, RUZE, null);
  assert.equal(r.enabled, true);
  assert.deepEqual(ids(r.ordinary), [2, 1]);
  assert.deepEqual(ids(r.system), [4, 3]);
  assert.equal(r.extra, null);
  // The route list inside the brand: no extra option.
  assert.equal(listOptions(LISTS, RUZE, 3).extra, null);
  // No brand: the unbranded lists.
  const n = listOptions(LISTS, { kind: 'none' }, null);
  assert.equal(n.enabled, true);
  assert.deepEqual(ids(n.ordinary), [6]);
  assert.deepEqual(n.system, []);
  // All brands, no route list: disabled, nothing to offer.
  assert.deepEqual(listOptions(LISTS, { kind: 'all' }, null), {
    enabled: false, ordinary: [], system: [], extra: null,
  });
  assert.equal(listOptions(LISTS, undefined, null).enabled, false);
  // Fork (integrations BRAND-GROUPS-SPEC I7) -- a group is scoped: enabled, the members' lists.
  const g = listOptions(LISTS, { kind: 'group', group: 'incubating', slugs: ['ruze', 'shala'] }, null);
  assert.equal(g.enabled, true);
  assert.deepEqual(ids(g.ordinary), [2, 5, 1]);
  assert.deepEqual(ids(g.system), [4, 3]);
  assert.equal(listOptions(LISTS, { kind: 'group', group: 'incubating', slugs: ['ruze', 'shala'] }, 6).extra.id, 6);
  // The stored shape (no slugs) is no scope: as All brands.
  assert.equal(listOptions(LISTS, { kind: 'group', group: 'incubating' }, null).enabled, false);
  // All brands, a route list: enabled with exactly that list (All lists is the template's own).
  assert.deepEqual(listOptions(LISTS, { kind: 'all' }, 5), {
    enabled: true, ordinary: [], system: [], extra: { id: 5, name: 'Shala' },
  });
  // Out of the selected brand: an extra option with the store's name.
  assert.deepEqual(listOptions(LISTS, RUZE, 5).extra, { id: 5, name: 'Shala' });
  // Unknown to the store (archived, deleted, not permitted): #<id>.
  assert.deepEqual(listOptions(LISTS, RUZE, 99).extra, { id: 99, name: '#99' });
  assert.deepEqual(listOptions(LISTS, { kind: 'all' }, 99).extra, { id: 99, name: '#99' });
  // An array store works too; an empty store offers nothing.
  assert.deepEqual(ids(listOptions(LISTS.results, RUZE, null).ordinary), [2, 1]);
  assert.deepEqual(listOptions({}, RUZE, null), {
    enabled: true, ordinary: [], system: [], extra: null,
  });
});

// I8 -- the one route builder, and the brandKey watcher's use of it.
test('list navigation', () => {
  const q = {
    segment: 'held', subscription_status: 'unsubscribed', lang: 'fr', search: 'gmail',
  };
  assert.deepEqual(listRoute(q, 7), { name: 'subscribers_list', params: { listID: '7' }, query: { lang: 'fr', search: 'gmail' } });
  assert.deepEqual(listRoute(q, null), { name: 'subscribers', query: { lang: 'fr', search: 'gmail' } });
  assert.deepEqual(listRoute(undefined, null), { name: 'subscribers', query: {} });
  assert.deepEqual(q.segment, 'held'); // not edited in place

  const subs = read('./views/Subscribers.vue');
  const w = subs.slice(subs.indexOf('\n    brandKey(now) {'), subs.indexOf('\n    },', subs.indexOf('\n    brandKey(now) {')));
  assert.match(w, /this\.\$router\.push\(listRoute\(this\.withRouteSearch\(\{ \.\.\.this\.\$route\.query \}\), null\)\);/);
  const pick = subs.slice(subs.indexOf('\n    onListSelect('), subs.indexOf('\n    },', subs.indexOf('\n    onListSelect(')));
  assert.match(pick, /this\.\$router\.push\(listRoute\(this\.withRouteSearch\(\{ \.\.\.this\.\$route\.query \}\), listId\)\);/);
});

// I9 -- the status gate, and the status pick's query.
test('status gate', () => {
  assert.equal(statusEnabled(5), true);
  assert.equal(statusEnabled(null), false);
  assert.equal(statusEnabled(undefined), false);
  assert.equal(statusEnabled(0), false);
  assert.equal(statusEnabled('5'), false); // the page's parsed listID only
  assert.deepEqual(SEGMENTS, ['active', 'pending', 'held', 'unsubscribed', 'blocked']);
  assert.deepEqual(statusQuery({ lang: 'fr', subscription_status: 'x' }, 'held'), { lang: 'fr', segment: 'held' });
  assert.deepEqual(statusQuery({ lang: 'fr', segment: 'held' }, ''), { lang: 'fr' });
  assert.deepEqual(statusQuery({ segment: 'held' }, 'bogus'), {});

  const subs = read('./views/Subscribers.vue');
  assert.match(subs, /statusOn\(\) \{\s*return statusEnabled\(this\.queryParams\.listID\);/);
  assert.match(subs, /data-cy="select-status"[\s\S]*?:disabled="!statusOn"/);
  assert.match(subs, /data-cy="select-list"[\s\S]*?:disabled="!listOpts\.enabled"/);
  assert.match(subs, /listOpts\(\) \{\s*return listOptions\(this\.lists, this\.brandSelection, this\.queryParams\.listID\);/);
});

// I11 -- the page's controls: no language picker, no segment or language chip.
test('Subscribers.vue controls', () => {
  const subs = read('./views/Subscribers.vue');
  const tpl = subs.slice(0, subs.indexOf('<script>'));
  assert.doesNotMatch(tpl, /select-lang/);
  assert.doesNotMatch(tpl, /chip-segment|chip-lang/);
  assert.doesNotMatch(tpl, /queryParams\.lang/);
  assert.doesNotMatch(subs, /clearGridFilter|showLangPicker|extraLang|PICKER_LANGS/);
  // Exactly the two dropdowns, Lists first, then the search in a column is-6.
  const selects = [...tpl.matchAll(/<b-select[^>]*data-cy="([^"]+)"/g)].map((m) => m[1]);
  assert.deepEqual(selects, ['select-list', 'select-status']);
  assert.ok(tpl.indexOf('select-status') < tpl.indexOf('<div class="column is-6">'));
  assert.ok(tpl.indexOf('<div class="column is-6">') < tpl.indexOf('data-cy="search"'));
  assert.match(tpl, /<optgroup v-if="listOpts\.system\.length"/);
  // Both write the route only.
  assert.doesNotMatch(tpl, /<b-select[^>]*v-model/);
  // Stage 4 S4-3/S4-4 -- the wiring: All lists ('' value, not Buefy's null placeholder) and the
  // extra option, the Status control shows the applied segment, a pick goes through statusQuery.
  assert.match(tpl, /:value="queryParams\.listID \|\| ''"/);
  assert.match(tpl, /<option value="">\{\{ \$t\('subscribers\.allLists'\) \}\}<\/option>/);
  assert.match(tpl, /<option v-if="listOpts\.extra" :value="listOpts\.extra\.id">/);
  assert.match(tpl, /:value="statusOn \? \(queryParams\.segment \|\| ''\) : ''"/);
  const script = subs.slice(subs.indexOf('<script>'));
  assert.match(script, /onStatusSelect\(segment\) \{[\s\S]*?statusQuery\(/);
  // The mount reads the route through the guard's parser (S4-1) and hydrates lang and segment.
  assert.match(script, /this\.queryParams\.listID = routeListIdOf\(this\.\$route\.params\);/);
  assert.doesNotMatch(script, /parseInt\(this\.\$route\.params\.listID/);
  assert.match(script, /this\.queryParams\.lang = /);
  assert.match(script, /this\.queryParams\.segment = /);
});

// I12 -- every displayed count formats; request bodies take the raw number.
test('Subscribers counts format, bodies do not', () => {
  const subs = read('./views/Subscribers.vue');
  assert.match(subs, /<span data-cy="count">\{\{ \$utils\.formatNumber\(subscribers\.total\) \}\}<\/span>/);
  assert.match(subs, /globals\.messages\.selectAll', \{ num: \$utils\.formatNumber\(subscribers\.total\) \}/);
  assert.match(subs, /globals\.messages\.numSelected', \{ num: \$utils\.formatNumber\(numSelectedSubscribers\) \}/);
  ['confirmBlocklist', 'confirmDelete'].forEach((k) => {
    assert.match(subs, new RegExp(`'subscribers\\.${k}', \\{ num: this\\.\\$utils\\.formatNumber\\(this\\.numSelectedSubscribers\\) \\}`), k);
  });
  assert.match(subs, /'subscribers\.confirmExport', \{ num: this\.\$utils\.formatNumber\(num\) \}/);
  // No unformatted displayed count is left (the single-row confirmation's literal 1 aside).
  assert.doesNotMatch(subs, /\{ num: this\.numSelectedSubscribers \}|\{ num: subscribers\.total \}|\{ num \}/);
  // Request bodies and parses never see a formatted string.
  assert.doesNotMatch(subs, /\$api\.[^\n]*formatNumber/);
  assert.doesNotMatch(subs, /parseInt\([^\n]*formatNumber/);
  assert.doesNotMatch(read('./brandScope.mjs'), /formatNumber/);
});

// SUBSCRIBERS-FILTERS-2-SPEC I9 -- the Lang column shows iff the route lang is absent, en or other;
// a cell renders a string/number trimmed and upper-cased, an em dash for empty, whitespace, absent
// or any other type.
test('Lang column and cell', () => {
  [undefined, null, '', 'en', 'other'].forEach((v) => assert.equal(langColumnShown(v), true, String(v)));
  ['es', 'fr', 'de', 'it', 'en_only', 'none', 'pt'].forEach((v) => assert.equal(langColumnShown(v), false, v));

  assert.equal(langCell({ lang: 'en' }), 'EN');
  assert.equal(langCell({ lang: ' pt-br ' }), 'PT-BR');
  assert.equal(langCell({ lang: 'fr-CA' }), 'FR-CA');
  assert.equal(langCell({ lang: 5 }), '5');
  assert.equal(langCell({ lang: 0 }), '0');
  [{ lang: '' }, { lang: '   ' }, {}, { lang: null }, { lang: true }, { lang: {} }, { lang: ['en'] },
    { lang: Number.NaN }, null, undefined, 'en', ['en']].forEach((a) => {
    assert.equal(langCell(a), LANG_CELL_EMPTY, JSON.stringify(a));
  });
  assert.equal(LANG_CELL_EMPTY, '—');

  // The page: the column on the route's lang, sortable as order_by=lang, the cell from attribs.
  const subs = read('./views/Subscribers.vue');
  assert.match(subs, /<b-table-column v-if="langColumnOn" v-slot="props" field="lang"[^>]*sortable>/);
  assert.match(subs, /langColumnOn\(\) \{\s*return langColumnShown\(this\.queryParams\.lang\);/);
  assert.match(subs, /return langCell\(row \? row\.attribs : undefined\);/);
  assert.match(subs, /order_by: this\.queryParams\.orderBy,/);
});

// SUBSCRIBERS-FILTERS-2-SPEC I10 -- the Subscribers Lists dropdown groups through the shared
// grouping (listGroups.mjs::groupLists, tested in listGroups.test.mjs), as the Campaigns page does.
test('Lists dropdown uses the shared grouping', () => {
  assert.match(read('./subscriberFilters.mjs'), /groupLists\(/);
  assert.match(read('./campaignsRouteQuery.mjs'), /return groupLists\(offered\);/);
  assert.deepEqual(listOptions(LISTS, RUZE, null).system.map((l) => l.name), ['RUZE First order', 'RUZE No-purchase D2']);
});

// Integrations PAGE-STATE-SPEC I2/I3/I4/I5 -- the Subscribers page's session state.
const BARE = to('subscribers', {});
const BARE_LANG = to('subscribers', { lang: 'fr' });
const OWN = (query = {}, params = {}) => to('subscribers', query, { ...params, [RESTORE_PARAM]: '1' });

test('PAGE-STATE I2: an advanced query always restores open -- never a hidden SQL filter', () => {
  const r = restoreSubscribersPage({ queryExp: "name ~* 'amy'", advanced: false }, BARE);
  assert.equal(r.advanced, true);
  assert.equal(r.queryExp, "name ~* 'amy'");
  assert.equal(restoreSubscribersPage({ queryExp: '  ', advanced: false }, BARE).advanced, false);
  assert.equal(restoreSubscribersPage({ queryExp: '', advanced: true }, BARE).advanced, true);
  // The round trip of an open advanced box keeps it open with its query.
  const saved = saveSubscribersPage({ queryExp: 'x = 1', advanced: true, search: 'ignored' });
  assert.equal(saved.search, '');
  assert.deepEqual(restoreSubscribersPage(saved, BARE), {
    queryExp: 'x = 1', advanced: true, orderBy: 'id', order: 'desc', page: 1,
  });
});

test('PAGE-STATE I3: only the visible page-set fields round-trip -- never lang, a selection or a modal', () => {
  const shown = {
    listID: 2, segment: 'held', search: ' amy ', queryExp: '', advanced: false, orderBy: 'email', order: 'asc', page: 4,
  };
  const others = {
    lang: 'fr', subStatus: 'confirmed', bulk: { all: true }, isFormVisible: true, curItem: { id: 1 },
  };
  const saved = saveSubscribersPage({ ...shown, ...others });
  assert.deepEqual(Object.keys(saved).sort(), ['advanced', 'listID', 'order', 'orderBy', 'page', 'queryExp', 'search', 'segment']);
  const stored = JSON.parse(JSON.stringify({ ...saved, subStatus: 'confirmed' }));
  // The URL half: the list, its Status and the search, the route's own lang riding along.
  assert.deepEqual(subscribersRestoreRoute(stored, BARE_LANG, [1, 2]), {
    name: 'subscribers_list',
    params: { listID: '2', [RESTORE_PARAM]: '1' },
    query: { lang: 'fr', search: 'amy', segment: 'held' },
  });
  // The data half, on the replace's landing.
  assert.deepEqual(restoreSubscribersPage(stored, OWN({ search: 'amy' }, { listID: '2' })), {
    queryExp: '', advanced: false, orderBy: 'email', order: 'asc', page: 4,
  });
  // No list: the plain route; nothing URL-held: no replace.
  assert.deepEqual(subscribersRestoreRoute({ search: 'amy' }, BARE, []), {
    name: 'subscribers', params: { [RESTORE_PARAM]: '1' }, query: { search: 'amy' },
  });
  assert.equal(subscribersRestoreRoute({ orderBy: 'email', page: 3 }, BARE, []), null);
  assert.equal(subscribersRestoreRoute({ subStatus: 'confirmed' }, BARE, []), null);
});

test('Review R2: the Status is kept with its list and restored only when the list is kept', () => {
  // Saved only with a list, and only one of SEGMENTS.
  assert.equal(saveSubscribersPage({ segment: 'held' }).segment, null);
  assert.equal(saveSubscribersPage({ listID: 2, segment: 'bogus' }).segment, null);
  assert.equal(saveSubscribersPage({ listID: 2, segment: 'unsubscribed' }).segment, 'unsubscribed');
  const stored = saveSubscribersPage({ listID: 2, segment: 'unsubscribed', search: 'amy' });
  assert.equal(subscribersRestoreRoute(stored, BARE, [2]).query.segment, 'unsubscribed');
  // List not offered: the plain route, no segment (the guard would drop it anyway).
  const r = subscribersRestoreRoute(stored, BARE, [1]);
  assert.equal(r.name, 'subscribers');
  assert.equal('segment' in r.query, false);
  assert.equal(subscribersRestoreRoute({ listID: 2, segment: 'nope' }, BARE, [2]).query.segment, undefined);
});

test('PAGE-STATE I4: a link wins, and the replace decision is false whenever the route has a page query', () => {
  const stored = saveSubscribersPage({
    listID: 2, search: 'amy', orderBy: 'email', page: 3,
  });
  ['segment', 'search', 'subscription_status'].forEach((k) => {
    const route = to('subscribers', { [k]: 'x' });
    assert.equal(subscribersRestoreRoute(stored, route, [2]), null, k);
    assert.equal(restoreSubscribersPage(stored, route), null, k);
  });
  // A list route (the Lists-page grid link) and the subscriber form route.
  assert.equal(subscribersRestoreRoute(stored, LIST({ segment: 'held' }), [2]), null);
  assert.equal(restoreSubscribersPage(stored, LIST()), null);
  assert.equal(subscribersRestoreRoute(stored, to('subscriber', {}, { id: '9' }), [2]), null);
  assert.equal(restoreSubscribersPage(stored, to('subscriber', {}, { id: '9' })), null);
  // The replace's own landing never replaces again (no loop).
  assert.equal(subscribersRestoreRoute(stored, OWN(), [2]), null);
  // The navbar's lang alone is not a link.
  assert.notEqual(subscribersRestoreRoute(stored, BARE_LANG, [2]), null);
  assert.notEqual(restoreSubscribersPage(stored, BARE_LANG), null);
  assert.equal(restoreSubscribersPage(null, BARE), null);
});

test('PAGE-STATE I5: a stored list not offered is All lists; unknown enums and bad pages default', () => {
  assert.deepEqual(offeredSubscriberListIds(LISTS, RUZE), [2, 1, 4, 3]);
  assert.deepEqual(offeredSubscriberListIds(LISTS, { kind: 'all' }), []);
  // List 5 is Shala's: not offered under RUZE -> All lists (and with nothing else kept, no replace).
  assert.equal(subscribersRestoreRoute({ listID: 5 }, BARE, offeredSubscriberListIds(LISTS, RUZE)), null);
  assert.deepEqual(subscribersRestoreRoute({ listID: 5, search: 'a' }, BARE, [1, 2]).name, 'subscribers');
  assert.equal(subscribersRestoreRoute({ listID: 2 }, BARE, offeredSubscriberListIds(LISTS, { kind: 'all' })), null);
  const bad = restoreSubscribersPage({ orderBy: 'password', order: 'up' }, BARE);
  assert.equal(bad.orderBy, 'id');
  assert.equal(bad.order, 'desc');
  [0, -2, 'x', 1.5].forEach((p) => assert.equal(restoreSubscribersPage({ page: p }, BARE).page, 1, String(p)));
  assert.equal(isBareSubscribersRoute(BARE), true);
  assert.equal(isBareSubscribersRoute(LIST()), false);
});
