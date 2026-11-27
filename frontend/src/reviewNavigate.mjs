// Fork (review navigation, integrations REVIEW-NAVIGATION-SPEC §4.1-§4.5) -- the Inspect window's
// block references and their click-through, pure half: what a finding's reference line says and
// whether it is a link, which thumbnail and standards links render, the postMessage shapes, the
// receiver's and the window's acceptance rules, the opener decision and the new-tab fallback URL.
// CampaignReview.vue (the window) and App.vue (the receiver in the admin window) do the DOM work.
//
// Protocol (same origin only; every message is posted with targetOrigin = our origin):
//   campaign page -> Inspect window   {type: 'lm-review:opener', campaignId}        on every Inspect click
//   Inspect window -> opener          {type: 'lm-review:select', campaignId, blockId, target, token}
//                                     (integrations REVIEW-LOCATIONS-SPEC D6: exactly one of blockId / target)
//   opener -> Inspect window          {type: 'lm-review:ack', token, result: 'received'}   at once
//   opener -> Inspect window          {type: 'lm-review:ack', token, result: 'selected'|'declined'|'unknown'}
// No `received` within ACK_TIMEOUT_MS -> the window opens the campaign's content page in a new tab
// (fallbackUrl, into the named window fallbackWindowName so repeated fallbacks reuse one tab).
// After `received`, a reply later than RESULT_TIMEOUT_MS is ignored.
//   fallback tab -> Inspect window     {type: 'lm-review:opener', campaignId}        at mount
// (2026-10-11) -- the fallback tab adopts itself as the window's editor, so one unreachable admin
// tab (closed, discarded by the browser, navigated away) costs one new tab, not one per click.

export const MSG_OPENER = 'lm-review:opener';
export const MSG_SELECT = 'lm-review:select';
export const MSG_ACK = 'lm-review:ack';

export const ACK_TIMEOUT_MS = 1500;
// The campaign page's re-post schedule for a message to the Inspect window (window.open navigates the
// window, so a message posted at once can land on the outgoing document): the opener handshake and
// the alt-generation row (integrations ALT-AUTOGEN-SPEC D9) share it -- ONE constant.
export const OPENER_POST_SCHEDULE_MS = Object.freeze([0, 300, 1000, 2500, 5000]);
// App.vue's cap on the second reply (covers a discard dialog left open); the Inspect window's
// own cap is RESULT_TIMEOUT_MS + 1000, so the receiver's `unknown` arrives first.
export const RESULT_TIMEOUT_MS = 20000;
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

// integrations REVIEW-LOCATIONS-SPEC D2/D3/D6 (I4): the closed list of field/tab targets -- the ONE
// home of the field vocabulary. Each target is the campaign page's tab, the control's EXISTING
// selector (a `name=`, `data-cy` or class already on the page; null = the tab only) and the label
// keys the window joins with " › " (each read with $tc(k, 1): the page's own keys where it has one).
export const TARGETS = Object.freeze({
  subject: { tab: 'campaign', selector: '[name="subject"]', labels: ['globals.terms.campaign', 'campaigns.subject'] },
  preheader: { tab: 'campaign', selector: '[name="preheader"]', labels: ['globals.terms.campaign', 'campaigns.preheader'] },
  lists: { tab: 'campaign', selector: '.list-selector', labels: ['globals.terms.campaign', 'globals.terms.lists'] },
  from: { tab: 'campaign', selector: '[name="from_email"]', labels: ['globals.terms.campaign', 'campaigns.review.field.sender'] },
  lang: { tab: 'campaign', selector: '[name="lang"]', labels: ['globals.terms.campaign', 'campaigns.lang'] },
  send_at: { tab: 'campaign', selector: '[data-cy="send_at"]', labels: ['globals.terms.campaign', 'campaigns.sendLater'] },
  send_delay: { tab: 'campaign', selector: '[data-cy="evergreen-delay"]', labels: ['globals.terms.campaign', 'campaigns.review.field.delay'] },
  campaign: { tab: 'campaign', selector: null, labels: ['globals.terms.campaign'] },
  headers: { tab: 'campaign', selector: '[name="headers"]', labels: ['globals.terms.campaign', 'campaigns.review.field.headers'] },
  archive_meta: { tab: 'archive', selector: '[name="archive_meta"]', labels: ['campaigns.archive', 'campaigns.archiveMeta'] },
  archive_slug: { tab: 'archive', selector: '[name="archive_slug"]', labels: ['campaigns.archive', 'campaigns.archiveSlug'] },
  content: { tab: 'content', selector: null, labels: ['campaigns.content'] },
});

