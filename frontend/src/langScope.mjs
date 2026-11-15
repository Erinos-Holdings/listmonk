// Fork (naming standard, integrations listmonk/NAMING-STANDARD-SPEC D2 §4.4; global language
// filter, integrations listmonk/GLOBAL-LANG-SPEC). The global language toggle
// `All | EN+ | ES | FR | DE | IT` plus `Other` while present (SUBSCRIBERS-FILTERS-2-SPEC D1/D2) beside the brand
// selector: a PRESENTATION filter like the brand selector -- it never reaches a server-side
// permission decision (GLOBAL-LANG D9). Its value is the store's `langSelection` getter, persisted
// in the pref `brand.lang` (the `brand.launch` pattern) by the store's selectLang mutation, whose
// two callers are LanguageToggle.vue and the router guard in main.js (a Subscribers route arriving
// with another language sets it; SUBSCRIBERS-FILTERS D3.3). The listing predicate is
// naming.mjs::langMatches (one statement, pinned with the server's query-campaigns lang filter).
//
// GLOBAL-LANG D1/D2 (superseding NAMING-STANDARD §3's "shown only where it filters"): the toggle is
// shown on every route with the navbar (hidden everywhere only when app.lang_enable is off, and the
// selection then reads as All), and each view declares how far the selection applies there --
// LANG_APPLICABILITY below is that table's ONE home: `all` (blue), `partial` (orange) or `none`
// (grey), each with a short description i18n key. Exhaustive: every named route in
// router/index.js has an entry (langScope.test.mjs reads the router file).
import { LANG_VALUES, normalizeLang } from './naming.mjs'; // eslint-disable-line import/extensions
import { scopeListIds, storeLists } from './brandScope.mjs'; // eslint-disable-line import/extensions

export { LANG_VALUES, normalizeLang };

export const LANG_PREF = 'brand.lang';

// SUBSCRIBERS-FILTERS-SPEC D1, narrowed by SUBSCRIBERS-FILTERS-2-SPEC D1 -- the toggle value only
// the Subscribers page applies (an unrecognised stored value). Every other view reads it as All (D2).
export const SUBSCRIBER_ONLY_LANGS = Object.freeze(['other']);

// SUBSCRIBERS-FILTERS D1 (I14) -- the button label of a value. labels: the caller's translations,
// { all, other } (a string is the All label alone, the older signature). en -> EN+ (the send
// language, the "+" convention of langs.js), other, a code -> upper case.
export function langLabel(v, labels = {}) {
  const l = typeof labels === 'string' ? { all: labels } : (labels || {});
  const n = normalizeLang(v);
  if (n === 'all') return l.all || 'All';
  if (n === 'en') return 'EN+';
  if (n === 'other') return l.other || 'Other';
  return n.toUpperCase();
}

// SUBSCRIBERS-FILTERS-2-SPEC D2 (I3) -- whether some list in the brand scope has a member whose
// stored language is the 'other' bucket: the server's has_other_lang (minimal GET /api/lists,
// mat_list_subscriber_stats) on the store's lists. The scope is brandScope.mjs::scopeListIds of
// the selection; under All brands (null) every store list counts (the store is already
// permission-scoped by the server).
export function otherLangPresent(lists, brandSelection) {
  const all = storeLists(lists);
  const ids = scopeListIds(all, brandSelection);
  return all.some((l) => l && (ids === null || ids.includes(l.id))
    && (l.has_other_lang === true || l.hasOtherLang === true));
}

// D2 (I4) -- the toggle's values: LANG_VALUES in order, without 'other' unless it is present in
// scope or it is the current selection (a selection never vanishes under the user). The toggle
// renders from this and is its only caller.
export function toggleValues(otherPresent, langSelection) {
  const keepOther = otherPresent === true || normalizeLang(langSelection) === 'other';
  return LANG_VALUES.filter((v) => v !== 'other' || keepOther);
}

// D1 -- the {lang} of the grey line: the button label on Subscribers; on every other view `en`
// reads bare EN (a campaign's language is never "plus").
export function langNoteLabel(v, onSubscribers, labels = {}) {
  if (!onSubscribers && normalizeLang(v) === 'en') return 'EN';
  return langLabel(v, labels);
}

// A selection's `lang` query parameter: '' (absent) for All, else the code -- EN is `en`, the SEND
// language (en plus no-language) on the Subscribers page, the stored language on Campaigns.
export function langParam(v) {
  const n = normalizeLang(v);
  return n === 'all' ? '' : n;
}

