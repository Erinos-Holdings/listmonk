// Fork (list join triggers, integrations LIST-JOIN-TRIGGER-SPEC D7 / I10). Run: yarn test:list-trigger
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import {
  TRIGGER_NONE, TRIGGER_FIRST_ORDER, TRIGGER_SKUS, TRIGGER_NO_PURCHASE, TRIGGER_POST_PURCHASE,
  triggerOption, skusToText, textToSkus, buildJoinTrigger, triggerBadge, storedDays, textToDays, daysValid,
  daysChoices, listTriggerItem, withTriggerItem, pickerLabel, buildListFormRequest, storedTrigger, storedTriggerSync,
  LIST_TYPE_BROADCAST, LIST_TYPE_JOURNEY, listTypeOf, journeyOption, buildListTypeTrigger,
  listTypeHelpKey, triggerHelpKeys, DAYS_PRESETS, POST_PURCHASE_DAYS_PRESETS, daysRadios, seriesStepKey,
  SERIES_WELCOME, SERIES_POST_PURCHASE, WELCOME_STEP_DAYS, POST_PURCHASE_STEP_DAYS,
  SERIES_REENGAGEMENT, REENGAGEMENT_DAYS, STEP_TRIGGERS, seriesOfTrigger,
  journeySaveBlocked, STANDARD_TRIGGERS, isStandardTrigger, journeyOptionDisabled, journeyLockedOf, formReadOnly,
  activeSwitchState, triggerChangeTurnsOff, activeWordOf, systemHiddenOf,
} from './listTrigger.mjs'; // eslint-disable-line import/extensions

