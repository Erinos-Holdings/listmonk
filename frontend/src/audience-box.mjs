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

export const audienceBox = (c) => {
  if (!c || !c.id) {
    return null;
  }
  if (!c.evergreen && isNotStarted(c)) {
    if (c.audience === null || c.audience === undefined) {
      return null;
    }
    const count = Number(c.audience) || 0;
    return {
      kind: 'audience',
      tone: count > 0 ? 'success' : 'danger',
      count,
      noLang: Number(c.audienceNoLang) || 0,
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
