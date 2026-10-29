// Fork (campaign matrix, integrations listmonk/CAMPAIGN-MATRIX-SPEC D4-D9). The pure half of the
// Campaign Matrix pane of Campaigns > Analytics: row shaping (the trust-tier fold, the launch
// filter), the %/$ | # cells, the totals row, the sort (Date default, missing last) and the
// per-session state. No Vue, no DOM; tested by campaignMatrix.test.mjs (yarn test).
//
// The rows come from GET /api/analytics/campaigns/matrix verbatim (camelCase: false):
// { id, name, status, evergreen, started_at, send_day, last_sent, brand, sent, views, view_clients:
//   { <client token>: n }, clicks, hard_bounces, soft_bounces, complaints, tracked, matched,
//   gp_rate, gp_thresholds, client_floor }. Every rate is computed here, over the row's own Sent;
// the server returns counts, the stored GP point and its stored thresholds -- never a rate or a tier.
import { clientTier } from './clientRows.mjs'; // eslint-disable-line import/extensions
import { matchesLaunch, normalizeLaunch } from './brandScope.mjs'; // eslint-disable-line import/extensions
import { normalizeMeasure } from './conversionCells.mjs'; // eslint-disable-line import/extensions

// D8: the three trust tiers, each its own column. CLIENT_TIERS (clientRows.mjs) is the tier map's
// one home -- this module folds tokens through clientTier and never names a token itself. The
// tiers are NEVER added together (no "human" figure): Total is a separate server count.
export const TIER_FIELDS = Object.freeze(['client', 'proxy', 'machine']);

// D5: the count columns that read as a rate over Sent in %/$ mode, left to right.
export const COUNT_FIELDS = Object.freeze(['views', ...TIER_FIELDS, 'clicks', 'hard_bounces', 'soft_bounces', 'complaints']);

// D9: Tracked / Matched -- sales in %/$ mode, orders in # mode; null = not covered.
export const CONVERSION_FIELDS = Object.freeze(['tracked', 'matched']);

export const SORT_FIELDS = Object.freeze(['name', 'brand', 'date', 'sent', ...COUNT_FIELDS, ...CONVERSION_FIELDS, 'gp_rate']);

export const MODES = Object.freeze(['pct', 'num']);

// D4: the kind pill -- the Campaigns page's kinds (campaignsRouteQuery.mjs SCOPE_KINDS), default All.
export const KINDS = Object.freeze(['all', 'broadcasts', 'automations']);

// D5: default sort Date descending; mode %/$; kind All.
export const DEFAULT_STATE = Object.freeze({
  sort: Object.freeze({ field: 'date', order: 'desc' }), mode: 'pct', kind: 'all',
});

// The sessionStorage key of { sort, mode, kind }.
export const STORAGE_KEY = 'analytics.matrix';

export const RATE_DIGITS = 2;

export const MISSING = '—';

const num = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const numOrNull = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : null);
const isObj = (v) => !!v && typeof v === 'object' && !Array.isArray(v);
const str = (v) => (typeof v === 'string' ? v : '');

// D8: foldTiers is the per-token counts summed per tier ({ client, proxy, machine }); Unknown ('')
// and untiered tokens fall in no tier. Each tier's figure is a sum of per-token distinct counts --
// a ceiling, never a count of people (one person on two clients of a tier counts twice).
export function foldTiers(viewClients) {
  const out = { client: 0, proxy: 0, machine: 0 };
  if (!isObj(viewClients)) {
    return out;
  }
  Object.keys(viewClients).forEach((token) => {
    const tier = token === '' ? null : clientTier(token);
    if (TIER_FIELDS.includes(tier)) {
      out[tier] += num(viewClients[token]);
    }
  });
  return out;
}

