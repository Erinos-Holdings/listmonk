// Fork (usage decisions, integrations USAGE-DECISIONS-SPEC D5). The pure half of the decisions
// section under the Brands > Usage journey guide: the header, the two series' columns and the full-
// width Both series, each bullet's number and state line, its history, and the control rules. The
// server owns every rule that matters (who may decide, the current state, voiding); this module only
// renders what GET /api/usage/decisions returns. Dates render in America/Denver (the pane's zone).
// No Vue, no DOM; tested by usageDecisions.test.mjs.

export const SECTION_NO_PURCHASE = 'no-purchase';
export const SECTION_POST_PURCHASE = 'post-purchase';
export const SECTION_BOTH = 'both';
// USAGE-REENGAGEMENT-SPEC D7/D9: the Curated Sunset Policy block (full width, below Both series).
export const SECTION_SUNSET_POLICY = 'sunset-policy';

const isObj = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

const dayFmt = new Intl.DateTimeFormat('en-US', {
  timeZone: 'America/Denver', year: 'numeric', month: '2-digit', day: '2-digit',
});

// denverDay('2026-10-31T04:30:00Z') -> '2026-10-30' (the Mountain calendar day). '' for a falsy or
// unparsable value.
export function denverDay(iso) {
  if (!iso) {
    return '';
  }
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return '';
  }
  const p = {};
  dayFmt.formatToParts(d).forEach((x) => { p[x.type] = x.value; });
  return `${p.year}-${p.month}-${p.day}`;
}

// The section's framing lines (2026-10-30 polish): "Spec published <published>" under the centered
// title, and "Approver: <approver>" at the bottom -- both from the GET.
export function publishedText(view, t = (k) => k) {
  const v = isObj(view) ? view : {};
  return t('decisions.published', { published: String(v.published || '') });
}

export function approverText(view, t = (k) => k) {
  const v = isObj(view) ? view : {};
  return t('decisions.approver', { approver: String(v.approver || '') });
}

// The labels set in bold at the start of a later line ("Recommended:", "NOTE:").
export const LEAD_LABELS = Object.freeze(['Recommended:', 'NOTE:']);

// textLines splits a bullet's text into its lines, each { lead, rest, recommended } with `lead` set
// in bold: the FIRST line's lead is its first sentence (through the first ". ", or the whole line
// when it has none -- "Entry.", "Exit on purchase (your W01–W05 request)."); a later line's lead is
// a LEAD_LABELS label it starts with, else ''. `recommended` (2026-10-31 polish 2) is true from the
// first later line starting "Recommended:" to the end -- the whole recommendation, numbered lines
// and NOTE included, is set in red. Presentation only: the stored text (and its hash) is unchanged.
export function textLines(text) {
  let recommended = false;
  return String(text == null ? '' : text).split('\n').map((line, i) => {
    if (i === 0) {
      const at = line.indexOf('. ');
      return at < 0 ? { lead: line, rest: '', recommended } : { lead: line.slice(0, at + 1), rest: line.slice(at + 1), recommended };
    }
    const label = LEAD_LABELS.find((l) => line.startsWith(l));
    recommended = recommended || label === 'Recommended:';
    return label ? { lead: label, rest: line.slice(label.length), recommended } : { lead: '', rest: line, recommended };
  });
}

// The items split into the guide's two columns plus the full-width Both series and Curated Sunset
// Policy blocks, in file order.
export function columnsOf(view) {
  const items = isObj(view) && Array.isArray(view.items) ? view.items.filter(isObj) : [];
  return {
    noPurchase: items.filter((i) => i.section === SECTION_NO_PURCHASE),
    postPurchase: items.filter((i) => i.section === SECTION_POST_PURCHASE),
    both: items.filter((i) => i.section === SECTION_BOTH),
    sunsetPolicy: items.filter((i) => i.section === SECTION_SUNSET_POLICY),
  };
}

const strings = (a) => Array.isArray(a) && a.every((s) => typeof s === 'string');

// USAGE-REENGAGEMENT-SPEC D8/D9: a section's stored non-decision table from the GET's `tables` --
// { columns, rows, note } verbatim -- or null when the view carries none (an older fork) or it is
// malformed (no columns, a ragged or non-string row). Rendered, never approvable.
export function tableOf(view, section) {
  const tables = isObj(view) && isObj(view.tables) ? view.tables : null;
  const tb = tables && isObj(tables[section]) ? tables[section] : null;
  if (!tb || !strings(tb.columns) || !tb.columns.length || !Array.isArray(tb.rows)) {
    return null;
  }
  if (!tb.rows.every((r) => strings(r) && r.length === tb.columns.length)) {
    return null;
  }
  return { columns: tb.columns, rows: tb.rows, note: typeof tb.note === 'string' ? tb.note : '' };
}

