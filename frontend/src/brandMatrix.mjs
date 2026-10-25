// Fork (brand matrix, integrations listmonk/BRAND-MATRIX-SPEC D2/D4/D5/D6). The pure half of the
// Dashboard's Brand Matrix pane: row shaping (display name, launch filter), the totals row, the
// rates, the %|# display mode, the sort (default directions, initial sort, missing-last) and the
// per-session persistence wrapper. No Vue, no DOM; tested by brandMatrix.test.mjs (yarn test).
//
// The rows come from GET /api/dashboard/brand-matrix verbatim (camelCase: false) -- counts only:
// { brand, subscribers: {total, active, held, unsubscribed, blocked}, sent, clicks, hard_bounces,
//   blocks, complaints, health: {status, spam_rate, spam_status, spam_flagged}, thresholds? }. Every rate is computed
// here, over the one windowed denominator `sent` (D4); the server never returns a rate.
// `thresholds` (DASHBOARDS-POLISH-SPEC D1) is the stored engagement-threshold object, absent on
// older documents; see engagementCell.
import { brandLabelOf, matchesLaunch } from './brandScope.mjs'; // eslint-disable-line import/extensions

// D4: the Engagement section's columns -- each a count over `sent`.
export const ENGAGEMENT_FIELDS = Object.freeze(['clicks', 'hard_bounces', 'blocks', 'complaints']);

// The subscriber columns -- always counts, never governed by the mode.
export const SUBSCRIBER_FIELDS = Object.freeze(['total', 'active', 'held', 'unsubscribed', 'blocked']);

// Every sortable column. The health chip is not sortable.
export const SORT_FIELDS = Object.freeze(['brand', ...SUBSCRIBER_FIELDS, 'sent', ...ENGAGEMENT_FIELDS, 'spam_rate']);

export const MODES = Object.freeze(['pct', 'num']);

// D6: the initial sort on page load is Clicks, descending; the mode is pct.
export const DEFAULT_STATE = Object.freeze({ sort: Object.freeze({ field: 'clicks', order: 'desc' }), mode: 'pct' });

// D6: the sessionStorage key of { sort: {field, order}, mode }.
export const STORAGE_KEY = 'dashboard.matrix';

// The rate digits: blocks and complaints are hundredths of a percent at healthy volume, so every
// engagement rate shows two decimals (one shared format for the section).
export const RATE_DIGITS = 2;

const num = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const numOrNull = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : null);
const isObj = (v) => !!v && typeof v === 'object' && !Array.isArray(v);

// DASHBOARDS-POLISH-SPEC D1: thresholdsOf is the stored engagement-thresholds object when it is
// one, else null (see engagementCell).
export function thresholdsOf(v) {
  return isObj(v) ? v : null;
}

// DASHBOARDS-POLISH-SPEC D1 (stage 2 F2): firstThresholds is the thresholds object of the FIRST row
// in brand order that carries one -- the totals row's source (the values are global constants, so
// any carrier is equivalent); null when no row carries one (the totals row then colors nothing).
export function firstThresholds(rows) {
  const carriers = (Array.isArray(rows) ? rows : []).filter((x) => x && thresholdsOf(x.thresholds));
  if (!carriers.length) {
    return null;
  }
  const brandOf = (x) => (typeof x.brand === 'string' ? x.brand : '');
  carriers.sort((a, b) => {
    if (brandOf(a) === brandOf(b)) {
      return 0;
    }
    return brandOf(a) < brandOf(b) ? -1 : 1;
  });
  return carriers[0].thresholds;
}

// shapeRow flattens one API row: { brand, label, total..blocked, sent (null when absent), the four
// numerators, spam_rate (null when absent), status ('' when the brand has no health document) }.
export function shapeRow(r, brands) {
  const s = (r && r.subscribers) || {};
  const h = (r && r.health) || {};
  const out = {
    brand: r.brand,
    label: brandLabelOf(r.brand, brands),
    sent: numOrNull(r.sent),
    spam_rate: numOrNull(h.spam_rate),
    // GP-SPAM-VISIBILITY-SPEC D2/D3: the stored spamRate status ('' when absent) and the stored
    // compliance flag (true only when the server says exactly true).
    spam_status: typeof h.spam_status === 'string' ? h.spam_status : '',
    spam_flagged: h.spam_flagged === true,
    status: typeof h.status === 'string' ? h.status : '',
    // DASHBOARDS-POLISH-SPEC D1: the stored engagement thresholds object, verbatim (null when the
    // brand's latest document carries none -- the row then colors nothing).
    thresholds: thresholdsOf(r.thresholds),
    totals: false,
  };
  SUBSCRIBER_FIELDS.forEach((k) => { out[k] = num(s[k]); });
  ENGAGEMENT_FIELDS.forEach((k) => { out[k] = num(r[k]); });
  return out;
}

