// Fork (journey form UX, integrations JOURNEY-UX-SPEC I1, I2, I4, I5, I8, I10; Amendment A1: IA3,
// IA8-IA11). Run: yarn test:campaign-list-ux
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import {
  CLASS_NONE, CLASS_PURCHASE, CLASS_NO_PURCHASE, ROW_EMPTY, ROW_MULTI, SEL_PLAIN,
  listJourneyClass, selectionClasses, selectionRow, selectionClassKey, journeyMatrix, selectionChange,
  journeyNotices, journeyPayload, delaySecs, hasReengageList, senderSection, isStepList, addableLists,
  manageListOptions,
} from './campaignListUx.mjs'; // eslint-disable-line import/extensions
import * as ux from './campaignListUx.mjs'; // eslint-disable-line import/extensions

const FIRST = { id: 1, name: 'Ruze first order', join_trigger: { kind: 'first_order' } };
const SKUS = { id: 2, name: 'Widget buyers', joinTrigger: { kind: 'skus', skus: ['a'] } };
const NP = { id: 3, name: 'Ruze still deciding', joinTrigger: { kind: 'no_purchase', days: 7 } };
const PLAIN = { id: 4, name: 'Ruze newsletter' };
const PLAIN2 = { id: 5, name: 'Ruze VIP', tags: ['holiday-2026'] };
const QUEUE = { id: 6, name: 'Ruze re-engage 1', tags: ['reengage:4:1'] };
const OPTIONS = [FIRST, SKUS, NP, PLAIN, PLAIN2, QUEUE];
const pick = (...rows) => rows.map((r) => ({ id: r.id, name: r.name }));
const selOf = (...rows) => selectionClasses(pick(...rows), OPTIONS);
const rowOf = (...rows) => selectionRow(selOf(...rows));
const keyOf = (...rows) => selectionClassKey(selOf(...rows));

// I1 -- D1 classification for both key spellings, every kind, null and garbage triggers.
test('I1: list journey class', () => {
  assert.equal(listJourneyClass({ join_trigger: { kind: 'first_order' } }), CLASS_PURCHASE);
  assert.equal(listJourneyClass({ joinTrigger: { kind: 'first_order' } }), CLASS_PURCHASE);
  assert.equal(listJourneyClass({ join_trigger: { kind: 'skus', skus: ['x'] } }), CLASS_PURCHASE);
  assert.equal(listJourneyClass({ joinTrigger: { kind: 'skus', skus: ['x'] } }), CLASS_PURCHASE);
  assert.equal(listJourneyClass({ join_trigger: { kind: 'no_purchase', days: 7 } }), CLASS_NO_PURCHASE);
  assert.equal(listJourneyClass({ joinTrigger: { kind: 'no_purchase', days: 14 } }), CLASS_NO_PURCHASE);
  [null, undefined, {}, { join_trigger: null }, { joinTrigger: null }, { join_trigger: 'first_order' },
    { join_trigger: [] }, { join_trigger: { kind: 'nope' } }, { joinTrigger: { days: 3 } }, { join_trigger: 7 }]
    .forEach((l) => assert.equal(listJourneyClass(l), CLASS_NONE, JSON.stringify(l)));
  // The wire spelling wins when both are present (storedTrigger).
  assert.equal(listJourneyClass({ join_trigger: null, joinTrigger: { kind: 'first_order' } }), CLASS_NONE);
});

test('I1/F6: selection resolved against the options; an unresolvable id is none', () => {
  const sel = selectionClasses([{ id: 3, name: 'x' }, { id: 99, name: 'Archived list' }], OPTIONS);
  assert.deepEqual(sel, [
    { id: 3, name: 'Ruze still deciding', cls: CLASS_NO_PURCHASE },
    { id: 99, name: 'Archived list', cls: CLASS_NONE },
  ]);
  assert.deepEqual(selectionClasses(null, OPTIONS), []);
  assert.deepEqual(selectionClasses([{ id: 1, name: 'a' }], null), [{ id: 1, name: 'a', cls: CLASS_NONE }]);
  assert.equal(selectionRow([]), ROW_EMPTY);
  assert.equal(selectionRow(undefined), ROW_EMPTY);
  assert.equal(rowOf(FIRST, PLAIN), ROW_MULTI);
  assert.equal(rowOf(FIRST), CLASS_PURCHASE);
  assert.equal(rowOf(SKUS), CLASS_PURCHASE);
  assert.equal(rowOf(NP), CLASS_NO_PURCHASE);
  assert.equal(rowOf(PLAIN), CLASS_NONE);
});

