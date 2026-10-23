// Fork (list join triggers, integrations LIST-JOIN-TRIGGER-SPEC D7 / I10). Run: yarn test:list-trigger
import test from 'node:test';
import assert from 'node:assert/strict';

import {
  TRIGGER_NONE, TRIGGER_FIRST_ORDER, TRIGGER_SKUS, TRIGGER_NO_PURCHASE,
  triggerOption, skusToText, textToSkus, buildJoinTrigger, triggerBadge, storedDays, textToDays, daysValid,
  listTriggerItem, withTriggerItem, pickerLabel, buildListFormRequest, storedTrigger, storedTriggerSync,
  LIST_TYPE_BROADCAST, LIST_TYPE_JOURNEY, listTypeOf, journeyOption, buildListTypeTrigger,
  listTypeHelpKey, triggerHelpKeys,
} from './listTrigger.mjs'; // eslint-disable-line import/extensions

// JOURNEY-UX-SPEC IA4 -- the selector's help follows the value (A1.4); A1.3's bold lead + prose.
test('IA4: type help per selector value; the Sends lead and prose per trigger radio', () => {
  assert.equal(listTypeHelpKey(LIST_TYPE_BROADCAST), 'lists.campaignTypeHelpBroadcast');
  assert.equal(listTypeHelpKey(LIST_TYPE_JOURNEY), 'lists.campaignTypeHelpJourney');
  [undefined, null, '', 'nope'].forEach((v) => assert.equal(listTypeHelpKey(v), 'lists.campaignTypeHelpBroadcast', String(v)));
  assert.deepEqual(triggerHelpKeys(TRIGGER_FIRST_ORDER), { lead: 'lists.joinTriggerFirstOrderSends', body: 'lists.joinTriggerLatency' });
  assert.deepEqual(triggerHelpKeys(TRIGGER_SKUS), { lead: 'lists.joinTriggerSkusSends', body: 'lists.joinTriggerLatency' });
  assert.deepEqual(triggerHelpKeys(TRIGGER_NO_PURCHASE), { lead: 'lists.joinTriggerNoPurchaseSends', body: 'lists.joinTriggerNoPurchaseHelp' });
  [TRIGGER_NONE, undefined, 'nope'].forEach((v) => assert.equal(triggerHelpKeys(v), null, String(v)));
});

// JOURNEY-UX-SPEC I7 -- the Broadcast | Journey selector (D8, F4).
test('I7: stored trigger <-> selector value and the Journey pane radio', () => {
  [null, undefined, { kind: 'nope' }, []].forEach((jt) => {
    assert.equal(listTypeOf(jt), LIST_TYPE_BROADCAST, JSON.stringify(jt));
    assert.equal(journeyOption(jt), TRIGGER_FIRST_ORDER, JSON.stringify(jt));
  });
  [{ kind: 'first_order' }, { kind: 'skus', skus: ['a'] }, { kind: 'no_purchase', days: 7 }].forEach((jt) => {
    assert.equal(listTypeOf(jt), LIST_TYPE_JOURNEY, jt.kind);
    assert.equal(journeyOption(jt), jt.kind);
  });
  // The camel-cased browser row reads the same.
  assert.equal(listTypeOf(storedTrigger({ joinTrigger: { kind: 'no_purchase', days: 3 } })), LIST_TYPE_JOURNEY);
});