// A bullet's number: "3", or '' for a Both series item (n null).
export function itemNumber(item) {
  return isObj(item) && Number.isInteger(item.n) ? String(item.n) : '';
}

// The line key of each stored decision (2026-10-31 polish adds current / recommended).
const LINE_KEYS = Object.freeze({
  accept: 'decisions.accepted',
  reject: 'decisions.rejected',
  current: 'decisions.keptCurrent',
  recommended: 'decisions.choseRecommended',
});

// One decision row's line: "Accepted by <name> · <date>", "Rejected by …", "Current kept by …",
// "Recommended chosen by …".
function decisionLine(row, t) {
  const key = LINE_KEYS[row.decision] || LINE_KEYS.accept;
  return t(key, { name: String(row.user_name || ''), date: denverDay(row.created_at) });
}

// choicesOf (2026-10-31 polish) is the item's toggle buttons, in order, from the server's
// `choices` (decisions.Choices: Current | Recommended | Reject on an item with a "Recommended:"
// line, else Accept | Reject; the page never re-derives the rule) -- each { value, label, help },
// the help a hover saying what the choice records. A GET without `choices` (an older server)
// falls back to Accept | Reject.
export function choicesOf(item, t = (k) => k) {
  const raw = isObj(item) && Array.isArray(item.choices) ? item.choices.filter((v) => Object.prototype.hasOwnProperty.call(LINE_KEYS, v)) : [];
  const values = raw.length ? raw : ['accept', 'reject'];
  const withRecommended = values.includes('recommended');
  return values.map((value) => ({
    value,
    label: t(`decisions.${value}`),
    help: t(value === 'reject' && withRecommended ? 'decisions.rejectRecommendedHelp' : `decisions.${value}Help`),
  }));
}

// The toggle shown selected (blue): a Reject being written (`pending`, its note open), else the
// stored decision when it is one of the item's choices; null (nothing selected) while awaiting.
export function selectedChoice(item, pending = null, t = (k) => k) {
  if (pending === 'reject') {
    return 'reject';
  }
  const cur = isObj(item) && isObj(item.current) ? item.current.decision : null;
  return choicesOf(item, t).some((c) => c.value === cur) ? cur : null;
}

// The bullet's ONE state line: decided -> { kind: 'accepted' | 'rejected', text, note } (the note
// only on a reject); undecided -> { kind: 'awaiting', text: "Awaiting since <since>", note: '' }.
export function stateLine(item, t = (k) => k) {
  const cur = isObj(item) && isObj(item.current) ? item.current : null;
  if (!cur) {
    return { kind: 'awaiting', text: t('decisions.awaiting', { since: String((item && item.since) || '') }), note: '' };
  }
  const reject = cur.decision === 'reject';
  return {
    kind: reject ? 'rejected' : 'accepted',
    text: decisionLine(cur, t),
    note: reject && typeof cur.note === 'string' ? cur.note : '',
  };
}

// The collapsed history: every stored row as the server ordered it (newest first), each with its
// line, its note, and the "text changed" mark on a voided row. The summary is "History (N)".
export function historyOf(item, t = (k) => k) {
  const rows = isObj(item) && Array.isArray(item.history) ? item.history.filter(isObj) : [];
  return {
    summary: t('decisions.history', { n: rows.length }),
    rows: rows.map((r) => ({
      text: decisionLine(r, t),
      note: r.decision === 'reject' && typeof r.note === 'string' ? r.note : '',
      voided: r.voided === true,
      mark: r.voided === true ? t('decisions.voided') : '',
    })),
  };
}

// The decision toggles show only when the server says this user may decide.
export function showControls(view) {
  return isObj(view) && view.can_decide === true;
}

// Reject's Submit is disabled while the trimmed note is empty (the server re-checks).
export function canSubmitReject(note) {
  return typeof note === 'string' && note.trim() !== '';
}

// The POST body for a decision: a reject carries the note as typed (the server trims and re-checks
// it); every other choice (accept / current / recommended) carries no note. The server refuses a
// decision outside the item's choices.
export function decisionBody(item, decision, note) {
  return decision === 'reject'
    ? { hash: item.hash, decision, note: String(note || '') }
    : { hash: item.hash, decision: String(decision), note: '' };
}

// A 409 is the stale-text refusal: show its message and reload the decisions.
export function isStale(err) {
  return !!(err && err.response && err.response.status === 409);
}

// The view with one item replaced by the POST's returned item (matched by key).
export function replaceItem(view, item) {
  if (!isObj(view) || !Array.isArray(view.items) || !isObj(item)) {
    return view;
  }
  return { ...view, items: view.items.map((i) => (isObj(i) && i.key === item.key ? item : i)) };
}