// CAMPAIGN-LISTS-SPEC I10 -- the sub-line as the Lists page renders it: the item's text, then
// ' · ' and the Active switch's word (activeWordOf). The asserted strings below are the pre-D8
// sub-lines, unchanged: the split moved the word into a colored span, nothing else.
const itemText = (item) => (item ? `${item.text} · ${activeWordOf(item.active).word}` : '');
const sublineText = (s) => (s.active === null ? s.text : `${s.text} · ${activeWordOf(s.active).word}`);

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
    // JOURNEY-LISTS-SPEC D12 / I13 -- no radio is preselected on an untriggered list.
    assert.equal(journeyOption(jt), '', JSON.stringify(jt));
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
  // JOURNEY-LISTS-SPEC D12 (replacing JOURNEY-UX F4's first_order default): Journey on a list with
  // no stored trigger preselects nothing, and a Journey save with no radio chosen is refused --
  // never a silent first_order.
  assert.throws(() => buildListTypeTrigger(LIST_TYPE_JOURNEY, journeyOption(null), '', ''), /lists\.journeyPickOption/);
  [undefined, null, '', TRIGGER_NONE, 'nope'].forEach((opt) => {
    assert.throws(() => buildListTypeTrigger(LIST_TYPE_JOURNEY, opt, '', ''), /lists\.journeyPickOption/, String(opt));
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

// LIFECYCLE-SERIES-SPEC I14 (rewriting the 3/7/15 pin): the No-purchase series step days are 2/5/10;
// a stored value outside them (Ruze's grandfathered 15, D2) stays a choice.
// USAGE-DAY10-SPEC I4 -- day 10 retired: the presets are 2/5 and a stored 10 is a legacy radio.
test('no_purchase: the days radios are 2/5 plus a stored value outside them', () => {
  assert.deepEqual(WELCOME_STEP_DAYS, [2, 5]);
  assert.deepEqual(DAYS_PRESETS, [2, 5]);
  assert.deepEqual(daysChoices(''), [2, 5]);
  assert.deepEqual(daysChoices('5'), [2, 5]);
  assert.deepEqual(daysChoices('10'), [2, 5, 10]);
  assert.deepEqual(daysChoices('15'), [2, 5, 15]);
  assert.deepEqual(daysChoices('30'), [2, 5, 30]);
  assert.deepEqual(daysRadios(TRIGGER_NO_PURCHASE, '10'), [
    { days: 2, legacy: false, help: 'series.welcomeDay2' },
    { days: 5, legacy: false, help: 'series.welcomeDay5' },
    { days: 10, legacy: true, help: 'series.welcomeLegacy' },
  ]);
  assert.equal(seriesStepKey(SERIES_WELCOME, 10), 'series.welcomeLegacy');
});

// LIFECYCLE-SERIES-SPEC I14 -- post_purchase days are 5/14/30; the legacy label and hover.
test('I14: post_purchase presets 5/14/30; a stored off-pattern value is a legacy radio', () => {
  assert.deepEqual(POST_PURCHASE_DAYS_PRESETS, [5, 14, 30]);
  assert.deepEqual(daysChoices('', TRIGGER_POST_PURCHASE), [5, 14, 30]);
  assert.deepEqual(daysChoices('7', TRIGGER_POST_PURCHASE), [5, 7, 14, 30]);
  assert.deepEqual(daysRadios(TRIGGER_NO_PURCHASE, '15'), [
    { days: 2, legacy: false, help: 'series.welcomeDay2' },
    { days: 5, legacy: false, help: 'series.welcomeDay5' },
    { days: 15, legacy: true, help: 'series.welcomeLegacy' },
  ]);
  assert.deepEqual(daysRadios(TRIGGER_POST_PURCHASE, ''), [
    { days: 5, legacy: false, help: 'series.postPurchaseDay5' },
    { days: 14, legacy: false, help: 'series.postPurchaseDay14' },
    { days: 30, legacy: false, help: 'series.postPurchaseDay30' },
  ]);
  // The document round-trips like no_purchase's.
  const stored = { kind: 'post_purchase', days: 14 };
  assert.equal(triggerOption(stored), TRIGGER_POST_PURCHASE);
  assert.equal(storedDays(stored), '14');
  assert.deepEqual(buildJoinTrigger(triggerOption(stored), '', storedDays(stored)), stored);
  assert.deepEqual(buildListTypeTrigger(LIST_TYPE_JOURNEY, TRIGGER_POST_PURCHASE, '', '30'), { kind: 'post_purchase', days: 30 });
  assert.equal(listTypeOf(stored), LIST_TYPE_JOURNEY);
  assert.equal(journeyOption(stored), TRIGGER_POST_PURCHASE);
  assert.deepEqual(triggerHelpKeys(TRIGGER_POST_PURCHASE), { lead: 'lists.joinTriggerNoPurchaseSends', body: 'lists.joinTriggerPostPurchaseHelp' });
});

// LIFECYCLE-SERIES-SPEC I16 -- every step radio and series leaf resolves a D5 description key that
// exists in en.json (one home each), and the labels the form binds exist too.
test('I16: every series description key exists in en.json', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  const keys = [
    ...[0, ...WELCOME_STEP_DAYS].map((d) => seriesStepKey(SERIES_WELCOME, d)),
    ...[0, ...POST_PURCHASE_STEP_DAYS].map((d) => seriesStepKey(SERIES_POST_PURCHASE, d)),
    seriesStepKey(SERIES_WELCOME, 15), seriesStepKey(SERIES_POST_PURCHASE, 7),
    ...daysRadios(TRIGGER_NO_PURCHASE, '15').map((r) => r.help),
    ...daysRadios(TRIGGER_POST_PURCHASE, '').map((r) => r.help),
    'lists.joinTriggerPostPurchase', 'lists.joinTriggerPostPurchaseHelp', 'lists.joinTriggerDaysLegacy',
    'lists.joinTriggerDaysPostPurchase',
  ];
  keys.forEach((k) => assert.equal(typeof en[k], 'string', k));
  assert.equal(new Set(keys.slice(0, 7)).size, 7); // seven distinct step texts (USAGE-DAY10-SPEC: day 10 retired)
  // USAGE-DAY10-SPEC I6 -- the retired day-10 text is gone; the legacy text names 2, 5.
  assert.equal(en['series.welcomeDay10'], undefined);
  assert.match(en['series.welcomeLegacy'], /\(2, 5 days\)/);
  assert.equal(seriesStepKey('nope', 5), null);
  // Stage 4 F2 -- each family's legacy text names its own standard.
  assert.equal(seriesStepKey(SERIES_WELCOME, 15), 'series.welcomeLegacy');
  assert.equal(seriesStepKey(SERIES_POST_PURCHASE, 7), 'series.postPurchaseLegacy');
  assert.match(en['series.postPurchaseLegacy'], /5, 14, 30/);
  // S10's pinned label values.
  assert.equal(en['campaigns.evergreen'], 'Automation');
  assert.equal(en['lists.joinTrigger'], 'Entry trigger');
  assert.equal(en['lists.joinTriggerNoPurchase'], 'No-purchase series step — days after joining the newsletter …');
  // Integrations DOC-ALIGNMENT-SPEC D5: the entry condition and the delivery clock are stated.
  assert.equal(
    en['lists.joinTriggerFirstOrder'],
    'Post-purchase series day 0 — a customer\'s first order of this brand, if subscribed to the brand\'s newsletter at the time (a transactional list is exempt)',
  );
  assert.equal(en['lists.joinTriggerPostPurchase'], 'Post-purchase series step — timed from delivery …');
  // No en.json value shows a user the retired terms.
  Object.entries(en).forEach(([k, v]) => assert.equal(/still deciding|no-purchase nudge|journey automation/i.test(v), false, k));
  // LIFECYCLE-SERIES §10 A1 -- on screen the first series is the No-purchase series; the journey
  // guide's heading is the ONE value allowed the industry name.
  assert.deepEqual(Object.keys(en).filter((k) => /welcome series/i.test(en[k])), ['dashboard.usage.guideWelcome']);
});

// USAGE-REENGAGEMENT-SPEC D6 / SUNSET-V4 D7 -- seriesStepKey extended with the reengagement family:
// its four standard days resolve to keys that exist; an off-standard day has no text (null, no legacy).
test('reengagement family: seriesStepKey on REENGAGEMENT_DAYS only', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.deepEqual([...REENGAGEMENT_DAYS], [180, 270, 360, 365]);
  REENGAGEMENT_DAYS.forEach((d) => {
    const k = seriesStepKey(SERIES_REENGAGEMENT, d);
    assert.equal(k, `series.reengagementDay${d}`);
    assert.equal(typeof en[k], 'string', k);
  });
  assert.equal(seriesStepKey(SERIES_REENGAGEMENT, 0), null);
  assert.equal(seriesStepKey(SERIES_REENGAGEMENT, 120), null);
  // Never a step-trigger family.
  assert.equal(STEP_TRIGGERS.map(seriesOfTrigger).includes(SERIES_REENGAGEMENT), false);
});

