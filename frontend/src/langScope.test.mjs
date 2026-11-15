// Fork (global language filter) -- integrations listmonk/GLOBAL-LANG-SPEC I1, I2, I3, I4, I4c and
// I6: langScope.mjs -- the applicability table covers the router exactly, its states per view
// (panes and the Subscribers route-wins case), the note only when filtered, the toggle hidden with
// languages off, the stale-response check and the by-query note; SUBSCRIBERS-FILTERS-SPEC I1-I4, I6
// and I14 (the toggle's values, the fold, the raw-selection readers, the grey line, the guard
// wiring, the labels).
// Run: node --test src/langScope.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import {
  APPLICABILITY_STATES, LANG_APPLICABILITY, LANG_VALUES, SUBSCRIBER_ONLY_LANGS, SUBSCRIBER_ROUTES, byQueryLangNote,
  effectiveLang, langApplicability, langLabel, langNote, langNoteLabel, langToggleVisible, routeLangLabel,
  subscriberResponseCurrent, subscribersLang, viewPaneValue, answeredFilter, bulkTarget, normalizeLang,
  otherLangPresent, toggleValues,
} from './langScope.mjs'; // eslint-disable-line import/extensions

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');
const ON = { lang_enabled: true };
const OFF = { lang_enabled: false };

// SUBSCRIBERS-FILTERS-SPEC I1, amended by SUBSCRIBERS-FILTERS-2-SPEC I1 -- the toggle's values are the
// send languages plus other and all (the literal pin is the deliberate act), every one of them a
// value the server's subscriber lang= filter accepts (models.IsSubscriberLangFilter: CampaignLangs +
// the three SubscriberLang* constants) -- a STRICT subset: the server keeps en_only and none.
// Read from the fork's Go source so the two cannot drift.
test('toggle values = server lang filter + all', () => {
  assert.deepEqual([...LANG_VALUES], ['all', 'en', 'es', 'fr', 'de', 'it', 'other']);
  assert.deepEqual([...SUBSCRIBER_ONLY_LANGS], ['other']);
  const campaigns = read('../../models/campaigns.go');
  const m = campaigns.match(/var CampaignLangs = \[\]string\{([^}]*)\}/);
  assert.ok(m, 'CampaignLangs not found');
  const codes = [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
  const lists = read('../../models/lists.go');
  const consts = ['SubscriberLangNone', 'SubscriberLangOther', 'SubscriberLangEnOnly'].map((n) => {
    const c = lists.match(new RegExp(`\\b${n}\\s*=\\s*"([^"]+)"`));
    assert.ok(c, n);
    return c[1];
  });
  const server = new Set([...codes, ...consts]);
  const toggle = LANG_VALUES.filter((v) => v !== 'all');
  toggle.forEach((v) => assert.ok(server.has(v), `${v} is not a server lang filter value`));
  assert.ok(toggle.length < server.size, 'the toggle is a strict subset of the server filter');
  // The two the toggle withdrew stay server values (advanced use, rollbacks) and normalise to All.
  ['en_only', 'none'].forEach((v) => {
    assert.ok(server.has(v), v);
    assert.ok(!LANG_VALUES.includes(v), v);
    assert.equal(normalizeLang(v), 'all', v);
  });
  SUBSCRIBER_ONLY_LANGS.forEach((v) => assert.ok(server.has(v), v));
});

// SUBSCRIBERS-FILTERS-2-SPEC I3 -- otherLangPresent: true iff some in-scope list has has_other_lang;
// every store list under All brands; a list outside the brand scope never counts.
test('otherLangPresent over the brand scope', () => {
  const lists = {
    results: [
      { id: 1, brand: 'ruze', has_other_lang: false },
      { id: 2, brand: 'ruze' },
      { id: 3, brand: 'bnkd', hasOtherLang: true },
      { id: 4, brand: '', has_other_lang: false },
    ],
  };
  const ALL = { kind: 'all' };
  const RUZE = { kind: 'brand', slug: 'ruze' };
  const BNKD = { kind: 'brand', slug: 'bnkd' };
  const NONE = { kind: 'none' };
  assert.equal(otherLangPresent(lists, ALL), true);
  assert.equal(otherLangPresent(lists, BNKD), true);
  assert.equal(otherLangPresent(lists, RUZE), false); // list 3 is out of scope
  assert.equal(otherLangPresent(lists, NONE), false);
  assert.equal(otherLangPresent([{ id: 4, brand: '', has_other_lang: true }], NONE), true);
  // A truthy non-boolean is not the server's true.
  assert.equal(otherLangPresent([{ id: 5, brand: '', has_other_lang: 'true' }], ALL), false);
  assert.equal(otherLangPresent(null, ALL), false);
  assert.equal(otherLangPresent({ results: [] }, ALL), false);
});

