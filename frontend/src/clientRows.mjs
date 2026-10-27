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
  // Fork (client vocabulary, integrations CLIENT-VOCABULARY-SPEC D2) -- the declared robots are
  // matched before the browser rows, the structural residue buckets after them.
  unfurler: 'Link preview (chat/social)',
  automation: 'Automated fetch (declared)',
  'apple-mail': 'Apple Mail (incl. privacy proxy)',
  'browser-ios': 'Browser (iOS)',
  'browser-android': 'Browser (Android)',
  'browser-windows': 'Browser (Windows)',
  'browser-mac': 'Browser (macOS)',
  'browser-linux': 'Browser (Linux)',
  'webview-ios': 'In-app browser (iOS)',
  'bare-mozilla': 'Unnamed client (bare Mozilla)',
  other: 'Other',
});

// CLIENT_TOKENS is the vocabulary, in the classifier's first-match order.
export const CLIENT_TOKENS = Object.freeze(Object.keys(CLIENT_LABELS));

export const OTHER = 'other';

// Fork (client vocabulary, integrations CLIENT-VOCABULARY-SPEC D8) -- the TRUST TIER. Grouping,
// not judgement: a property of the token in the same class as its display label, and this is its
// ONE home, so consumers read one definition instead of each re-deriving it.
//
//   client  -- the fetch declares software a person uses. A CEILING on human opens, not a count
//              of them: corporate scanners prefetch with spoofed mainstream-browser UAs, so some
//              of this is machine traffic that no User-Agent work can separate out.
//   proxy   -- a provider intermediary fetched for a real mailbox: a real inbox exists, the open
//              timing is unreliable. `apple-mail` is tiered here CONSERVATIVELY, not concluded:
//              Mail Privacy Protection and a real on-device open cannot be told apart by UA, and
//              `proxy` is true under either reading where `client` would assert a person opened
//              it. Re-tiering it is a post-release decision (spec H6).
//   machine -- declared non-interactive software. No person is implied by the fetch itself (an
//              `unfurler` hit does imply a person SHARED the link).
//
// `other`, `bare-mozilla` and Unknown (client '') claim NO tier -- see UNTIERED_TOKENS.
//
// This map is the tier's one home, but the i18n string `analytics.clientsTierHint` RESTATES
// tier membership in prose for the reader (it names Apple Mail's conservative tiering and what
// "No tier claimed" covers). Nothing ties them, so a re-tier here -- H6's `apple-mail` decision
// above all -- must edit that hint in the same change (review L2).
export const CLIENT_TIERS = Object.freeze({
  'outlook-windows': 'client',
  'outlook-mac': 'client',
  'outlook-mobile': 'client',
  thunderbird: 'client',
  'browser-ios': 'client',
  'browser-android': 'client',
  'browser-windows': 'client',
  'browser-mac': 'client',
  'browser-linux': 'client',
  'webview-ios': 'client',
  'gmail-proxy': 'proxy',
  yahoo: 'proxy',
  'apple-mail': 'proxy',
  automation: 'machine',
  unfurler: 'machine',
});

// TIERS is the render order of the three subtotals. They are shown SEPARATELY and are NEVER
// summed into one published figure: `client` over-counts (spoofed scanners), `proxy` is a mailbox
// but not an open, and the untiered remainder is unknown -- a sum would be exactly the "confident
// opens" number the data cannot support (D8).
export const TIERS = Object.freeze(['client', 'proxy', 'machine']);

// UNTIERED_TOKENS are the vocabulary tokens that deliberately claim no tier. Unknown (client '')
// is untiered too, as is any token this map has not heard of (vocabulary drift).
export const UNTIERED_TOKENS = Object.freeze(['other', 'bare-mozilla']);

export const UNTIERED = 'untiered';

// TIER_ROWS is the render order of the tier footer rows -- the three tiers, then the untiered
// remainder (`other`, the bare-Mozilla bucket and Unknown) -- with each row's i18n key. Part of
// the tier's single home (D8): both render sites read this instead of each spelling it out.
export const TIER_ROWS = Object.freeze([
  Object.freeze({ key: 'client', i18n: 'analytics.clientsTierClient' }),
  Object.freeze({ key: 'proxy', i18n: 'analytics.clientsTierProxy' }),
  Object.freeze({ key: 'machine', i18n: 'analytics.clientsTierMachine' }),
  Object.freeze({ key: UNTIERED, i18n: 'analytics.clientsTierUntiered' }),
]);

// clientTier is a token's trust tier, or 'untiered' when it claims none (D8).
export function clientTier(token) {
  return Object.prototype.hasOwnProperty.call(CLIENT_TIERS, token) ? CLIENT_TIERS[token] : UNTIERED;
}

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
      // Rendered by Campaign Analytics only: the Dashboard's clients panel no longer shows a
      // Combined column (integrations BRAND-USAGE-V2-SPEC D8). Kept -- Analytics renders it.
      combined: views + clicks,
      unknown,
      other: client === OTHER,
    };
  });
  return sortClientRows(out, DEFAULT_SORT.field, DEFAULT_SORT.order, locale);
}

// clientTierSubtotals groups a shaped table's rows by trust tier (D8), over Views and Clicks only
// -- NOT Combined: a tier-combined figure is close to the very number D8 refuses, and the
// Dashboard panel has no Combined column anyway (BRAND-USAGE-V2-SPEC D8).
//
// It returns exactly the four keys {client, proxy, machine, untiered} and deliberately NO total:
// the three tiers are rendered separately and never summed (J9).
export function clientTierSubtotals(rows) {
  const out = {
    client: { views: 0, clicks: 0 },
    proxy: { views: 0, clicks: 0 },
    machine: { views: 0, clicks: 0 },
    [UNTIERED]: { views: 0, clicks: 0 },
  };
  (Array.isArray(rows) ? rows : []).forEach((r) => {
    const client = typeof r.client === 'string' ? r.client.trim() : '';
    // Unknown (no UA, and every pre-vocabulary row) claims no tier.
    const tier = client === '' ? UNTIERED : clientTier(client);
    const bucket = out[tier] || out[UNTIERED];
    bucket.views += num(r.views);
    bucket.clicks += num(r.clicks);
  });
  return out;
}

// clientTotals sums a shaped table's columns (the denominators of each row's share).
export function clientTotals(rows) {
  return (Array.isArray(rows) ? rows : []).reduce((t, r) => ({
    views: t.views + num(r.views),
    clicks: t.clicks + num(r.clicks),
    combined: t.combined + num(r.combined),
  }), { views: 0, clicks: 0, combined: 0 });
}
