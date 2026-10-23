// Fork (journey form UX, integrations JOURNEY-UX-SPEC I1-I5, I8, I10). Run: yarn test:campaign-list-ux
import test from 'node:test';
import assert from 'node:assert/strict';

import {
  CLASS_NONE, CLASS_PURCHASE, CLASS_NO_PURCHASE, ROW_EMPTY, ROW_MULTI,
  listJourneyClass, selectionClasses, selectionRow, journeyMatrix, selectionChange, manualToggle,
  journeyNotices, journeyPayload, delaySecs,
} from './campaignListUx.mjs'; // eslint-disable-line import/extensions

const FIRST = { id: 1, name: 'Ruze first order', join_trigger: { kind: 'first_order' } };
const SKUS = { id: 2, name: 'Widget buyers', joinTrigger: { kind: 'skus', skus: ['a'] } };
const NP = { id: 3, name: 'Ruze still deciding', joinTrigger: { kind: 'no_purchase', days: 7 } };
const PLAIN = { id: 4, name: 'Ruze newsletter' };
const OPTIONS = [FIRST, SKUS, NP, PLAIN];
const pick = (...rows) => rows.map((r) => ({ id: r.id, name: r.name }));
const rowOf = (...rows) => selectionRow(selectionClasses(pick(...rows), OPTIONS));

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

// I2 -- the D2 matrix, row by row.
test('I2: no lists -- every journey control hidden', () => {
  const m = journeyMatrix({ ...ED, row: ROW_EMPTY, evergreen: true });
  assert.deepEqual(vis(m), [false, false, false]);
  assert.equal(m.forceEvergreenOff, false);
});

test('I2: >= 2 lists -- Journey hidden and submitted false, Send later shown, delay hidden; the one-list hint', () => {
  const m = journeyMatrix({ ...ED, row: ROW_MULTI, evergreen: false });
  assert.deepEqual(vis(m), [false, true, false]);
  assert.equal(m.forceEvergreenOff, true);
  assert.equal(m.oneListHint, false);
  // A stored evergreen (or the pre-flip one) turned off by the second list shows the hint.
  assert.equal(journeyMatrix({
    ...ED, row: ROW_MULTI, evergreen: false, storedEvergreen: true,
  }).oneListHint, true);
  assert.equal(journeyMatrix({
    ...ED, row: ROW_MULTI, evergreen: false, shadow: { evergreen: true, sendLater: false },
  }).oneListHint, true);
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: ROW_MULTI, evergreen: true }), { evergreen: true, sendDelayDays: 3 }).evergreen, false);
});

test('I2: one trigger-less list -- all shown, delay iff evergreen', () => {
  assert.deepEqual(vis(journeyMatrix({ ...ED, row: CLASS_NONE, evergreen: false })), [true, true, false]);
  assert.deepEqual(vis(journeyMatrix({ ...ED, row: CLASS_NONE, evergreen: true })), [true, true, true]);
});

test('I2: one purchase list -- Journey shown; Send later hidden while evergreen on; delay shown with evergreen', () => {
  assert.deepEqual(vis(journeyMatrix({ ...ED, row: CLASS_PURCHASE, evergreen: true })), [true, false, true]);
  // Turned off by the user: Send later reappears.
  assert.deepEqual(vis(journeyMatrix({ ...ED, row: CLASS_PURCHASE, evergreen: false })), [true, true, false]);
  assert.equal(journeyMatrix({ ...ED, row: CLASS_PURCHASE, evergreen: true }).zeroDelay, false);
});

test('I2: one no_purchase list -- delay hidden and zeroed while evergreen on', () => {
  const m = journeyMatrix({ ...ED, row: CLASS_NO_PURCHASE, evergreen: true });
  assert.deepEqual(vis(m), [true, false, false]);
  assert.equal(m.zeroDelay, true);
  const off = journeyMatrix({ ...ED, row: CLASS_NO_PURCHASE, evergreen: false });
  assert.deepEqual(vis(off), [true, true, false]);
});

test('I2/F6: an unresolvable list id renders the trigger-less row', () => {
  const row = selectionRow(selectionClasses([{ id: 99, name: 'Gone' }], OPTIONS));
  assert.equal(row, CLASS_NONE);
  assert.deepEqual(vis(journeyMatrix({ ...ED, row, evergreen: false })), [true, true, false]);
  const r = selectionChange({
    ...ED, source: 'user', row, toggles: { evergreen: false, sendLater: false },
  });
  assert.deepEqual(r, { toggles: { evergreen: false, sendLater: false }, shadow: null });
  assert.deepEqual(
    journeyNotices(selectionClasses([{ id: 99, name: 'Gone' }], OPTIONS), false),
    { noPurchase: false, broadcastToTrigger: false, triggerLists: [] },
  );
});

