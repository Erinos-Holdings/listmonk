// Fork (page state, integrations listmonk/PAGE-STATE-SPEC §3). The Bounces page's session state
// (pageState.mjs): the sort and the page. A route ?campaign_id= (a link scope, D4) or ?source=
// (a row's source link) wins (D5) and neither is kept: the page shows no source filter and offers
// no control to clear one, so a restored source would be a hidden filter the menu could no longer
// clear (D3: only values the user can SEE set). No Vue; tested by bouncesPage.test.mjs.
import {
  SORT_ORDERS, enumOf, pageNumberOf, routeHasAny,
} from './pageState.mjs'; // eslint-disable-line import/extensions

export const BOUNCES_PAGE = 'bounces';
export const BOUNCES_LINK_KEYS = Object.freeze(['campaign_id', 'source']);
// D7 -- the sortable columns.
export const BOUNCE_SORT_FIELDS = Object.freeze(['email', 'campaign', 'source', 'type', 'created_at']);

// Review R1(c): a fetch under a link (campaign_id or source) saves page 1 -- its page belongs to the
// linked set, which a later bare entry does not show.
export function saveBouncesPage(qp) {
  const q = qp || {};
  const linked = !!q.campaign_id || !!q.source;
  return {
    orderBy: enumOf(q.orderBy, BOUNCE_SORT_FIELDS, 'created_at'),
    order: enumOf(q.order, SORT_ORDERS, 'desc'),
    page: linked ? 1 : pageNumberOf(q.page),
  };
}

// D5 -- the sort and page to apply on mount, or null (today's route/default state): null when the
// route carries a campaign_id or source, or nothing is stored.
export function restoreBouncesPage(stored, query) {
  if (!stored || typeof stored !== 'object' || routeHasAny(query, BOUNCES_LINK_KEYS)) {
    return null;
  }
  return saveBouncesPage({ orderBy: stored.orderBy, order: stored.order, page: stored.page });
}