// shapeRow flattens one API row. A client_floor row's tier columns are null (its views predate the
// client tokens); a missing Sent is null; an uncovered conversion measure is null.
export function shapeRow(r, brandLabel = (s) => s) {
  const brand = str(r && r.brand);
  const floor = !!(r && r.client_floor === true);
  const tiers = foldTiers(r && r.view_clients);
  const out = {
    id: r.id,
    name: str(r.name),
    evergreen: r.evergreen === true,
    brand,
    label: brand ? brandLabel(brand) : '',
    // D4: a broadcast's Mountain send date; a journey's last sent day in the window (its Date cell
    // is '—' with a hover).
    date: r.evergreen === true ? null : (str(r.send_day) || null),
    lastSent: str(r.last_sent) || null,
    sent: numOrNull(r.sent),
    views: num(r.views),
    clicks: num(r.clicks),
    hard_bounces: num(r.hard_bounces),
    soft_bounces: num(r.soft_bounces),
    complaints: num(r.complaints),
    clientFloor: floor,
    gp_rate: numOrNull(r.gp_rate),
    gpThresholds: isObj(r.gp_thresholds) ? r.gp_thresholds : null,
    totals: false,
  };
  TIER_FIELDS.forEach((k) => { out[k] = floor ? null : tiers[k]; });
  CONVERSION_FIELDS.forEach((k) => { out[k] = normalizeMeasure(r[k]); });
  return out;
}

// D6: the navbar launch filter narrows rows by the campaign's brand exactly as it narrows Brand
// Matrix rows; a row with no single brand survives only All.
export function filterByLaunch(rows, launch, docs) {
  const l = normalizeLaunch(launch);
  if (l === 'all') {
    return rows;
  }
  return rows.filter((r) => r.brand !== '' && matchesLaunch(r.brand, l, docs));
}

// shapeRows is the API rows flattened, then narrowed by the launch filter. The server has applied
// the permission scope, the brand selector (list_id) and the kind.
export function shapeRows(apiRows, { launch = 'all', docs = [], brandLabel } = {}) {
  const rows = (Array.isArray(apiRows) ? apiRows : []).filter((r) => r && typeof r.id === 'number');
  return filterByLaunch(rows.map((r) => shapeRow(r, brandLabel)), launch, docs);
}

// D5: the totals row (two or more rows, as the Brand Matrix): per count column the sum over the rows
// carrying a value, with that column's own Sent -- the sum of those same rows' Sent (the
// conversionCells.mjs sumCovered precedent), so a rate is never diluted by rows without the value;
// a column with no valued row is null. Conversions sum the covered rows (null when none is). No
// date, brand or GP.
export function totalsRow(rows, label = 'Total') {
  const r = Array.isArray(rows) ? rows : [];
  if (r.length < 2) {
    return null;
  }
  const out = {
    id: null,
    name: label,
    evergreen: false,
    brand: '',
    label: '',
    date: null,
    lastSent: null,
    clientFloor: false,
    gp_rate: null,
    gpThresholds: null,
    totals: true,
    colSent: {},
  };
  const sends = r.map((x) => x.sent).filter((v) => typeof v === 'number');
  out.sent = sends.length ? sends.reduce((t, v) => t + v, 0) : null;
  COUNT_FIELDS.forEach((k) => {
    const valued = r.filter((x) => typeof x[k] === 'number');
    out[k] = valued.length ? valued.reduce((t, x) => t + x[k], 0) : null;
    const s = valued.map((x) => x.sent).filter((v) => typeof v === 'number');
    out.colSent[k] = s.length ? s.reduce((t, v) => t + v, 0) : null;
  });
  CONVERSION_FIELDS.forEach((k) => {
    const covered = r.map((x) => x[k]).filter(Boolean);
    out[k] = covered.length
      ? covered.reduce((t, m) => ({ orders: t.orders + m.orders, sales: t.sales + m.sales }), { orders: 0, sales: 0 })
      : null;
  });
  return out;
}

// sentFor is a column's denominator: the totals row's per-column Sent, else the row's Sent.
function sentFor(row, field) {
  if (row.totals && row.colSent && Object.prototype.hasOwnProperty.call(row.colSent, field)) {
    return row.colSent[field];
  }
  return row.sent;
}

