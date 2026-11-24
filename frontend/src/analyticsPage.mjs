// Fork (page state, integrations listmonk/PAGE-STATE-SPEC §3/D8). Campaign Analytics' Campaign pane
// keeps its search in the URL (?id=&from=&to=, pushed by the form). Its session state is the last
// searched campaign ids; on a bare entry (none of id/from/to) whose pane is Campaign, the page
// replaces ONCE to them (a single remount, as a shared link would). Never when the route has a query
// (a link wins; that is also what stops a loop). The restore runs on entry only: entering on Matrix
// and clicking the Campaign pill shows an empty pane. The Matrix pane's state is kept by its own
// module. No Vue; tested by analyticsPage.test.mjs.
import { routeHasAny } from './pageState.mjs'; // eslint-disable-line import/extensions

export const ANALYTICS_PAGE = 'analytics';
export const ANALYTICS_LINK_KEYS = Object.freeze(['id', 'from', 'to']);

// A positive-integer id, as the route string; else null.
function idOf(v) {
  const s = typeof v === 'number' ? String(v) : v;
  return typeof s === 'string' && /^[1-9]\d*$/.test(s) && Number.isSafeInteger(Number(s)) ? s : null;
}

// D6 -- the campaign ids a search applied (the route the page mounted on), normalized: one or
// repeated, garbage dropped, duplicates collapsed; null when there is no id (nothing was searched).
// Review R3: never from/to -- the form pushes them as absolute times, so a kept range would freeze
// (yesterday's end of day in a tab left open); the landing applies the default range instead, as
// the Campaigns page's id-only link does.
export function saveAnalyticsPage(query) {
  const q = query && typeof query === 'object' ? query : {};
  const raw = Array.isArray(q.id) ? q.id : [q.id];
  const id = [];
  raw.forEach((v) => {
    const s = idOf(v);
    if (s !== null && !id.includes(s)) {
      id.push(s);
    }
  });
  return id.length === 0 ? null : { id };
}

// Review R6 -- a mount with from/to and no id is a search the user cleared (every campaign removed
// and submitted): the kept search is removed.
export function analyticsClearsState(query) {
  const q = query && typeof query === 'object' ? query : {};
  return saveAnalyticsPage(q) === null && (q.from !== undefined || q.to !== undefined);
}

// D8/I4 -- the query to replace to, or null: only on a bare entry (no id/from/to) whose initial
// pane is Campaign, with a stored search.
export function analyticsRestoreQuery(stored, query, pane) {
  if (pane !== 'campaign' || routeHasAny(query, ANALYTICS_LINK_KEYS)) {
    return null;
  }
  return saveAnalyticsPage(stored);
}
