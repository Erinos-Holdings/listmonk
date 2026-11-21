// Fork (campaign-page audience box). The one rule for the Campaign page's audience box --
// which tone it takes, which number it shows and which hover text explains the number --
// kept as a pure function so it is testable (npm run test:audience) and so the page and
// the Campaigns list read the same states:
//   audience  a draft / scheduled broadcast: the LIVE count it would send to now
//             (server field `audience`, filled by core.wantsAudience); green when there is
//             somebody, red when there is nobody (the list page's red 0).
//   sending   a broadcast in progress (running / paused) or an evergreen of any not-done
//             status: sent so far, blue.
//   sent      finished / cancelled: the sent count, grey.
// null means "no box" -- a new campaign, or an estimate the server could not make.

export const SEND_LANG_EN = 'en';

const isDone = (c) => c.status === 'finished' || c.status === 'cancelled';
const isNotStarted = (c) => c.status === 'draft' || c.status === 'scheduled';

// Fork (smart sending, integrations SMART-SENDING-SPEC D3) -- the not-started broadcast's To send:
// `audience` minus `audience_skipped` (the recipients Smart Sending would skip if it went out now),
// the number it would send to now; `total` is the raw audience (the hover's "{n} of {total}", and
// what the EN+ split partitions). null when the server sent no audience. Shared by the Campaign
// page box and the Campaigns list's To send stat.
// `on` (user 2026-11-07) is the Smart Sending switch AS THE FORM SHOWS IT, so the box previews a
// toggle live before the save: off = nothing skipped, the count is the whole audience. The
// server's `audienceSkipped` is computed from the STORED switch, so after a save the two agree.
// Fork (60-day opens, integrations SIXTY-DAY-OPENS-SPEC D4) -- `silent` is the 60-day opens switch AS
// THE FORM SHOWS IT, { on, count }: off = no silent skips; on = `count` (the live preview's number,
// GET /api/campaigns/:id/silent-audience, which excludes Smart Sending skips) or, when count is null,
// the server's stored share (`audienceSilentSkipped`). Omitted (null) = the stored toggle applies (the
// Campaigns list). The server's `audienceSkipped` is the TOTAL (Smart Sending + silent), so its Smart
// Sending share is audienceSkipped - audienceSilentSkipped. With the form's Smart Sending off but
// unsaved, the preview's silent count still excludes the stored Smart Sending skips -- a lower bound on
// skips until saved (the overlap is unknown to the page).
export const skipSplitOf = (c, on = true, silent = null) => {
  const storedSilent = Math.max(0, Number(c && c.audienceSilentSkipped) || 0);
  const storedTotal = Math.max(0, Number(c && c.audienceSkipped) || 0);
  const smart = on ? Math.max(0, storedTotal - storedSilent) : 0;
  let quiet = storedSilent;
  if (silent) {
    const n = silent.count === null || silent.count === undefined ? storedSilent : Math.max(0, Number(silent.count) || 0);
    quiet = silent.on ? n : 0;
  }
  return { smart, silent: quiet };
};

export const toSendOf = (c, on = true, silent = null) => {
  if (!c || c.audience === null || c.audience === undefined) {
    return null;
  }
  const total = Number(c.audience) || 0;
  const split = skipSplitOf(c, on, silent);
  const skipped = Math.min(total, split.smart + split.silent);
  return { count: total - skipped, skipped, total };
};

// Fork (60-day opens, D4) -- whether the toggle is on as the page reads it: the server's effective
// top-level field (`silent_skip`, camelCased to `silentSkip`), off when absent. Never read from attribs.
export const silentSkipOf = (c) => !!(c && c.silentSkip === true);

// The owned-key fold for `attribs.silent_skip`: on writes `silent_skip: true`, off removes the key;
// either way the camelCased `silentSkip` junk key is removed. Returns a new object; attribs may be null.
export const foldSilentSkip = (attribs, on) => {
  const out = { ...(attribs || {}) };
  delete out.silent_skip;
  delete out.silentSkip;
  if (on) {
    out.silent_skip = true;
  }
  return out;
};

