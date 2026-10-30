// Fork (brand usage, integrations listmonk/BRAND-USAGE-V2-SPEC D4/I8; DASHBOARDS-POLISH-SPEC
// D2/I3; DASHBOARDS-POLISH2-SPEC D2/I2). The pure half of the Campaigns page's route-query read
// and of the list-filter state it seeds: the Brands page's Usage pane links to /campaigns -- each
// stoplight circle (integrations LIFECYCLE-SERIES-SPEC §10 A9, brandUsage.mjs::circleLink) with
// ?list_id=<its steps' lists>&evergreen=true (step and SKU circles: the Automations scope, which
// shows the evergreens the Broadcasts scope hides) or ?id=<its campaigns>&evergreen=all (the
// No-purchase Day 0 circles), and the Cadence / Last broadcast cells with the brand's active list
// set and evergreen=false&status=finished (the Broadcasts scope forced). Tested by
// campaignsRouteQuery.test.mjs (yarn test); Campaigns.vue holds the wiring only.
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
//
// DASHBOARDS-POLISH3-SPEC D2/D3 (I2/I3):
// - The scope pill is Broadcasts | Automations | All. The page's internal representation is
//   `evergreen`: false (Broadcasts), true (Automations), null (All -- the param is OMITTED, so the
//   server returns both kinds). The sticky pref is the string `campaigns.scopeKind`
//   (broadcasts|automations|all); the legacy boolean `campaigns.scopeAutomations` is read only
//   while scopeKind is unset and is never written again (the dashboard.pane precedent). The route
//   `evergreen` override gains exactly "all" (All for that load), never written to a pref.
// - A route `status` (one value of the campaign status enum; anything else ignored) is the ROUTE
//   STATUS SCOPE: sent as GET /api/campaigns `status`, shown as a dismissible chip beside the
//   pill, cleared by the chip's dismissal and by any real list, brand-selector or scope-pill
//   interaction, never written to a pref, and -- a fourth narrowing -- it suppresses "select all".
//
// MATRIX-POLISH-SPEC D10.2 (I5): a route `id` (repeatable; the Brand Usage "Last broadcast" link) is
// the ROUTE ID SCOPE: only those campaigns (GET /api/campaigns `id`), the kind pill at All for that
// view (never written to the pref), a dismissible "N selected campaigns" chip, cleared by exactly
// the route status scope's interactions, and -- a fifth narrowing -- it suppresses "select all".

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
// "false", null (All) when exactly "all" (POLISH3 D3), else the sticky pref (`prefEvergreen`: true,
// false or null -- evergreenOfKind(scopeKindPref(...)); anything else reads as false).
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
  let evergreen = prefEvergreen === null ? null : prefEvergreen === true;
  if (rawEvergreen === 'true') {
    evergreen = true;
  } else if (rawEvergreen === 'false') {
    evergreen = false;
  } else if (rawEvergreen === 'all') {
    evergreen = null;
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
// MORE than one page of an UNNARROWED query -- never under the single filter, the route list scope,
// the brand-selector scope, (POLISH3 D2) the route status scope or (MATRIX-POLISH-SPEC D10.2) the
// route id scope, since the by-query bulk delete carries only query + evergreen.
export function canSelectAll({
  bulkAll = false, total = 0, perPage = 0, listID = null, routeListIDs = null, scopeListIds = null, routeStatus = null,
  routeIDs = null,
} = {}) {
  return !bulkAll && total > perPage && (listID === null || listID === undefined)
    && !hasRouteScope(routeListIDs) && !scopeListIds && !routeStatus && !hasRouteScope(routeIDs);
}

// ---------------------------------------------------------------------------------------------
// MATRIX-POLISH-SPEC D10.2 -- the route id scope (the Brand Usage "Last broadcast" link)
// ---------------------------------------------------------------------------------------------

// routeIdsOf is the route id scope from $route.query: the valid `id` values (one or repeated;
// positive-integer strings, garbage dropped, duplicates collapsed, order kept), or null when none.
// Sent as GET /api/campaigns' repeatable `id` (only those campaigns, ANDed with every other filter
// and the permission scope); in-page state only, never a pref.
export function routeIdsOf(query) {
  const q = query && typeof query === 'object' ? query : {};
  const raw = Array.isArray(q.id) ? q.id : [q.id];
  const ids = [];
  raw.forEach((v) => {
    const id = listIdOf(v);
    if (id !== null && !ids.includes(id)) {
      ids.push(id);
    }
  });
  return ids.length ? ids : null;
}

// routeEvergreen is the page's initial kind state given the route id scope: All (null) while ids
// are set -- the linked campaigns show whatever their kind -- else the route/pref value. Applied to
// the query state only: never written to the scope-kind pref.
export function routeEvergreen(evergreen, routeIDs) {
  return hasRouteScope(routeIDs) ? null : evergreen;
}

// ---------------------------------------------------------------------------------------------
// DASHBOARDS-POLISH3-SPEC D2 -- the route status scope
// ---------------------------------------------------------------------------------------------

// The campaign status enum (models/campaigns.go CampaignStatus*): the only values a route `status`
// may carry.
export const CAMPAIGN_STATUSES = Object.freeze(['draft', 'scheduled', 'running', 'paused', 'finished', 'cancelled']);

// routeStatusOf is the route status scope from $route.query: exactly one enum value (a repeated
// param reads its first element, like evergreen), else null (ignored).
export function routeStatusOf(query) {
  const q = query && typeof query === 'object' ? query : {};
  const v = first(q.status);
  return typeof v === 'string' && CAMPAIGN_STATUSES.includes(v) ? v : null;
}

// The interactions that clear the route status scope: the chip's dismissal, a real Lists-select
// pick (or All lists), any brand-selector change, and the scope pill.
export const STATUS_CLEARING = Object.freeze(['dismiss', 'list', 'brand', 'scope']);

// routeStatusAfter is the route status scope after an interaction: cleared (null) by a clearing
// one, else unchanged (paging, sorting, a search keep it).
export function routeStatusAfter(current, interaction) {
  if (STATUS_CLEARING.includes(interaction)) {
    return null;
  }
  return CAMPAIGN_STATUSES.includes(current) ? current : null;
}

// MATRIX-POLISH-SPEC D10.2 -- routeIdsAfter is the route id scope after an interaction: cleared
// (null) by exactly the route status scope's clearing interactions (STATUS_CLEARING: the chip's
// dismissal, a real list pick, a brand-selector change, the kind pill -- whose click then applies
// and writes the pref normally), else unchanged (paging, sorting, a search keep it).
export function routeIdsAfter(current, interaction) {
  if (STATUS_CLEARING.includes(interaction)) {
    return null;
  }
  return hasRouteScope(current) ? current : null;
}

// ---------------------------------------------------------------------------------------------
// DASHBOARDS-POLISH3-SPEC D3 -- the All pill and its pref
// ---------------------------------------------------------------------------------------------

export const SCOPE_KINDS = Object.freeze(['broadcasts', 'automations', 'all']);
export const SCOPE_KIND_PREF = 'campaigns.scopeKind';
export const LEGACY_SCOPE_PREF = 'campaigns.scopeAutomations';

// scopeKindPref is the standing scope kind: the string pref when valid, else the legacy boolean
// (true -> automations) -- read only while the string pref is unset -- else broadcasts.
export function scopeKindPref(kindPref, legacyAutomations) {
  if (SCOPE_KINDS.includes(kindPref)) {
    return kindPref;
  }
  return legacyAutomations === true ? 'automations' : 'broadcasts';
}

// evergreenOfKind / kindOfEvergreen map the pill's kind to the page's evergreen state and back:
// broadcasts false, automations true, all null.
export function evergreenOfKind(kind) {
  if (kind === 'automations') {
    return true;
  }
  if (kind === 'all') {
    return null;
  }
  return false;
}

export function kindOfEvergreen(evergreen) {
  if (evergreen === true) {
    return 'automations';
  }
  if (evergreen === null) {
    return 'all';
  }
  return 'broadcasts';
}

// kindStatusParams is the GET /api/campaigns kind/status params for the page state: evergreen sent
// as a boolean unless All (null -> omitted), status sent only under the route status scope, and
// (MATRIX-POLISH-SPEC D10.2) the repeatable id only under the route id scope.
export function kindStatusParams({ evergreen = false, routeStatus = null, routeIDs = null } = {}) {
  const p = {};
  if (evergreen === true || evergreen === false) {
    p.evergreen = evergreen;
  }
  if (CAMPAIGN_STATUSES.includes(routeStatus)) {
    p.status = routeStatus;
  }
  if (hasRouteScope(routeIDs)) {
    p.id = routeIDs;
  }
  return p;
}

// The three boolean consumers of `evergreen`, pinned (stage 2 F2):
// - the by-query bulk delete carries evergreen only when it is a boolean (All omits it, so the
//   whole-query delete matches exactly the both-kinds view on screen);
export function deleteEvergreenParam(evergreen) {
  return evergreen === true || evergreen === false ? { evergreen } : {};
}

// - the running-campaign progress poll runs whenever broadcasts are displayed (All included);
export function pollsStats(evergreen) {
  return evergreen !== true;
}

// - New pre-enables the evergreen toggle only from Automations.
export function preEnablesEvergreen(evergreen) {
  return evergreen === true;
}

// displayedRunning is whether the poll has anything on screen to follow: a displayed running
// BROADCAST. A running evergreen never finishes, so counting it under All would keep a 1/sec poll
// alive forever (the reason Automations is never polled); in Broadcasts no evergreen is displayed,
// so this equals the old "any displayed row running" rule there.
export function displayedRunning(rows) {
  return (Array.isArray(rows) ? rows : []).some((c) => c && c.status === 'running' && !c.evergreen);
}
