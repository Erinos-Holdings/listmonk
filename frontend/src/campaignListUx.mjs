// Fork (journey form UX, integrations JOURNEY-UX-SPEC D1-D4, D13, D14; Amendment A1). The pure half
// of the campaign form's journey block: what the selected lists' join triggers say about the
// Journey automation / Send later / Delay controls, the re-derived defaults on a class change, the
// notices and the journey fields of the save payload; plus the sender section's visibility (A1.9)
// and the add-membership picker filter (A1.2). The .vue files only bind. The server is the control
// (validateCampaignFields refuses a delayed evergreen on a no_purchase list, D5; a session cannot
// add a member to a step list, A1.1); this is the mirror. Tested by campaignListUx.test.mjs.

import {
  storedTrigger, triggerOption, isStepTrigger, TRIGGER_FIRST_ORDER, TRIGGER_SKUS,
} from './listTrigger.mjs'; // eslint-disable-line import/extensions

export const CLASS_NONE = 'none';
export const CLASS_PURCHASE = 'purchase';
// The STEP class -- a list the integrations sweep fills: a no_purchase (Welcome series) or, since
// LIFECYCLE-SERIES-SPEC S2/F11, a post_purchase (Post-purchase series) step list. Its value keeps
// the historical 'no_purchase' spelling; every step rule (the zero-delay row, the locked
// automation, the step-list notice, isStepList, addableLists) keys on it.
export const CLASS_NO_PURCHASE = 'no_purchase';

// The D2 rows: a single list's class, or one of these.
export const ROW_EMPTY = 'empty';
export const ROW_MULTI = 'multi';

// The A1.12 class of a whole selection: what a re-derive keys on. Empty and trigger-less selections
// (a single trigger-less list, or several of them) are one class, PLAIN, so an innocent list swap
// keeps the user's toggles and schedule; a single trigger list is its own class; several lists with
// a trigger among them are MULTI.
export const SEL_PLAIN = 'plain';

// The machine-written tag every re-engagement queue list carries (reengage:<brand list id>:1|2,
// add-listmonk-brand.ts) -- the A1.14 note's key.
export const REENGAGE_TAG_PREFIX = 'reengage:';

// listJourneyClass is a list row's class from its stored join_trigger (D1), read under either key
// spelling (storedTrigger). No row, no trigger or an unknown one is none.
export function listJourneyClass(list) {
  const opt = triggerOption(storedTrigger(list));
  if (opt === TRIGGER_FIRST_ORDER || opt === TRIGGER_SKUS) return CLASS_PURCHASE;
  if (isStepTrigger(opt)) return CLASS_NO_PURCHASE;
  return CLASS_NONE;
}

// selectionClasses resolves the selected lists ({id, name}) against the list options by id: one
// {id, name, cls} per selected list. An id with no option row classifies none (F6 -- e.g. a
// campaign saved on a now-archived list; the store loads active lists only).
export function selectionClasses(selected, options) {
  const opts = Array.isArray(options) ? options : [];
  return (Array.isArray(selected) ? selected : []).map((l) => {
    const row = opts.find((o) => o && o.id === l.id);
    return { id: l.id, name: (row && row.name) || l.name || '', cls: row ? listJourneyClass(row) : CLASS_NONE };
  });
}

// selectionRow is the D2 matrix row for the resolved selection.
export function selectionRow(sel) {
  const s = Array.isArray(sel) ? sel : [];
  if (s.length === 0) return ROW_EMPTY;
  if (s.length > 1) return ROW_MULTI;
  return s[0].cls;
}

// selectionClassKey is the A1.12 class of a resolved selection (see SEL_PLAIN).
export function selectionClassKey(sel) {
  const s = Array.isArray(sel) ? sel : [];
  if (s.every((l) => l.cls === CLASS_NONE)) return SEL_PLAIN;
  if (s.length > 1) return ROW_MULTI;
  return s[0].cls;
}

const isTriggerRow = (row) => row === CLASS_PURCHASE || row === CLASS_NO_PURCHASE;