const ED = { editable: true, evergreenEnabled: true };
const vis = (m) => [m.showEvergreen, m.showSendLater, m.showDelay];
const locks = (m) => [m.lockEvergreen, m.lockSendLater];

// I2 -- the D2 matrix, row by row (trigger rows per A1.13: IA9 below).
test('I2: no lists -- every journey control hidden', () => {
  const m = journeyMatrix({ ...ED, row: ROW_EMPTY, evergreen: true });
  assert.deepEqual(vis(m), [false, false, false]);
  assert.deepEqual(locks(m), [false, false]);
  assert.equal(m.forceEvergreenOff, false);
});

test('I2: >= 2 lists -- Journey hidden and submitted false, Send later shown, delay hidden; the one-list hint', () => {
  const m = journeyMatrix({ ...ED, row: ROW_MULTI, evergreen: false });
  assert.deepEqual(vis(m), [false, true, false]);
  assert.deepEqual(locks(m), [false, false]);
  assert.equal(m.forceEvergreenOff, true);
  assert.equal(m.oneListHint, false);
  // A stored evergreen (or a form one, e.g. the Automations New button's) shows the hint.
  assert.equal(journeyMatrix({
    ...ED, row: ROW_MULTI, evergreen: false, storedEvergreen: true,
  }).oneListHint, true);
  assert.equal(journeyMatrix({ ...ED, row: ROW_MULTI, evergreen: true }).oneListHint, true);
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: ROW_MULTI, evergreen: true }), { evergreen: true, sendDelayDays: 3 }).evergreen, false);
});

test('I2: one trigger-less list -- all shown and operable, delay iff evergreen', () => {
  assert.deepEqual(vis(journeyMatrix({ ...ED, row: CLASS_NONE, evergreen: false })), [true, true, false]);
  assert.deepEqual(vis(journeyMatrix({ ...ED, row: CLASS_NONE, evergreen: true })), [true, true, true]);
  assert.deepEqual(locks(journeyMatrix({ ...ED, row: CLASS_NONE, evergreen: true })), [false, false]);
});

test('I2/F6: an unresolvable list id renders the trigger-less row', () => {
  const row = selectionRow(selectionClasses([{ id: 99, name: 'Gone' }], OPTIONS));
  assert.equal(row, CLASS_NONE);
  assert.deepEqual(vis(journeyMatrix({ ...ED, row, evergreen: false })), [true, true, false]);
  assert.equal(selectionClassKey(selectionClasses([{ id: 99, name: 'Gone' }], OPTIONS)), SEL_PLAIN);
  assert.deepEqual(
    journeyNotices(selectionClasses([{ id: 99, name: 'Gone' }], OPTIONS), false),
    { noPurchase: false, broadcastToTrigger: false, triggerLists: [] },
  );
});

test('I2/F7: automations disabled -- no lock, no Journey toggle, no re-derive on; the warning still shows', () => {
  const off = { editable: true, evergreenEnabled: false };
  const m = journeyMatrix({ ...off, row: CLASS_PURCHASE, evergreen: false });
  assert.deepEqual(vis(m), [false, true, false]);
  assert.deepEqual(locks(m), [false, false]);
  assert.deepEqual(vis(journeyMatrix({ ...off, row: CLASS_NONE, evergreen: false })), [false, true, false]);
  // A stored evergreen keeps the gate open (today's `|| form.evergreen`), operable.
  const stored = journeyMatrix({ ...off, row: CLASS_PURCHASE, evergreen: true });
  assert.equal(stored.showEvergreen, true);
  assert.equal(stored.lockEvergreen, false);
  assert.deepEqual(
    selectionChange({
      ...off, source: 'user', prevClass: SEL_PLAIN, nextClass: CLASS_PURCHASE, toggles: { evergreen: false, sendLater: true },
    }),
    { evergreen: false, sendLater: true },
  );
  assert.equal(journeyNotices(selOf(FIRST), false).broadcastToTrigger, true);
});

