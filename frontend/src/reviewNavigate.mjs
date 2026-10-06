// Fork (review navigation, integrations REVIEW-NAVIGATION-SPEC §4.1-§4.5) -- the Inspect window's
// block references and their click-through, pure half: what a finding's reference line says and
// whether it is a link, which thumbnail and standards links render, the postMessage shapes, the
// receiver's and the window's acceptance rules, the opener decision and the new-tab fallback URL.
// CampaignReview.vue (the window) and App.vue (the receiver in the admin window) do the DOM work.
//
// Protocol (same origin only; every message is posted with targetOrigin = our origin):
//   campaign page -> Inspect window   {type: 'lm-review:opener', campaignId}        on every Inspect click
//   Inspect window -> opener          {type: 'lm-review:select', campaignId, blockId, token}
//   opener -> Inspect window          {type: 'lm-review:ack', token, result: 'received'}   at once
//   opener -> Inspect window          {type: 'lm-review:ack', token, result: 'selected'|'declined'|'unknown'}
// No `received` within ACK_TIMEOUT_MS -> the window opens the campaign's content page in a new tab
// (fallbackUrl). After `received`, a reply later than RESULT_TIMEOUT_MS is ignored.

export const MSG_OPENER = 'lm-review:opener';
export const MSG_SELECT = 'lm-review:select';
export const MSG_ACK = 'lm-review:ack';

export const ACK_TIMEOUT_MS = 1500;
export const RESULT_TIMEOUT_MS = 15000;
// The campaign page polls the builder this long for a block to select after a deep-link mount.
export const SELECT_POLL_MS = 250;
export const SELECT_POLL_CAP_MS = 10000;

export const RESULTS = ['received', 'selected', 'declined', 'unknown'];
const FINAL_RESULTS = ['selected', 'declined', 'unknown'];

// The builder's ids: freshId's `block-<ms>-<n>` and the official `block-official-*`. Anything else
// is ignored on every path and never reaches a selector, a router query or a URL.
const BLOCK_ID = /^block-[A-Za-z0-9-]+$/;

export function isValidBlockId(id) {
  return typeof id === 'string' && BLOCK_ID.test(id);
}

export function isPositiveInt(n) {
  return Number.isInteger(n) && n > 0;
}

// ------------------------------------------------------------------ message shapes

export function openerMessage(campaignId) {
  return { type: MSG_OPENER, campaignId };
}

export function selectMessage(campaignId, blockId, token) {
  return {
    type: MSG_SELECT, campaignId, blockId, token,
  };
}

export function ackMessage(token, result) {
  return { type: MSG_ACK, token, result };
}

// A random per-click token.
export function newToken(rand = Math.random) {
  return `${Date.now().toString(36)}-${Math.floor(rand() * 1e12).toString(36)}`;
}

// ------------------------------------------------------------------ the receiver (App.vue)

// The `lm-review:select` a receiver may act on, or null (ignored, NO reply). All must hold: our
// origin; a top-level source (never a frame -- the builder iframe renders author Html blocks), not
// this window; the type; a positive integer campaign id; a valid block id; a token string; and the
// receiver is not itself the Inspect window (its route is not `campaignReview`).
export function acceptSelect({
  origin, ourOrigin, source, self, data, routeName,
}) {
  if (!ourOrigin || origin !== ourOrigin) return null;
  if (!source || source === self) return null;
  let topLevel = false;
  try {
    topLevel = source.top === source;
  } catch (e) {
    topLevel = false;
  }
  if (!topLevel) return null;
  if (!data || typeof data !== 'object' || data.type !== MSG_SELECT) return null;
  if (routeName === 'campaignReview') return null;
  if (!isPositiveInt(data.campaignId) || !isValidBlockId(data.blockId)) return null;
  if (typeof data.token !== 'string' || !data.token) return null;
  return { campaignId: data.campaignId, blockId: data.blockId, token: data.token };
}

// Where the receiver sends a request: the current campaign page selects in place (an event, never
// a router push onto the current page -- the router-view is keyed on fullPath); any other route
// navigates (the leave guard may decline).
export function receiverRoute({ routeName, routeId, campaignId }) {
  return routeName === 'campaign' && String(routeId) === String(campaignId) ? 'emit' : 'push';
}

// The router location for a navigating request.
export function campaignLocation(campaignId, blockId) {
  return {
    name: 'campaign', params: { id: String(campaignId) }, query: { select: blockId }, hash: '#content',
  };
}

// A push that rejects: the leave guard's Cancel (`next(false)` -> aborted) is `declined`; any other
// failure (a redirect, an error) is `unknown`.
export function pushFailureResult(err, isAborted) {
  return isAborted(err) ? 'declined' : 'unknown';
}

// ------------------------------------------------------------------ the Inspect window

// The window's current opener is reachable: open, and its origin readable and ours.
export function openerReachable(win, ourOrigin) {
  if (!win) return false;
  try {
    if (win.closed) return false;
    return win.location.origin === ourOrigin;
  } catch (e) {
    return false;
  }
}