// D1 (S2-14): the toggle is hidden everywhere while app.lang_enable is off.
export function langToggleVisible(serverConfig) {
  return !!(serverConfig && serverConfig.lang_enabled === true);
}

// D1: the selection a view applies -- All whenever the toggle is hidden (no languages to filter).
// SUBSCRIBERS-FILTERS D2 (I2): the ONE home of the fold -- a subscriber-only language is All on
// every view that reads this (every view but Subscribers); no consumer re-derives it.
export function effectiveLang(selection, serverConfig) {
  if (!langToggleVisible(serverConfig)) return 'all';
  const n = normalizeLang(selection);
  return SUBSCRIBER_ONLY_LANGS.includes(n) ? 'all' : n;
}

// SUBSCRIBERS-FILTERS D2 -- the raw selection the Subscribers page and the router guard apply (the
// store's langSubscribers): the normalised value while the toggle is visible, else All.
export function subscribersLang(selection, serverConfig) {
  return langToggleVisible(serverConfig) ? normalizeLang(selection) : 'all';
}

export const APPLICABILITY_STATES = Object.freeze(['all', 'partial', 'none']);

// D4: the three routes that render Subscribers.vue.
export const SUBSCRIBER_ROUTES = Object.freeze(['subscribers', 'subscribers_list', 'subscriber']);

const entry = (state, desc) => Object.freeze({ state, desc });
const NONE = entry('none', 'naming.langApp.none');

// D3. Route name -> { state, desc } | { panes: { <pane>: {state, desc} }, view, defaultPane } |
// { subscribers: true }. A paned view commits '<view>:<pane>' through the store's setViewPane
// mutation; the defaultPane applies before it has (or when the value names another view).
export const LANG_APPLICABILITY = Object.freeze({
  '404_page': NONE,
  dashboard: entry('partial', 'naming.langApp.dashboard'),
  lists: entry('partial', 'naming.langApp.lists'),
  list: entry('partial', 'naming.langApp.lists'),
  forms: NONE,
  brands: Object.freeze({
    view: 'brands',
    defaultPane: 'matrix',
    panes: Object.freeze({
      matrix: entry('partial', 'naming.langApp.brandsMatrix'),
      usage: entry('partial', 'naming.langApp.brandsUsage'),
      health: entry('none', 'naming.langApp.brandLevel'),
    }),
  }),
  journeyPreview: NONE,
  brand: entry('none', 'naming.langApp.brandLevel'),
  subscribers: Object.freeze({ subscribers: true }),
  subscribers_list: Object.freeze({ subscribers: true }),
  subscriber: Object.freeze({ subscribers: true }),
  import: NONE,
  // Bounces is grey by decision (D3): an operations log read per address, not an aggregate.
  bounces: NONE,
  campaigns: entry('all', 'naming.langApp.campaigns'),
  media: NONE,
  templates: entry('all', 'naming.langApp.templates'),
  campaignAnalytics: Object.freeze({
    view: 'analytics',
    defaultPane: 'matrix',
    panes: Object.freeze({
      matrix: entry('all', 'naming.langApp.analyticsMatrix'),
      campaign: entry('none', 'naming.langApp.oneCampaign'),
    }),
  }),
  campaignReview: NONE,
  campaign: NONE,
  userProfile: NONE,
  settings: NONE,
  logs: NONE,
  users: NONE,
  userRoles: NONE,
  listRoles: NONE,
  maintenance: NONE,
});

// The '<view>:<pane>' value the paned views commit (store setViewPane).
export function viewPaneValue(view, pane) {
  return `${view}:${pane}`;
}

function paneOf(def, paneValue) {
  const s = typeof paneValue === 'string' ? paneValue : '';
  const prefix = `${def.view}:`;
  const p = s.startsWith(prefix) ? s.slice(prefix.length) : '';
  return Object.prototype.hasOwnProperty.call(def.panes, p) ? p : def.defaultPane;
}

