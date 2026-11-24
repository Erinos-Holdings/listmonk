// Fork (page state, integrations listmonk/PAGE-STATE-SPEC D1/D2/D7). The ONE reader/writer of the
// list pages' per-browser-session state: one sessionStorage key per page (`page.<name>`, a JSON
// object). A new browser session starts at defaults; a reload or another visit within the session
// restores. Each page's restore/save rules live in its own module (campaignsRouteQuery.mjs,
// subscriberFilters.mjs, listsPage.mjs, mediaPage.mjs, bouncesPage.mjs, usersPage.mjs,
// analyticsPage.mjs); the Vue files only call them. No Vue; tested by pageState.test.mjs.
// Import is excluded (its fields are write parameters, spec §3) and must never import this module.
import { sessionStore } from './brandMatrix.mjs'; // eslint-disable-line import/extensions

// The store the Vue passes in: window.sessionStorage, or null where reading it throws or it is
// absent (brandMatrix.mjs::sessionStore, reused as D2 says).
export { sessionStore };

export const PAGE_STATE_PREFIX = 'page.';

const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

// D2/I1 -- the stored object for `name`, or null: a missing store, a throwing accessor, bad JSON or
// a non-object (null, an array, a number, a string) all read null. Never an error.
export function loadPageState(store, name) {
  try {
    const raw = store ? store.getItem(PAGE_STATE_PREFIX + name) : null;
    if (typeof raw !== 'string') {
      return null;
    }
    const v = JSON.parse(raw);
    return isObject(v) ? v : null;
  } catch {
    return null;
  }
}

// D2/I1 -- write `obj` for `name`; a missing store, a throwing accessor or an unserialisable value
// is ignored (the state simply is not kept). Never an error, never a toast.
export function savePageState(store, name, obj) {
  try {
    if (store && isObject(obj)) {
      store.setItem(PAGE_STATE_PREFIX + name, JSON.stringify(obj));
    }
  } catch {
    // Ignore -- the state simply isn't kept.
  }
}

// ---------------------------------------------------------------------------------------------
// Shared validation (D7) for the per-page restore functions.
// ---------------------------------------------------------------------------------------------

// D7/I5 -- a page number is a positive integer, else 1 (0, -2, "x", 1.5 all read 1). A numeric
// string of a positive integer is accepted (a route value).
export function pageNumberOf(v) {
  const n = typeof v === 'string' && /^\d+$/.test(v) ? Number(v) : v;
  return Number.isSafeInteger(n) && n > 0 ? n : 1;
}

// D7/I5 -- a restored enum value: kept when it is in `allowed`, else `def`.
export function enumOf(v, allowed, def) {
  return allowed.includes(v) ? v : def;
}

// D7 -- a restored text field (a search box): a string, else ''.
export function textOf(v) {
  return typeof v === 'string' ? v : '';
}

// D7/I5 -- a restored list id: kept only if it is a positive integer among `offeredIds` (the lists
// the page currently offers under the global selection); else null ("All lists").
export function keptListId(id, offeredIds) {
  return Number.isInteger(id) && id > 0 && Array.isArray(offeredIds) && offeredIds.includes(id) ? id : null;
}

// D5 -- true when the route query carries any of `keys` (a parameter the page reads): the link wins.
export function routeHasAny(query, keys) {
  const q = isObject(query) ? query : {};
  return keys.some((k) => Object.prototype.hasOwnProperty.call(q, k) && q[k] !== undefined);
}

// D8 -- the in-memory route param a page's own restore replace carries, so the remounted page can
// tell its own restore (apply the non-URL fields too) from a link (apply nothing). Never in the URL:
// a reload or a shared link does not carry it.
export const RESTORE_PARAM = 'pageRestore';

export function isOwnRestore(params) {
  return isObject(params) && params[RESTORE_PARAM] === '1';
}

export const SORT_ORDERS = Object.freeze(['asc', 'desc']);
