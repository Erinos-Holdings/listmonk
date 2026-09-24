// Fork (brands UX, integrations BRANDS-UX-SPEC D3/D4). The ONE place the Brands table and the
// Dashboard's SES per-brand table get their sort order and their "hide unlaunched" rules, so the
// two views never re-state a rank. Pure: no Vue, no DOM, tested with Node's built-in runner
// (`npm run test:sort`). Display order only -- every status it ranks was computed by the
// integrations BrandHealth Lambda; nothing here decides health.
//
// Direction: "desc" is worst / largest first (the first click on any non-Brand column), "asc" the
// reverse. Every comparator takes (a, b, order) with the direction already applied; numbers and
// days put a missing value LAST in both directions. The tiebreak is always Brand A->Z.

// Chip statuses, worst first. Any other string ranks below ok (9).
export const STATUS_RANK = Object.freeze({
  issues: 0, warn: 1, unknown: 2, ok: 3,
});
const STATUS_OTHER = 9;

// CloudWatch alarm states, worst first, then "no alarm pair" (alarms present, both null) and
// "no SES data" (alarms null). An unrecognised state string ranks as INSUFFICIENT_DATA (the
// Lambda's own coercion in matchAlarms).
export const ALARM_RANK = Object.freeze({
  ALARM: 0, INSUFFICIENT_DATA: 1, OK: 2, NO_PAIR: 3, NO_SES: 4,
});

const has = (o, k) => Object.prototype.hasOwnProperty.call(o, k);
const dir = (order) => (order === 'desc' ? -1 : 1);

export function statusRank(s) {
  return typeof s === 'string' && has(STATUS_RANK, s) ? STATUS_RANK[s] : STATUS_OTHER;
}

export function alarmRank(alarms) {
  if (!alarms || typeof alarms !== 'object') {
    return ALARM_RANK.NO_SES;
  }
  const states = [alarms.complaints, alarms.bounces].filter((s) => s !== null && s !== undefined);
  if (!states.length) {
    return ALARM_RANK.NO_PAIR;
  }
  const ranks = states.map((s) => (typeof s === 'string' && ['ALARM', 'INSUFFICIENT_DATA', 'OK'].includes(s)
    ? ALARM_RANK[s] : ALARM_RANK.INSUFFICIENT_DATA));
  return Math.min(...ranks);
}

// Lower rank = worse = "larger": ascending is best first, descending worst first.
export function compareStatus(x, y, order = 'asc') {
  return (statusRank(y) - statusRank(x)) * dir(order);
}

export function compareAlarms(x, y, order = 'asc') {
  return (alarmRank(y) - alarmRank(x)) * dir(order);
}

const isNum = (v) => typeof v === 'number' && !Number.isNaN(v);

// Missing (null / non-number) is last in BOTH directions.
export function compareNumber(x, y, order = 'asc') {
  const mx = !isNum(x);
  const my = !isNum(y);
  if (mx || my) {
    return Number(mx) - Number(my);
  }
  return (x - y) * dir(order);
}

// The document's `day` (YYYY-MM-DD) compares as a string; a missing day is last in both directions.
export function compareDay(x, y, order = 'asc') {
  const mx = typeof x !== 'string' || !x;
  const my = typeof y !== 'string' || !y;
  if (mx || my) {
    return Number(mx) - Number(my);
  }
  if (x === y) {
    return 0;
  }
  return (x < y ? -1 : 1) * dir(order);
}

// Case-insensitive localeCompare on the DISPLAYED name.
export function compareName(nameOf) {
  return (a, b, order = 'asc') => {
    const x = String(nameOf(a) || '').toLowerCase();
    const y = String(nameOf(b) || '').toLowerCase();
    return x.localeCompare(y) * dir(order);
  };
}

// The secondary key is always Brand ascending, whatever the primary direction.
export function withBrandTiebreak(cmp, nameOf) {
  const byName = compareName(nameOf);
  return (a, b, order) => cmp(a, b, order) || byName(a, b, 'asc');
}

// A sorted copy. `field` 'name' (or one with no comparator) sorts by the displayed name;
// `comparators` maps every other sortable field to a row comparator (a, b, order).
export function sortRows(rows, field, order, nameOf, comparators = {}) {
  const primary = field !== 'name' && comparators && typeof comparators[field] === 'function'
    ? comparators[field] : compareName(nameOf);
  const o = order === 'desc' ? 'desc' : 'asc';
  const cmp = withBrandTiebreak(primary, nameOf);
  return [...(Array.isArray(rows) ? rows : [])].sort((a, b) => cmp(a, b, o));
}

// Brands page (D4): "unlaunched" is the document's own overall status, and nothing else. The
// Lambda's overallStatus returns unknown only when no input is warn/issues and no behavioural
// input has data, so a row showing a warn/issues chip can never hide.
export function isUnlaunchedBrandDoc(doc) {
  return !!doc && doc.status === 'unknown';
}

// SES pane (D4): an unknown row with no sends. A warn/issues row can never match.
export function isUnlaunchedSesRow(row) {
  return !!row && row.status === 'unknown' && !(row.sends > 0);
}
