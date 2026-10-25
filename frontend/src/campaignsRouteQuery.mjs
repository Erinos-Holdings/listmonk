// Fork (brand usage, integrations listmonk/BRAND-USAGE-V2-SPEC D4/I8). The pure half of the
// Campaigns page's route-query read: the Dashboard's Brand Usage stoplight cells link to
// /campaigns?list_id=<id>&evergreen=true -- the list preselected and the Automations scope
// selected (welcome and trigger campaigns are evergreens, which the Broadcasts scope hides).
// Tested by campaignsRouteQuery.test.mjs (yarn test).
//
// Applied to the page's query state ONLY: the result is never written to the sticky
// `campaigns.scopeAutomations` pref -- a link must not flip the user's standing preference.

// A repeated query param arrives as an array; the first element is the one read.
const first = (v) => (Array.isArray(v) ? v[0] : v);

// campaignsRouteQuery returns the initial { listID, evergreen } from $route.query: listID the
// list_id when it is a positive integer string (garbage is ignored -> null, "All lists");
// evergreen true when the query says exactly "true", else the sticky pref (`prefEvergreen`).
// eslint-disable-next-line import/prefer-default-export
export function campaignsRouteQuery(query, prefEvergreen = false) {
  const q = query && typeof query === 'object' ? query : {};
  const rawList = first(q.list_id);
  const listID = typeof rawList === 'string' && /^[1-9]\d*$/.test(rawList) && Number.isSafeInteger(Number(rawList))
    ? Number(rawList)
    : null;
  const evergreen = first(q.evergreen) === 'true' ? true : prefEvergreen === true;
  return { listID, evergreen };
}
