// Fork (page state, integrations listmonk/PAGE-STATE-SPEC §3/D8). Campaign Analytics' Campaign pane
// keeps its search in the URL (?id=&from=&to=, pushed by the form). Its session state is the last
// searched query; on a bare entry (none of id/from/to) whose pane is Campaign, the page replaces
// ONCE to it (a single remount, as a shared link would). Never when the route has a query (a link
// wins; that is also what stops a loop). The Matrix pane's state is kept by its own module. No Vue;
// tested by analyticsPage.test.mjs.
import { routeHasAny } from './pageState.mjs'; // eslint-disable-line import/extensions

export const ANALYTICS_PAGE = 'analytics';
export const ANALYTICS_LINK_KEYS = Object.freeze(['id', 'from', 'to']);

// A positive-integer id / a non-negative unix time, as the route string; else null.
function idOf(v) {
  const s = typeof v === 'number' ? String(v) : v;
  return typeof s === 'string' && /^[1-9]\d*$/.test(s) && Number.isSafeInteger(Number(s)) ? s : null;
}

function unixOf(v) {
  const s = typeof v === 'number' ? String(v) : v;
  return typeof s === 'string' && /^\d+$/.test(s) && Number.isSafeInteger(Number(s)) ? s : null;
}

// D6 -- the query a search applied (the route the page mounted on), normalized: the campaign ids
// (one or repeated, garbage dropped, duplicates collapsed), from and to (unix seconds) when present;
// null when there is no id (nothing was searched).
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
  if (id.length === 0) {
    return null;
  }
  const out = { id };
  const from = unixOf(q.from);
  const to = unixOf(q.to);
  if (from !== null) {
    out.from = from;
  }
  if (to !== null) {
    out.to = to;
  }
  return out;
}

// D8/I4 -- the query to replace to, or null: only on a bare entry (no id/from/to) whose initial
// pane is Campaign, with a stored search.
export function analyticsRestoreQuery(stored, query, pane) {
  if (pane !== 'campaign' || routeHasAny(query, ANALYTICS_LINK_KEYS)) {
    return null;
  }
  return saveAnalyticsPage(stored);
}