// I8 -- D2.2: not editable (finished/running/scheduled-not-editable, or started): stored state.
test('I8: not editable -- the matrix changes and locks nothing, and selectionChange flips nothing', () => {
  [ROW_EMPTY, ROW_MULTI, CLASS_NONE, CLASS_PURCHASE, CLASS_NO_PURCHASE].forEach((row) => {
    const m = journeyMatrix({
      row, editable: false, evergreenEnabled: true, evergreen: true, storedEvergreen: true,
    });
    assert.deepEqual(m, {
      apply: false,
      showEvergreen: true,
      showSendLater: true,
      showDelay: true,
      lockEvergreen: false,
      lockSendLater: false,
      forceEvergreenOff: false,
      zeroDelay: false,
      oneListHint: false,
    }, row);
    const p = journeyPayload(m, { evergreen: true, sendDelayDays: 7, sendDelaySecs: 7 * 86400 });
    assert.deepEqual(p, { evergreen: true, send_at: null, send_delay_secs: 7 * 86400 }, row);
    const r = selectionChange({
      source: 'user', editable: false, prevClass: SEL_PLAIN, nextClass: row, evergreenEnabled: true, toggles: { evergreen: false, sendLater: true },
    });
    assert.deepEqual(r, { evergreen: false, sendLater: true }, row);
  });
  // A paused-started evergreen on a no_purchase list keeps its delay field (rendered as stored).
  assert.equal(journeyMatrix({
    row: CLASS_NO_PURCHASE, editable: false, evergreenEnabled: true, evergreen: true,
  }).showDelay, true);
});

// IA9 -- A1.13: the trigger-list hard lock.
test('IA9: one purchase list -- Journey locked on, Send later hidden, delay editable', () => {
  const m = journeyMatrix({ ...ED, row: CLASS_PURCHASE, evergreen: true });
  assert.deepEqual(vis(m), [true, false, true]);
  assert.deepEqual(locks(m), [true, true]);
  assert.equal(m.zeroDelay, false);
  assert.equal(journeyPayload(m, { evergreen: true, sendDelayDays: 7 }).send_delay_secs, 7 * 86400);
  assert.equal(journeyNotices(selOf(FIRST), true).broadcastToTrigger, false);
});

test('IA9: one no_purchase list -- Journey locked on, Send later hidden, delay hidden and 0', () => {
  const m = journeyMatrix({ ...ED, row: CLASS_NO_PURCHASE, evergreen: true });
  assert.deepEqual(vis(m), [true, false, false]);
  assert.deepEqual(locks(m), [true, true]);
  assert.equal(m.zeroDelay, true);
  assert.equal(journeyPayload(m, { evergreen: true, sendDelayDays: 7, sendDelaySecs: 3600 }).send_delay_secs, 0);
});

test('IA9: a stored contradiction renders stored, disabled, with the warning -- never auto-flipped on open', () => {
  [CLASS_PURCHASE, CLASS_NO_PURCHASE].forEach((row) => {
    // An API-made broadcast on a trigger list: Journey shown off and disabled, the warning shown.
    const m = journeyMatrix({ ...ED, row, evergreen: false });
    assert.equal(m.showEvergreen, true, row);
    assert.equal(m.lockEvergreen, true, row);
    assert.equal(journeyPayload(m, { evergreen: false }).evergreen, false, row);
    const sel = row === CLASS_PURCHASE ? selOf(FIRST) : selOf(NP);
    assert.equal(journeyNotices(sel, false).broadcastToTrigger, true, row);
    // Opening it flips nothing.
    assert.deepEqual(selectionChange({
      ...ED, source: 'load', prevClass: SEL_PLAIN, nextClass: row, toggles: { evergreen: false, sendLater: true },
    }), { evergreen: false, sendLater: true }, row);
  });
});

test('IA9: a stored send_at on a locked row is shown disabled and round-trips -- never nulled', () => {
  const at = new Date('2026-11-01T10:00:00Z');
  [CLASS_PURCHASE, CLASS_NO_PURCHASE].forEach((row) => {
    const m = journeyMatrix({
      ...ED, row, evergreen: false, sendLater: true,
    });
    assert.equal(m.showSendLater, true, row);
    assert.equal(m.lockSendLater, true, row);
    assert.deepEqual(journeyPayload(m, { evergreen: false, sendLater: true, sendAt: at }), { evergreen: false, send_at: at, send_delay_secs: 0 }, row);
  });
});