// D4 -- a clone never carries the toggle: the copied attribs without `silent_skip` / `silentSkip`, so
// every new campaign, cloned or not, starts off. A new object; null stays null.
export const cloneAttribs = (attribs) => {
  if (!attribs) {
    return attribs;
  }
  const out = { ...attribs };
  delete out.silent_skip;
  delete out.silentSkip;
  return out;
};

// Fork (smart sending, SMART-SENDING-SPEC D4/D5; Stage 4 R1) -- whether Smart Sending is on for a
// campaign as the page reads it: the server's effective top-level field (`smart_sending`, which the
// API client camelCases to `smartSending`), true when absent. NEVER read from `attribs`: it holds
// only an explicit `false`, never the effective value. (Since ATTRIBS-CASE-SPEC the campaign reads
// keep nested attribs keys verbatim; a `smartSending` key is only legacy junk on an older row.)
export const smartSendingOf = (c) => !(c && c.smartSending === false);

// The owned-key fold for `attribs.smart_sending` (Stage 4 R1): `off` writes `smart_sending: false`,
// otherwise the key is omitted (on). Either way BOTH the stored key and the camelCased `smartSending`
// a round-tripped attribs JSON may carry are removed first, so the junk key can never be posted.
// Returns a new object; `attribs` may be null.
export const foldSmartSending = (attribs, off) => {
  const out = { ...(attribs || {}) };
  delete out.smart_sending;
  delete out.smartSending;
  if (off) {
    out.smart_sending = false;
  }
  return out;
};

export const audienceBox = (c, on = true, silent = null) => {
  if (!c || !c.id) {
    return null;
  }
  if (!c.evergreen && isNotStarted(c)) {
    const ts = toSendOf(c, on, silent);
    if (!ts) {
      return null;
    }
    return {
      kind: 'audience',
      tone: ts.count > 0 ? 'success' : 'danger',
      count: ts.count,
      noLang: Number(c.audienceNoLang) || 0,
      skipped: ts.skipped,
      total: ts.total,
    };
  }
  const count = Number(c.sent) || 0;
  if (isDone(c)) {
    return {
      kind: 'sent', tone: 'grey', count, noLang: 0,
    };
  }
  // An evergreen keeps a per-recipient send record, so its sent count splits too (server
  // fields `sent_en` / `sent_no_lang`, English evergreens only, both counted from the same
  // rows); a broadcast's does not. `en` is carried rather than derived because
  // campaigns.sent is a separate counter that need not equal the recorded rows.
  const evergreen = !!c.evergreen;
  return {
    kind: 'sending',
    tone: 'info',
    count,
    noLang: evergreen ? (Number(c.sentNoLang) || 0) : 0,
    en: evergreen ? (Number(c.sentEn) || 0) : null,
  };
};

// Whether the count's hover text carries the EN+ split ("n en + m no language") -- an
// English audience (or an English evergreen's sends) with at least one no-language row,
// exactly the Lists grid's rule. Used by the Campaign page box and the Campaigns list's
// To send stat.
export const hasEnSplit = (lang, kind, noLang) => (kind === 'audience' || kind === 'sending')
  && (lang || '').toLowerCase() === SEND_LANG_EN && (Number(noLang) || 0) > 0;

// The two numbers of that split. An audience splits its own count (en = count - noLang, the
// Lists grid's arithmetic; the query guarantees noLang <= count); an evergreen's sends carry
// their English share explicitly. Never negative.
export const enSplit = (count, noLang, en) => {
  const none = Math.max(0, Number(noLang) || 0);
  const e = en === null || en === undefined ? (Number(count) || 0) - none : Number(en) || 0;
  return { en: Math.max(0, e), none };
};
