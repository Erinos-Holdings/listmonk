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

// The most recent current decision across every item (2026-11-03 polish): the row whose created_at
// is latest, or null while nothing is decided. A reject counts -- it is a recorded click too.
export function latestDecision(view) {
  const items = isObj(view) && Array.isArray(view.items) ? view.items.filter(isObj) : [];
  let best = null;
  let bestAt = -Infinity;
  items.forEach((i) => {
    const at = isObj(i.current) ? new Date(i.current.created_at).getTime() : NaN;
    if (!Number.isNaN(at) && at > bestAt) {
      best = i.current;
      bestAt = at;
    }
  });
  return best;
}

// The section's one framing line under the centered title (2026-11-03 polish): "Spec published
// <published> · " then "Approved by <name>" -- whoever recorded the most recent decision, with no
// date (2026-11-09: the approval dates are per item) -- or, while nothing is decided, "Pending approval by <approver>"
// (the GET's `approver`: the current brands:decide role holders), "Pending approval" when none.
export function publishedText(view, t = (k) => k) {
  const v = isObj(view) ? view : {};
  const published = t('decisions.published', { published: String(v.published || '') });
  const last = latestDecision(v);
  let status;
  if (last) {
    status = t('decisions.approvedBy', { name: String(last.user_name || '') });
  } else if (v.approver) {
    status = t('decisions.pendingBy', { approver: String(v.approver) });
  } else {
    status = t('decisions.pending');
  }
  return `${published} · ${status}`;
}

// The labels set in bold at the start of a later line ("Recommended:", "NOTE:").
export const LEAD_LABELS = Object.freeze(['Recommended:', 'NOTE:']);

// A later line's first sentence is a bold heading lead when it has at most this many words
// ("Smart Sending."); longer first sentences are prose. Lines starting with a LEAD_LABELS label
// keep the label as the lead.
export const SHORT_LEAD_WORDS = 3;

// opensParagraph (user 2026-11-07): every later line of a bullet renders as a paragraph (a gap
// above it) except a numbered sub-point ("1. …", "2. …"), which stays tight under the line above.
// The first line never opens one. One family rule for every item; the Vue adds the class.
export function opensParagraph(line, index) {
  if (!Number.isInteger(index) || index <= 0 || !line) {
    return false;
  }
  return !/^\d+\.\s/.test(`${line.lead || ''}${line.rest || ''}`);
}

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
    if (label) {
      return { lead: label, rest: line.slice(label.length), recommended };
    }
    // A later line that opens with a short heading sentence -- at most SHORT_LEAD_WORDS words ending
    // in a period, "Smart Sending." -- takes that sentence as its bold lead (user 2026-11-07). A
    // line whose first sentence is longer is an ordinary paragraph with no lead.
    const at = line.indexOf('. ');
    const first = at < 0 ? line : line.slice(0, at + 1);
    // A numbered line ("1. A rule.") is not a heading: the lead must carry a letter.
    if (first.endsWith('.') && /[A-Za-z]/.test(first) && first.trim().split(/\s+/).length <= SHORT_LEAD_WORDS) {
      return { lead: first, rest: line.slice(first.length), recommended };
    }
    return { lead: '', rest: line, recommended };
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
// LIFECYCLE-DOCS-SPEC D9: a stored current / recommended on an item whose choices are now
// Accept | Reject (its "Recommended:" line removed once implemented) shows Accept -- it approved the
// text as it then read; a stored reject is always a choice; anything else selects nothing.
export function selectedChoice(item, pending = null, t = (k) => k) {
  if (pending === 'reject') {
    return 'reject';
  }
  const curRow = isObj(item) && isObj(item.current) ? item.current : null;
  const cur = curRow ? curRow.decision : null;
  const values = choicesOf(item, t).map((c) => c.value);
  // D5 amendment 2026-11-08: an adopted reject (its note became the text) reads as approved, so
  // Accept is the selected toggle, not Reject.
  if (cur === 'reject' && curRow.adopted === true && values.includes('accept')) {
    return 'accept';
  }
  if (values.includes(cur)) {
    return cur;
  }
  if ((cur === 'current' || cur === 'recommended') && values.includes('accept')) {
    return 'accept';
  }
  return null;
}