// journeyMatrix is what the form renders and submits for a selection row (D2 as amended by A1.13),
// ANDed with the existing render gate serverConfig.evergreen_enabled || form.evergreen (F7).
// editable is canEdit && !isStarted (D2.2): when false the stored state renders exactly as before
// (apply false, today's visibility, nothing forced or locked). evergreen / sendLater are the form's
// values -- on a locked row they are either the re-derived defaults or a STORED campaign's state,
// rendered as stored (never flipped on open). storedEvergreen is the saved campaign's flag, for the
// one-list hint.
//   showEvergreen / showSendLater / showDelay -- the controls rendered.
//   lockEvergreen / lockSendLater -- rendered but disabled (A1.13: a single trigger list).
//   forceEvergreenOff -- the payload sends evergreen false (the >= 2 lists row).
//   zeroDelay -- the payload sends a 0 delay (the no_purchase row while the delay is hidden, D4).
//   oneListHint -- the >= 2 lists row turned a stored (or form) evergreen off.
export function journeyMatrix({
  row, editable, evergreenEnabled, evergreen, sendLater = false, storedEvergreen = false,
}) {
  const gate = !!evergreenEnabled || !!evergreen;
  const out = {
    apply: false,
    showEvergreen: gate,
    showSendLater: true,
    showDelay: gate && !!evergreen,
    lockEvergreen: false,
    lockSendLater: false,
    forceEvergreenOff: false,
    zeroDelay: false,
    oneListHint: false,
  };
  if (!editable) return out;
  out.apply = true;
  switch (row) {
    case ROW_EMPTY:
      out.showEvergreen = false;
      out.showSendLater = false;
      out.showDelay = false;
      break;
    case ROW_MULTI:
      out.showEvergreen = false;
      out.showDelay = false;
      out.forceEvergreenOff = true;
      out.oneListHint = !!storedEvergreen || !!evergreen;
      break;
    default:
      break;
  }
  if (!isTriggerRow(row)) return out;

  if (evergreenEnabled) {
    // A1.13 -- the hard lock: Journey automation is on and its switch disabled; Send later is
    // hidden, except a stored schedule, which renders as stored and disabled (and round-trips).
    out.showEvergreen = true;
    out.lockEvergreen = true;
    out.showSendLater = !!sendLater;
    out.lockSendLater = true;
  } else {
    // F7 -- automations disabled: no lock (only the D3 warning); today's Send later rule.
    out.showSendLater = !evergreen;
  }
  if (row === CLASS_NO_PURCHASE) {
    out.showDelay = false;
    out.zeroDelay = !!evergreen;
  }
  return out;
}

// selectionChange is the A1.12 re-derive (it supersedes D2.1's shadow rule): run on a selection
// change the user made in this session (source 'user') or a new campaign's preselected lists
// (source 'preselect') -- never on opening a stored campaign (source 'load'), never when not
// editable (D2.2) -- and only when the selection's CLASS changes (selectionClassKey; an innocent
// trigger-less swap keeps the toggles and the schedule). The toggles are set from the new class:
// plain -> both off; a single purchase / no_purchase list -> Journey on, Send later off (only while
// automations are enabled, F7 -- else unchanged); multi -> Journey off, Send later kept. Manual
// changes after that stick until the next class change. Returns the toggles.
export function selectionChange({
  source, editable, prevClass, nextClass, evergreenEnabled, toggles,
}) {
  const cur = { evergreen: !!toggles.evergreen, sendLater: !!toggles.sendLater };
  if (source === 'load' || !editable || prevClass === nextClass) return cur;
  if (nextClass === SEL_PLAIN) return { evergreen: false, sendLater: false };
  if (nextClass === ROW_MULTI) return { ...cur, evergreen: false };
  if (isTriggerRow(nextClass) && evergreenEnabled) return { evergreen: true, sendLater: false };
  return cur;
}