export function isValidTarget(t) {
  return typeof t === 'string' && Object.prototype.hasOwnProperty.call(TARGETS, t);
}

// A target's {tab, selector} (I4), or null.
export function targetOf(t) {
  return isValidTarget(t) ? { tab: TARGETS[t].tab, selector: TARGETS[t].selector } : null;
}

// D6: a request names exactly one place -- a valid block id OR a valid target, the other absent.
// Returns {blockId, target} (the absent one null), or null.
export function navRef(blockId, target) {
  const noBlock = blockId === undefined || blockId === null;
  const noTarget = target === undefined || target === null;
  if (!noBlock && noTarget && isValidBlockId(blockId)) return { blockId, target: null };
  if (noBlock && !noTarget && isValidTarget(target)) return { blockId: null, target };
  return null;
}

// Whether a select result answers a request: the same block id or the same target.
export function sameRef(a, b) {
  return !!a && !!b && (a.blockId || null) === (b.blockId || null) && (a.target || null) === (b.target || null);
}

// The campaign page's field focus (DOM, called after the tab switch): scroll the control's wrapper
// into view when it exists and focus the control when it is enabled. Never fails -- an absent or
// disabled control leaves the tab switch alone. Returns whether a control took focus.
export function focusControl(root, selector) {
  const el = root && selector ? root.querySelector(selector) : null;
  if (!el) return false;
  if (typeof el.scrollIntoView === 'function') el.scrollIntoView({ block: 'center' });
  const ctl = typeof el.matches === 'function' && el.matches('input, select, textarea') ? el : el.querySelector('input, select, textarea');
  if (!ctl || ctl.disabled || typeof ctl.focus !== 'function') return false;
  ctl.focus({ preventScroll: true });
  return true;
}

// ------------------------------------------------------------------ a finding's location (D2/D3)

// Exact tokens -> target.
const TOKENS = Object.freeze({
  subject: 'subject',
  preheader: 'preheader',
  'attribs.preheader': 'preheader',
  lists: 'lists',
  from: 'from',
  'attribs.lang': 'lang',
  '<html lang>': 'lang',
  send_at: 'send_at',
  send_delay_secs: 'send_delay',
  status: 'campaign',
  archive_meta: 'archive_meta',
  archive_slug: 'archive_slug',
  ...Object.fromEntries(['body', 'body_source', 'content', 'preview', 'structure', 'images', 'audience', 'pixel', 'logo',
    'clip', 'data', 'cid', 'embed', 'unsized', 'button', 'footer'].map((k) => [k, 'content'])),
});

// The bare tokens a free-text location's FIRST matching word maps by.
const WORD_TOKENS = new Set(Object.keys(TOKENS).filter((k) => /^[a-z_]+$/.test(k)));

