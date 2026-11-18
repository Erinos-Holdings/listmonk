// Fork (integrations listmonk/CAMPAIGN-STATUS-FILTER-SPEC D4/D6/D7). The pure half of the
// Campaigns page's Status header filter and its Paused timestamp row. Tested by
// campaignStatusFilter.test.mjs (yarn test); Campaigns.vue holds the wiring only.
//
// - pausedAtOf (D4, I7): the Paused row's value -- shown only for a paused row with paused_at set.
// - statusOptions (D6, I8): the dropdown's items -- All first, then the present statuses in
//   CAMPAIGN_STATUSES order, plus the selected status even at a count of 0.
// - campaignFilterParams / statusCountParams (D7, I9): the ONE builder of the filter params sent
//   to GET /api/campaigns and to GET /api/campaigns/statuses (the latter minus status). Paging,
//   ordering and no_body are the list call's own additions, never here.
//
// The status pick itself (pickStatus, D6/I10) sits beside selectList in campaignsRouteQuery.mjs:
// it sets the page's ONE status state, queryParams.routeStatus.

import { CAMPAIGN_STATUSES, kindStatusParams, listParam } from './campaignsRouteQuery.mjs'; // eslint-disable-line import/extensions
import { langParam } from './langScope.mjs'; // eslint-disable-line import/extensions

// pausedAtOf is the Paused row's timestamp for a raw (camelCased) campaign row: its pausedAt when
// the row's status is paused and the value is set, else null. A resumed campaign keeps its stored
// paused_at (nothing clears it) but shows no row.
export function pausedAtOf(row) {
  if (!row || row.status !== 'paused') {
    return null;
  }
  const v = row.pausedAt;
  return v === null || v === undefined || v === '' ? null : v;
}

// statusOptions is the dropdown's ordered items, [{ value, count }]: value null is "All statuses"
// (count null). counts is the D5 response ([{ status, count }]); null means "not loaded" (the
// request failed) -- then every status is offered without a count (count null), the filter still
// works and the list endpoint stays the authority. Otherwise only statuses with a count above 0,
// in CAMPAIGN_STATUSES order, plus the selected status at 0 when it is absent (the selection is
// always visible and reversible).
export function statusOptions(counts, selected) {
  const out = [{ value: null, count: null }];
  if (!Array.isArray(counts)) {
    CAMPAIGN_STATUSES.forEach((s) => out.push({ value: s, count: null }));
    return out;
  }
  const byStatus = {};
  counts.forEach((c) => {
    if (c && CAMPAIGN_STATUSES.includes(c.status) && Number.isFinite(c.count) && c.count > 0) {
      byStatus[c.status] = c.count;
    }
  });
  CAMPAIGN_STATUSES.forEach((s) => {
    if (byStatus[s] > 0) {
      out.push({ value: s, count: byStatus[s] });
    } else if (s === selected) {
      out.push({ value: s, count: 0 });
    }
  });
  return out;
}

// campaignFilterParams is the filter params of GET /api/campaigns for the page state: the
// sanitised search (always sent, as before), kind/status/id (kindStatusParams), list_id
// (listParam: the single filter, else the route list scope, else the brand-selector scope; absent
// under "All lists") and lang (absent under All).
// state: { query, evergreen, routeStatus, routeIDs, listID, routeListIDs, scopeListIds, langSelection }
export function campaignFilterParams(state) {
  const s = state || {};
  const params = {
    query: String(s.query || '').replace(/[^\p{L}\p{N}\s]/gu, ' '),
    ...kindStatusParams({ evergreen: s.evergreen, routeStatus: s.routeStatus, routeIDs: s.routeIDs }),
  };
  const listID = listParam({ listID: s.listID, routeListIDs: s.routeListIDs, scopeListIds: s.scopeListIds });
  if (listID !== undefined) {
    params.list_id = listID;
  }
  const lang = langParam(s.langSelection);
  if (lang) {
    params.lang = lang;
  }
  return params;
}

// statusCountParams is the GET /api/campaigns/statuses params: exactly campaignFilterParams minus
// status.
export function statusCountParams(state) {
  const { status, ...rest } = campaignFilterParams(state); // eslint-disable-line no-unused-vars
  return rest;
}