test('I7: Broadcast sends null; Journey builds today\'s documents and never null', () => {
  [TRIGGER_FIRST_ORDER, TRIGGER_SKUS, TRIGGER_NO_PURCHASE, TRIGGER_NONE, undefined].forEach((opt) => {
    assert.equal(buildListTypeTrigger(LIST_TYPE_BROADCAST, opt, 'a', '7'), null, String(opt));
  });
  const stored = [{ kind: 'first_order' }, { kind: 'skus', skus: ['widget-a', 'b-2'] }, { kind: 'no_purchase', days: 7 }];
  stored.forEach((jt) => {
    const got = buildListTypeTrigger(listTypeOf(jt), journeyOption(jt), skusToText(jt.skus), storedDays(jt));
    assert.deepEqual(got, jt, jt.kind);
    assert.deepEqual(got, buildJoinTrigger(triggerOption(jt), skusToText(jt.skus), storedDays(jt)), jt.kind);
  });
  // F4: Journey on a list with no stored trigger preselects first_order; no radio still emits a kind.
  assert.deepEqual(buildListTypeTrigger(LIST_TYPE_JOURNEY, journeyOption(null), '', ''), { kind: 'first_order' });
  [undefined, null, '', TRIGGER_NONE, 'nope'].forEach((opt) => {
    assert.deepEqual(buildListTypeTrigger(LIST_TYPE_JOURNEY, opt, '', ''), { kind: 'first_order' }, String(opt));
  });
  assert.deepEqual(buildListTypeTrigger(LIST_TYPE_JOURNEY, TRIGGER_NO_PURCHASE, '', ' 14 '), { kind: 'no_purchase', days: 14 });
});

test('option <-> document round-trip', () => {
  assert.equal(triggerOption(null), TRIGGER_NONE);
  assert.equal(triggerOption(undefined), TRIGGER_NONE);
  assert.equal(triggerOption({ kind: 'nope' }), TRIGGER_NONE);
  assert.equal(triggerOption([]), TRIGGER_NONE);
  assert.equal(triggerOption({ kind: 'first_order' }), TRIGGER_FIRST_ORDER);
  assert.equal(triggerOption({ kind: 'skus', skus: ['a'] }), TRIGGER_SKUS);

  assert.equal(buildJoinTrigger(TRIGGER_NONE, 'a'), null);
  assert.deepEqual(buildJoinTrigger(TRIGGER_FIRST_ORDER, 'ignored'), { kind: 'first_order' });
  const stored = { kind: 'skus', skus: ['widget-a', 'b-2'] };
  assert.deepEqual(buildJoinTrigger(triggerOption(stored), skusToText(stored.skus)), stored);
  assert.deepEqual(buildJoinTrigger(triggerOption({ kind: 'first_order' }), ''), { kind: 'first_order' });
});

// NO-PURCHASE-STEPS-SPEC I10 -- the no_purchase kind: option <-> document, the days field, the
// picker label and the sub-line item.
test('no_purchase: option <-> document round-trip and the days field', () => {
  const stored = { kind: 'no_purchase', days: 7 };
  assert.equal(triggerOption(stored), TRIGGER_NO_PURCHASE);
  assert.equal(storedDays(stored), '7');
  assert.deepEqual(buildJoinTrigger(triggerOption(stored), '', storedDays(stored)), stored);
  assert.deepEqual(buildJoinTrigger(TRIGGER_NO_PURCHASE, 'ignored', ' 14 '), { kind: 'no_purchase', days: 14 });
  // The days field starts empty for every other kind; the other kinds ignore it.
  assert.equal(storedDays({ kind: 'first_order' }), '');
  assert.equal(storedDays(null), '');
  assert.equal(storedDays({ kind: 'no_purchase', days: 'x' }), '');
  assert.deepEqual(buildJoinTrigger(TRIGGER_FIRST_ORDER, '', '7'), { kind: 'first_order' });
  // Non-digits send null (the server refuses -- it is the control); digits pass whatever the range.
  ['', ' ', '7.5', '-3', 'seven', null, undefined, '1e2'].forEach((bad) => assert.equal(textToDays(bad), null, String(bad)));
  assert.equal(textToDays('366'), 366);
  assert.deepEqual(buildJoinTrigger(TRIGGER_NO_PURCHASE, '', ''), { kind: 'no_purchase', days: null });
  // The inline range mirror: 1 and 365 are the limits.
  assert.equal(daysValid('1'), true);
  assert.equal(daysValid('365'), true);
  ['0', '366', '', '2.5'].forEach((bad) => assert.equal(daysValid(bad), false, bad));
});