test('no_purchase: picker label and sub-line item (LIFECYCLE-SERIES-SPEC S10 phrases)', () => {
  const l = { name: 'Ruze Still Deciding', join_trigger: { kind: 'no_purchase', days: 7 } };
  assert.equal(pickerLabel(l), 'Ruze Still Deciding — No-purchase series day 7');
  assert.equal(itemText(listTriggerItem(l)), 'No-purchase series day 7 · pending sync · Inactive');
  const live = { ...l, trigger_sync: { state: 'live', slug: 'ruze-no-purchase-56' }, trigger_active: true };
  assert.equal(itemText(listTriggerItem(live)), 'No-purchase series day 7 · live · Active');
  assert.equal(sublineText(withTriggerItem('#56 · Ruze', live)), '#56 · Ruze · No-purchase series day 7 · live · Active');
  const pp = { name: 'Ruze day 14', join_trigger: { kind: 'post_purchase', days: 14 } };
  assert.equal(pickerLabel(pp), 'Ruze day 14 — Post-purchase series day 14');
  assert.equal(itemText(listTriggerItem(pp)), 'Post-purchase series day 14 · pending sync · Inactive');
  // The camel-cased browser row reads the same.
  const camel = {
    name: 'Step 2', joinTrigger: { kind: 'no_purchase', days: 14 }, triggerSync: { state: 'invalid', error: 'x' }, triggerActive: true,
  };
  assert.equal(pickerLabel(camel), 'Step 2 — No-purchase series day 14');
  assert.equal(itemText(listTriggerItem(camel)), 'No-purchase series day 14 · invalid · Active');
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
  assert.equal(listTriggerItem({}), null);
  assert.equal(listTriggerItem(null), null);
  assert.equal(itemText(listTriggerItem({ join_trigger: { kind: 'first_order' } })), 'Post-purchase series day 0 · pending sync · Inactive');
  assert.equal(itemText(listTriggerItem({ join_trigger: { kind: 'skus', skus: ['a'] }, trigger_sync: { state: 'live', slug: 's' }, trigger_active: true })), 'joins on SKU purchase · live · Active');
  const inv = itemText(listTriggerItem({ join_trigger: { kind: 'first_order' }, trigger_sync: { state: 'invalid', error: 'secret detail' } }));
  assert.equal(inv, 'Post-purchase series day 0 · invalid · Inactive');
  assert.equal(inv.includes('secret'), false);
  const activeFo = { join_trigger: { kind: 'first_order' }, trigger_sync: { state: 'live' }, trigger_active: true };
  assert.equal(sublineText(withTriggerItem('#7 · Acme', activeFo)), '#7 · Acme · Post-purchase series day 0 · live · Active');
  assert.deepEqual(withTriggerItem('#5', { name: 'x' }), { text: '#5', active: null });
});

