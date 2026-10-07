// Fork (client stats, integrations CLIENT-STATS-SPEC D9). Row shaping for the "Email clients"
// tables on Campaign Analytics and the Dashboard: the token's label (the token itself as the
// fallback), the combined column (views + clicks -- presentation, not a stored fact), the Unknown
// row and the sort order. Pure: no Vue, no DOM, tested with Node's built-in runner
// (`npm run test:clients`). The counts come from GET /api/campaigns/analytics/clients and
// GET /api/dashboard/clients as they are.
//
// CLIENT_LABELS is the label map of the CLOSED vocabulary the fork's classifier emits
// (cmd/public.go classifyClient). Extending the vocabulary means touching the classifier, its test
// table and this map in the same change (D2: deliberately not pinned by a cross-language test --
// a drifted token renders as the bare token, I6's fallback).
//
// Default order: Views desc, then Clicks desc, then label A->Z, with "other" second-to-last and
// Unknown (client "") ALWAYS last. A column sort keeps Unknown last whatever the column or
// direction and falls back to the default order on ties.

export const CLIENT_LABELS = Object.freeze({
  'gmail-proxy': 'Gmail (image proxy)',
  yahoo: 'Yahoo Mail',
  'outlook-windows': 'Outlook desktop (Windows)',
  'outlook-mac': 'Outlook (Mac)',
  thunderbird: 'Thunderbird',
  'outlook-mobile': 'Outlook app (iOS/Android)',
  'apple-mail': 'Apple Mail (incl. privacy proxy)',
  'browser-ios': 'Browser (iOS)',
  'browser-android': 'Browser (Android)',
  'browser-windows': 'Browser (Windows)',
  'browser-mac': 'Browser (macOS)',
  'browser-linux': 'Browser (Linux)',
  other: 'Other',
});

// CLIENT_TOKENS is the vocabulary, in the classifier's first-match order.
export const CLIENT_TOKENS = Object.freeze(Object.keys(CLIENT_LABELS));

export const OTHER = 'other';

export const DEFAULT_SORT = Object.freeze({ field: 'views', order: 'desc' });

export const SORT_FIELDS = Object.freeze(['name', 'views', 'clicks', 'combined']);

// clientLabel is the display label of a token, or the token itself when the map has none.
export function clientLabel(token) {
  return Object.prototype.hasOwnProperty.call(CLIENT_LABELS, token) ? CLIENT_LABELS[token] : token;
}

const num = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);

function compareName(a, b, locale) {
  return a.name.localeCompare(b.name, locale) || a.client.localeCompare(b.client);
}

function compareDefault(a, b, locale) {
  return (b.views - a.views) || (b.clicks - a.clicks) || compareName(a, b, locale);
}

// sortClientRows returns a new array sorted by field ('name', 'views', 'clicks' or 'combined') in
// order ('asc' or 'desc'): Unknown always last; on the default sort "other" next-to-last; ties
// broken by the default order.
export function sortClientRows(rows, field = DEFAULT_SORT.field, order = DEFAULT_SORT.order, locale = 'en') {
  const dir = order === 'asc' ? 1 : -1;
  const isDefault = field === DEFAULT_SORT.field && order === DEFAULT_SORT.order;
  return [...rows].sort((a, b) => {
    if (a.unknown !== b.unknown) {
      return a.unknown ? 1 : -1;
    }
    if (isDefault && a.other !== b.other) {
      return a.other ? 1 : -1;
    }
    let c = 0;
    if (field === 'name') {
      c = compareName(a, b, locale) * dir;
    } else if (field === 'views' || field === 'clicks' || field === 'combined') {
      c = (a[field] - b[field]) * dir;
    }
    return c || compareDefault(a, b, locale);
  });
}

// shapeClientRows maps API rows {client, views, clicks} to table rows
// {client, name, views, clicks, combined, unknown, other}, sorted in the default order.
export function shapeClientRows(rows, { locale = 'en', unknownLabel = 'Unknown' } = {}) {
  const out = (Array.isArray(rows) ? rows : []).map((r) => {
    const client = typeof r.client === 'string' ? r.client.trim() : '';
    const unknown = client === '';
    const views = num(r.views);
    const clicks = num(r.clicks);
    return {
      client,
      name: unknown ? unknownLabel : clientLabel(client),
      views,
      clicks,
      combined: views + clicks,
      unknown,
      other: client === OTHER,
    };
  });
  return sortClientRows(out, DEFAULT_SORT.field, DEFAULT_SORT.order, locale);
}

// clientTotals sums a shaped table's columns (the denominators of each row's share).
export function clientTotals(rows) {
  return (Array.isArray(rows) ? rows : []).reduce((t, r) => ({
    views: t.views + num(r.views),
    clicks: t.clicks + num(r.clicks),
    combined: t.combined + num(r.combined),
  }), { views: 0, clicks: 0, combined: 0 });
}