// SUBSCRIBERS-FILTERS-2-SPEC I4 -- toggleValues includes other iff present OR selected; otherwise
// LANG_VALUES in order. The toggle renders from it and is its only caller.
test('toggleValues shows Other only while present or selected', () => {
  const noOther = ['all', 'en', 'es', 'fr', 'de', 'it'];
  assert.deepEqual(toggleValues(false, 'all'), noOther);
  assert.deepEqual(toggleValues(false, 'fr'), noOther);
  assert.deepEqual(toggleValues(false, 'none'), noOther); // a withdrawn value reads as All
  assert.deepEqual(toggleValues(true, 'all'), [...LANG_VALUES]);
  assert.deepEqual(toggleValues(false, 'other'), [...LANG_VALUES]);
  assert.deepEqual(toggleValues(false, ' Other '), [...LANG_VALUES]);
  assert.deepEqual(toggleValues('yes', 'all'), noOther);
  const toggle = read('./components/LanguageToggle.vue');
  assert.match(toggle, /return toggleValues\(otherLangPresent\(this\.lists, this\.brandSelection\), this\.langSelection\);/);
  assert.match(toggle, /v-for="v in values"/);
  assert.ok(!toggle.includes('LANG_VALUES'), 'the toggle renders from toggleValues only');
  const root = new URL('./', import.meta.url).pathname;
  const callers = [];
  const walk = (dir) => readdirSync(dir).forEach((f) => {
    const p = `${dir}${f}`;
    if (statSync(p).isDirectory()) {
      walk(`${p}/`);
    } else if (/\.(js|mjs|vue)$/.test(f) && !/\.test\.mjs$/.test(f) && readFileSync(p, 'utf8').includes('toggleValues(')) {
      callers.push(p.slice(root.length));
    }
  });
  walk(root);
  assert.deepEqual(callers.sort(), ['components/LanguageToggle.vue', 'langScope.mjs']);
});

// I2.
test('effectiveLang folds subscriber-only languages', () => {
  LANG_VALUES.forEach((v) => {
    const want = SUBSCRIBER_ONLY_LANGS.includes(v) ? 'all' : v;
    assert.equal(effectiveLang(v, ON), want, v);
    assert.equal(effectiveLang(v, OFF), 'all', v);
    // The raw selection Subscribers applies: verbatim while visible, All when hidden.
    assert.equal(subscribersLang(v, ON), v, v);
    assert.equal(subscribersLang(v, OFF), 'all', v);
  });
  assert.equal(effectiveLang('pt', ON), 'all');
  assert.equal(subscribersLang('pt', ON), 'all');
  // The store's getters are these two functions (the fold has one home).
  const store = read('./store/index.js');
  assert.match(store, /langEffective: \(state\) => effectiveLang\(state\.langStored, state\[models\.serverConfig\]\)/);
  assert.match(store, /langSubscribers: \(state\) => subscribersLang\(state\.langStored, state\[models\.serverConfig\]\)/);
});