test('no_purchase: picker label and sub-line item', () => {
  const l = { name: 'Ruze Still Deciding', join_trigger: { kind: 'no_purchase', days: 7 } };
  assert.equal(pickerLabel(l), 'Ruze Still Deciding — joins when still deciding (day 7)');
  assert.equal(listTriggerItem(l), 'joins on day 7 no-purchase · pending sync');
  const live = { ...l, trigger_sync: { state: 'live', slug: 'ruze-no-purchase-56' } };
  assert.equal(listTriggerItem(live), 'joins on day 7 no-purchase · live');
  assert.equal(withTriggerItem('#56 · Ruze', live), '#56 · Ruze · joins on day 7 no-purchase · live');
  // The camel-cased browser row reads the same.
  const camel = { name: 'Step 2', joinTrigger: { kind: 'no_purchase', days: 14 }, triggerSync: { state: 'invalid', error: 'x' } };
  assert.equal(pickerLabel(camel), 'Step 2 — joins when still deciding (day 14)');
  assert.equal(listTriggerItem(camel), 'joins on day 14 no-purchase · invalid');
  assert.deepEqual(triggerBadge(storedTrigger(camel), storedTriggerSync(camel)), { state: 'invalid', key: 'lists.triggerSyncInvalid', params: { error: 'x' } });
});

test('SKU textarea <-> array: one per line, trimmed, blank lines dropped', () => {
  assert.equal(skusToText(['a', 'b']), 'a\nb');
  assert.equal(skusToText(undefined), '');
  assert.deepEqual(textToSkus('  A-1 \n\n b\r\n   \nc'), ['A-1', 'b', 'c']);
  assert.deepEqual(textToSkus(''), []);
  assert.deepEqual(textToSkus(null), []);
});

test('badge per trigger_sync state', () => {
  const ft = { kind: 'first_order' };
  assert.equal(triggerBadge(null, { state: 'live', slug: 'x', synced_at: 't' }), null);
  assert.deepEqual(triggerBadge(ft, null), { state: 'pending', key: 'lists.triggerSyncPending', params: {} });
  const live = triggerBadge(ft, { state: 'live', slug: 'acme-first-order', synced_at: '2026-10-14T05:05:12Z' });
  assert.deepEqual(live, { state: 'live', key: 'lists.triggerSyncLive', params: { time: '2026-10-14T05:05:12Z' } });
  const err = 'acme already has a first-order trigger on list 7';
  const invalid = triggerBadge(ft, { state: 'invalid', error: err, synced_at: 't' });
  assert.deepEqual(invalid, { state: 'invalid', key: 'lists.triggerSyncInvalid', params: { error: err } });
});

test('sub-line item: the kind and the state word, never the error text', () => {
  assert.equal(listTriggerItem({}), '');
  assert.equal(listTriggerItem(null), '');
  assert.equal(listTriggerItem({ join_trigger: { kind: 'first_order' } }), 'joins on first order · pending sync');
  assert.equal(listTriggerItem({ join_trigger: { kind: 'skus', skus: ['a'] }, trigger_sync: { state: 'live', slug: 's' } }), 'joins on SKU purchase · live');
  const inv = listTriggerItem({ join_trigger: { kind: 'first_order' }, trigger_sync: { state: 'invalid', error: 'secret detail' } });
  assert.equal(inv, 'joins on first order · invalid');
  assert.equal(inv.includes('secret'), false);
  assert.equal(withTriggerItem('#7 · Acme', { join_trigger: { kind: 'first_order' }, trigger_sync: { state: 'live' } }), '#7 · Acme · joins on first order · live');
  assert.equal(withTriggerItem('#5', { name: 'x' }), '#5');
});