// rateOf is count / Sent, or null when the count is missing or Sent is 0 or absent.
export function rateOf(row, field) {
  const sent = sentFor(row, field);
  const v = row[field];
  if (typeof v !== 'number' || !(typeof sent === 'number' && sent > 0)) {
    return null;
  }
  return v / sent;
}

// displayValue is the DISPLAYED quantity of a cell (what it shows and sorts by): a count column's
// rate (%/$) or count (#); a conversion's sales (%/$) or orders (#); Sent and GP as they are; null
// when missing.
export function displayValue(row, field, mode = DEFAULT_STATE.mode) {
  if (COUNT_FIELDS.includes(field)) {
    return mode === 'num' ? numOrNull(row[field]) : rateOf(row, field);
  }
  if (CONVERSION_FIELDS.includes(field)) {
    const m = row[field];
    if (!m) {
      return null;
    }
    return mode === 'num' ? m.orders : m.sales;
  }
  if (field === 'sent' || field === 'gp_rate') {
    return numOrNull(row[field]);
  }
  return null;
}

// conversionValue is a row's measure back in the server's shape ({ orders, sales_usd }), or null when
// not covered -- what conversionCells.mjs::scopedCell takes (D9: the cell reuses it).
export function conversionValue(row, field) {
  const m = row && row[field];
  return m ? { orders: m.orders, sales_usd: m.sales } : null;
}

const pct = (v) => `${(v * 100).toFixed(RATE_DIGITS)}%`;

// cellText is a numeric cell's text: '—' for a missing value, a rate as a percentage, a count
// through fmt, a sales figure through usd. The conversion cells also have conversionCells.mjs's
// scopedCell for the '—'-with-no-doc rule; this is the same rule over the shaped (normalized) value.
export function cellText(row, field, mode, fmt = (n) => String(n), usd = (n) => `$${n}`) {
  const v = displayValue(row, field, mode);
  if (v === null) {
    return MISSING;
  }
  if (field === 'gp_rate' || (COUNT_FIELDS.includes(field) && mode !== 'num')) {
    return pct(v);
  }
  if (CONVERSION_FIELDS.includes(field) && mode !== 'num') {
    return usd(v);
  }
  return fmt(v);
}

// D7: the GP S% cell's class -- a mechanical comparison of the send day's rate against the
// thresholds STORED on the document that supplied it (Brands.vue's rateLevel rule; the fork holds no
// threshold value): issues when rate >= issues, warn when >= warn, else none. The Brand Matrix's
// status classes (never brandMatrix.mjs::spamCell, which colours by today's status).
export const GP_CLASS = Object.freeze({ issues: 'has-text-danger', warn: 'has-text-warning-dark' });

export function gpLevel(rate, t) {
  if (typeof rate !== 'number' || !isObj(t) || typeof t.warn !== 'number' || typeof t.issues !== 'number') {
    return '';
  }
  if (rate >= t.issues) {
    return 'issues';
  }
  return rate >= t.warn ? 'warn' : '';
}

export function gpClass(row) {
  if (!row || row.totals) {
    return '';
  }
  const level = gpLevel(row.gp_rate, row.gpThresholds);
  return level ? GP_CLASS[level] : '';
}

// gpTip is the GP cell's hover as { key, params } (i18n), or null: a journey has no single send day;
// a broadcast without a brand has no Postmaster domain; a broadcast with no point names the brand
// and day.
export function gpTip(row) {
  if (!row || row.totals) {
    return null;
  }
  if (row.evergreen) {
    return { key: 'analytics.matrix.gpJourney', params: {} };
  }
  if (!row.brand) {
    return { key: 'analytics.matrix.gpNoBrand', params: {} };
  }
  if (row.gp_rate === null) {
    return { key: 'analytics.matrix.gpNone', params: { brand: row.label || row.brand, date: row.date || '' } };
  }
  return { key: 'analytics.matrix.gpHelp', params: {} };
}

// The Date cell: a broadcast's send date; a journey '—' with "Journey — last sent <date>".
export function dateCell(row) {
  if (!row || row.totals) {
    return { text: '', tip: null };
  }
  if (row.evergreen) {
    return { text: MISSING, tip: { key: 'analytics.matrix.journeyLastSent', params: { date: row.lastSent || MISSING } } };
  }
  return { text: row.date || MISSING, tip: null };
}