// IA8 -- A1.12: the re-derive keys on the selection's CLASS; the shadow mechanism is gone.
test('IA8: selection classes -- empty and trigger-less selections are one class', () => {
  assert.equal(selectionClassKey([]), SEL_PLAIN);
  assert.equal(keyOf(PLAIN), SEL_PLAIN);
  assert.equal(keyOf(PLAIN, PLAIN2), SEL_PLAIN);
  assert.equal(keyOf(FIRST), CLASS_PURCHASE);
  assert.equal(keyOf(SKUS), CLASS_PURCHASE);
  assert.equal(keyOf(NP), CLASS_NO_PURCHASE);
  assert.equal(keyOf(FIRST, PLAIN), ROW_MULTI);
  assert.equal(keyOf(NP, SKUS), ROW_MULTI);
});

test('IA8: a class change re-derives the toggles from the new class', () => {
  const ch = (prev, next, toggles) => selectionChange({
    ...ED, source: 'user', prevClass: prev, nextClass: next, toggles,
  });
  const on = { evergreen: true, sendLater: false };
  const sched = { evergreen: false, sendLater: true };
  assert.deepEqual(ch(SEL_PLAIN, CLASS_PURCHASE, sched), on);
  assert.deepEqual(ch(SEL_PLAIN, CLASS_NO_PURCHASE, sched), on);
  assert.deepEqual(ch(CLASS_PURCHASE, CLASS_NO_PURCHASE, { evergreen: true, sendLater: false }), on);
  assert.deepEqual(ch(CLASS_PURCHASE, SEL_PLAIN, on), { evergreen: false, sendLater: false });
  assert.deepEqual(ch(CLASS_PURCHASE, ROW_MULTI, on), { evergreen: false, sendLater: false });
  assert.deepEqual(ch(SEL_PLAIN, ROW_MULTI, { evergreen: true, sendLater: false }), { evergreen: false, sendLater: false });
  // A new campaign's preselect re-derives like a user pick.
  assert.deepEqual(selectionChange({
    ...ED, source: 'preselect', prevClass: SEL_PLAIN, nextClass: CLASS_NO_PURCHASE, toggles: { evergreen: false, sendLater: false },
  }), on);
});

test('IA8: a same-class swap keeps the toggles and the schedule', () => {
  const ch = (prev, next, toggles) => selectionChange({
    ...ED, source: 'user', prevClass: prev, nextClass: next, toggles,
  });
  // A scheduled broadcast whose newsletter list is swapped (or a second plain list added).
  const sched = { evergreen: false, sendLater: true };
  assert.deepEqual(ch(keyOf(PLAIN), keyOf(PLAIN2), sched), sched);
  assert.deepEqual(ch(keyOf(PLAIN), keyOf(PLAIN, PLAIN2), sched), sched);
  // The Automations New button's evergreen survives picking a welcome (trigger-less) list.
  assert.deepEqual(ch(selectionClassKey([]), keyOf(PLAIN), { evergreen: true, sendLater: false }), { evergreen: true, sendLater: false });
  // One purchase list for another.
  assert.deepEqual(ch(keyOf(FIRST), keyOf(SKUS), { evergreen: true, sendLater: false }), { evergreen: true, sendLater: false });
});

test('IA8: a manual change sticks until the next class change; load never flips', () => {
  // On a trigger-less list the user switches Journey on by hand; adding a second plain list
  // (same class) keeps it; a trigger list (class change) re-derives.
  let t = { evergreen: true, sendLater: false };
  t = selectionChange({
    ...ED, source: 'user', prevClass: SEL_PLAIN, nextClass: SEL_PLAIN, toggles: t,
  });
  assert.deepEqual(t, { evergreen: true, sendLater: false });
  t = selectionChange({
    ...ED, source: 'user', prevClass: SEL_PLAIN, nextClass: ROW_MULTI, toggles: { evergreen: false, sendLater: true },
  });
  assert.deepEqual(t, { evergreen: false, sendLater: true });
  [CLASS_PURCHASE, CLASS_NO_PURCHASE, ROW_MULTI, SEL_PLAIN].forEach((next) => {
    assert.deepEqual(selectionChange({
      ...ED, source: 'load', prevClass: SEL_PLAIN, nextClass: next, toggles: { evergreen: false, sendLater: true },
    }), { evergreen: false, sendLater: true }, next);
  });
});

