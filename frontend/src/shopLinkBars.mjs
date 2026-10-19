// Fork (shop-link attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC D5). Bar shaping for the
// "Shop Link Attribution" chart on Campaign Analytics: the order and labels of the bars. Pure: no
// Vue, no DOM, tested with Node's built-in runner (`yarn test`). Display only -- the counts come
// from GET /api/campaigns/analytics/shop_links ([{kind, count}]) as they are.
//
// Order: the four stored kinds ALWAYS, in rank order (Self, Creator, Founder, Corporate), each zero
// when absent; then No link (none) and Unknown (unknown) only when non-zero; then any other kind the
// backend returned, verbatim, by count desc (ties by kind A->Z).

export const RANK_KINDS = Object.freeze(['self', 'creator', 'founder', 'corporate']);
export const OPTIONAL_KINDS = Object.freeze(['none', 'unknown']);

const num = (v) => {
  const n = typeof v === 'number' ? v : Number(v);
  return Number.isFinite(n) ? n : 0;
};

// shapeShopLinkBars returns { labels, data } for a bar chart. labels maps each of RANK_KINDS and
// OPTIONAL_KINDS to its display string (the kind itself when a label is missing).
export function shapeShopLinkBars(rows, labels = {}) {
  const counts = new Map();
  (Array.isArray(rows) ? rows : []).forEach((r) => {
    if (!r || typeof r.kind !== 'string') {
      return;
    }
    counts.set(r.kind, (counts.get(r.kind) || 0) + num(r.count));
  });

  const label = (k) => (typeof labels[k] === 'string' && labels[k] !== '' ? labels[k] : k);
  const out = { labels: [], data: [] };
  const push = (lbl, n) => {
    out.labels.push(lbl);
    out.data.push(n);
  };

  RANK_KINDS.forEach((k) => push(label(k), counts.get(k) || 0));
  OPTIONAL_KINDS.forEach((k) => {
    const n = counts.get(k) || 0;
    if (n !== 0) {
      push(label(k), n);
    }
  });

  const known = new Set([...RANK_KINDS, ...OPTIONAL_KINDS]);
  [...counts.entries()]
    .filter(([k]) => !known.has(k))
    .sort((a, b) => (b[1] - a[1]) || a[0].localeCompare(b[0], 'en'))
    .forEach(([k, n]) => push(k, n));

  return out;
}
