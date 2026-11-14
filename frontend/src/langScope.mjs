// Fork (naming standard, integrations listmonk/NAMING-STANDARD-SPEC D2 §4.4; global language
// filter, integrations listmonk/GLOBAL-LANG-SPEC). The global language toggle
// `All | EN | ES | FR | DE | IT` beside the brand selector: a PRESENTATION filter like the brand
// selector -- it never reaches a server-side permission decision (GLOBAL-LANG D9). Its value is the
// store's `langSelection` getter, persisted in the pref `brand.lang` (the `brand.launch` pattern) by
// the store's selectLang mutation, whose one caller is LanguageToggle.vue. The listing predicate is
// naming.mjs::langMatches (one statement, pinned with the server's query-campaigns lang filter).
//
// GLOBAL-LANG D1/D2 (superseding NAMING-STANDARD §3's "shown only where it filters"): the toggle is
// shown on every route with the navbar (hidden everywhere only when app.lang_enable is off, and the
// selection then reads as All), and each view declares how far the selection applies there --
// LANG_APPLICABILITY below is that table's ONE home: `all` (blue), `partial` (orange) or `none`
// (grey), each with a short description i18n key. Exhaustive: every named route in
// router/index.js has an entry (langScope.test.mjs reads the router file).
import { LANG_VALUES, normalizeLang } from './naming.mjs'; // eslint-disable-line import/extensions

export { LANG_VALUES, normalizeLang };

export const LANG_PREF = 'brand.lang';

// The button label of a value: All, or the upper-cased code.
export function langLabel(v, allLabel = 'All') {
  const n = normalizeLang(v);
  return n === 'all' ? allLabel : n.toUpperCase();
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
export function effectiveLang(selection, serverConfig) {
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

// D4: the route's `lang` as the page applies it ('' = All languages; any other string verbatim,
// including the values the toggle cannot express: en_only, none, other).
function routeLangOf(query) {
  const q = query || {};
  return typeof q.lang === 'string' ? q.lang : '';
}

// D2: { state, desc, pageLang? } for a route. On the Subscribers routes the route wins: when its
// lang (absent = All) differs from the selection's parameter, the state is partial and the
// description is the page filter (pageLang = the route value, 'all' for none). An unknown route
// is none.
export function langApplicability(routeName, { pane, query, selection } = {}) {
  const def = Object.prototype.hasOwnProperty.call(LANG_APPLICABILITY, routeName) ? LANG_APPLICABILITY[routeName] : NONE;
  if (def.subscribers) {
    const routeLang = routeLangOf(query);
    if (routeLang !== langParam(selection)) {
      return { state: 'partial', desc: 'naming.langApp.pageFilter', pageLang: routeLang || 'all' };
    }
    return { state: 'all', desc: 'naming.langApp.subscribers' };
  }
  if (def.panes) {
    const e = def.panes[paneOf(def, pane)];
    return { state: e.state, desc: e.desc };
  }
  return { state: def.state, desc: def.desc };
}

// D2/I3: the grey line under the toggle -- shown when the selection is not All, and (stage 4
// S4-5) also under All when the page's own language filter differs from it (the Subscribers
// page-filter case: a lang=fr link under All reads "Page filter: FR", so colour is never the only
// signal). null otherwise, else { key, lang, pageLang? } for the component to translate.
export function langNote(selection, applicability) {
  const sel = normalizeLang(selection);
  if (!applicability) {
    return null;
  }
  if (sel === 'all') {
    return applicability.pageLang && applicability.pageLang !== 'all'
      ? { key: applicability.desc, lang: sel, pageLang: applicability.pageLang } : null;
  }
  const out = { key: applicability.desc, lang: sel };
  if (applicability.pageLang) {
    out.pageLang = applicability.pageLang;
  }
  return out;
}

// D4 (stage 4 S4-7): the ONE label of a Subscribers route language value, as the page's own picker
// names it -- shared by Subscribers.vue (the by-query note) and LanguageToggle.vue (the page-filter
// line). labels = { all, none, other, name(code), sendName(code) }: the caller's translations
// (i18n and langs.js stay out of this pure module). '' / 'all' -> all; en_only -> the stored English
// name; en -> the send-language name (EN+); none / other; a code -> its name, else the raw value.
export function routeLangLabel(v, labels) {
  if (!v || v === 'all') {
    return labels.all;
  }
  if (v === 'none') {
    return labels.none;
  }
  if (v === 'other') {
    return labels.other;
  }
  if (v === 'en_only') {
    return labels.name('en');
  }
  if (v === 'en') {
    return labels.sendName('en');
  }
  return labels.name(v) || v;
}

// D4 (S2-1/S2-4, I4): the Subscribers entry redirect, decided in the router's beforeEach. Only a
// navigation INTO one of the three Subscribers routes FROM a route that is not one of them, only
// when the incoming query carries neither `lang` nor `segment` (a Lists-grid cell or a lang= link
// is explicit and never rewritten), and only when the selection is not All: the same route with
// lang=<the selection's parameter> (EN -> en, the send language). null otherwise.
export function langEntryRedirect(to, from, selection) {
  const sel = normalizeLang(selection);
  if (!to || sel === 'all' || !SUBSCRIBER_ROUTES.includes(to.name)) {
    return null;
  }
  if (from && SUBSCRIBER_ROUTES.includes(from.name)) {
    return null;
  }
  const q = to.query || {};
  const has = (k) => Object.prototype.hasOwnProperty.call(q, k);
  if (has('lang') || has('segment')) {
    return null;
  }
  return {
    name: to.name, params: { ...(to.params || {}) }, query: { ...q, lang: langParam(sel) }, hash: to.hash || '',
  };
}

// D4 (S2-1, I4c): a subscribers query response is applied only when it answers the latest query
// issued by the page that is still mounted.
export function subscriberResponseCurrent(seq, latestSeq, mounted) {
  return mounted === true && Number.isInteger(seq) && seq === latestSeq;
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