test('IA8: the shadow mechanism is gone', () => {
  assert.equal(ux.manualToggle, undefined);
  const src = readFileSync(new URL('./campaignListUx.mjs', import.meta.url), 'utf8');
  assert.equal(/shadow/i.test(src.replace(/supersedes D2\.1's shadow rule/, '')), false);
  const vue = readFileSync(new URL('./views/Campaign.vue', import.meta.url), 'utf8');
  assert.equal(/journeyShadow|manualToggle/.test(vue), false);
});

// I4 -- D3 notices.
test('I4: the no_purchase notice -- exactly one no_purchase list', () => {
  assert.equal(journeyNotices(selOf(NP), true).noPurchase, true);
  assert.equal(journeyNotices(selOf(NP), false).noPurchase, true);
  assert.equal(journeyNotices(selOf(NP, PLAIN), false).noPurchase, false);
  assert.equal(journeyNotices(selOf(FIRST), true).noPurchase, false);
  assert.equal(journeyNotices([], true).noPurchase, false);
});

test('I4: the broadcast-to-trigger warning -- evergreen off and a trigger list among several, or a contradiction on one', () => {
  assert.deepEqual(journeyNotices(selOf(PLAIN, FIRST), false), { noPurchase: false, broadcastToTrigger: true, triggerLists: ['Ruze first order'] });
  assert.deepEqual(journeyNotices(selOf(PLAIN, SKUS, NP), false).triggerLists, ['Widget buyers', 'Ruze still deciding']);
  assert.equal(journeyNotices(selOf(PLAIN, SKUS), false).broadcastToTrigger, true);
  assert.equal(journeyNotices(selOf(PLAIN, PLAIN2), false).broadcastToTrigger, false);
  assert.equal(journeyNotices(selOf(PLAIN), false).broadcastToTrigger, false);
  assert.equal(journeyNotices([], false).broadcastToTrigger, false);
  // A single trigger list under the lock submits evergreen on: no warning.
  assert.equal(journeyNotices(selOf(NP), true).broadcastToTrigger, false);
});

// I5 -- D4: the submitted delay is 0 whenever the matrix hides the delay under no_purchase.
test('I5: no_purchase delay is submitted as 0 regardless of an earlier edit', () => {
  const m = journeyMatrix({ ...ED, row: CLASS_NO_PURCHASE, evergreen: true });
  [0, 1, 7, 365].forEach((days) => {
    assert.equal(journeyPayload(m, { evergreen: true, sendDelayDays: days, sendDelaySecs: 3600 }).send_delay_secs, 0, String(days));
  });
  // Purchase and trigger-less rows keep the delay, the raw seconds preserved while untouched.
  const p = journeyMatrix({ ...ED, row: CLASS_PURCHASE, evergreen: true });
  assert.equal(journeyPayload(p, { evergreen: true, sendDelayDays: 3 }).send_delay_secs, 3 * 86400);
  assert.equal(journeyPayload(p, { evergreen: true, sendDelayDays: 0, sendDelaySecs: 3600 }).send_delay_secs, 3600);
  // Evergreen off: no delay.
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: CLASS_NONE, evergreen: false }), { evergreen: false, sendDelayDays: 5 }).send_delay_secs, 0);
  assert.equal(delaySecs(2, 2 * 86400 + 5), 2 * 86400 + 5);
  assert.equal(delaySecs(-4, 0), 0);
  assert.equal(delaySecs('x', 0), 0);
});

// I10 -- a hidden Send later submits send_at null.
test('I10: Send later hidden => send_at null; shown => the scheduled minute', () => {
  const at = new Date('2026-11-01T10:00:00Z');
  const f = { evergreen: false, sendLater: true, sendAt: at };
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: ROW_EMPTY, evergreen: false }), f).send_at, null);
  // A locked trigger row with the re-derived defaults (Send later off).
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: CLASS_PURCHASE, evergreen: true }), { ...f, evergreen: true, sendLater: false }).send_at, null);
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: CLASS_NO_PURCHASE, evergreen: true }), { ...f, evergreen: true, sendLater: false }).send_at, null);
  // F7: automations disabled, a purchase list's broadcast schedules.
  const f7 = { editable: true, evergreenEnabled: false };
  assert.equal(journeyPayload(journeyMatrix({ ...f7, row: CLASS_PURCHASE, evergreen: false }), f).send_at, at);
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: ROW_MULTI, evergreen: false }), f).send_at, at);
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: CLASS_NONE, evergreen: false }), { ...f, sendLater: false }).send_at, null);
});

