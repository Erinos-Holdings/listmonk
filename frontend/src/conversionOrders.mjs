// Fork (campaign conversions, 2026-10-29) -- the Conversions card's drill-down: the stored orders
// behind the Tracked and Matched totals (GET /api/campaigns/analytics/conversion_orders), how they
// arrived over the days after the send, sorting, and the CSV download. Rendering only -- the rows
// are stored by the integrations CampaignConversions Lambda; "day after send" is a display
// difference (purchase time minus the campaign's start), blank for an evergreen, whose start is a
// watermark and not a send. No Vue, no DOM; tested by conversionOrders.test.mjs (node --test).

const DAY_MS = 24 * 3600 * 1000;

// The arrival buckets: D0 (the send's own first 24 h) through D5, then everything later.
export const DAY_BUCKETS = Object.freeze(['D0', 'D1', 'D2', 'D3', 'D4', 'D5', 'D6+']);

const num = (v) => (Number.isFinite(Number(v)) ? Number(v) : 0);

// dayAfterSend is whole days from the campaign's start to the purchase (0 = within 24 h), or null
// for an evergreen, a missing start, or a purchase before the start.
export function dayAfterSend(row) {
  if (!row || row.evergreen || !row.started_at) {
    return null;
  }
  const d = Date.parse(row.purchased_at) - Date.parse(row.started_at);
  return Number.isFinite(d) && d >= 0 ? Math.floor(d / DAY_MS) : null;
}

// shapeOrderRows normalizes the API rows and adds `day` (dayAfterSend).
export function shapeOrderRows(rows) {
  return (Array.isArray(rows) ? rows : []).map((r) => ({
    order_id: String(r.order_id || ''),
    campaign_id: num(r.campaign_id),
    campaign_name: String(r.campaign_name || ''),
    purchased_at: String(r.purchased_at || ''),
    started_at: r.started_at || null,
    evergreen: !!r.evergreen,
    tracked: !!r.tracked,
    matched: !!r.matched,
    sales_usd: num(r.sales_usd),
    day: dayAfterSend(r),
  }));
}

// arrivals is the orders per DAY_BUCKETS bucket (orders with no day -- evergreens -- are counted in
// `undated`), with each bucket's sales; `max` is the busiest bucket's order count (bar scale).
// `measure` ('tracked' | 'matched') keeps only the rows that measure credits, so the bars sum to
// that measure's order count; the card charts Matched (2026-10-29: the union of both measures
// summed to neither total beside it). Omitted, every row counts.
export function arrivals(rows, measure) {
  const buckets = DAY_BUCKETS.map((label) => ({ label, orders: 0, sales: 0 }));
  let undated = 0;
  (rows || []).filter((r) => !measure || r[measure]).forEach((r) => {
    if (r.day === null || r.day === undefined) {
      undated += 1;
      return;
    }
    const b = buckets[Math.min(r.day, DAY_BUCKETS.length - 1)];
    b.orders += 1;
    b.sales += num(r.sales_usd);
  });
  return { buckets, undated, max: Math.max(0, ...buckets.map((b) => b.orders)) };
}

export const SORT_FIELDS = Object.freeze(['purchased_at', 'campaign_name', 'day', 'sales_usd', 'tracked', 'matched']);
export const DEFAULT_ORDER_SORT = Object.freeze({ field: 'purchased_at', order: 'desc' });

// sortOrderRows sorts by one field; a missing day sorts last in either direction; ties keep the
// newest purchase first, then the order id, so the order is stable.
export function sortOrderRows(rows, sort = DEFAULT_ORDER_SORT) {
  const field = SORT_FIELDS.includes(sort.field) ? sort.field : DEFAULT_ORDER_SORT.field;
  const dir = sort.order === 'asc' ? 1 : -1;
  const val = (r) => {
    if (field === 'purchased_at') return Date.parse(r.purchased_at) || 0;
    if (field === 'tracked' || field === 'matched') return r[field] ? 1 : 0;
    return r[field];
  };
  return [...(rows || [])].sort((a, b) => {
    const va = val(a);
    const vb = val(b);
    if (va === null && vb !== null) return 1;
    if (vb === null && va !== null) return -1;
    if (va !== vb) {
      const c = typeof va === 'string' ? va.localeCompare(vb) : va - vb;
      return dir * c;
    }
    return (Date.parse(b.purchased_at) || 0) - (Date.parse(a.purchased_at) || 0) || a.order_id.localeCompare(b.order_id);
  });
}

const CSV_HEADER = ['order_id', 'purchased_at_utc', 'campaign_id', 'campaign_name', 'day_after_send', 'sales_usd', 'tracked', 'matched'];

const csvField = (v) => {
  const s = v === null || v === undefined ? '' : String(v);
  return /[",\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
};

// A spreadsheet treats a cell beginning with = + - @ (or a tab/CR) as a formula, so every text
// column (the order id and the user-typed campaign name) is prefixed with an apostrophe when it
// starts with one (OWASP CSV injection guidance; Stage 4b F3).
const safeText = (s) => (/^[=+\-@\t\r]/.test(s) ? `'${s}` : s);

// toCsv is the drill-down as CSV text (header + one line per row, CRLF), sales in cents precision so
// the export keeps what the card rounds away, purchase times in UTC ISO form.
export function toCsv(rows) {
  const lines = [CSV_HEADER.join(',')];
  (rows || []).forEach((r) => {
    const at = Date.parse(r.purchased_at);
    lines.push([
      safeText(r.order_id),
      Number.isFinite(at) ? new Date(at).toISOString() : '',
      r.campaign_id,
      safeText(r.campaign_name),
      r.day === null || r.day === undefined ? '' : r.day,
      num(r.sales_usd).toFixed(2),
      r.tracked ? 'yes' : 'no',
      r.matched ? 'yes' : 'no',
    ].map(csvField).join(','));
  });
  return `${lines.join('\r\n')}\r\n`;
}

// csvFileName is conversions-c<ids>-<YYYY-MM-DD>.csv (at most three ids named, then "+N").
export function csvFileName(campaignIds, date = new Date()) {
  const ids = (campaignIds || []).map(Number).filter(Number.isFinite);
  const named = ids.slice(0, 3).join('-') + (ids.length > 3 ? `+${ids.length - 3}` : '');
  return `conversions-c${named || 'none'}-${date.toISOString().slice(0, 10)}.csv`;
}
