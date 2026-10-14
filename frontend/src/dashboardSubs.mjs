// Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC D2/D3/D7, Revision 2 R1/R2). The pure
// half of the Dashboard's Subscribers container: which segment lines it shows (rows), which
// language columns the expanded table carries (columns), and -- Revision 2 -- which lines read
// rate-first and over WHICH denominator (cells, engagement lines), so the Vue cannot pick another.
// The grid is the API's subscribers.grid -- the Lists grid's shape summed over the scope's public
// lists. Tested by dashboardSubs.test.mjs (npm run test:dashboard-subs).
import { gridLines } from './gridLines.mjs'; // eslint-disable-line import/extensions
import { rateCell } from './campaignRates.mjs'; // eslint-disable-line import/extensions

// D2. The segment lines in display order, each with its grid key and label key. Blocklisted is
// the grid's blocked segment under the user-facing word. R1: Held, Unsubscribed and Blocklisted
// read rate-first over the column's total; Active (its share is implied by the rest) and Pending
// stay plain counts.
const SEGMENTS = [
  { key: 'active', label: 'lists.grid.active', rate: false },
  { key: 'held', label: 'lists.grid.held', rate: true },
  { key: 'unsubscribed', label: 'lists.grid.unsubscribed', rate: true },
  { key: 'blocked', label: 'subscribers.status.blocklisted', rate: true },
  { key: 'pending', label: 'lists.grid.pending', rate: false },
];

// The rows: every segment, Pending only when the all row counts someone pending (no double
// opt-in list, no Pending line -- as the Lists grid hides it for single opt-in lists). Orphans is
// not a grid row; the view adds it (a plain count, shown in every scope -- R3).
export function subscriberRows(subscriberGrid) {
  const all = (subscriberGrid && subscriberGrid.all) || {};
  return SEGMENTS.filter((s) => s.key !== 'pending' || all.pending > 0);
}

// D3. The expanded table's columns: all, then the Lists grid's language lines (gridLines), each
// { key, row, none } -- none is the no-language subset on the en column (the EN+ tooltip).
export function subscriberColumns(subscriberGrid, langEnabled) {
  return gridLines(subscriberGrid, langEnabled);
}

// R1. pctCount is the rate-first pair { pct, count }: pct is count / denominator as a percentage
// with one decimal -- campaignRates.mjs::rateCell, the Campaigns list's own formatting -- or null
// when the denominator is 0 (or absent), in which case the line shows the count alone. fmt
// formats the count (the Vue passes $utils.formatNumber; en-US by default).
export function pctCount(count, denominator, fmt) {
  return rateCell(count, denominator, 1, fmt);
}

// R1. segmentCell is one segment line of one column ({ key, row } -- the collapsed list is the
// all column): { count, denominator }, the denominator THAT column's total for a rate segment and
// null for a plain one (Active, Pending).
export function segmentCell(column, segment) {
  const row = (column && column.row) || {};
  return {
    count: row[segment.key] || 0,
    denominator: segment.rate ? (row.total || 0) : null,
  };
}

// R1/R2. The Messages-sent lines in display order, every one over Messages sent (the Campaigns
// list's rate over Sent, CAMPAIGN-RATES-SPEC D1). Bounces keeps get-campaign-stats' definition
// (every bounce row); Complaints is a named subset beneath it, its label carrying help.
export function engagementLines(engagement, messages) {
  const e = engagement || {};
  return [
    { key: 'views', label: 'campaigns.views' },
    { key: 'clicks', label: 'campaigns.clicks' },
    { key: 'bounces', label: 'globals.terms.bounces' },
    { key: 'complaints', label: 'dashboard.ses.complaints', help: 'dashboard.complaintsHelp' },
  ].map((l) => ({ ...l, count: e[l.key] || 0, denominator: messages || 0 }));
}