// journeyNotices is the D3 notices (UI only, never blocking): noPurchase when exactly one list is
// selected and it is a no_purchase list; broadcastToTrigger (with the trigger lists' names) when
// the submitted evergreen is off and any selected list carries a trigger. Under the A1.13 lock a
// single trigger list's evergreen is off only in a stored contradiction or with automations
// disabled (F7), so on one list the warning shows exactly then; on several lists whenever one of
// them carries a trigger.
export function journeyNotices(sel, submittedEvergreen) {
  const s = Array.isArray(sel) ? sel : [];
  const triggerLists = s.filter((l) => l.cls !== CLASS_NONE).map((l) => l.name);
  return {
    noPurchase: s.length === 1 && s[0].cls === CLASS_NO_PURCHASE,
    broadcastToTrigger: !submittedEvergreen && triggerLists.length > 0,
    triggerLists,
  };
}

// hasReengageList is the A1.14 re-engagement queue note: any selected list whose option row carries
// a tag with the reengage: prefix.
export function hasReengageList(selected, options) {
  const opts = Array.isArray(options) ? options : [];
  return (Array.isArray(selected) ? selected : []).some((l) => {
    const row = opts.find((o) => o && o.id === l.id);
    const tags = row && Array.isArray(row.tags) ? row.tags : [];
    return tags.some((t) => typeof t === 'string' && t.trim().startsWith(REENGAGE_TAG_PREFIX));
  });
}

// senderSection is A1.9's sender block: nothing before a list is selected; then the persona picker
// when the lists map to a loaded brands row, else the read-only fallback From (unmapped, a
// derivation error, or the brands fetch failed).
export function senderSection(listCount, hasSenderRow) {
  const any = Number(listCount) > 0;
  return { picker: any && !!hasSenderRow, fallback: any && !hasSenderRow };
}

// delaySecs is the evergreen delay the form sends: the raw stored seconds when the days field
// still shows them (untouched), else days * 86400.
export function delaySecs(days, raw) {
  const d = Math.max(0, Number(days) || 0);
  const r = Number(raw) || 0;
  return Math.round(r / 86400) === d ? r : d * 86400;
}

// journeyPayload is the journey fields of the save payload for a matrix answer and the form
// values (sendAt is the minute-truncated Date the form would schedule). A hidden toggle submits
// its forced value: evergreen false on the >= 2 lists row; Send later off -- send_at null --
// wherever it is hidden (I10); a 0 delay wherever the matrix hides it under no_purchase (D4/I5). A
// stored schedule on a locked row is shown (disabled) and so round-trips (A1.13).
export function journeyPayload(m, f) {
  const evergreen = !!f.evergreen && !m.forceEvergreenOff;
  const sendLater = !!f.sendLater && m.showSendLater;
  let delay = 0;
  if (evergreen && !m.zeroDelay) delay = delaySecs(f.sendDelayDays, f.sendDelaySecs);
  return {
    evergreen,
    send_at: sendLater ? (f.sendAt || null) : null,
    send_delay_secs: delay,
  };
}

// isStepList is a step list (no_purchase or post_purchase -- the step class): no session may add a
// member to it (A1.1; LIFECYCLE-SERIES-SPEC S2).
export function isStepList(list) {
  return listJourneyClass(list) === CLASS_NO_PURCHASE;
}

// addableLists is an add-membership picker's options (A1.2): every list but the step lists,
// except one whose id is in keepIds (the subscriber's stored memberships -- an existing membership
// stays visible and removable, and may be re-added before a save).
export function addableLists(options, keepIds = []) {
  const keep = new Set(Array.isArray(keepIds) ? keepIds : []);
  return (Array.isArray(options) ? options : []).filter((l) => !isStepList(l) || keep.has(l.id));
}

// manageListOptions is the Manage-lists modal's single picker (A1.2, stage A1 F2): filtered only
// while the action is add; remove and unsubscribe keep every option -- cleanup's only bulk UI.
// Also used to prune the modal's selection when the action changes to add.
export function manageListOptions(options, action) {
  const opts = Array.isArray(options) ? options : [];
  return action === 'add' ? addableLists(opts) : opts;
}