// I3 -- only the toggle, the store, the guard and the Subscribers page read the raw selection.
test('only Subscribers reads the raw selection', () => {
  const root = new URL('./', import.meta.url).pathname;
  const allowed = new Set(['views/Subscribers.vue', 'components/LanguageToggle.vue', 'store/index.js', 'main.js']);
  const files = [];
  const walk = (dir) => readdirSync(dir).forEach((f) => {
    const p = `${dir}${f}`;
    if (statSync(p).isDirectory()) {
      walk(`${p}/`);
    } else if (/\.(js|mjs|vue)$/.test(f) && !/\.test\.mjs$/.test(f)) {
      files.push(p.slice(root.length));
    }
  });
  walk(root);
  assert.ok(files.length > 50, `found ${files.length} files`);
  const needles = ["'langSelection'", "'langSubscribers'", 'getters.langSelection', 'getters.langSubscribers'];
  files.filter((f) => !allowed.has(f)).forEach((f) => {
    // Stage 4 S4-3 -- the stored value itself is the store's alone.
    if (f !== 'store/index.js') {
      assert.ok(!readFileSync(`${root}${f}`, 'utf8').includes('langStored'), `${f} reads langStored`);
    }
    const src = readFileSync(`${root}${f}`, 'utf8');
    needles.forEach((n) => assert.ok(!src.includes(n), `${f} reads ${n}`));
  });
  // And the four that may, do.
  assert.match(read('./views/Subscribers.vue'), /mapGetters\(\['langSubscribers'\]\)/);
  assert.match(read('./components/LanguageToggle.vue'), /mapGetters\(\['langSelection'/);
  assert.match(read('./main.js'), /store\.getters\.langSubscribers/);
});

// I4 -- a subscriber-only selection reads grey with the Subscribers-only description on every
// non-Subscribers route (panes included), and applies on the Subscribers routes.
test('subscriber-only selection is grey off Subscribers', () => {
  SUBSCRIBER_ONLY_LANGS.forEach((sel) => {
    Object.keys(LANG_APPLICABILITY).forEach((r) => {
      const panes = LANG_APPLICABILITY[r].panes ? Object.keys(LANG_APPLICABILITY[r].panes) : [''];
      panes.forEach((p) => {
        const a = langApplicability(r, { selection: sel, pane: p ? viewPaneValue(LANG_APPLICABILITY[r].view, p) : '' });
        if (SUBSCRIBER_ROUTES.includes(r)) {
          assert.deepEqual(a, { state: 'all', desc: 'naming.langApp.subscribersStored' }, `${r} ${sel}`);
        } else {
          assert.deepEqual(a, { state: 'none', desc: 'naming.langApp.subscribersOnly' }, `${r}:${p} ${sel}`);
        }
      });
    });
    assert.deepEqual(langApplicability('nope', { selection: sel }), { state: 'none', desc: 'naming.langApp.subscribersOnly' });
  });
  // The toggle reads it with the raw selection.
  assert.match(read('./components/LanguageToggle.vue'), /langApplicability\(r\.name, \{ pane: this\.viewPane, selection: this\.langSelection \}\)/);
});

// I14 -- button labels (pure, translations passed in) and the note's {lang}.
test('labels', () => {
  const labels = { all: 'Tout', other: 'Autre' };
  assert.deepEqual(LANG_VALUES.map((v) => langLabel(v, labels)), ['Tout', 'EN+', 'ES', 'FR', 'DE', 'IT', 'Autre']);
  assert.deepEqual(LANG_VALUES.map((v) => langLabel(v)), ['All', 'EN+', 'ES', 'FR', 'DE', 'IT', 'Other']);
  assert.equal(langLabel('fr', 'All'), 'FR'); // the older string signature
  assert.equal(langLabel('pt', labels), 'Tout');
  // SUBSCRIBERS-FILTERS-2-SPEC D1 -- the withdrawn values normalise to All (their EN / None
  // branches are deleted).
  assert.equal(langLabel('en_only', labels), 'Tout');
  assert.equal(langLabel('none', labels), 'Tout');
  // The note: EN+ on Subscribers, bare EN elsewhere; every other value as its button.
  assert.equal(langNoteLabel('en', true), 'EN+');
  assert.equal(langNoteLabel('en', false), 'EN');
  assert.equal(langNoteLabel('other', false, labels), 'Autre');
  assert.equal(langNoteLabel('fr', true), 'FR');
  // The note keys: send language for a toggle code, stored language for a subscriber-only value.
  ['en', 'es', 'fr', 'de', 'it'].forEach((v) => assert.equal(langApplicability('subscribers', { selection: v }).desc, 'naming.langApp.subscribers'));
  const en = JSON.parse(read('../../i18n/en.json'));
  assert.equal(en['naming.langApp.subscribers'], '{lang}: subscribers by send language');
  assert.equal(en['naming.langApp.subscribersStored'], '{lang}: subscribers by stored language');
  assert.match(en['naming.langApp.subscribersOnly'], /applies to Subscribers only/);
  assert.ok(en['naming.langOther'], 'naming.langOther');
  // SUBSCRIBERS-FILTERS-2-SPEC D1 -- naming.langNone is deleted with its value.
  assert.equal(en['naming.langNone'], undefined);
  const toggle = read('./components/LanguageToggle.vue');
  assert.ok(!toggle.includes('naming.langNone'));
  assert.match(toggle, /return langLabel\(v, this\.labels\(\)\);/);
  assert.match(toggle, /langNoteLabel\(lang, this\.onSubscribers, this\.labels\(\)\)/);
});

// Stage 4 S4-7 -- the one label mapper of a Subscribers route language value.
test('route language label', () => {
  const labels = {
    all: 'All languages', other: 'Other', name: (c) => ({ en: 'English', fr: 'Français' }[c] || ''), sendName: (c) => `${c}+`,
  };
  assert.equal(routeLangLabel('', labels), 'All languages');
  assert.equal(routeLangLabel('all', labels), 'All languages');
  assert.equal(routeLangLabel(undefined, labels), 'All languages');
  assert.equal(routeLangLabel('other', labels), 'Other');
  // SUBSCRIBERS-FILTERS-2-SPEC D1 -- the en_only / none branches are deleted (the guard drops such
  // a route): an unnamed value reads raw.
  assert.equal(routeLangLabel('none', labels), 'none');
  assert.equal(routeLangLabel('en_only', labels), 'en_only');
  assert.equal(routeLangLabel('en', labels), 'en+');
  assert.equal(routeLangLabel('fr', labels), 'Français');
  assert.equal(routeLangLabel('pt', labels), 'pt');
  // The by-query note of Subscribers uses it (the toggle's page-filter line is retired,
  // SUBSCRIBERS-FILTERS D3).
  assert.match(read('./views/Subscribers.vue'), /return routeLangLabel\(l, \{/);
});

// Stage 4 S4-4 -- the wiring the pure tests cannot see (I4's and I4c's enforcement points, I6's
// call sites, D4's busy guards); each pin fails if its call is reverted.
test('global language wiring', () => {
  // SUBSCRIBERS-FILTERS I6 -- both sites decide through subscribersRouteFix with the raw selection
  // (langSubscribers), commit commitLang before the navigation proceeds, and replace.
  const main = read('./main.js');
  const fixAt = main.indexOf('function subscribersFixOf(');
  const fixFn = main.slice(fixAt, main.indexOf('\n}\n', fixAt));
  assert.match(fixFn, /subscribersRouteFix\(to, store\.getters\.langSubscribers, langToggleVisible\(store\.state\.serverConfig\)\)/);
  assert.match(fixFn, /if \(fix\.commitLang\) \{\s*store\.commit\('selectLang', fix\.commitLang\);\s*\}\s*return fix\.query \?/);
  assert.match(fixFn, /replace: true/);
  assert.doesNotMatch(main, /langEntryRedirect/);
  const guard = main.slice(main.indexOf('router.beforeEach('), main.indexOf('router.afterEach('));
  assert.match(guard, /if \(langConfigReady\) \{\s*const langRedirect = subscribersFixOf\(to\);\s*if \(langRedirect\) \{\s*next\(langRedirect\);/);
  const init = main.slice(main.indexOf('async function initConfig('), main.indexOf('app.$mount('));
  assert.match(init, /langConfigReady = true;\s*const landed = router\.currentRoute;\s*const langRedirect = subscribersFixOf\(landed\);/);
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
  assert.match(subs, /langSubscribers\(now\) \{[\s\S]*?this\.onLangSelect\(want\);/);
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

  // The three Subscribers routes: the selection applies (SUBSCRIBERS-FILTERS D3 -- the guard makes
  // the route agree before mount, so there is no page-filter state): by send language for a toggle
  // code, by stored language for a subscriber-only value.
  SUBSCRIBER_ROUTES.forEach((r) => {
    ['all', 'en', 'es', 'fr', 'de', 'it'].forEach((sel) => {
      assert.deepEqual(langApplicability(r, { selection: sel }), { state: 'all', desc: 'naming.langApp.subscribers' }, `${r} ${sel}`);
    });
    SUBSCRIBER_ONLY_LANGS.forEach((sel) => {
      assert.deepEqual(langApplicability(r, { selection: sel }), { state: 'all', desc: 'naming.langApp.subscribersStored' });
    });
  });
  assert.doesNotMatch(read('./langScope.mjs'), /pageFilter|pageLang/);

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
  // SUBSCRIBERS-FILTERS-2-SPEC D1 (S2-3) -- Other keeps the stored-language note.
  const p = langApplicability('subscribers', { selection: 'other' });
  assert.deepEqual(langNote('other', p), { key: 'naming.langApp.subscribersStored', lang: 'other' });
  // A withdrawn value is All: no note.
  assert.equal(langNote('none', langApplicability('subscribers', { selection: 'none' })), null);
  assert.deepEqual(langNote('other', langApplicability('campaigns', { selection: 'other' })), { key: 'naming.langApp.subscribersOnly', lang: 'other' });
  // SUBSCRIBERS-FILTERS D3 retired the page-filter line: under All there is never a note.
  assert.equal(langNote('all', langApplicability('subscribers', { selection: 'all' })), null);
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

// I4 (GLOBAL-LANG's entry redirect) is retired: its cases are folded into subscriberFilters.test.mjs
// "route fix" (SUBSCRIBERS-FILTERS-SPEC I5).

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
