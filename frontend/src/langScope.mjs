// Fork (naming standard, integrations listmonk/NAMING-STANDARD-SPEC D2 §4.4). The global language
// toggle `All | EN | ES | FR | DE | IT` beside the brand selector: a PRESENTATION filter like the
// brand selector -- it never reaches a server-side permission decision. Its value is the store's
// `langSelection` getter, persisted in the pref `brand.lang` (the `brand.launch` pattern) by the
// store's selectLang mutation, whose one caller is LanguageToggle.vue. The predicate itself is
// naming.mjs::langMatches (one statement, pinned with the server's query-campaigns lang filter).
//
// Shown only where it filters: Campaigns, the Campaign Matrix (Analytics) and Templates
// -- never Lists, Subscribers, Dashboard, Media or the per-campaign Analytics pane (§3).
import { LANG_VALUES, normalizeLang } from './naming.mjs'; // eslint-disable-line import/extensions

export { LANG_VALUES, normalizeLang };

export const LANG_PREF = 'brand.lang';

// The route names the toggle is shown (and applied) on.
export const LANG_ROUTES = Object.freeze(['campaigns', 'templates', 'campaignAnalytics']);

export function langToggleShown(routeName) {
  return LANG_ROUTES.includes(routeName);
}

// The button label of a value: All, or the upper-cased code.
export function langLabel(v, allLabel = 'All') {
  const n = normalizeLang(v);
  return n === 'all' ? allLabel : n.toUpperCase();
}

// The Campaigns page's `lang` query parameter: '' (absent) for All.
export function langParam(v) {
  const n = normalizeLang(v);
  return n === 'all' ? '' : n;
}