// The bullet's ONE state line: decided -> { kind: 'accepted' | 'rejected', text, note } (the note
// only on a reject); undecided -> { kind: 'awaiting', text: "Awaiting since <since>", note: '' }.
// LIFECYCLE-DOCS-SPEC D5: an approval whose `current.modified` is set (the approver modified the
// concept off-page after deciding) reads "Approved as modified by <who> · <date>"; a reject keeps
// "Rejected by …" and its note -- a reject is not an approval to modify -- UNLESS the server marks
// it `adopted` (D5 amendment 2026-11-08: the reject note was adopted into the text, a `who` entry
// naming the rejecter after the row's hash): then it reads as approved as modified by them, with
// no note on the state line; the reject row and its note stay in History.
export function stateLine(item, t = (k) => k) {
  const cur = isObj(item) && isObj(item.current) ? item.current : null;
  if (!cur) {
    return { kind: 'awaiting', text: t('decisions.awaiting', { since: String((item && item.since) || '') }), note: '' };
  }
  const modified = isObj(cur.modified) ? cur.modified : null;
  const reject = cur.decision === 'reject' && !(cur.adopted === true && modified);
  const mod = !reject ? modified : null;
  return {
    kind: reject ? 'rejected' : 'accepted',
    text: mod
      ? t('decisions.approvedModified', { name: String(mod.who || ''), date: String(mod.date || '') })
      : decisionLine(cur, t),
    note: reject && typeof cur.note === 'string' ? cur.note : '',
  };
}

// One changelog entry's History line (LIFECYCLE-DOCS-SPEC D7): the item's first entry is its
// publication ("<line> · <date>"); a later reapprove entry "Re-approval requested · …"; an entry
// with `who` "Modified by <who> · …"; any other "Edited · …". Plain text, rendered through
// interpolation only.
function changeLine(c, first, t) {
  const p = {
    who: String(c.who || ''), date: String(c.date || ''), source: String(c.source || ''), line: String(c.line || ''),
  };
  if (first) {
    return t('decisions.logPublished', p);
  }
  if (c.reapprove === true) {
    return t('decisions.logReapprove', p);
  }
  return t(c.who ? 'decisions.logModified' : 'decisions.logEdited', p);
}

// The collapsed history (LIFECYCLE-DOCS-SPEC D7): the stored decision rows and the item's
// changelog entries interleaved, newest first by day -- a row by its Denver day (denverDay), an
// entry by its `date`. On the same day a decision row sorts above an entry, so the publication
// entry sits under that day's decision and a same-day Slack modification shows beneath the day's
// decision (accepted: the entry carries a day, not a time). Within each kind the server's order is
// kept (rows newest first; entries newest first, i.e. file order reversed). Each line is
// { text, note, voided, mark, kind: 'decision' | 'change' }; a voided row is marked "before
// re-approval". The summary is "History (N)", N = rows + entries.
export function historyOf(item, t = (k) => k) {
  const rows = isObj(item) && Array.isArray(item.history) ? item.history.filter(isObj) : [];
  const changes = isObj(item) && Array.isArray(item.changelog) ? item.changelog.filter(isObj) : [];
  const decided = rows.map((r) => ({
    day: denverDay(r.created_at),
    line: {
      text: decisionLine(r, t),
      note: r.decision === 'reject' && typeof r.note === 'string' ? r.note : '',
      voided: r.voided === true,
      mark: r.voided === true ? t('decisions.voided') : '',
      kind: 'decision',
    },
  }));
  const changed = changes.map((c, i) => ({
    day: String(c.date || ''),
    line: {
      text: changeLine(c, i === 0, t), note: '', voided: false, mark: '', kind: 'change',
    },
  })).reverse();
  // A stable merge of the two newest-first lists: a row goes first unless the entry's day is later.
  const out = [];
  let i = 0;
  let j = 0;
  while (i < decided.length || j < changed.length) {
    if (j >= changed.length || (i < decided.length && decided[i].day >= changed[j].day)) {
      out.push(decided[i].line);
      i += 1;
    } else {
      out.push(changed[j].line);
      j += 1;
    }
  }
  return { summary: t('decisions.history', { n: out.length }), rows: out };
}

// Integrations SMART-SENDING-SPEC D11 -- the red note under an item's state line: the `callout` of
// the item's LAST changelog entry when it is a non-empty string, else '' (no note). An earlier
// entry's callout is only what the note said then, so the next entry clears it.
export function calloutOf(item) {
  const cl = isObj(item) && Array.isArray(item.changelog) ? item.changelog : [];
  const last = cl.length ? cl[cl.length - 1] : null;
  return isObj(last) && typeof last.callout === 'string' ? last.callout.trim() : '';
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