test('I2/F7: automations disabled -- no Journey toggle, no auto-enable; the warning still shows', () => {
  const off = { editable: true, evergreenEnabled: false };
  assert.deepEqual(vis(journeyMatrix({ ...off, row: CLASS_PURCHASE, evergreen: false })), [false, true, false]);
  assert.deepEqual(vis(journeyMatrix({ ...off, row: CLASS_NONE, evergreen: false })), [false, true, false]);
  // A stored evergreen keeps the gate open (today's `|| form.evergreen`).
  assert.equal(journeyMatrix({ ...off, row: CLASS_PURCHASE, evergreen: true }).showEvergreen, true);
  const r = selectionChange({
    ...off, source: 'user', row: CLASS_PURCHASE, toggles: { evergreen: false, sendLater: true },
  });
  assert.deepEqual(r, { toggles: { evergreen: false, sendLater: true }, shadow: null });
  assert.equal(journeyNotices(selectionClasses(pick(FIRST), OPTIONS), false).broadcastToTrigger, true);
});

// I8 -- D2.2: not editable (finished/running/scheduled-not-editable, or started): stored state.
test('I8: not editable -- the matrix changes nothing and selectionChange flips nothing', () => {
  [ROW_EMPTY, ROW_MULTI, CLASS_NONE, CLASS_PURCHASE, CLASS_NO_PURCHASE].forEach((row) => {
    const m = journeyMatrix({
      row, editable: false, evergreenEnabled: true, evergreen: true, storedEvergreen: true,
    });
    assert.deepEqual(m, {
      apply: false, showEvergreen: true, showSendLater: true, showDelay: true, forceEvergreenOff: false, zeroDelay: false, oneListHint: false,
    }, row);
    const p = journeyPayload(m, { evergreen: true, sendDelayDays: 7, sendDelaySecs: 7 * 86400 });
    assert.deepEqual(p, { evergreen: true, send_at: null, send_delay_secs: 7 * 86400 }, row);
    const r = selectionChange({
      source: 'user', editable: false, row, evergreenEnabled: true, toggles: { evergreen: false, sendLater: true },
    });
    assert.deepEqual(r, { toggles: { evergreen: false, sendLater: true }, shadow: null }, row);
  });
  // A paused-started evergreen on a no_purchase list keeps its delay field (rendered as stored).
  assert.equal(journeyMatrix({
    row: CLASS_NO_PURCHASE, editable: false, evergreenEnabled: true, evergreen: true,
  }).showDelay, true);
});

// I3 -- D2.1 auto-enable.
test('I3: a trigger list chosen on a new campaign auto-enables; the default is off', () => {
  const base = { ...ED, source: 'user', toggles: { evergreen: false, sendLater: true } };
  assert.deepEqual(
    selectionChange({ ...base, row: CLASS_PURCHASE }),
    { toggles: { evergreen: true, sendLater: false }, shadow: { evergreen: false, sendLater: true } },
  );
  assert.deepEqual(
    selectionChange({ ...base, row: CLASS_NO_PURCHASE, source: 'preselect' }),
    { toggles: { evergreen: true, sendLater: false }, shadow: { evergreen: false, sendLater: true } },
  );
  // A trigger-less list changes nothing.
  assert.deepEqual(
    selectionChange({ ...base, row: CLASS_NONE }),
    { toggles: { evergreen: false, sendLater: true }, shadow: null },
  );
});

test('I3: opening a stored campaign never flips anything', () => {
  [CLASS_PURCHASE, CLASS_NO_PURCHASE, ROW_MULTI].forEach((row) => {
    const r = selectionChange({
      ...ED, source: 'load', row, toggles: { evergreen: false, sendLater: true },
    });
    assert.deepEqual(r, { toggles: { evergreen: false, sendLater: true }, shadow: null }, row);
  });
});

test('I3: the override survives; a manual toggle drops the shadow', () => {
  let s = selectionChange({
    ...ED, source: 'user', row: CLASS_PURCHASE, toggles: { evergreen: false, sendLater: false },
  });
  assert.equal(s.toggles.evergreen, true);
  // The user turns it back off: the matrix does not re-force it, and the D3 warning shows.
  s = manualToggle({ toggles: s.toggles, key: 'evergreen', value: false });
  assert.deepEqual(s, { toggles: { evergreen: false, sendLater: false }, shadow: null });
  const m = journeyMatrix({ ...ED, row: CLASS_PURCHASE, evergreen: s.toggles.evergreen });
  assert.equal(journeyPayload(m, { ...s.toggles }).evergreen, false);
  assert.equal(journeyNotices(selectionClasses(pick(FIRST), OPTIONS), false).broadcastToTrigger, true);
  // A later undo to a trigger-less list keeps the override (no shadow to restore).
  s = manualToggle({ toggles: s.toggles, key: 'sendLater', value: true });
  assert.deepEqual(
    selectionChange({
      ...ED, source: 'user', row: CLASS_NONE, toggles: s.toggles, shadow: s.shadow,
    }),
    { toggles: { evergreen: false, sendLater: true }, shadow: null },
  );
});

