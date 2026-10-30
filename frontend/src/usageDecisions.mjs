// Fork (usage decisions, integrations USAGE-DECISIONS-SPEC D5). The pure half of the decisions
// section under the Brands > Usage journey guide: the header, the two series' columns and the full-
// width Both series, each bullet's number and state line, its history, and the control rules. The
// server owns every rule that matters (who may decide, the current state, voiding); this module only
// renders what GET /api/usage/decisions returns. Dates render in America/Denver (the pane's zone).
// No Vue, no DOM; tested by usageDecisions.test.mjs.

export const SECTION_NO_PURCHASE = 'no-purchase';
export const SECTION_POST_PURCHASE = 'post-purchase';
export const SECTION_BOTH = 'both';

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

// The section header: "Approver: <approver> · Spec published <published>", both from the GET.
export function headerText(view, t = (k) => k) {
  const v = isObj(view) ? view : {};
  return t('decisions.header', { approver: String(v.approver || ''), published: String(v.published || '') });
}

// The items split into the guide's two columns plus the full-width Both series, in file order.
export function columnsOf(view) {
  const items = isObj(view) && Array.isArray(view.items) ? view.items.filter(isObj) : [];
  return {
    noPurchase: items.filter((i) => i.section === SECTION_NO_PURCHASE),
    postPurchase: items.filter((i) => i.section === SECTION_POST_PURCHASE),
    both: items.filter((i) => i.section === SECTION_BOTH),
  };
}

// A bullet's number: "3", or '' for a Both series item (n null).
export function itemNumber(item) {
  return isObj(item) && Number.isInteger(item.n) ? String(item.n) : '';
}

// One decision row's line: "Accepted by <name> · <date>" / "Rejected by <name> · <date>".
function decisionLine(row, t) {
  const key = row.decision === 'reject' ? 'decisions.rejected' : 'decisions.accepted';
  return t(key, { name: String(row.user_name || ''), date: denverDay(row.created_at) });
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

// Accept / Reject show only when the server says this user may decide.
export function showControls(view) {
  return isObj(view) && view.can_decide === true;
}

// Reject's Submit is disabled while the trimmed note is empty (the server re-checks).
export function canSubmitReject(note) {
  return typeof note === 'string' && note.trim() !== '';
}

// The POST body for a decision: an accept carries no note; a reject carries the note as typed (the
// server trims and re-checks it).
export function decisionBody(item, decision, note) {
  return decision === 'reject'
    ? { hash: item.hash, decision, note: String(note || '') }
    : { hash: item.hash, decision: 'accept', note: '' };
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