// Token patterns, tried in order; `campaign` is the other-campaign link, `plain` stays plain.
const PATTERNS = [
  [/^list:\d+$/, 'lists'],
  [/^header:\S+$/, 'headers'],
  [/^footer:\S+$/, 'content'],
  [/^(?:preview )?(?:a|img)\[\d+\]$/, 'content'],
  [/^https?:\/\/\S+$/, 'content'],
];
// A string shaped like a pattern that failed one (list:abc, campaign:0, template:<id>): plain.
const PATTERN_SHAPED = /^(?:(?:list|header|campaign|footer|template):|(?:preview )?(?:a|img)\[)/;
// A host (D5.2) -- tried after the exact tokens, so `attribs.lang` never reads as one.
const HOST = /^[a-z0-9-]+(?:\.[a-z0-9-]+)+$/;

// D2's matching rule (D and A findings alike): {target} | {campaignId} | null (plain).
export function locationRef(location) {
  if (typeof location !== 'string') return null;
  const s = location.trim().toLowerCase().replace(/^i\d+b? /, '');
  if (!s) return null;
  if (Object.prototype.hasOwnProperty.call(TOKENS, s)) return { target: TOKENS[s] };
  const camp = /^campaign:(\d+)$/.exec(s);
  if (camp) {
    const id = Number(camp[1]);
    return isPositiveInt(id) ? { campaignId: id } : null;
  }
  const p = PATTERNS.find(([re]) => re.test(s));
  if (p) return { target: p[1] };
  if (PATTERN_SHAPED.test(s)) return null;
  if (HOST.test(s)) return { target: 'content' };
  const word = s.split(/[^a-z0-9_]+/).find((w) => WORD_TOKENS.has(w));
  return word ? { target: TOKENS[word] } : null;
}

// D3: the other campaign's page (opened in a new tab), built like fallbackUrl; null for a bad id.
export function campaignUrl(base, campaignId) {
  if (!isPositiveInt(campaignId)) return null;
  return `${String(base || '').replace(/\/+$/, '')}/campaigns/${campaignId}`;
}

// ------------------------------------------------------------------ message shapes

export function openerMessage(campaignId) {
  return { type: MSG_OPENER, campaignId };
}

// REVIEW-LOCATIONS-SPEC D6: `target` beside `blockId`, exactly one of them set (the other null).
export function selectMessage(campaignId, blockId, token, target = null) {
  return {
    type: MSG_SELECT, campaignId, blockId: blockId || null, target: target || null, token,
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
// REVIEW-LOCATIONS-SPEC D6: a valid block id OR a valid target, exactly one (navRef).
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
  const ref = navRef(data.blockId, data.target);
  if (!isPositiveInt(data.campaignId) || !ref) return null;
  if (typeof data.token !== 'string' || !data.token) return null;
  return { campaignId: data.campaignId, ...ref, token: data.token };
}

// Where the receiver sends a request: the current campaign page selects in place (an event, never
// a router push onto the current page -- the router-view is keyed on fullPath); any other route
// navigates (the leave guard may decline).
export function receiverRoute({ routeName, routeId, campaignId }) {
  return routeName === 'campaign' && String(routeId) === String(campaignId) ? 'emit' : 'push';
}

// The router location for a navigating request: a block -> `#content` + `?select=`; a target (D6)
// -> its tab's hash + `?target=`.
export function campaignLocation(campaignId, blockId, target = null) {
  const t = targetOf(target);
  if (t && !blockId) {
    return {
      name: 'campaign', params: { id: String(campaignId) }, query: { target }, hash: `#${t.tab}`,
    };
  }
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

// An `lm-review:opener` handshake the window adopts as its current opener: our origin, a
// top-level source that is not this window (the same source rule as acceptSelect), the type, and
// this window's campaign.
export function acceptOpener({
  origin, ourOrigin, source, self, data, campaignId,
}) {
  if (!ourOrigin || origin !== ourOrigin) return false;
  if (!source || source === self) return false;
  let topLevel = false;
  try {
    topLevel = source.top === source;
  } catch (e) {
    topLevel = false;
  }
  if (!topLevel) return false;
  return !!data && typeof data === 'object' && data.type === MSG_OPENER && data.campaignId === campaignId;
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

// The click's first step: `ignore` (an invalid campaign id, or not exactly one valid block id /
// target -- nothing happens), `post` to a reachable opener, or `newTab`.
export function clickDecision({
  opener, ourOrigin, campaignId, blockId, target,
}) {
  if (!isPositiveInt(campaignId) || !navRef(blockId, target)) return 'ignore';
  return openerReachable(opener, ourOrigin) ? 'post' : 'newTab';
}

// After waiting ACK_TIMEOUT_MS: `received` -> wait for the result; nothing -> the new tab.
export function afterAckWait(result) {
  return result === 'received' ? 'wait' : 'newTab';
}

// The toast for a final result (an i18n key), or null. `field` (D6): a target's result, field wording.
export function resultToast(result, field = false) {
  if (!FINAL_RESULTS.includes(result)) return null;
  return field ? `campaigns.review.nav.field.${result}` : `campaigns.review.nav.${result}`;
}

export function isFinalResult(result) {
  return FINAL_RESULTS.includes(result);
}

// The new-tab fallback: the campaign's content page, selecting on load. `base` is the router base
// (`/admin`). null for an invalid id (never navigated). A target (D6): its tab, `?target=`.
export function fallbackUrl(base, campaignId, blockId, target = null) {
  const ref = navRef(blockId, target);
  if (!isPositiveInt(campaignId) || !ref) return null;
  const b = String(base || '').replace(/\/+$/, '');
  if (ref.target) return `${b}/campaigns/${campaignId}?target=${ref.target}#${TARGETS[ref.target].tab}`;
  return `${b}/campaigns/${campaignId}?select=${encodeURIComponent(blockId)}#content`;
}

// The window the new-tab fallback opens into: one per campaign, so a second fallback navigates
// the first tab instead of opening another.
export function fallbackWindowName(campaignId) {
  return `lm-review-editor-${campaignId}`;
}

// The campaign page's one-time read of `?select=` / `?target=` (D6): {blockId, target} with exactly
// one valid, or null.
export function selectFromQuery(query) {
  const q = query || {};
  return navRef(q.select, q.target);
}

// ------------------------------------------------------------------ the Inspect run (integrations
// REVIEW-TRIM-2-SPEC D4, I12): a window opened by Inspect starts clean.
//
// Inspect opens the window with `?run=<token>` (the click's token -- the same one its alts messages
// carry). The signal travels in the URL, synchronously: a postMessage routinely lands on the
// outgoing document. A window whose URL carries `run`:
//   - renders the STARTING state at first render (the checklist with "Starting inspection" and the
//     live alts row) and never a report whose job is older than the run: it records the latest
//     job id at its first load as `baseJob`, and renders the first latest row whose job id differs
//     from `baseJob` or whose status is `running`;
//   - ends on the latest report with a line when no job follows: the campaign page's `failed`
//     message (a POST /reviews failure, 409/503 included) -> "The inspection could not be started.";
//     no message within RUN_FALLBACK_MS -> "No new inspection started.". Nothing spins forever.
// A window without `run` (the Review link, a reload once the run is over) renders the latest report.
//   campaign page -> Inspect window  {type: 'lm-review:run', campaignId, run, phase: 'failed'}
export const MSG_RUN = 'lm-review:run';
export const RUN_FALLBACK_MS = 90000;
const RUN_TOKEN = /^[A-Za-z0-9-]{1,64}$/;

// The window's one-time read of `?run=` (a valid token or null).
export function runFromQuery(query) {
  const v = query && query.run;
  return typeof v === 'string' && RUN_TOKEN.test(v) ? v : null;
}

// Post-release fix (2026-11-05): the run's BASE job id travels in the URL too (`?base=<job id>` or
// `base=none`), read synchronously at first render -- the campaign page knows the latest job id
// when it clicks. Capturing it at the window's first load raced the start: alt generation can
// return in under a second, so the first load already saw the NEW job and the window waited for a
// different one until the 90 s bound. `undefined` = no `base` param (an older page): the window
// falls back to the first-load capture.
export function runBaseFromQuery(query) {
  const v = query && query.base;
  if (typeof v !== 'string' || !v) return undefined;
  if (v === 'none') return null;
  return RUN_TOKEN.test(v) ? v : undefined;
}

export function runMessage(campaignId, run, phase = 'failed') {
  return {
    type: MSG_RUN, campaignId, run, phase,
  };
}

// The window takes an `lm-review:run` only from our origin, from its opener, for its own campaign
// and its own run.
export function acceptRun({
  origin, ourOrigin, source, opener, data, campaignId, run,
}) {
  if (!ourOrigin || origin !== ourOrigin) return false;
  if (!source || !opener || source !== opener) return false;
  if (!data || typeof data !== 'object' || data.type !== MSG_RUN) return false;
  if (data.campaignId !== campaignId || !run || data.run !== run) return false;
  return data.phase === 'failed';
}

// The pure state rule (I12). `run` is runFromQuery's token (null: not opened by Inspect); `baseJob`
// the latest row's job id at the first load (null: no row; undefined: not loaded yet); `row` the
// latest review row ({job_id, status}); `failed` whether the run's `failed` message arrived;
// `elapsedMs` since the window mounted. Returns {mode, line?}:
//   latest   -- render the latest state as today (no run, or the run's job is here);
//   starting -- the starting state: no report, no early findings, no banners;
//   failed / timeout -- the latest report, with `line` (an i18n key) above it.
export function runView({
  run, baseJob, row, failed = false, elapsedMs = 0,
}) {
  if (!run) return { mode: 'latest' };
  if (baseJob !== undefined && row && typeof row === 'object') {
    const jobId = typeof row.job_id === 'string' && row.job_id ? row.job_id : null;
    if (row.status === 'running' || jobId !== baseJob) return { mode: 'latest' };
  }
  if (failed) return { mode: 'failed', line: 'campaigns.review.run.failed' };
  if (elapsedMs >= RUN_FALLBACK_MS) return { mode: 'timeout', line: 'campaigns.review.run.timeout' };
  return { mode: 'starting' };
}

// ------------------------------------------------------------------ the finding (§4.1-§4.3)

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

// §4.1/§4.2 (polish pass, user 2026-10-06): the reference -- the thumbnail IS the click-through.
//   `image` -- a linkable `where` (not an official footer, a valid block id) with an image to show:
//              `where.image` first, else the finding's `evidence`, each only when isImageUrl passes
//              (this host's uploads); the window renders the image with the block-number badge
//              over its top-right corner, the whole thing the link;
//   `badge` -- a linkable `where` with no image: the number badge alone is the link, then the label;
//   `field`  -- integrations REVIEW-LOCATIONS-SPEC D2/D3: no block `where`, and the `location`
//              maps to a target (locationRef): "Location: <Tab> › <Field>" (or "Content"), the
//              text the link; `labels` are TARGETS' i18n keys;
//   `campaign` -- `campaign:<id>` (D1.8b, D1.4c): "Location: Campaign <id>", a new-tab link;
//   `plain` -- an id that fails validation, or a `location` that maps to nothing:
//              `locationPlain`, else the raw `location` (an older report);
//   `none`  -- nothing to show.
// `text` is the label words ("the <label>") for image/badge -- the number is the badge. The window
// never re-derives a name. REVIEW-LOCATIONS-SPEC D1: an official footer's block is linkable too --
// a badge, never a thumbnail (its block is in the document; selecting it shows where it is).
export function referenceLine(finding, origin) {
  const f = finding || {};
  const w = f.where;
  if (w && typeof w === 'object' && typeof w.label === 'string' && Number.isInteger(w.n)) {
    if (!isValidBlockId(w.block)) {
      return {
        kind: 'plain', text: `the ${w.label} (block ${w.n})`, n: w.n, blockId: null,
      };
    }
    const own = typeof w.image === 'string' ? w.image.trim() : '';
    const ev = typeof f.evidence === 'string' ? f.evidence.trim() : '';
    let image = null;
    if (w.type !== 'OfficialFooter') {
      if (own && isImageUrl(origin, own)) image = own;
      else if (ev && isImageUrl(origin, ev)) image = ev;
    }
    const base = {
      text: `the ${w.label}`, n: w.n, blockId: w.block,
    };
    return image ? { kind: 'image', ...base, image } : { kind: 'badge', ...base };
  }
  const ref = locationRef(f.location);
  if (ref && ref.target) return { kind: 'field', target: ref.target, labels: TARGETS[ref.target].labels };
  if (ref && ref.campaignId) return { kind: 'campaign', campaignId: ref.campaignId };
  if (typeof f.locationPlain === 'string' && f.locationPlain) {
    return { kind: 'plain', text: f.locationPlain };
  }
  if (typeof f.location === 'string' && f.location) {
    return { kind: 'plain', text: f.location };
  }
  return { kind: 'none', text: '' };
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