test('I3: shadows restored when the selection change is undone; zero lists restores too', () => {
  const before = { evergreen: false, sendLater: true };
  let s = selectionChange({
    ...ED, source: 'user', row: CLASS_NO_PURCHASE, toggles: before,
  });
  // A second list: evergreen off, the original shadow kept.
  s = selectionChange({
    ...ED, source: 'user', row: ROW_MULTI, toggles: s.toggles, shadow: s.shadow,
  });
  assert.deepEqual(s, { toggles: { evergreen: false, sendLater: false }, shadow: before });
  // Back to one trigger list: re-enabled, same shadow.
  s = selectionChange({
    ...ED, source: 'user', row: CLASS_PURCHASE, toggles: s.toggles, shadow: s.shadow,
  });
  assert.deepEqual(s, { toggles: { evergreen: true, sendLater: false }, shadow: before });
  // Swapped for a trigger-less list: restored.
  assert.deepEqual(
    selectionChange({
      ...ED, source: 'user', row: CLASS_NONE, toggles: s.toggles, shadow: s.shadow,
    }),
    { toggles: before, shadow: null },
  );
  // Deselected to zero lists: restored.
  assert.deepEqual(
    selectionChange({
      ...ED, source: 'user', row: ROW_EMPTY, toggles: s.toggles, shadow: s.shadow,
    }),
    { toggles: before, shadow: null },
  );
});

test('I3: >= 2 lists from a stored or preset evergreen flips it off with a shadow', () => {
  const r = selectionChange({
    ...ED, source: 'user', row: ROW_MULTI, toggles: { evergreen: true, sendLater: false },
  });
  assert.deepEqual(r, { toggles: { evergreen: false, sendLater: false }, shadow: { evergreen: true, sendLater: false } });
  // Nothing to change: no shadow is minted.
  assert.deepEqual(
    selectionChange({
      ...ED, source: 'user', row: ROW_MULTI, toggles: { evergreen: false, sendLater: true },
    }),
    { toggles: { evergreen: false, sendLater: true }, shadow: null },
  );
  assert.deepEqual(
    selectionChange({
      ...ED, source: 'user', row: CLASS_PURCHASE, toggles: { evergreen: true, sendLater: false },
    }),
    { toggles: { evergreen: true, sendLater: false }, shadow: null },
  );
});

// I4 -- D3 notices.
test('I4: the no_purchase notice -- exactly one no_purchase list', () => {
  const sel = (...rows) => selectionClasses(pick(...rows), OPTIONS);
  assert.equal(journeyNotices(sel(NP), true).noPurchase, true);
  assert.equal(journeyNotices(sel(NP), false).noPurchase, true);
  assert.equal(journeyNotices(sel(NP, PLAIN), false).noPurchase, false);
  assert.equal(journeyNotices(sel(FIRST), true).noPurchase, false);
  assert.equal(journeyNotices([], true).noPurchase, false);
});

test('I4: the broadcast-to-trigger warning -- evergreen off and any trigger list, any count', () => {
  const sel = (...rows) => selectionClasses(pick(...rows), OPTIONS);
  assert.deepEqual(journeyNotices(sel(FIRST), false), { noPurchase: false, broadcastToTrigger: true, triggerLists: ['Ruze first order'] });
  assert.equal(journeyNotices(sel(FIRST), true).broadcastToTrigger, false);
  assert.equal(journeyNotices(sel(NP), false).broadcastToTrigger, true);
  assert.deepEqual(journeyNotices(sel(PLAIN, SKUS, NP), false).triggerLists, ['Widget buyers', 'Ruze still deciding']);
  assert.equal(journeyNotices(sel(PLAIN, SKUS), false).broadcastToTrigger, true);
  assert.equal(journeyNotices(sel(PLAIN), false).broadcastToTrigger, false);
  assert.equal(journeyNotices([], false).broadcastToTrigger, false);
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
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: CLASS_PURCHASE, evergreen: true }), { ...f, evergreen: true }).send_at, null);
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: CLASS_NO_PURCHASE, evergreen: true }), { ...f, evergreen: true }).send_at, null);
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: CLASS_PURCHASE, evergreen: false }), f).send_at, at);
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: ROW_MULTI, evergreen: false }), f).send_at, at);
  assert.equal(journeyPayload(journeyMatrix({ ...ED, row: CLASS_NONE, evergreen: false }), { ...f, sendLater: false }).send_at, null);
});