// CAMPAIGN-LISTS-SPEC I10 (D8) -- the state word is split off the text: the item carries the
// switch's state, the text never holds the word, and the view colors it green Active / red Inactive.
test('I10: Active/Inactive split off the sub-line and colored', () => {
  const on = { join_trigger: { kind: 'first_order' }, trigger_sync: { state: 'live' }, trigger_active: true };
  const off = { joinTrigger: { kind: 'no_purchase', days: 2 } };
  assert.deepEqual(listTriggerItem(on), { text: 'Post-purchase series day 0 · live', active: true });
  assert.deepEqual(listTriggerItem(off), { text: 'No-purchase series day 2 · pending sync', active: false });
  assert.deepEqual(withTriggerItem('#7', on), { text: '#7 · Post-purchase series day 0 · live', active: true });
  assert.deepEqual(activeWordOf(true), { word: 'Active', cls: 'has-text-success' });
  assert.deepEqual(activeWordOf(false), { word: 'Inactive', cls: 'has-text-danger' });
});

// CAMPAIGN-LISTS-SPEC D6 / I6 -- the server's system_hidden, under either spelling, never derived.
test('systemHiddenOf reads the server flag under either spelling, never the tags', () => {
  assert.equal(systemHiddenOf({ system_hidden: true }), true);
  assert.equal(systemHiddenOf({ systemHidden: true }), true);
  assert.equal(systemHiddenOf({ systemHidden: false, journeyLocked: true }), false);
  assert.equal(systemHiddenOf({ tags: ['reengage:5:4'], joinTrigger: { kind: 'first_order' } }), false);
  assert.equal(systemHiddenOf({}), false);
  assert.equal(systemHiddenOf(null), false);
  assert.equal(systemHiddenOf({ system_hidden: 'true' }), false);
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
  assert.equal(itemText(listTriggerItem(row)), 'Post-purchase series day 0 · live · Inactive');
  assert.equal(itemText(listTriggerItem({ ...row, triggerActive: true })), 'Post-purchase series day 0 · live · Active');
  assert.equal(pickerLabel(row), 'Acme first order — Post-purchase series day 0');
  // The wire spelling still wins when both are present, and an absent trigger is none either way.
  assert.deepEqual(storedTrigger({ join_trigger: null, joinTrigger: { kind: 'skus' } }), null);
  assert.equal(storedTrigger({}), undefined);
  assert.equal(listTriggerItem({ joinTrigger: null }), null);
});