// The API client camel-cases every response (utils.camelKeys, deep), so a browser row carries
// joinTrigger / triggerSync / syncedAt. Every reader accepts both spellings (found 2026-10-19:
// the form preset to None for a live first-order list).
test('readers accept the camel-cased row the browser actually holds', () => {
  const row = { name: 'Acme first order', joinTrigger: { kind: 'first_order' }, triggerSync: { state: 'live', syncedAt: '2026-10-17T06:10:38Z' } };
  assert.deepEqual(storedTrigger(row), { kind: 'first_order' });
  assert.deepEqual(storedTriggerSync(row), { state: 'live', syncedAt: '2026-10-17T06:10:38Z' });
  assert.equal(triggerOption(storedTrigger(row)), TRIGGER_FIRST_ORDER);
  assert.equal(triggerBadge(storedTrigger(row), storedTriggerSync(row)).params.time, '2026-10-17T06:10:38Z');
  assert.equal(listTriggerItem(row), 'joins on first order · live');
  assert.equal(pickerLabel(row), 'Acme first order — joins on first purchase');
  // The wire spelling still wins when both are present, and an absent trigger is none either way.
  assert.deepEqual(storedTrigger({ join_trigger: null, joinTrigger: { kind: 'skus' } }), null);
  assert.equal(storedTrigger({}), undefined);
  assert.equal(listTriggerItem({ joinTrigger: null }), '');
});

test('picker label', () => {
  assert.equal(pickerLabel({ name: 'Acme first order', join_trigger: { kind: 'first_order' } }), 'Acme first order — joins on first purchase');
  assert.equal(pickerLabel({ name: 'Widget buyers', join_trigger: { kind: 'skus', skus: ['a'] } }), 'Widget buyers — joins on SKU purchase');
  assert.equal(pickerLabel({ name: 'Acme' }), 'Acme');
});

test('the request built from a FULL GET row carries only the allow-listed keys (F3)', () => {
  const row = {
    id: 7,
    uuid: 'u',
    name: 'Acme first order',
    type: 'private',
    optin: 'single',
    status: 'active',
    tags: ['x', 'brand:acme', 'from:Acme <hello@acme.test>'],
    description: 'd',
    brand: 'acme',
    subscriber_count: 3,
    subscriber_statuses: {},
    subscriber_grid: { all: {} },
    health: { status: 'ok' },
    health_tag: 'acme',
    sunset: { oldestDays: 1, asOf: 't' },
    join_trigger: { kind: 'skus', skus: ['old'] },
    transactional: true,
    trigger_set_by: 'someone',
    trigger_set_at: 't',
    trigger_sync: { state: 'live', slug: 's', synced_at: 't' },
    created_at: 't',
    updated_at: 't',
  };
  // Edit, no permission: join_trigger from the section, no transactional.
  const req = buildListFormRequest(row, 'acme', { joinTrigger: { kind: 'first_order' }, canTransactional: false, transactional: true });
  assert.deepEqual(Object.keys(req).sort(), ['brand', 'description', 'join_trigger', 'name', 'optin', 'status', 'tags', 'type']);
  assert.deepEqual(req.tags, ['x']);
  assert.deepEqual(req.join_trigger, { kind: 'first_order' });
  assert.equal(req.brand, 'acme');

  // Edit, with the permission: transactional rides along.
  const adminReq = buildListFormRequest(row, 'acme', { joinTrigger: null, canTransactional: true, transactional: false });
  assert.equal(adminReq.transactional, false);
  assert.equal(adminReq.join_trigger, null);
  assert.equal(Object.keys(adminReq).length, 9);

  // Create (no section): neither trigger field.
  const fresh = {
    name: 'New', type: 'private', optin: 'single', status: 'active', tags: [],
  };
  const createReq = buildListFormRequest(fresh, 'acme');
  assert.deepEqual(Object.keys(createReq).sort(), ['brand', 'name', 'optin', 'status', 'tags', 'type']);

  // The brand is still required.
  assert.throws(() => buildListFormRequest(row, ''), /brand is required/);
});