// The first-click direction: text columns ascending, every other descending.
export function defaultOrder(field) {
  return field === 'name' || field === 'brand' ? 'asc' : 'desc';
}

// clickSort: a NEW column takes its default direction, the current column toggles, an unknown field
// leaves the sort unchanged (brandMatrix.mjs::clickSort's rule).
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

const cmpText = (a, b, locale) => a.localeCompare(b, locale, { sensitivity: 'base' });

// sortRows: by the displayed quantity in the mode. A missing value ranks last in BOTH orders; Date
// puts journeys after broadcasts in both orders, broadcasts by send date and journeys by last sent;
// an unbranded row's Brand is missing. Ties fall back to the name, then the id. The totals row is
// never passed in.
export function sortRows(rows, sort = DEFAULT_STATE.sort, mode = DEFAULT_STATE.mode, locale = 'en') {
  const field = SORT_FIELDS.includes(sort && sort.field) ? sort.field : DEFAULT_STATE.sort.field;
  const dir = sort && sort.order === 'asc' ? 1 : -1;
  const tie = (a, b) => cmpText(a.name, b.name, locale) || (a.id - b.id);
  const missingLast = (va, vb) => {
    if (va === null && vb === null) {
      return 0;
    }
    return va === null ? 1 : -1;
  };
  return [...(Array.isArray(rows) ? rows : [])].sort((a, b) => {
    if (field === 'name') {
      return (cmpText(a.name, b.name, locale) * dir) || (a.id - b.id);
    }
    if (field === 'brand') {
      const va = a.label || null;
      const vb = b.label || null;
      if (va === null || vb === null) {
        return missingLast(va, vb) || tie(a, b);
      }
      return (cmpText(va, vb, locale) * dir) || tie(a, b);
    }
    if (field === 'date') {
      if (a.evergreen !== b.evergreen) {
        return a.evergreen ? 1 : -1;
      }
      const va = a.evergreen ? a.lastSent : a.date;
      const vb = b.evergreen ? b.lastSent : b.date;
      if (!va || !vb) {
        return missingLast(va || null, vb || null) || tie(a, b);
      }
      return ((va < vb ? -1 : va > vb ? 1 : 0) * dir) || tie(a, b); // eslint-disable-line no-nested-ternary
    }
    const va = displayValue(a, field, mode);
    const vb = displayValue(b, field, mode);
    if (va === null || vb === null) {
      return missingLast(va, vb) || tie(a, b);
    }
    return ((va - vb) * dir) || tie(a, b);
  });
}

// tableRows is the sorted rows with the totals row (when present) pinned last.
export function tableRows(rows, sort, mode, { locale = 'en', totalsLabel = 'Total' } = {}) {
  const sorted = sortRows(rows, sort, mode, locale);
  const t = totalsRow(rows, totalsLabel);
  return t ? [...sorted, t] : sorted;
}

// The persisted state, validated -- anything unreadable is the default.
export function normalizeState(s) {
  const sort = s && s.sort;
  const okSort = sort && SORT_FIELDS.includes(sort.field) && (sort.order === 'asc' || sort.order === 'desc');
  return {
    sort: okSort ? { field: sort.field, order: sort.order } : { ...DEFAULT_STATE.sort },
    mode: s && MODES.includes(s.mode) ? s.mode : DEFAULT_STATE.mode,
    kind: s && KINDS.includes(s.kind) ? s.kind : DEFAULT_STATE.kind,
  };
}

// loadState / saveState: the session state (the Vue passes window.sessionStorage through
// brandMatrix.mjs::sessionStore). A missing store, a throwing accessor or bad JSON is the default.
export function loadState(store) {
  try {
    const raw = store ? store.getItem(STORAGE_KEY) : null;
    return normalizeState(raw ? JSON.parse(raw) : null);
  } catch {
    return normalizeState(null);
  }
}

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