test('picker label', () => {
  assert.equal(pickerLabel({ name: 'Acme first order', join_trigger: { kind: 'first_order' } }), 'Acme first order — Post-purchase series day 0');
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

// JOURNEY-LISTS-SPEC D12 / I13 -- no preselected trigger; Save blocked until a radio is chosen; the
// standard radios disabled without lists:journeys; a locked list read-only without it.
test('I13: no preselected trigger, Save blocked, standard radios and locked lists by permission', () => {
  assert.equal(journeyOption(null), '');
  assert.equal(journeySaveBlocked(LIST_TYPE_JOURNEY, journeyOption(null)), true);
  assert.equal(journeySaveBlocked(LIST_TYPE_JOURNEY, TRIGGER_SKUS), false);
  assert.equal(journeySaveBlocked(LIST_TYPE_BROADCAST, ''), false);
  assert.equal(journeyOption({ kind: 'first_order' }), TRIGGER_FIRST_ORDER, 'a stored trigger opens selected');

  STANDARD_TRIGGERS.forEach((opt) => {
    assert.equal(isStandardTrigger(opt), true, opt);
    assert.equal(journeyOptionDisabled(opt, false), true, opt);
    assert.equal(journeyOptionDisabled(opt, true), false, opt);
  });
  assert.equal(journeyOptionDisabled(TRIGGER_SKUS, false), false, 'SKUs stay enabled');
  assert.deepEqual([...STANDARD_TRIGGERS].sort(), ['first_order', 'no_purchase', 'post_purchase']);

  // journey_locked is the server's field (never re-derived): read under either spelling.
  const locked = { journey_locked: true, join_trigger: { kind: 'skus', skus: ['a'] } };
  assert.equal(journeyLockedOf(locked), true);
  assert.equal(journeyLockedOf({ journeyLocked: true }), true);
  assert.equal(journeyLockedOf({ join_trigger: { kind: 'first_order' } }), false, 'the client never derives the lock');
  assert.equal(formReadOnly(locked, false), true);
  assert.equal(formReadOnly(locked, true), false);
  assert.equal(formReadOnly({ journey_locked: false }, false), false);
});

test('D12: the Active switch state', () => {
  const fo = { join_trigger: { kind: 'first_order' } };
  // No trigger, or no manage permission: hidden.
  assert.equal(activeSwitchState({}).show, false);
  assert.equal(activeSwitchState(fo, false).show, false);
  // Inactive, no running automation: on is disabled with the hint.
  let s = activeSwitchState({ ...fo, running_automation: false });
  assert.deepEqual(s, {
    show: true, active: false, canTurnOn: false, canTurnOff: false, hintKey: 'lists.activeNeedsRunning', warningKey: '',
  });
  // Unknown running state (a multi-list row) is treated as not running.
  assert.equal(activeSwitchState(fo).canTurnOn, false);
  // Inactive with a running automation: on is allowed, and the warning shows.
  s = activeSwitchState({ ...fo, runningAutomation: true });
  assert.equal(s.canTurnOn, true);
  assert.equal(s.warningKey, 'lists.activeRunningNotActive');
  assert.equal(s.hintKey, '');
  // Active: off always allowed, no warning.
  s = activeSwitchState({ ...fo, triggerActive: true, runningAutomation: false });
  assert.equal(s.active, true);
  assert.equal(s.canTurnOff, true);
  assert.equal(s.warningKey, '');
  // A locked list shows the switch too (Marketing's).
  assert.equal(activeSwitchState({ ...fo, journey_locked: true }).show, true);

  // Every key the form renders exists in en.json.
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  ['lists.activeNeedsRunning', 'lists.activeRunningNotActive', 'lists.activeTurnsOff', 'lists.journeyLockedNotice',
    'lists.journeyStandardAdminOnly', 'lists.activeSwitch', 'lists.activeHelp', 'lists.journeyPickOption', 'lists.journeyLocked',
  ].forEach((k) => assert.equal(typeof en[k], 'string', k));
});

test('D12/D3: the "Saving turns Active off" notice', () => {
  const stored = {
    join_trigger: { kind: 'skus', skus: ['widget'] }, brand: 'acme', transactional: false, trigger_active: true,
  };
  assert.equal(triggerChangeTurnsOff(stored, { joinTrigger: { kind: 'skus', skus: [' WIDGET '] }, brand: 'acme', transactional: false }), false, 'by value');
  assert.equal(triggerChangeTurnsOff(stored, { joinTrigger: { kind: 'skus', skus: ['other'] } }), true);
  assert.equal(triggerChangeTurnsOff(stored, { joinTrigger: null }), true, 'clearing');
  assert.equal(triggerChangeTurnsOff(stored, { brand: 'beta' }), true);
  assert.equal(triggerChangeTurnsOff(stored, { transactional: true }), true);
  assert.equal(triggerChangeTurnsOff(stored, {}), false, 'absent keys are not changes');
  assert.equal(triggerChangeTurnsOff({ ...stored, trigger_active: false }, { brand: 'beta' }), false, 'an inactive list');
  const step = { joinTrigger: { kind: 'no_purchase', days: 2 }, triggerActive: true };
  assert.equal(triggerChangeTurnsOff(step, { joinTrigger: { kind: 'no_purchase', days: 5 } }), true);
});
