// Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D8/D11). The pure half of the
// Tracked and Matched cells on Campaign Analytics, the Dashboard Overview and the Brand Matrix:
// the { orders, sales } texts (each surface lays them out) and the tooltip. Rendering only -- every rule (who is
// covered, what an order is worth) is the integrations CampaignConversions Lambda's; the server
// returns stored sums and the coverage verdict (null = not covered). No Vue, no DOM; tested by
// conversionCells.test.mjs (node --test).
//
//   uncovered (null) -> '—'          (a missing campaign is not zero)
//   covered, no rows -> 0 / $0        (a real zero)
//   no conversions doc at all -> '—' everywhere

export const MEASURES = Object.freeze(['tracked', 'matched']);

export const MISSING = '—';

const num = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);

// normalizeMeasure is { orders, sales } from a server measure in either key style (sales_usd from a
// camelCase:false read, salesUsd from the camelCased dashboard counts), or null when the server sent
// null/absent (not covered).
export function normalizeMeasure(v) {
  if (!v || typeof v !== 'object') {
    return null;
  }
  return { orders: num(v.orders), sales: num(v.sales_usd !== undefined ? v.sales_usd : v.salesUsd) };
}

// normalizeMeta is the conversions doc's projection { asOf, trackedFloor, matchedFloor, trackedStale }
// in either key style, or null when there is no doc.
export function normalizeMeta(m) {
  if (!m || typeof m !== 'object') {
    return null;
  }
  const pick = (snake, camel) => (m[snake] !== undefined ? m[snake] : m[camel]);
  const asOf = pick('as_of', 'asOf');
  if (asOf === null || asOf === undefined) {
    return null;
  }
  return {
    asOf: String(asOf),
    trackedFloor: String(pick('tracked_floor', 'trackedFloor') || ''),
    matchedFloor: String(pick('matched_floor', 'matchedFloor') || ''),
    trackedStale: pick('tracked_stale', 'trackedStale') === true,
  };
}

// formatUsd is a whole-dollar figure ("$1,235"); the CSV export keeps cents (conversionOrders.mjs).
export function formatUsd(n) {
  return `$${num(n).toLocaleString('en-US', { maximumFractionDigits: 0 })}`;
}

// conversionCell is a cell's { orders, sales } text. A null value (not covered, or no doc) is
// { orders: '—', sales: '' }; a covered value shows its counts, zero included.
export function conversionCell(value, fmt = (n) => num(n).toLocaleString('en-US')) {
  const v = normalizeMeasure(value);
  if (!v) {
    return { orders: MISSING, sales: '' };
  }
  return { orders: fmt(v.orders), sales: formatUsd(v.sales) };
}

// sumCovered is one measure summed over the campaigns that are covered for it, or null when none is
// (Campaign Analytics; with one campaign selected an uncovered measure is therefore '—').
export function sumCovered(campaigns, measure) {
  const rows = (Array.isArray(campaigns) ? campaigns : []).map((c) => normalizeMeasure(c && c[measure])).filter(Boolean);
  if (!rows.length) {
    return null;
  }
  return rows.reduce((t, r) => ({ orders: t.orders + r.orders, sales: t.sales + r.sales }), { orders: 0, sales: 0 });
}

// analyticsCells is the Campaign Analytics header cells from GET /api/campaigns/analytics/conversions
// ({ as_of, tracked_floor, matched_floor, tracked_stale, campaigns: [...] }): { meta, tracked, matched }.
export function analyticsCells(resp, fmt) {
  const meta = normalizeMeta(resp);
  const out = { meta };
  MEASURES.forEach((m) => {
    const sum = meta ? sumCovered(resp && resp.campaigns, m) : null;
    out[m] = conversionCell(sum ? { orders: sum.orders, sales_usd: sum.sales } : null, fmt);
  });
  return out;
}

// scopedCell is a Dashboard cell (Overview, Brand Matrix): the stored sum when a doc exists, '—'
// when none does. Those sums add up stored rows only (uncovered campaigns contribute nothing).
export function scopedCell(value, meta, fmt) {
  return conversionCell(normalizeMeta(meta) ? value : null, fmt);
}

// The tooltip's i18n keys: the measure's definition (with its floor), then "As of", then the
// stale note when Tracked is from an earlier snapshot.
export const TOOLTIP_KEYS = Object.freeze({
  tracked: 'analytics.conversions.trackedHelp',
  matched: 'analytics.conversions.matchedHelp',
  asOf: 'analytics.conversions.asOf',
  stale: 'analytics.conversions.stale',
  none: 'analytics.conversions.none',
});

// conversionTooltip is the tooltip text for a measure. t is the i18n function ($t). With no doc it
// is the definition plus the "not computed yet" line.
export function conversionTooltip(measure, meta, t) {
  const m = normalizeMeta(meta);
  const floor = m ? (measure === 'tracked' ? m.trackedFloor : m.matchedFloor).slice(0, 10) : '';
  const parts = [t(TOOLTIP_KEYS[measure], { floor })];
  if (!m) {
    parts.push(t(TOOLTIP_KEYS.none));
    return parts.join(' ');
  }
  parts.push(t(TOOLTIP_KEYS.asOf, { asOf: m.asOf }));
  if (measure === 'tracked' && m.trackedStale) {
    parts.push(t(TOOLTIP_KEYS.stale));
  }
  return parts.join(' ');
}
