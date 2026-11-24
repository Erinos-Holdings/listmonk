// Fork (campaign lists, integrations CAMPAIGN-LISTS-SPEC D7, I9). The Lists page's "View system
// lists" toggle: default OFF, persisted per viewer through the prefs (key lists.showSystem). OFF
// sends hide_system=true, which the server answers with the listing minus its system_hidden lists
// (the query-lists predicate, the SQL twin of cmd isHiddenSystemList). Tested by listsPage.test.mjs.
import {
  SORT_ORDERS, enumOf, pageNumberOf, routeHasAny, textOf,
} from './pageState.mjs'; // eslint-disable-line import/extensions

export const SHOW_SYSTEM_PREF = 'lists.showSystem';

// showSystemLists is the toggle's value for a stored pref: true only for exactly true -- unset
// (null) and anything else is the default, off.
export function showSystemLists(prefValue) {
  return prefValue === true;
}

// systemListsParams is the Lists query parameters for the toggle: hide_system=true exactly when it
// is off; nothing when on (today's listing).
export function systemListsParams(showSystem) {
  return showSystemLists(showSystem) ? {} : { hide_system: true };
}

// ---------------------------------------------------------------------------------------------
// Integrations PAGE-STATE-SPEC §3 -- the Lists page's session state (pageState.mjs): the search,
// the status (active / archived), the sort and the page. A route ?status= wins (D5); the "View
// system lists" toggle keeps its own pref.
// ---------------------------------------------------------------------------------------------

export const LISTS_PAGE = 'lists';
export const LISTS_LINK_KEYS = Object.freeze(['status']);
export const LIST_STATUSES = Object.freeze(['active', 'archived']);
// D7 -- core.go listQuerySortFields as the page's columns sort them (Name, Type, the grid counts,
// Created, Updated).
export const LIST_SORT_FIELDS = Object.freeze([
  'name', 'type', 'created_at', 'updated_at', 'subscriber_count', 'active_count', 'held_count', 'unsubscribed_count',
  'pending_count', 'blocked_count',
]);

export function saveListsPage(qp) {
  const q = qp || {};
  return {
    query: textOf(q.query),
    status: enumOf(q.status, LIST_STATUSES, 'active'),
    orderBy: enumOf(q.orderBy, LIST_SORT_FIELDS, 'created_at'),
    order: enumOf(q.order, SORT_ORDERS, 'desc'),
    page: pageNumberOf(q.page),
  };
}

// D5/D7 -- the state to apply, or null (today's route/default state) when the route carries a
// ?status= or nothing is stored.
export function restoreListsPage(stored, query) {
  if (!stored || typeof stored !== 'object' || routeHasAny(query, LISTS_LINK_KEYS)) {
    return null;
  }
  return saveListsPage(stored);
}