// healthDocs is the matrix rows' own health documents as the launch filter reads them
// ({ brand, status }) -- a brand with no document has none, and so is launched (D2, D5).
export function healthDocs(apiRows) {
  return (Array.isArray(apiRows) ? apiRows : [])
    .filter((r) => r && r.health && typeof r.health.status === 'string')
    .map((r) => ({ brand: r.brand, status: r.health.status }));
}

// D2: shapeRows is the API rows narrowed by the launch filter (brandScope.mjs matchesLaunch over the
// rows' own health documents) and flattened. The server has already applied the brand scope.
export function shapeRows(apiRows, { brands = [], launch = 'all' } = {}) {
  const rows = (Array.isArray(apiRows) ? apiRows : []).filter((r) => r && typeof r.brand === 'string' && r.brand !== '');
  const docs = healthDocs(rows);
  return rows.filter((r) => matchesLaunch(r.brand, launch, docs)).map((r) => shapeRow(r, brands));
}

// D2/I5: the totals row -- every count summed; sent summed over the rows that carry one (null when
// none does); its rates are then summed numerators over summed sends, never an average of rates.
// No spam rate and no status (Postmaster's rates are per domain, never aggregated). Returned only
// when more than one row is present (null otherwise).
export function totalsRow(rows, label = 'All brands') {
  const r = Array.isArray(rows) ? rows : [];
  if (r.length < 2) {
    return null;
  }
  const out = {
    brand: '',
    label,
    sent: null,
    spam_rate: null,
    spam_status: '',
    spam_flagged: false,
    status: '',
    thresholds: firstThresholds(r),
    totals: true,
  };
  [...SUBSCRIBER_FIELDS, ...ENGAGEMENT_FIELDS].forEach((k) => {
    out[k] = r.reduce((t, x) => t + num(x[k]), 0);
  });
  const sends = r.map((x) => x.sent).filter((v) => typeof v === 'number');
  out.sent = sends.length ? sends.reduce((t, v) => t + v, 0) : null;
  return out;
}

// D4: a rate is count / sent, or null when sent is 0 or absent.
export function rateOf(row, field) {
  const sent = row ? row.sent : null;
  if (!(typeof sent === 'number' && Number.isFinite(sent) && sent > 0)) {
    return null;
  }
  return num(row[field]) / sent;
}

function isEngagement(field) {
  return ENGAGEMENT_FIELDS.includes(field);
}

// D6: the DISPLAYED quantity of a cell -- an engagement column's rate in pct mode (null when there
// is no denominator), its count in num mode; every other column its own value (spam_rate and sent
// may be null).
export function displayValue(row, field, mode = DEFAULT_STATE.mode) {
  if (isEngagement(field)) {
    return mode === 'num' ? num(row[field]) : rateOf(row, field);
  }
  if (field === 'brand') {
    return row.label;
  }
  if (field === 'sent' || field === 'spam_rate') {
    return numOrNull(row[field]);
  }
  return num(row[field]);
}

const MISSING = '—';

// cellText is a cell's text: '—' for a missing value (no denominator, no spam rate, no sends), a
// rate as a percentage, a count through fmt (the Vue passes $utils.formatNumber).
export function cellText(row, field, mode, fmt = (n) => String(n)) {
  const v = displayValue(row, field, mode);
  if (field === 'brand') {
    return v;
  }
  if (v === null) {
    return MISSING;
  }
  if ((isEngagement(field) && mode !== 'num') || field === 'spam_rate') {
    return `${(v * 100).toFixed(RATE_DIGITS)}%`;
  }
  return fmt(v);
}

// GP-SPAM-VISIBILITY-SPEC D3: the GP Spam % cell's presentation, chosen from the STORED status
// (the fork holds no threshold). issues → the danger text color, warn → the dark warning text color
// (never yellow text); ok/unknown/absent → default. When the stored compliance verdict flags
// user-reported spam but the status is not warn/issues, the cell gets a dotted underline and the
// flagged tooltip -- while the color shows the problem, the color wins and the tooltip is the
// plain one. The totals row carries no spam signal. Returns { cls, style, tooltip } (tooltip is an
// i18n key); Dashboard.vue binds them and decides nothing.
export const SPAM_HELP = 'dashboard.matrix.spamHelp';
export const SPAM_FLAGGED_HELP = 'dashboard.matrix.spamFlaggedHelp';
const SPAM_STATUS_CLASS = Object.freeze({ issues: 'has-text-danger', warn: 'has-text-warning-dark' });