// D2: { state, desc } for a route. An unknown route is none. SUBSCRIBERS-FILTERS D3: on the
// Subscribers routes the router guard makes the route's lang agree with the selection before the
// page mounts (subscriberFilters.mjs::subscribersRouteFix), so there is no page-filter (partial)
// state any more: the selection applies -- by send language, or by stored language for a
// subscriber-only value. D2 (I4): off Subscribers a subscriber-only selection is grey with the
// Subscribers-only description, whatever the route's own entry says.
export function langApplicability(routeName, { pane, selection } = {}) {
  const def = Object.prototype.hasOwnProperty.call(LANG_APPLICABILITY, routeName) ? LANG_APPLICABILITY[routeName] : NONE;
  const subscriberOnly = SUBSCRIBER_ONLY_LANGS.includes(normalizeLang(selection));
  if (def.subscribers) {
    return { state: 'all', desc: subscriberOnly ? 'naming.langApp.subscribersStored' : 'naming.langApp.subscribers' };
  }
  if (subscriberOnly) {
    return { state: 'none', desc: 'naming.langApp.subscribersOnly' };
  }
  if (def.panes) {
    const e = def.panes[paneOf(def, pane)];
    return { state: e.state, desc: e.desc };
  }
  return { state: def.state, desc: def.desc };
}

// D2/I3: the grey line under the toggle -- shown when the selection is not All (SUBSCRIBERS-FILTERS
// D3 retired the page-filter line: the route always agrees with the selection). null otherwise,
// else { key, lang } for the component to translate.
export function langNote(selection, applicability) {
  const sel = normalizeLang(selection);
  if (!applicability || sel === 'all') {
    return null;
  }
  return { key: applicability.desc, lang: sel };
}

// D4 (stage 4 S4-7): the ONE label of a Subscribers route language value -- the by-query note of
// Subscribers.vue. labels = { all, other, name(code), sendName(code) }: the caller's translations
// (i18n and langs.js stay out of this pure module). '' / 'all' -> all; en -> the send-language name
// (EN+); other; a code -> its name, else the raw value. (SUBSCRIBERS-FILTERS-2-SPEC D1: the guard
// drops a route en_only / none, so neither reaches here.)
export function routeLangLabel(v, labels) {
  if (!v || v === 'all') {
    return labels.all;
  }
  if (v === 'other') {
    return labels.other;
  }
  if (v === 'en') {
    return labels.sendName('en');
  }
  return labels.name(v) || v;
}

// The Subscribers entry redirect (GLOBAL-LANG D4's langEntryRedirect) is retired: every
// navigation to a Subscribers route passes subscriberFilters.mjs::subscribersRouteFix
// (SUBSCRIBERS-FILTERS-SPEC D3), called from both main.js sites.

// D4 (S2-1, I4c): a subscribers query response is applied only when it answers the latest query
// issued by the page that is still mounted.
export function subscriberResponseCurrent(seq, latestSeq, mounted) {
  return mounted === true && Number.isInteger(seq) && seq === latestSeq;
}

// D4 (stage 4b S4b-1): the target of a Subscribers bulk verb (blocklist, delete, the Manage-lists
// add / remove / unsubscribe) -- 'query' ONLY when "select all" is on, 'ids' when rows are checked,
// else 'none' (send nothing). Never a by-ID -> by-query fallthrough: a selection that emptied (a
// query answered while the modal was open) is no request, not the whole filter.
export function bulkTarget({ all, checkedCount } = {}) {
  if (all === true) {
    return 'query';
  }
  return Number.isInteger(checkedCount) && checkedCount > 0 ? 'ids' : 'none';
}

// D4 (stage 4b S4b-2): the filter a by-query body, the export and their confirmation's language
// line are built from -- the queryParams the ACCEPTED answer answered (the snapshot taken when it
// was applied), never the live object, which the search box edits before any query runs. null
// (nothing answered yet) is the busy state; the caller sends nothing.
export function answeredFilter(snapshot) {
  return snapshot && typeof snapshot === 'object' ? { ...snapshot } : null;
}

// D4 (S2-3, I6): the language line shown before every by-query write and export -- '' when the
// request carries no `lang`, else fmt(lang) (the component passes a translated sentence built
// from its own label of the value). params is a request body ({ lang }) or the export's
// [key, value] pairs.
export function byQueryLangNote(params, fmt = (l) => `Language: ${l}`) {
  let lang = '';
  if (Array.isArray(params)) {
    const hit = params.find((p) => Array.isArray(p) && p[0] === 'lang');
    lang = hit ? hit[1] : '';
  } else if (params && typeof params === 'object') {
    lang = params.lang;
  }
  return typeof lang === 'string' && lang !== '' ? fmt(lang) : '';
}