// IA10 -- A1.14: the re-engagement queue note keys off the reengage: tag prefix.
test('IA10: the re-engagement note -- any selected list tagged reengage:', () => {
  assert.equal(hasReengageList(pick(QUEUE), OPTIONS), true);
  assert.equal(hasReengageList(pick(PLAIN, QUEUE), OPTIONS), true);
  assert.equal(hasReengageList(pick(PLAIN, PLAIN2), OPTIONS), false);
  assert.equal(hasReengageList([], OPTIONS), false);
  assert.equal(hasReengageList(null, null), false);
  // The tags are read from the option row, not the selected chip; an unresolvable id is not one.
  assert.equal(hasReengageList([{ id: 99, name: 'x', tags: ['reengage:1:2'] }], OPTIONS), false);
  // The prefix only: a free tag that merely mentions it does not count; the retired one is not it.
  [['re-engage'], ['my reengage:1'], ['repermission:4'], ['reengage'], [7]].forEach((tags) => {
    assert.equal(hasReengageList([{ id: 7 }], [{ id: 7, tags }]), false, JSON.stringify(tags));
  });
  assert.equal(hasReengageList([{ id: 7 }], [{ id: 7, tags: ['reengage:12:2'] }]), true);
});

// IA11 -- A1.9: the sender section renders only with a non-empty selection; the fallback From only
// then, and only without a brands row (unmapped, a derivation error, or the brands fetch failed).
test('IA11: sender section visibility', () => {
  assert.deepEqual(senderSection(0, true), { picker: false, fallback: false });
  assert.deepEqual(senderSection(0, false), { picker: false, fallback: false });
  assert.deepEqual(senderSection(1, true), { picker: true, fallback: false });
  assert.deepEqual(senderSection(2, false), { picker: false, fallback: true });
  assert.deepEqual(senderSection(undefined, true), { picker: false, fallback: false });
});

// IA3 -- A1.2: the add-membership pickers.
test('IA3: step lists are absent as a new addition; an existing membership stays', () => {
  assert.equal(isStepList(NP), true);
  assert.equal(isStepList(FIRST), false);
  assert.equal(isStepList({ join_trigger: { kind: 'no_purchase', days: 3 } }), true);
  const ids = (ls) => ls.map((l) => l.id);
  assert.deepEqual(ids(addableLists(OPTIONS)), [1, 2, 4, 5, 6]);
  assert.deepEqual(ids(addableLists(OPTIONS, [3, 4])), [1, 2, 3, 4, 5, 6]);
  assert.deepEqual(addableLists(null), []);
});

test('IA3: the Manage-lists picker is filtered only under action add, restored on change', () => {
  const ids = (ls) => ls.map((l) => l.id);
  assert.deepEqual(ids(manageListOptions(OPTIONS, 'add')), [1, 2, 4, 5, 6]);
  assert.deepEqual(ids(manageListOptions(OPTIONS, 'remove')), [1, 2, 3, 4, 5, 6]);
  assert.deepEqual(ids(manageListOptions(OPTIONS, 'unsubscribe')), [1, 2, 3, 4, 5, 6]);
  // Pruning a selection made under remove when the action changes to add.
  assert.deepEqual(ids(manageListOptions([NP, PLAIN], 'add')), [4]);
});

// Every i18n key the A1 bindings name exists in en.json.
test('A1.6: the amendment keys are in en.json', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  ['campaigns.evergreenDelayHelp', 'campaigns.evergreenTriggerLocked', 'campaigns.reengageQueueNote',
    'lists.noPurchaseManualAdd', 'lists.noPurchasePublic', 'lists.campaignTypeHelpBroadcast', 'lists.campaignTypeHelpJourney',
    'lists.joinTriggerFirstOrderSends', 'lists.joinTriggerSkusSends', 'lists.joinTriggerNoPurchaseSends',
    'lists.joinTriggerLatency', 'lists.joinTriggerNoPurchaseHelp', 'lists.tagsHelp'].forEach((k) => {
    assert.equal(typeof en[k], 'string', k);
  });
  assert.equal(en['lists.campaignTypeHelp'], undefined);
  assert.equal(/repermission/.test(en['lists.tagsHelp']), false);
  assert.equal(/hand-add|uses up/.test(en['lists.joinTriggerNoPurchaseHelp']), false);
});
