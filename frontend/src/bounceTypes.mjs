// Fork (bounce types, integrations BOUNCE-TYPES-SPEC D3/I3). The Bounces header's split on
// Campaign Analytics: "3 hard, 1 soft" after the grey count. Pure: no Vue, no DOM, tested with
// Node's built-in runner (bounceTypes.test.mjs). Display only -- the counts come from
// GET /api/campaigns/analytics/bounce_types as they are.

// The display order (D1): hard, soft, complaint. A type outside it is ignored.
export const BOUNCE_TYPE_ORDER = Object.freeze(['hard', 'soft', 'complaint']);

const defaultFormat = new Intl.NumberFormat('en-US').format;

// bounceSplitText returns "<count> <label>, ..." for every type with a count above zero, in
// BOUNCE_TYPE_ORDER, counts formatted by fmt (the Vue passes $utils.formatNumber) and labels from
// labels[type]. Empty string when rows are absent or malformed, or every type is zero.
export function bounceSplitText(rows, labels, fmt = defaultFormat) {
  if (!Array.isArray(rows)) {
    return '';
  }
  const counts = {};
  rows.forEach((r) => {
    if (!r || typeof r !== 'object' || !BOUNCE_TYPE_ORDER.includes(r.type)) {
      return;
    }
    if (typeof r.count === 'number' && Number.isFinite(r.count) && r.count > 0) {
      counts[r.type] = (counts[r.type] || 0) + r.count;
    }
  });
  const l = labels && typeof labels === 'object' ? labels : {};
  return BOUNCE_TYPE_ORDER
    .filter((t) => counts[t] > 0)
    .map((t) => `${fmt(counts[t])} ${l[t] || t}`)
    .join(', ');
}
