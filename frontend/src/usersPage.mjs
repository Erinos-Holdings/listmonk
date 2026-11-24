// Fork (page state, integrations listmonk/PAGE-STATE-SPEC §3). The Users page's session state
// (pageState.mjs): the search and the sort. The listing is unpaginated, so no page is kept (review
// R7; add it when Users gains pagination). The page reads no route query, so no link is overridden
// (D5). No Vue; tested by usersPage.test.mjs.
import { SORT_ORDERS, enumOf, textOf } from './pageState.mjs'; // eslint-disable-line import/extensions

export const USERS_PAGE = 'users';
// D7 -- the sortable columns (and the default id).
export const USER_SORT_FIELDS = Object.freeze(['id', 'username', 'status', 'name', 'created_at', 'updated_at', 'last_login']);

export function saveUsersPage(qp) {
  const q = qp || {};
  return {
    query: textOf(q.query),
    orderBy: enumOf(q.orderBy, USER_SORT_FIELDS, 'id'),
    order: enumOf(q.order, SORT_ORDERS, 'asc'),
  };
}

// D7 -- the state to apply on mount, or null (today's defaults) when nothing is stored.
export function restoreUsersPage(stored) {
  return stored && typeof stored === 'object' ? saveUsersPage(stored) : null;
}
