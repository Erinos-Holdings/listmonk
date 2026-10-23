// Fork (journey form UX, integrations JOURNEY-UX-SPEC D1-D4, D13, D14). The pure half of the
// campaign form's journey block: what the selected lists' join triggers say about the Journey
// automation / Send later / Delay controls, the auto-enable on a selection change, the two
// notices and the journey fields of the save payload. Campaign.vue only binds. The server is the
// control (validateCampaignFields refuses a delayed evergreen on a no_purchase list, D5); this is
// the mirror. Tested by campaignListUx.test.mjs.

import {
  storedTrigger, triggerOption, TRIGGER_FIRST_ORDER, TRIGGER_SKUS, TRIGGER_NO_PURCHASE,
} from './listTrigger.mjs'; // eslint-disable-line import/extensions

export const CLASS_NONE = 'none';
export const CLASS_PURCHASE = 'purchase';
export const CLASS_NO_PURCHASE = 'no_purchase';

// The D2 rows: a single list's class, or one of these.
export const ROW_EMPTY = 'empty';
export const ROW_MULTI = 'multi';

// listJourneyClass is a list row's class from its stored join_trigger (D1), read under either key
// spelling (storedTrigger). No row, no trigger or an unknown one is none.
export function listJourneyClass(list) {
  const opt = triggerOption(storedTrigger(list));
  if (opt === TRIGGER_FIRST_ORDER || opt === TRIGGER_SKUS) return CLASS_PURCHASE;
  if (opt === TRIGGER_NO_PURCHASE) return CLASS_NO_PURCHASE;
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

// journeyMatrix is what the form renders and submits for a selection row (D2), ANDed with the
// existing render gate serverConfig.evergreen_enabled || form.evergreen (F7). editable is
// canEdit && !isStarted (D2.2): when false the stored state renders exactly as before (apply
// false, today's visibility, nothing forced). storedEvergreen is the saved campaign's flag and
// shadow the pre-auto-enable toggles (selectionChange), for the one-list hint.
//   showEvergreen / showSendLater / showDelay -- the controls rendered.
//   forceEvergreenOff -- the payload sends evergreen false (the >= 2 lists row).
//   zeroDelay -- the payload sends a 0 delay (the no_purchase row while the delay is hidden, D4).
//   oneListHint -- the >= 2 lists row turned a stored (or pre-flip) evergreen off.
export function journeyMatrix({
  row, editable, evergreenEnabled, evergreen, storedEvergreen = false, shadow = null,
}) {
  const gate = !!evergreenEnabled || !!evergreen;
  const out = {
    apply: false,
    showEvergreen: gate,
    showSendLater: true,
    showDelay: gate && !!evergreen,
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
      out.oneListHint = !!storedEvergreen || !!evergreen || !!(shadow && shadow.evergreen);
      break;
    case CLASS_PURCHASE:
      out.showSendLater = !evergreen;
      break;
    case CLASS_NO_PURCHASE:
      out.showSendLater = !evergreen;
      out.showDelay = false;
      out.zeroDelay = !!evergreen;
      break;
    default:
      break;
  }
  return out;
}

// selectionChange is the D2.1 auto-enable, run on a selection change the user made in this
// session (source 'user') or a new campaign's preselected lists (source 'preselect') -- never on
// opening a stored campaign (source 'load': unchanged), never when not editable (D2.2). A single
// purchase or no_purchase list SETS evergreen on and Send later off (only while automations are
// enabled, F7); >= 2 lists sets evergreen off. A default flip, not a lock: the user may switch it
// back. The toggles as they were before the first flip are kept as the shadow and restored when
// the selection returns to a trigger-less single list or to no lists. Returns {toggles, shadow}.
export function selectionChange({
  source, editable, row, evergreenEnabled, toggles, shadow = null,
}) {
  const cur = { evergreen: !!toggles.evergreen, sendLater: !!toggles.sendLater };
  if (source === 'load' || !editable) return { toggles: cur, shadow };

  let next = null;
  if (row === ROW_MULTI) {
    next = { ...cur, evergreen: false };
  } else if ((row === CLASS_PURCHASE || row === CLASS_NO_PURCHASE) && evergreenEnabled) {
    next = { evergreen: true, sendLater: false };
  }

  if (next === null) {
    // A trigger-less single list, no lists, or a trigger list with automations disabled: undo.
    if (shadow) return { toggles: { ...shadow }, shadow: null };
    return { toggles: cur, shadow: null };
  }
  const changed = next.evergreen !== cur.evergreen || next.sendLater !== cur.sendLater;
  if (!shadow && !changed) return { toggles: cur, shadow: null };
  return { toggles: next, shadow: shadow || cur };
}

// manualToggle is the user switching Journey automation or Send later by hand: the value is the
// user's own choice from then on, so the shadow is dropped -- an override survives a later undo.
export function manualToggle({ toggles, key, value }) {
  return { toggles: { ...toggles, [key]: !!value }, shadow: null };
}

// journeyNotices is the D3 notices (UI only, never blocking): noPurchase when exactly one list is
// selected and it is a no_purchase list; broadcastToTrigger (with the trigger lists' names) when
// the submitted evergreen is off and any selected list carries a trigger, whatever the count.
export function journeyNotices(sel, submittedEvergreen) {
  const s = Array.isArray(sel) ? sel : [];
  const triggerLists = s.filter((l) => l.cls !== CLASS_NONE).map((l) => l.name);
  return {
    noPurchase: s.length === 1 && s[0].cls === CLASS_NO_PURCHASE,
    broadcastToTrigger: !submittedEvergreen && triggerLists.length > 0,
    triggerLists,
  };
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
// wherever it is hidden (I10); a 0 delay wherever the matrix hides it under no_purchase (D4/I5).
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