export function spamCell(row) {
  const status = row && !row.totals && typeof row.spam_status === 'string' ? row.spam_status : '';
  const cls = Object.prototype.hasOwnProperty.call(SPAM_STATUS_CLASS, status) ? SPAM_STATUS_CLASS[status] : '';
  const quietFlag = !!(row && !row.totals && row.spam_flagged === true) && cls === '';
  return {
    cls,
    style: quietFlag ? 'text-decoration: underline dotted;' : '',
    tooltip: quietFlag ? SPAM_FLAGGED_HELP : SPAM_HELP,
  };
}

// DASHBOARDS-POLISH-SPEC D1/I2: the Engagement rate coloring. The fork holds no threshold or floor
// VALUE -- every number compared against comes from the `thresholds` object the integrations
// BrandHealth Lambda stored on the brand's document (lib/brand-health.ts MATRIX_THRESHOLDS),
// projected verbatim by get-dashboard-brand-matrix; comparing the displayed rate against it is
// rendering. One red tier (the GP Spam % issues class); Blocks has no threshold; % mode only.
//   clicks       red when rate <  clicks.warnBelow,  only at sent >= clicks.minSent
//   hard_bounces red when rate >= hardBounces.warn,  only at sent >= hardBounces.minSent
//   complaints   red when rate >= complaints.warn,   only at sent >= complaints.minSent
//                                                     OR complaints >= complaints.minCount
// The rate compared is the DISPLAYED one (rounded to RATE_DIGITS of a percent, as cellText shows
// it), so a figure never reads on the other side of its header's target.
// The floors: for clicks and hard bounces they mirror the SES classify-or-stay-silent floors, so
// the matrix never flags what the Brands page reads ok/unknown. Complaints deliberately BREAK that
// property (integrations DASHBOARDS-POLISH2-SPEC D1, user decision 2026-10-25: the stored
// complaints.minCount is 1, so any complaint at a rate >= warn colors). The values -- and that
// decision -- live in lib/brand-health.ts; this helper only applies whatever floor is stored.
// (thresholdsOf / firstThresholds sit above shapeRow, which reads them.)
export const THRESHOLD_KEYS = Object.freeze({ clicks: 'clicks', hard_bounces: 'hardBounces', complaints: 'complaints' });
export const BREACH_CLASS = 'has-text-danger';

const isNum = (v) => typeof v === 'number' && Number.isFinite(v);

// breachesThreshold is the pure comparison: the stored thresholds object, an engagement field, the
// displayed rate (a fraction) and the row's counts ({ sent, complaints }). A missing object, an
// unknown field, a malformed entry, a non-number rate or an unmet floor is false.
export function breachesThreshold(thresholds, field, rate, counts = {}) {
  const key = Object.prototype.hasOwnProperty.call(THRESHOLD_KEYS, field) ? THRESHOLD_KEYS[field] : null;
  const t = key && thresholdsOf(thresholds) ? thresholds[key] : null;
  if (!isObj(t) || !isNum(rate) || !isNum(t.minSent)) {
    return false;
  }
  const sent = isNum(counts.sent) ? counts.sent : 0;
  let floorMet = sent >= t.minSent;
  if (field === 'complaints' && isNum(t.minCount) && isNum(counts.complaints) && counts.complaints >= t.minCount) {
    floorMet = true;
  }
  if (!floorMet) {
    return false;
  }
  if (field === 'clicks') {
    return isNum(t.warnBelow) && rate < t.warnBelow;
  }
  return isNum(t.warn) && rate >= t.warn;
}

// displayedRate is the rate as cellText shows it -- rounded to RATE_DIGITS decimals of a percent --
// as a fraction; null when there is no rate.
export function displayedRate(row, field) {
  const r = rateOf(row, field);
  if (r === null) {
    return null;
  }
  const scale = 10 ** RATE_DIGITS;
  return Math.round(Number((r * 100).toFixed(RATE_DIGITS)) * scale) / (100 * scale);
}

// engagementCell is an Engagement cell's presentation: { cls } -- BREACH_CLASS when the displayed
// rate crosses the row's stored threshold with its floor met; '' otherwise. The totals row reads
// the thresholds totalsRow copied from its first carrier. DASHBOARDS-POLISH3-SPEC D1 (replacing
// POLISH D1's "% mode only"): the breach is a ROW property decided from the rate in BOTH modes --
// the mode only picks which figure (rate or count) wears it, so `mode` no longer changes the result.
// eslint-disable-next-line no-unused-vars
export function engagementCell(row, field, mode = DEFAULT_STATE.mode) {
  if (!row) {
    return { cls: '' };
  }
  const hit = breachesThreshold(row.thresholds, field, displayedRate(row, field), { sent: row.sent, complaints: row.complaints });
  return { cls: hit ? BREACH_CLASS : '' };
}