// An `lm-review:opener` handshake the window adopts as its current opener: our origin, the type,
// this window's campaign, and a source to post to.
export function acceptOpener({
  origin, ourOrigin, source, data, campaignId,
}) {
  return !!ourOrigin && origin === ourOrigin && !!source
    && !!data && typeof data === 'object' && data.type === MSG_OPENER && data.campaignId === campaignId;
}

// The result an `lm-review:ack` carries for THIS click, or null (another origin, another source,
// another token, not an ack, or an unknown result).
export function acceptAck({
  origin, ourOrigin, source, expectedSource, data, token,
}) {
  if (!ourOrigin || origin !== ourOrigin) return null;
  if (expectedSource && source !== expectedSource) return null;
  if (!data || typeof data !== 'object' || data.type !== MSG_ACK) return null;
  if (typeof token !== 'string' || data.token !== token) return null;
  return RESULTS.includes(data.result) ? data.result : null;
}

// The click's first step: `ignore` (an invalid block id or campaign id -- nothing happens), `post`
// to a reachable opener, or `newTab`.
export function clickDecision({
  opener, ourOrigin, campaignId, blockId,
}) {
  if (!isPositiveInt(campaignId) || !isValidBlockId(blockId)) return 'ignore';
  return openerReachable(opener, ourOrigin) ? 'post' : 'newTab';
}

// After waiting ACK_TIMEOUT_MS: `received` -> wait for the result; nothing -> the new tab.
export function afterAckWait(result) {
  return result === 'received' ? 'wait' : 'newTab';
}

// The toast for a final result (an i18n key), or null.
export function resultToast(result) {
  return {
    selected: 'campaigns.review.nav.selected',
    declined: 'campaigns.review.nav.declined',
    unknown: 'campaigns.review.nav.unknown',
  }[result] || null;
}

export function isFinalResult(result) {
  return FINAL_RESULTS.includes(result);
}

// The new-tab fallback: the campaign's content page, selecting on load. `base` is the router base
// (`/admin`). null for an invalid id (never navigated).
export function fallbackUrl(base, campaignId, blockId) {
  if (!isPositiveInt(campaignId) || !isValidBlockId(blockId)) return null;
  const b = String(base || '').replace(/\/+$/, '');
  return `${b}/campaigns/${campaignId}?select=${encodeURIComponent(blockId)}#content`;
}

// The campaign page's one-time read of `?select=` (a valid id or null).
export function selectFromQuery(query) {
  const v = query && query.select;
  return isValidBlockId(v) ? v : null;
}

// ------------------------------------------------------------------ the finding (§4.1-§4.3)

// §4.1: the reference line. `link` -- "the <label> (block <n>)" with the name as the click-through;
// `text` -- the same words, plain (an official footer, which is compiled from a template and has
// nothing to select, or an id that fails validation); `plain` -- `locationPlain`, else the raw
// `location` (an older report); `none`. The window never re-derives a name.
export function referenceLine(finding) {
  const f = finding || {};
  const w = f.where;
  if (w && typeof w === 'object' && typeof w.label === 'string' && Number.isInteger(w.n)) {
    const text = `the ${w.label} (block ${w.n})`;
    const linkable = w.type !== 'OfficialFooter' && isValidBlockId(w.block);
    return {
      kind: linkable ? 'link' : 'text', text, label: w.label, n: w.n, blockId: linkable ? w.block : null,
    };
  }
  if (typeof f.locationPlain === 'string' && f.locationPlain) {
    return { kind: 'plain', text: f.locationPlain };
  }
  if (typeof f.location === 'string' && f.location) {
    return { kind: 'plain', text: f.location };
  }
  return { kind: 'none', text: '' };
}

// Only this listmonk host's own uploads render as a thumbnail (Stage 4 finding 17 of
// CAMPAIGN-INSPECT): evidence and `where.image` are model- or author-supplied text, and an
// arbitrary URL in an <img src> would be fetched by the browser. Same-origin, so no host is
// hard-coded.
export function isImageUrl(origin, s) {
  if (typeof s !== 'string' || !origin) {
    return false;
  }
  const v = s.trim();
  return v.startsWith(`${origin}/uploads/`) && /^\S+\.(png|jpe?g|gif|webp)(\?\S*)?$/i.test(v);
}

// §4.2: the thumbnail of the image a finding is located at, or null -- never one the evidence
// thumbnail already shows.
export function thumbnailUrl(origin, finding) {
  const w = finding && finding.where;
  const img = w && typeof w.image === 'string' ? w.image.trim() : '';
  if (!img || !isImageUrl(origin, img)) return null;
  const ev = finding && typeof finding.evidence === 'string' ? finding.evidence.trim() : '';
  if (ev === img && isImageUrl(origin, ev)) return null;
  return img;
}

// §4.3: the item's standards links, https only (anything else is not rendered). The report is the
// only source.
export function standardLinks(item) {
  const refs = (item && Array.isArray(item.references)) ? item.references : [];
  return refs.filter((r) => {
    if (!r || typeof r.label !== 'string' || !r.label || typeof r.url !== 'string') return false;
    try {
      return new URL(r.url.trim()).protocol === 'https:';
    } catch (e) {
      return false;
    }
  }).map((r) => ({ label: r.label, url: r.url.trim() }));
}
