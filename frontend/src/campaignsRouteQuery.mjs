// Fork (brand usage, integrations listmonk/BRAND-USAGE-V2-SPEC D4/I8; DASHBOARDS-POLISH-SPEC
// D2/I3). The pure half of the Campaigns page's route-query read: the Dashboard's Brand Usage
// cells link to /campaigns -- the welcome/trigger cells with ?list_id=<id>&evergreen=true (the list
// preselected, the Automations scope selected: those campaigns are evergreens, which the Broadcasts
// scope hides), the Journeys circles with ?evergreen=true (no list), and the Cadence / Last
// broadcast cells with ?list_id=<master>&evergreen=false (the Broadcasts scope forced).
// Tested by campaignsRouteQuery.test.mjs (yarn test).
//
// The `evergreen` override is symmetric (DASHBOARDS-POLISH-SPEC D2, superseding the
// BRAND-USAGE-V2 stage-3 "only exactly true overrides" rule): exactly "true" forces Automations
// and exactly "false" forces Broadcasts for that load; anything else keeps the sticky pref.
// Applied to the page's query state ONLY: the result is never written to the sticky
// `campaigns.scopeAutomations` pref -- a link must not flip the user's standing preference.

// A repeated query param arrives as an array; the first element is the one read.
const first = (v) => (Array.isArray(v) ? v[0] : v);

// campaignsRouteQuery returns the initial { listID, evergreen } from $route.query: listID the
// list_id when it is a positive integer string (garbage is ignored -> null, "All lists");
// evergreen true when the query says exactly "true", false when exactly "false", else the
// sticky pref (`prefEvergreen`).
// eslint-disable-next-line import/prefer-default-export
export function campaignsRouteQuery(query, prefEvergreen = false) {
  const q = query && typeof query === 'object' ? query : {};
  const rawList = first(q.list_id);
  const listID = typeof rawList === 'string' && /^[1-9]\d*$/.test(rawList) && Number.isSafeInteger(Number(rawList))
    ? Number(rawList)
    : null;
  const rawEvergreen = first(q.evergreen);
  let evergreen = prefEvergreen === true;
  if (rawEvergreen === 'true') {
    evergreen = true;
  } else if (rawEvergreen === 'false') {
    evergreen = false;
  }
  return { listID, evergreen };
}
