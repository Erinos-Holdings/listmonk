// Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC D2/D3/D7). The pure half of the
// Dashboard's Subscribers container: which segment lines it shows (rows) and which language
// columns the expanded table carries (columns). The grid is the API's subscribers.grid -- the
// Lists grid's shape summed over the scope's public lists. Tested by dashboardSubs.test.mjs
// (npm run test:dashboard-subs).
import { gridLines } from './gridLines.mjs'; // eslint-disable-line import/extensions

// D2. The segment lines in display order, each with its grid key and label key. Blocklisted is
// the grid's blocked segment under the user-facing word.
const SEGMENTS = [
  { key: 'active', label: 'lists.grid.active' },
  { key: 'held', label: 'lists.grid.held' },
  { key: 'unsubscribed', label: 'lists.grid.unsubscribed' },
  { key: 'blocked', label: 'subscribers.status.blocklisted' },
  { key: 'pending', label: 'lists.grid.pending' },
];

// The rows: every segment, Pending only when the all row counts someone pending (no double
// opt-in list, no Pending line -- as the Lists grid hides it for single opt-in lists). Orphans is
// not a grid row; the view adds it (All brands only).
export function subscriberRows(subscriberGrid) {
  const all = (subscriberGrid && subscriberGrid.all) || {};
  return SEGMENTS.filter((s) => s.key !== 'pending' || all.pending > 0);
}

// D3. The expanded table's columns: all, then the Lists grid's language lines (gridLines), each
// { key, row, none } -- none is the no-language subset on the en column (the EN+ tooltip).
export function subscriberColumns(subscriberGrid, langEnabled) {
  return gridLines(subscriberGrid, langEnabled);
}
