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

// Fork (brand usage, integrations BRAND-USAGE-V2-SPEC D7/I5). The expanded segment x
// send-language table's %|# toggle (the Brand Matrix's mechanics): per browser SESSION under
// sessionStorage key dashboard.overview { mode }, default pct.
export const OVERVIEW_MODES = Object.freeze(['pct', 'num']);
export const OVERVIEW_DEFAULT = Object.freeze({ mode: 'pct' });
export const OVERVIEW_STORAGE_KEY = 'dashboard.overview';

// D7/I5: an expanded-table cell's text in the mode. The mode governs only RATE-BEARING cells:
// a plain segment (Active, Pending -- null denominator) and a 0-denominator cell are the count in
// both modes. pct shows the rate alone, num the count -- no parentheses in either. fmt formats
// counts (the Vue passes $utils.formatNumber: comma-separated). The Total row is a plain count
// and never comes through here.
export function subsCellText(column, segment, mode = OVERVIEW_DEFAULT.mode, fmt = (n) => String(n)) {
  const c = segmentCell(column, segment);
  if (c.denominator === null || mode === 'num') {
    return fmt(c.count);
  }
  const p = pctCount(c.count, c.denominator, fmt);
  return p.pct || p.count;
}

export function normalizeOverviewState(s) {
  return { mode: s && OVERVIEW_MODES.includes(s.mode) ? s.mode : OVERVIEW_DEFAULT.mode };
}

// I5: a missing store, a throwing accessor or bad JSON is the default, never an error.
export function loadOverviewState(store) {
  try {
    const raw = store ? store.getItem(OVERVIEW_STORAGE_KEY) : null;
    return normalizeOverviewState(raw ? JSON.parse(raw) : null);
  } catch {
    return normalizeOverviewState(null);
  }
}

// A failing store is ignored. Returns whether it wrote.
export function saveOverviewState(store, state) {
  try {
    if (!store) {
      return false;
    }
    store.setItem(OVERVIEW_STORAGE_KEY, JSON.stringify(normalizeOverviewState(state)));
    return true;
  } catch {
    return false;
  }
}

// R1/R2. The Messages-sent lines in display order, every one over Messages sent (the Campaigns
// list's rate over Sent, CAMPAIGN-RATES-SPEC D1). Fork (brand matrix, integrations
// BRAND-MATRIX-SPEC D8) -- the bounce line is H. Bounces (engagement.hard_bounces, camelCased by
// the API client); the every-row bounces field is unchanged on the wire and no longer shown, so
// Complaints is no longer a subset of a shown line and its label is plain.
export function engagementLines(engagement, messages) {
  const e = engagement || {};
  return [
    { key: 'views', label: 'campaigns.views' },
    { key: 'clicks', label: 'campaigns.clicks' },
    { key: 'hardBounces', label: 'dashboard.hardBouncesLine' },
    { key: 'complaints', label: 'dashboard.complaintsLine' },
  ].map((l) => ({ ...l, count: e[l.key] || 0, denominator: messages || 0 }));
}
