// Fork (shop-link attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC D5; shop-link rate,
// SHOP-LINK-RATE-SPEC D6). Bar shaping for the "Shop Link Attribution" chart on Campaign Analytics:
// the bars, their order, labels and end-of-bar texts, in two views. Pure: no Vue, no DOM, tested
// with Node's built-in runner (`yarn test`). Display only -- the numbers come from
// GET /api/campaigns/analytics/shop_links ([{kind, count, audience, snapshots}]) as they are.
//
// counts (default): the four stored kinds ALWAYS (zero-filled), No link (none) and Unknown
// (unknown) and any other kind only when non-zero; sorted by count desc, ties in rank order
// (self, creator, founder, corporate, none, unknown, then other kinds A->Z). Text = the count.
// (Replaces the parent spec's rank-order sort.)
//
// rate: one bar per kind with a non-null audience and (count > 0 or audience > 0); value =
// count / audience as a percentage, text "4.9% (7 of 142)". unknown is never shown (no
// denominator). Drift is shown, not hidden: the numerator is the kind held today, the denominator
// the kind held at send, so a kind can carry clicks against audience 0 (value 0, text "n of 0") or
// read over 100% (shown as computed). Sorted by rate desc, ties by count desc, then rank order.
// When no row has an audience the rate view has no bars; whether to show the "no snapshot" note
// is the view's one definition (CampaignAnalytics.vue shopLinksNoSnapshot).

export const RANK_KINDS = Object.freeze(['self', 'creator', 'founder', 'corporate']);
export const OPTIONAL_KINDS = Object.freeze(['none', 'unknown']);
export const VIEWS = Object.freeze(['counts', 'rate']);

const TIE_ORDER = [...RANK_KINDS, ...OPTIONAL_KINDS];

const num = (v) => {
  const n = typeof v === 'number' ? v : Number(v);
  return Number.isFinite(n) ? n : 0;
};

// Rank tiebreak: the known kinds in TIE_ORDER, other kinds after them A->Z.
const byRank = (a, b) => {
  const ia = TIE_ORDER.indexOf(a);
  const ib = TIE_ORDER.indexOf(b);
  if (ia >= 0 || ib >= 0) {
    return (ia < 0 ? TIE_ORDER.length : ia) - (ib < 0 ? TIE_ORDER.length : ib);
  }
  return a.localeCompare(b, 'en');
};

// shapeShopLinkBars returns { labels, data, texts } for a bar chart. labels maps each of
// RANK_KINDS and OPTIONAL_KINDS to its display string (the kind itself when a label is missing);
// view is 'counts' (default) or 'rate'; fmt formats a count (the locale formatter in the view;
// String by default).
// Fork (shop-link header total, 2026-10-19) -- the Rate view's header: the aggregate rate, every
// kind's clicks over every kind's snapshot audience (kinds with a null audience -- unknown, or a
// selection with no snapshot -- contribute neither). Returns { pct, count, audience }; pct is null
// when there is no audience (the view then falls back to the counts header).
export function aggregateShopLinkRate(rows, fmt = String) {
  let count = 0;
  let audience = 0;
  (Array.isArray(rows) ? rows : []).forEach((r) => {
    if (!r || typeof r.kind !== 'string' || r.audience === null || r.audience === undefined) {
      return;
    }
    count += num(r.count);
    audience += num(r.audience);
  });
  if (audience <= 0) {
    return { pct: null, count: fmt(count), audience: fmt(audience) };
  }
  return { pct: `${((100 * count) / audience).toFixed(1)}%`, count: fmt(count), audience: fmt(audience) };
}

// Fork (hide No link, 2026-10-19) -- opts.hideNone drops the `none` bar from either view (the
// user's toggle beside Counts | Rate); the row is still in the input, only the bar is hidden.
export function shapeShopLinkBars(rows, labels = {}, view = 'counts', fmt = String, opts = {}) {
  const counts = new Map();
  const audiences = new Map();
  (Array.isArray(rows) ? rows : []).forEach((r) => {
    if (!r || typeof r.kind !== 'string') {
      return;
    }
    if (opts.hideNone && r.kind === 'none') {
      return;
    }
    counts.set(r.kind, (counts.get(r.kind) || 0) + num(r.count));
    if (r.audience !== null && r.audience !== undefined) {
      audiences.set(r.kind, (audiences.get(r.kind) || 0) + num(r.audience));
    }
  });

  const label = (k) => (typeof labels[k] === 'string' && labels[k] !== '' ? labels[k] : k);
  const out = { labels: [], data: [], texts: [] };
  const push = (k, value, text) => {
    out.labels.push(label(k));
    out.data.push(value);
    out.texts.push(text);
  };

  if (view === 'rate') {
    const bars = [];
    audiences.forEach((a, k) => {
      const c = counts.get(k) || 0;
      if (k === 'unknown' || (c <= 0 && a <= 0)) {
        return;
      }
      if (a > 0) {
        const pct = (100 * c) / a;
        bars.push({
          k, c, rate: pct, text: `${pct.toFixed(1)}% (${fmt(c)} of ${fmt(a)})`,
        });
      } else {
        bars.push({
          k, c, rate: 0, text: `${fmt(c)} of 0`,
        });
      }
    });
    bars
      .sort((x, y) => (y.rate - x.rate) || (y.c - x.c) || byRank(x.k, y.k))
      .forEach((b) => push(b.k, b.rate, b.text));
    return out;
  }

  const bars = [];
  RANK_KINDS.forEach((k) => bars.push({ k, c: counts.get(k) || 0 }));
  counts.forEach((c, k) => {
    if (!RANK_KINDS.includes(k) && c !== 0) {
      bars.push({ k, c });
    }
  });
  bars
    .sort((x, y) => (y.c - x.c) || byRank(x.k, y.k))
    .forEach((b) => push(b.k, b.c, fmt(b.c)));
  return out;
}
