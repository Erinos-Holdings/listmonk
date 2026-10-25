// Fork (brand usage, integrations listmonk/BRAND-USAGE-V2-SPEC D4/I8; DASHBOARDS-POLISH-SPEC
// D2/I3; DASHBOARDS-POLISH2-SPEC D2/I2). The pure half of the Campaigns page's route-query read
// and of the list-filter state it seeds: the Dashboard's Brand Usage cells link to /campaigns --
// the welcome/trigger cells with ?list_id=<id>&evergreen=true (the list preselected, the
// Automations scope selected: those campaigns are evergreens, which the Broadcasts scope hides),
// the Journeys circles with ?list_id=<each of the brand's active lists>&evergreen=true, and the
// Cadence / Last broadcast cells with the same brand list set and evergreen=false (the Broadcasts
// scope forced). Tested by campaignsRouteQuery.test.mjs (yarn test); Campaigns.vue holds the
// wiring only.
//
// list_id is read as ONE value or an ARRAY (a repeated param): each a positive-integer string,
// garbage entries dropped, duplicates collapsed, order preserved. ONE id is the in-page single
// list filter (the Lists select preselected). SEVERAL are the ROUTE LIST SCOPE: sent as the
// list_id array in the single filter's precedence slot (it beats the global brand-selector
// scope), shown in the Lists select as a disabled "n linked lists" placeholder, cleared by any
// real list selection (or All lists) and by ANY brand-selector change, and -- like the single
// filter and the selector scope -- it suppresses the "select all N" by-query bulk delete, which
// carries only query + evergreen and would delete campaigns the page is not showing.
//
// The `evergreen` override is symmetric (DASHBOARDS-POLISH-SPEC D2, superseding the
// BRAND-USAGE-V2 stage-3 "only exactly true overrides" rule): exactly "true" forces Automations
// and exactly "false" forces Broadcasts for that load; anything else keeps the sticky pref.
// Applied to the page's query state ONLY: nothing here writes the sticky
// `campaigns.scopeAutomations` pref (or any pref) -- a link must not flip the user's standing
// preference.

// A repeated query param arrives as an array; the first element is the one read (evergreen).
const first = (v) => (Array.isArray(v) ? v[0] : v);

const listIdOf = (v) => (typeof v === 'string' && /^[1-9]\d*$/.test(v) && Number.isSafeInteger(Number(v))
  ? Number(v)
  : null);

// The Lists select's value for the disabled route-scope placeholder option.
export const ROUTE_SCOPE_OPTION = 'route-scope';

// campaignsRouteQuery returns the initial { listID, routeListIDs, evergreen } from $route.query:
// the valid list_id values (one or repeated; garbage dropped, duplicates collapsed, order kept) --
// exactly one is listID (routeListIDs null), two or more are routeListIDs (listID null), none is
// neither ("All lists"); evergreen true when the query says exactly "true", false when exactly
// "false", else the sticky pref (`prefEvergreen`).
export function campaignsRouteQuery(query, prefEvergreen = false) {
  const q = query && typeof query === 'object' ? query : {};
  const raw = Array.isArray(q.list_id) ? q.list_id : [q.list_id];
  const ids = [];
  raw.forEach((v) => {
    const id = listIdOf(v);
    if (id !== null && !ids.includes(id)) {
      ids.push(id);
    }
  });
  const rawEvergreen = first(q.evergreen);
  let evergreen = prefEvergreen === true;
  if (rawEvergreen === 'true') {
    evergreen = true;
  } else if (rawEvergreen === 'false') {
    evergreen = false;
  }
  return {
    listID: ids.length === 1 ? ids[0] : null,
    routeListIDs: ids.length > 1 ? ids : null,
    evergreen,
  };
}

const hasRouteScope = (ids) => Array.isArray(ids) && ids.length > 0;

// listParam is the GET /api/campaigns list_id for the page state: the single filter, else the
// route list scope (the same precedence slot), else the global brand-selector scope; undefined =
// no param ("All lists" with no scope).
export function listParam({ listID = null, routeListIDs = null, scopeListIds = null } = {}) {
  if (listID !== null && listID !== undefined) {
    return listID;
  }
  if (hasRouteScope(routeListIDs)) {
    return routeListIDs;
  }
  if (scopeListIds) {
    return scopeListIds;
  }
  return undefined;
}

// listSelectValue is the Lists select's displayed value: the placeholder option while the route
// scope is active, else the single filter (null = "All lists").
export function listSelectValue({ listID = null, routeListIDs = null } = {}) {
  return hasRouteScope(routeListIDs) ? ROUTE_SCOPE_OPTION : listID;
}

// selectList is the filter state after the user picks in the Lists select: a real list (or All
// lists, null) becomes the single filter and CLEARS the route scope. The placeholder itself is
// disabled; were it ever emitted it changes nothing.
export function selectList(state, value) {
  const s = state || {};
  if (value === ROUTE_SCOPE_OPTION) {
    return { listID: s.listID ?? null, routeListIDs: s.routeListIDs ?? null };
  }
  return { listID: value === undefined ? null : value, routeListIDs: null };
}

// afterBrandChange is the filter state after ANY global brand-selector change: the route scope is
// cleared (it must never outlive a navbar brand choice), and a single filter outside the new scope
// resets to All lists (GLOBAL-BRAND-SPEC D4).
export function afterBrandChange(state, scopeListIds) {
  const s = state || {};
  let listID = s.listID ?? null;
  if (listID !== null && scopeListIds && !scopeListIds.includes(listID)) {
    listID = null;
  }
  return { listID, routeListIDs: null };
}

// canSelectAll guards the "select all N" whole-query affordance: offered only when the page shows
// MORE than one page of an UNNARROWED query -- never under the single filter, the route list scope
// or the brand-selector scope, since the by-query bulk delete carries only query + evergreen.
export function canSelectAll({
  bulkAll = false, total = 0, perPage = 0, listID = null, routeListIDs = null, scopeListIds = null,
} = {}) {
  return !bulkAll && total > perPage && (listID === null || listID === undefined)
    && !hasRouteScope(routeListIDs) && !scopeListIds;
}
