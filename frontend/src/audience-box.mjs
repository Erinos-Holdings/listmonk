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
  return {
    kind: 'sending', tone: 'info', count, noLang: 0,
  };
};

// Whether the count's hover text is the EN+ split ("n en + m no language") -- an English
// audience with at least one no-language row, exactly the Lists grid's rule -- or the plain
// state text. Used by the Campaign page box and the Campaigns list's To send stat.
export const hasEnSplit = (lang, kind, noLang) => kind === 'audience'
  && (lang || '').toLowerCase() === SEND_LANG_EN && (Number(noLang) || 0) > 0;