// D6: the first-click direction of a column -- brand ascending, every other descending.
export function defaultOrder(field) {
  return field === 'brand' ? 'asc' : 'desc';
}

// D6: clickSort is the sort after a header click (the matrix table owns its own header since the
// 2026-10-24 layout pass -- no Buefy direction in play). A click on a NEW column takes that
// column's default direction; a click on the current column toggles it; an unsortable field
// leaves the sort unchanged.
export function clickSort(current, field) {
  const cur = current || DEFAULT_STATE.sort;
  if (!SORT_FIELDS.includes(field)) {
    return { ...cur };
  }
  if (cur.field !== field) {
    return { field, order: defaultOrder(field) };
  }
  return { field, order: cur.order === 'asc' ? 'desc' : 'asc' };
}

// D6/I6: sortRows sorts by the displayed quantity in the given mode. A missing value (null) ranks
// last in BOTH orders; ties fall back to the display name. The totals row is never passed in.
export function sortRows(rows, sort = DEFAULT_STATE.sort, mode = DEFAULT_STATE.mode, locale = 'en') {
  const field = SORT_FIELDS.includes(sort && sort.field) ? sort.field : DEFAULT_STATE.sort.field;
  const dir = sort && sort.order === 'asc' ? 1 : -1;
  const byName = (a, b) => a.label.localeCompare(b.label, locale, { sensitivity: 'base' }) || a.brand.localeCompare(b.brand);
  return [...(Array.isArray(rows) ? rows : [])].sort((a, b) => {
    if (field === 'brand') {
      return byName(a, b) * dir;
    }
    const va = displayValue(a, field, mode);
    const vb = displayValue(b, field, mode);
    if (va === null || vb === null) {
      if (va === vb) {
        return byName(a, b);
      }
      return va === null ? 1 : -1;
    }
    return ((va - vb) * dir) || byName(a, b);
  });
}

// D2: tableRows is the sorted rows with the totals row (when present) pinned last.
export function tableRows(rows, sort, mode, { locale = 'en', totalsLabel = 'All brands' } = {}) {
  const sorted = sortRows(rows, sort, mode, locale);
  const t = totalsRow(rows, totalsLabel);
  return t ? [...sorted, t] : sorted;
}

// D6/I7: the persisted state, validated -- anything unreadable is the default.
export function normalizeState(s) {
  const sort = s && s.sort;
  const okSort = sort && SORT_FIELDS.includes(sort.field) && (sort.order === 'asc' || sort.order === 'desc');
  return {
    sort: okSort ? { field: sort.field, order: sort.order } : { ...DEFAULT_STATE.sort },
    mode: s && MODES.includes(s.mode) ? s.mode : DEFAULT_STATE.mode,
  };
}

// D1: the Dashboard panes. Without brands:get only the Overview exists. The sticky pref is the
// string dashboard.pane; the legacy boolean dashboard.sesPane is read only while dashboard.pane is
// unset (true = ses). BRAND-USAGE-SPEC D2: `usage` (the Brand Usage pane) sits between matrix and ses.
export const PANES = Object.freeze(['overview', 'matrix', 'usage', 'ses']);

export function initialPane(canSes, storedPane, legacySesPane) {
  if (!canSes) {
    return 'overview';
  }
  if (PANES.includes(storedPane)) {
    return storedPane;
  }
  if (storedPane === undefined || storedPane === null) {
    return legacySesPane === true ? 'ses' : 'overview';
  }
  return 'overview';
}

// D6/I7: sessionStore is w.sessionStorage, or null where reading it throws (a sandboxed frame or
// blocked site data throw on the property access itself) or it is absent.
export function sessionStore(w) {
  try {
    return w && w.sessionStorage ? w.sessionStorage : null;
  } catch {
    return null;
  }
}

// D6/I7: loadState reads the session state from store (the Vue passes window.sessionStorage). A
// missing store, a throwing accessor or bad JSON is the default, never an error.
export function loadState(store) {
  try {
    const raw = store ? store.getItem(STORAGE_KEY) : null;
    return normalizeState(raw ? JSON.parse(raw) : null);
  } catch {
    return normalizeState(null);
  }
}

// D6/I7: saveState writes the session state; a failing store is ignored. Returns whether it wrote.
export function saveState(store, state) {
  try {
    if (!store) {
      return false;
    }
    store.setItem(STORAGE_KEY, JSON.stringify(normalizeState(state)));
    return true;
  } catch {
    return false;
  }
}
