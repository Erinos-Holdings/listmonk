// Fork (global language filter) -- integrations listmonk/GLOBAL-LANG-SPEC I1, I2, I3, I4, I4c and
// I6: langScope.mjs -- the applicability table covers the router exactly, its states per view
// (panes and the Subscribers route-wins case), the note only when filtered, the toggle hidden with
// languages off, the Subscribers entry redirect, the stale-response check and the by-query note.
// Run: node --test src/langScope.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  APPLICABILITY_STATES, LANG_APPLICABILITY, SUBSCRIBER_ROUTES, byQueryLangNote, effectiveLang, langApplicability,
  langEntryRedirect, langNote, langToggleVisible, routeLangLabel, subscriberResponseCurrent, viewPaneValue,
  answeredFilter, bulkTarget,
} from './langScope.mjs'; // eslint-disable-line import/extensions

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');

// Stage 4 S4-7 -- the one label mapper of a Subscribers route language value.
test('route language label', () => {
  const labels = {
    all: 'All languages', none: 'No language', other: 'Other', name: (c) => ({ en: 'English', fr: 'Français' }[c] || ''), sendName: (c) => `${c}+`,
  };
  assert.equal(routeLangLabel('', labels), 'All languages');
  assert.equal(routeLangLabel('all', labels), 'All languages');
  assert.equal(routeLangLabel(undefined, labels), 'All languages');
  assert.equal(routeLangLabel('none', labels), 'No language');
  assert.equal(routeLangLabel('other', labels), 'Other');
  assert.equal(routeLangLabel('en_only', labels), 'English');
  assert.equal(routeLangLabel('en', labels), 'en+');
  assert.equal(routeLangLabel('fr', labels), 'Français');
  assert.equal(routeLangLabel('pt', labels), 'pt');
  // Both screens use it.
  assert.match(read('./views/Subscribers.vue'), /return routeLangLabel\(l, \{/);
  assert.match(read('./components/LanguageToggle.vue'), /return routeLangLabel\(v, \{/);
});

// Stage 4 S4-4 -- the wiring the pure tests cannot see (I4's and I4c's enforcement points, I6's
// call sites, D4's busy guards); each pin fails if its call is reverted.
test('global language wiring', () => {
  const main = read('./main.js');
  const guard = main.slice(main.indexOf('router.beforeEach('), main.indexOf('router.afterEach('));
  assert.match(guard, /langEntryRedirect\(to, from, store\.getters\.langEffective\)/);
  assert.match(guard, /next\(langRedirect\)/);
  const init = main.slice(main.indexOf('async function initConfig('), main.indexOf('app.$mount('));
  assert.match(init, /langEntryRedirect\(landed, null, store\.getters\.langEffective\)/);
  assert.match(init, /router\.replace\(langRedirect\)/);

  const api = read('./api/index.js');
  assert.match(api, /'store' in resp\.config && \(typeof resp\.config\.accept !== 'function' \|\| resp\.config\.accept\(\)\)/);
  const getSubs = api.slice(api.indexOf('export const getSubscribers'), api.indexOf('export const getSubscriber ='));
  assert.match(getSubs, /accept: opts\.accept/);

  const subs = read('./views/Subscribers.vue');
  assert.match(subs, /getSubscribers\(qp, \{ accept: current \}\)/);
  assert.match(subs, /subscriberResponseCurrent\(seq, this\.reqSeq, this\.pageMounted === true\)/);
  assert.match(subs, /answeredSeq: -1,/); // S4-1: no answer yet = busy
  assert.match(subs, /byQueryBusy\(\) \{\s*return !!this\.loading\.subscribers \|\| this\.answeredSeq !== this\.reqSeq;/);
  // The busy guard on the by-query verbs and the export.
  const method = (name) => {
    const at = subs.indexOf(`\n    ${name}(`);
    assert.ok(at > -1, name);
    return subs.slice(at, subs.indexOf('\n    },', at));
  };
  ['blocklistSubscribers', 'deleteSubscribers', 'exportSubscribers'].forEach((m) => assert.match(method(m), /if \(this\.byQueryBusy\) \{\s*return;/, m));
  // add / remove / unsubscribe by query: the busy guard sits in the one by-query condition.
  assert.match(method('bulkChangeLists'), /\|\| this\.byQueryBusy \|\|[^\n]*\{\s*this\.isBulkListFormVisible = false;[\s\S]{0,200}?\s*return;/);
  assert.match(method('showBulkListForm'), /this\.bulk\.all && this\.byQueryBusy/);
  // The by-query language note on Blocklist, Delete, Export and the Manage-lists modal (I6).
  assert.match(method('blocklistSubscribers'), /withLangNote\(this\.\$t\('subscribers\.confirmBlocklist'/);
  assert.match(method('deleteSubscribers'), /withLangNote\(this\.\$t\('subscribers\.confirmDelete'/);
  assert.match(method('exportSubscribers'), /withLangNote\(this\.\$t\('subscribers\.confirmExport'/);
  assert.match(subs, /:lang-note="bulkListLangNote"/);
  assert.match(subs, /bulkListLangNote\(\) \{[\s\S]*?byQueryLangNote\(gridFilterOf\(filter\),/);
  assert.match(read('./views/SubscriberBulkList.vue'), /<p v-if="langNote"/);
  // The toggle rewrites the route; the read query goes through gridFilterOf.
  assert.match(subs, /langEffective\(now\) \{[\s\S]*?this\.onLangSelect\(want\);/);
  assert.match(subs, /gridFilter\(\) \{\s*return gridFilterOf\(this\.queryParams\);/);

  // Stage 4b S4b-3 -- the round-1 fixes, each revertible only by failing here.
  // S4-2: no failed-query un-busy in querySubscribers.
  assert.doesNotMatch(method('querySubscribers'), /\.catch\(/);
  assert.match(method('querySubscribers'), /this\.answeredSeq = seq;\s*this\.answeredQuery = asked;/);
  // S4-1: the subscriber route queries too -- the list query is outside the params.id branch.
  const mountedAt = subs.indexOf('\n  mounted() {');
  const mounted = subs.slice(mountedAt, subs.indexOf('\n  },', mountedAt));
  assert.match(mounted, /if \(this\.\$route\.params\.id\) \{[\s\S]*?\n {4}\}\n[\s\S]*?\n {4}this\.afterListsLoaded\(\(\) => this\.querySubscribers\(\)\);/);
  assert.doesNotMatch(mounted, /\} else \{/);
  // S4-3: closing the form keeps the route's lang (only lang).
  assert.match(method('onFormClose'), /\{ lang: this\.\$route\.query\.lang \}/);
  assert.match(method('onFormClose'), /this\.\$router\.push\(\{ name: 'subscribers', query \}\)/);
  // S4b-1: by query only under select-all, never a by-ID fallthrough.
  ['blocklistSubscribers', 'deleteSubscribers', 'bulkChangeLists'].forEach((m) => {
    assert.match(method(m), /bulkTarget\(\{ all: this\.bulk\.all, checkedCount: this\.bulk\.checked\.length \}\)/, m);
    assert.doesNotMatch(method(m), /!this\.bulk\.all && this\.bulk\.checked\.length > 0/, m);
  });
  ['blocklistSubscribers', 'deleteSubscribers'].forEach((m) => {
    assert.match(method(m), /if \(target === 'none' \|\| \(target === 'query' && !filter\)\) \{\s*return;/, m);
  });
  assert.match(method('bulkChangeLists'), /if \(target !== 'query' \|\| this\.byQueryBusy \|\| !filter\) \{/);
  // S4b-2: every by-query body and the export from the answered snapshot, never the live params.
  ['blocklistSubscribers', 'deleteSubscribers', 'bulkChangeLists'].forEach((m) => {
    assert.match(method(m), /const filter = answeredFilter\(this\.answeredQuery\);/, m);
    assert.match(method(m), /subscriberQueryBody\(filter, this\.scopeListIds\)/, m);
  });
  assert.match(method('deleteSubscribers'), /all: filter\.queryExp\.trim\(\) === '' && filter\.search\.trim\(\) === ''/);
  assert.match(method('exportSubscribers'), /subscriberExportParams\(filter, this\.scopeListIds, ids\)/);
  // Stage 4c S4c-2 -- the load-bearing lines: the snapshot is a COPY; the export and the modal's
  // language line read the answered filter. S4c-3: the no-request close says so.
  assert.match(method('querySubscribers'), /const asked = \{ \.\.\.this\.queryParams \};/);
  assert.match(method('exportSubscribers'), /const filter = answeredFilter\(this\.answeredQuery\);/);
  assert.match(method('bulkListLangNote'), /const filter = answeredFilter\(this\.answeredQuery\);/);
  assert.match(method('bulkChangeLists'), /this\.isBulkListFormVisible = false;\s*\/\/[^\n]*\n\s*this\.\$utils\.toast\(this\.\$t\('subscribers\.listChangeNotApplied'\), 'is-warning'\);\s*return;/);
  assert.doesNotMatch(subs, /subscriberQueryBody\(this\.queryParams/);
  assert.doesNotMatch(subs, /subscriberExportParams\(this\.queryParams/);
});

// Stage 4b S4b-1/S4b-2 -- the pure halves.
test('bulk target and the answered filter', () => {
  assert.equal(bulkTarget({ all: true, checkedCount: 0 }), 'query');
  assert.equal(bulkTarget({ all: true, checkedCount: 20 }), 'query');
  assert.equal(bulkTarget({ all: false, checkedCount: 3 }), 'ids');
  assert.equal(bulkTarget({ all: false, checkedCount: 0 }), 'none'); // never a by-query fallthrough
  assert.equal(bulkTarget({}), 'none');
  assert.equal(bulkTarget(), 'none');
  assert.equal(bulkTarget({ all: 'yes', checkedCount: 0 }), 'none');

  assert.equal(answeredFilter(null), null);
  assert.equal(answeredFilter(undefined), null);
  const snap = { search: 'gmail', queryExp: '', lang: 'es' };
  const f = answeredFilter(snap);
  assert.deepEqual(f, snap);
  assert.notEqual(f, snap); // a copy: the snapshot is never edited through it
});

// I1 -- agreement with the router file's text: every `name: '<x>'` route, no more, no fewer.
test('applicability covers the router', () => {
  const src = readFileSync(new URL('./router/index.js', import.meta.url), 'utf8');
  const names = [...src.matchAll(/^\s*name:\s*'([^']+)'/gm)].map((m) => m[1]).sort();
  assert.ok(names.length >= 20, `router parse found ${names.length} routes`);
  assert.deepEqual(Object.keys(LANG_APPLICABILITY).sort(), names);
});

// I2 -- D3's table.
test('applicability table', () => {
  const st = (route, opts) => langApplicability(route, opts).state;
  ['campaigns', 'templates'].forEach((r) => assert.equal(st(r), 'all'));
  ['dashboard', 'lists', 'list'].forEach((r) => assert.equal(st(r), 'partial'));
  ['media', 'import', 'bounces', 'forms', 'settings', 'users', 'userRoles', 'listRoles', 'logs', 'maintenance',
    'userProfile', 'campaign', 'campaignReview', 'journeyPreview', '404_page', 'brand'].forEach((r) => assert.equal(st(r), 'none', r));
  // An unknown route is none.
  assert.equal(st('nope'), 'none');
  assert.equal(st(undefined), 'none');

  // Analytics panes via the setViewPane value; Matrix by default.
  assert.equal(st('campaignAnalytics', { pane: viewPaneValue('analytics', 'matrix') }), 'all');
  assert.equal(st('campaignAnalytics', { pane: viewPaneValue('analytics', 'campaign') }), 'none');
  assert.equal(st('campaignAnalytics', { pane: '' }), 'all');
  assert.equal(st('campaignAnalytics', { pane: 'brands:health' }), 'all'); // another view's pane

  // Brands panes.
  assert.equal(st('brands', { pane: 'brands:matrix' }), 'partial');
  assert.equal(st('brands', { pane: 'brands:usage' }), 'partial');
  assert.equal(st('brands', { pane: 'brands:health' }), 'none');
  assert.equal(st('brands'), 'partial');
  const matrixDesc = langApplicability('brands', { pane: 'brands:matrix' }).desc;
  assert.notEqual(matrixDesc, langApplicability('brands', { pane: 'brands:usage' }).desc);

  // The three Subscribers routes: all when the route's lang is the selection's parameter, else
  // partial with the page filter -- the route wins.
  SUBSCRIBER_ROUTES.forEach((r) => {
    assert.deepEqual(langApplicability(r, { query: { lang: 'es' }, selection: 'es' }), { state: 'all', desc: 'naming.langApp.subscribers' });
    assert.deepEqual(langApplicability(r, { query: {}, selection: 'all' }), { state: 'all', desc: 'naming.langApp.subscribers' });
    const page = (pageLang) => ({ state: 'partial', desc: 'naming.langApp.pageFilter', pageLang });
    assert.deepEqual(langApplicability(r, { query: {}, selection: 'es' }), page('all'));
    assert.deepEqual(langApplicability(r, { query: { lang: 'none' }, selection: 'es' }), page('none'));
    assert.deepEqual(langApplicability(r, { query: { lang: 'fr' }, selection: 'all' }), page('fr'));
    // EN is the send language en; the stored-English en_only is a different filter.
    assert.equal(langApplicability(r, { query: { lang: 'en' }, selection: 'en' }).state, 'all');
    assert.equal(langApplicability(r, { query: { lang: 'en_only' }, selection: 'en' }).state, 'partial');
  });

  // Every entry's state is one of the three, every description an i18n key.
  Object.keys(LANG_APPLICABILITY).forEach((r) => {
    const a = langApplicability(r, { selection: 'fr', query: { lang: 'fr' } });
    assert.ok(APPLICABILITY_STATES.includes(a.state), r);
    assert.match(a.desc, /^naming\.langApp\./, r);
  });
});

// I3.
test('note only when filtered', () => {
  const a = langApplicability('dashboard');
  assert.equal(langNote('all', a), null);
  assert.equal(langNote(undefined, a), null);
  assert.deepEqual(langNote('es', a), { key: 'naming.langApp.dashboard', lang: 'es' });
  const p = langApplicability('subscribers', { query: { lang: 'none' }, selection: 'es' });
  assert.deepEqual(langNote('es', p), { key: 'naming.langApp.pageFilter', lang: 'es', pageLang: 'none' });
  // Stage 4 S4-5: under All, a page whose own language filter differs (a lang=fr link) still shows
  // its page-filter line; an unfiltered page under All shows none.
  const frUnderAll = langApplicability('subscribers', { query: { lang: 'fr' }, selection: 'all' });
  assert.deepEqual(langNote('all', frUnderAll), { key: 'naming.langApp.pageFilter', lang: 'all', pageLang: 'fr' });
  assert.equal(langNote('all', langApplicability('subscribers', { query: {}, selection: 'all' })), null);
  assert.equal(langNote('all', langApplicability('lists')), null);
  // Grey views still carry their line when filtered.
  assert.ok(langNote('fr', langApplicability('media')));

  assert.equal(langToggleVisible({ lang_enabled: true }), true);
  assert.equal(langToggleVisible({ lang_enabled: false }), false);
  assert.equal(langToggleVisible({}), false);
  assert.equal(langToggleVisible([]), false); // the store's pre-config value
  assert.equal(langToggleVisible(null), false);
  assert.equal(effectiveLang('fr', { lang_enabled: true }), 'fr');
  assert.equal(effectiveLang('fr', { lang_enabled: false }), 'all');
  assert.equal(effectiveLang('pt', { lang_enabled: true }), 'all');
});

// I4.
test('entry redirect', () => {
  const dash = { name: 'dashboard', query: {} };
  const to = (name, query = {}, params = {}) => ({
    name, query, params, hash: '',
  });
  // Entry from outside, no lang/segment, selection != All: redirected with the parameter.
  assert.deepEqual(langEntryRedirect(to('subscribers'), dash, 'es'), to('subscribers', { lang: 'es' }));
  const listRedirect = langEntryRedirect(to('subscribers_list', { search: 'x' }, { listID: '4' }), dash, 'en');
  assert.deepEqual(listRedirect, to('subscribers_list', { search: 'x', lang: 'en' }, { listID: '4' }));
  assert.equal(langEntryRedirect(to('subscriber', {}, { id: '9' }), { name: undefined }, 'de').query.lang, 'de');
  assert.equal(langEntryRedirect(to('subscribers'), null, 'fr').query.lang, 'fr'); // first load
  // Selection All: never.
  assert.equal(langEntryRedirect(to('subscribers'), dash, 'all'), null);
  // An explicit lang or segment (a Lists-grid cell, its whole-column cells) is never rewritten.
  assert.equal(langEntryRedirect(to('subscribers_list', { segment: 'active' }), dash, 'es'), null);
  assert.equal(langEntryRedirect(to('subscribers_list', { lang: 'none' }), dash, 'es'), null);
  assert.equal(langEntryRedirect(to('subscribers', { lang: '' }), dash, 'es'), null);
  // Between the three routes: not an entry (the page's own All languages sticks).
  SUBSCRIBER_ROUTES.forEach((from) => {
    SUBSCRIBER_ROUTES.forEach((t) => assert.equal(langEntryRedirect(to(t), { name: from }, 'es'), null, `${from}->${t}`));
  });
  // Any other target: never.
  assert.equal(langEntryRedirect(to('lists'), dash, 'es'), null);
  assert.equal(langEntryRedirect(to('campaigns'), dash, 'es'), null);
});

// I4c.
test('stale subscribers responses are rejected', () => {
  assert.equal(subscriberResponseCurrent(3, 3, true), true);
  assert.equal(subscriberResponseCurrent(2, 3, true), false); // an older request of the same page
  assert.equal(subscriberResponseCurrent(3, 3, false), false); // the page has been destroyed
  assert.equal(subscriberResponseCurrent(undefined, undefined, true), false);
});

// I6.
test('by-query note', () => {
  const fmt = (l) => `Only ${l.toUpperCase()}`;
  assert.equal(byQueryLangNote({ search: 'a', lang: 'es' }, fmt), 'Only ES');
  assert.equal(byQueryLangNote({ search: 'a' }, fmt), '');
  assert.equal(byQueryLangNote({ lang: '' }, fmt), '');
  assert.equal(byQueryLangNote({ lang: null }, fmt), '');
  assert.equal(byQueryLangNote([['search', 'a'], ['list_id', '3'], ['lang', 'none']], fmt), 'Only NONE');
  assert.equal(byQueryLangNote([['search', 'a']], fmt), '');
  assert.equal(byQueryLangNote(null, fmt), '');
  assert.equal(byQueryLangNote({ lang: 'fr' }), 'Language: fr');
});
