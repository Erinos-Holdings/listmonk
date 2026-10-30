// Fork (list join triggers, integrations LIST-JOIN-TRIGGER-SPEC D7). The pure half of the Lists
// form's "Who joins automatically" section, the Lists page sub-line item and the campaign list
// picker label. The server stores join_trigger ({kind: 'first_order'}, {kind: 'skus', skus},
// {kind: 'no_purchase', days} -- NO-PURCHASE-STEPS-SPEC D7, a No-purchase series step -- or
// {kind: 'post_purchase', days} -- LIFECYCLE-SERIES-SPEC S2, a Post-purchase series step) and
// the reconciler's write-back trigger_sync ({synced_at, state: 'live'|'invalid', slug?, error?});
// this module only maps them to and from the form and formats them -- the fork stores and
// renders, never computes. Tested by listTrigger.test.mjs.

import { buildListRequest } from './listBrand.mjs'; // eslint-disable-line import/extensions

export const TRIGGER_NONE = 'none';
export const TRIGGER_FIRST_ORDER = 'first_order';
export const TRIGGER_SKUS = 'skus';
// NO-PURCHASE-STEPS-SPEC D1/D7 -- a "still deciding" step: {kind: 'no_purchase', days: N}, N 1-365.
export const TRIGGER_NO_PURCHASE = 'no_purchase';
// LIFECYCLE-SERIES-SPEC S2 -- a Post-purchase series step: {kind: 'post_purchase', days: N}, N 1-365
// on the server (shape only); the reconciler accepts 5, 14 and 30.
export const TRIGGER_POST_PURCHASE = 'post_purchase';
export const DAYS_MIN = 1;
export const DAYS_MAX = 365;

// The two STEP kinds (a list the integrations sweep fills) -- the server's isStepTriggerKind.
export const STEP_TRIGGERS = Object.freeze([TRIGGER_NO_PURCHASE, TRIGGER_POST_PURCHASE]);

export function isStepTrigger(option) {
  return STEP_TRIGGERS.includes(option);
}

// LIFECYCLE-SERIES-SPEC S7 -- the standard step days. Cross-repo literals (the SES_DAILY_ZONE
// precedent; no test spans the repos): integrations lib/no-purchase.ts WELCOME_STEP_DAYS and
// POST_PURCHASE_STEP_DAYS carry the same values -- change both in one release.
export const WELCOME_STEP_DAYS = Object.freeze([2, 5, 10]);
export const POST_PURCHASE_STEP_DAYS = Object.freeze([5, 14, 30]);

// The two series (the Usage pane's groups and the Lists form's step radios): a series family per
// step kind. Day 0 of the No-purchase series is the brand newsletter list's automation; day 0 of the
// Post-purchase series is the first-order list.
export const SERIES_WELCOME = 'welcome';
export const SERIES_POST_PURCHASE = 'postPurchase';

export function seriesOfTrigger(option) {
  if (option === TRIGGER_NO_PURCHASE) return SERIES_WELCOME;
  if (option === TRIGGER_POST_PURCHASE) return SERIES_POST_PURCHASE;
  return null;
}

// seriesStepKey is the ONE resolver of a series step's D5 description (LIFECYCLE-SERIES-SPEC
// D5/S10): the i18n key of (family, days) -- `series.<family>Day<days>` for a standard day (0 or a
// standard step day), else the family's own legacy description `series.<family>Legacy` (Stage 4
// F2 -- each names its own standard days); null for an unknown family. Each text has one home in
// en.json; the Lists form's radios and the Usage pane's leaves both resolve through here.
export function seriesStepKey(family, days) {
  let std = null;
  if (family === SERIES_WELCOME) std = [0, ...WELCOME_STEP_DAYS];
  if (family === SERIES_POST_PURCHASE) std = [0, ...POST_PURCHASE_STEP_DAYS];
  if (!std) return null;
  return std.includes(days) ? `series.${family}Day${days}` : `series.${family}Legacy`;
}

// The keys the list form sends (F3) -- never the spread GET row, whose sunset, health,
// subscriber_grid, trigger_* and transactional keys must not ride back to the server.
export const LIST_REQUEST_KEYS = ['name', 'type', 'optin', 'status', 'tags', 'description'];

const isObj = (v) => !!v && typeof v === 'object' && !Array.isArray(v);

// storedTrigger / storedTriggerSync read the two server fields off a list row under EITHER key
// spelling: the API client camel-cases every response by default (api/index.js, utils.camelKeys,
// deep), so a row from GET /api/lists carries joinTrigger / triggerSync (and synced_at as
// syncedAt), while a test fixture or a verbatim read carries the wire names. Reading only the
// wire names left every stored trigger invisible in the browser (found 2026-10-19: the form
// preset to None for a live first-order list, and a Save from there would have cleared it).
export function storedTrigger(list) {
  const l = list || {};
  return l.join_trigger !== undefined ? l.join_trigger : l.joinTrigger;
}

export function storedTriggerSync(list) {
  const l = list || {};
  return l.trigger_sync !== undefined ? l.trigger_sync : l.triggerSync;
}

// triggerOption is the radio value for a stored join_trigger.
export function triggerOption(joinTrigger) {
  if (!isObj(joinTrigger)) return TRIGGER_NONE;
  if (joinTrigger.kind === TRIGGER_FIRST_ORDER) return TRIGGER_FIRST_ORDER;
  if (joinTrigger.kind === TRIGGER_SKUS) return TRIGGER_SKUS;
  if (joinTrigger.kind === TRIGGER_NO_PURCHASE) return TRIGGER_NO_PURCHASE;
  if (joinTrigger.kind === TRIGGER_POST_PURCHASE) return TRIGGER_POST_PURCHASE;
  return TRIGGER_NONE;
}

// storedDays is the days input's value for a stored join_trigger: its days for a step trigger
// (no_purchase or post_purchase), else '' (the field starts empty for every other kind).
export function storedDays(joinTrigger) {
  if (!isStepTrigger(triggerOption(joinTrigger))) return '';
  const d = joinTrigger.days;
  return Number.isInteger(d) ? String(d) : '';
}

// textToDays is the days number the form sends for the input's text: a whole number written as
// digits (surrounding spaces ignored), else null. The range 1-365 is the server's rule (D1); an
// out-of-range or missing value is sent as-is and refused there (400 lists.triggerInvalid).
export function textToDays(text) {
  const s = String(text == null ? '' : text).trim();
  return /^\d{1,6}$/.test(s) ? Number(s) : null;
}

// daysValid mirrors the server's range for the field's inline state only -- the server decides.
export function daysValid(text) {
  const d = textToDays(text);
  return d !== null && d >= DAYS_MIN && d <= DAYS_MAX;
}

// DAYS_PRESETS are the form's no_purchase (No-purchase series) days choices -- LIFECYCLE-SERIES-SPEC
// S7 (2/5/10, replacing the provisional 3/7/15). The server still accepts 1-365 (D1); this narrows
// only what the form offers. POST_PURCHASE_DAYS_PRESETS are the post_purchase choices.
export const DAYS_PRESETS = WELCOME_STEP_DAYS;
export const POST_PURCHASE_DAYS_PRESETS = POST_PURCHASE_STEP_DAYS;

// daysPresetsFor is the presets of a step option (no_purchase when the option is not a step).
export function daysPresetsFor(option) {
  return option === TRIGGER_POST_PURCHASE ? POST_PURCHASE_DAYS_PRESETS : DAYS_PRESETS;
}

// daysChoices is the radio options for the days field: the option's presets, plus a stored value
// outside them (kept so a Save of an older list never changes its days unseen -- D2, Ruze's
// grandfathered 15), ascending.
export function daysChoices(daysText, option = TRIGGER_NO_PURCHASE) {
  const d = textToDays(daysText);
  const out = [...daysPresetsFor(option)];
  if (d !== null && !out.includes(d)) out.push(d);
  return out.sort((a, b) => a - b);
}

// daysRadios is the days field's radios for a step option: daysChoices, each with `legacy` (a
// stored value outside the presets -- labeled "N days (legacy)") and `help`, the D5 description
// key it hovers with (seriesStepKey; the legacy description for a legacy value). `storedText` is
// the STORED days, passed only when the stored trigger is this option's kind.
export function daysRadios(option, storedText) {
  const presets = daysPresetsFor(option);
  const family = seriesOfTrigger(option) || SERIES_WELCOME;
  return daysChoices(storedText, option).map((days) => ({
    days, legacy: !presets.includes(days), help: seriesStepKey(family, days),
  }));
}

// skusToText is the textarea text for a stored SKU array, one per line.
export function skusToText(skus) {
  return (Array.isArray(skus) ? skus : []).join('\n');
}

// textToSkus is the SKU array for the textarea text: one per line, trimmed, blank lines dropped.
// The server lower-cases and de-duplicates.
export function textToSkus(text) {
  return String(text == null ? '' : text).split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
}

// buildJoinTrigger is the join_trigger the form sends for a radio value, the textarea text and the
// days input text: null for none (clears), the document otherwise.
export function buildJoinTrigger(option, skusText, daysText) {
  if (option === TRIGGER_FIRST_ORDER) return { kind: TRIGGER_FIRST_ORDER };
  if (option === TRIGGER_SKUS) return { kind: TRIGGER_SKUS, skus: textToSkus(skusText) };
  if (isStepTrigger(option)) return { kind: option, days: textToDays(daysText) };
  return null;
}

// Fork (journey form UX, integrations JOURNEY-UX-SPEC D8/D14 F4). The list form's Broadcast |
// Journey selector over the same join_trigger: Journey iff a trigger is stored; Broadcast (the
// retired None) saves join_trigger null. The Journey pane's radio always carries a kind
// (journeyOption), so a Journey-labeled save never stores null.
export const LIST_TYPE_BROADCAST = 'broadcast';
export const LIST_TYPE_JOURNEY = 'journey';

// listTypeOf is the selector value for a stored join_trigger.
export function listTypeOf(joinTrigger) {
  return triggerOption(joinTrigger) === TRIGGER_NONE ? LIST_TYPE_BROADCAST : LIST_TYPE_JOURNEY;
}

// journeyOption is the Journey pane's radio for a stored join_trigger: its kind, else first_order.
export function journeyOption(joinTrigger) {
  const opt = triggerOption(joinTrigger);
  return opt === TRIGGER_NONE ? TRIGGER_FIRST_ORDER : opt;
}

// buildListTypeTrigger is the join_trigger the form sends for the selector value and the Journey
// pane's values: null for Broadcast; for Journey the buildJoinTrigger document, a missing or none
// radio reading first_order -- never null.
export function buildListTypeTrigger(listType, option, skusText, daysText) {
  if (listType !== LIST_TYPE_JOURNEY) return null;
  const opt = [TRIGGER_FIRST_ORDER, TRIGGER_SKUS, ...STEP_TRIGGERS].includes(option) ? option : TRIGGER_FIRST_ORDER;
  return buildJoinTrigger(opt, skusText, daysText);
}

// Fork (integrations JOURNEY-UX-SPEC A1.4) -- the selector's help follows the selection: one
// i18n key per value (Broadcast for anything that is not Journey).
export function listTypeHelpKey(listType) {
  return listType === LIST_TYPE_JOURNEY ? 'lists.campaignTypeHelpJourney' : 'lists.campaignTypeHelpBroadcast';
}

// A1.3 -- a trigger radio's help: a bold "Sends ..." lead sentence, then the prose. null for a
// radio that carries no help (none, or an unknown option).
export function triggerHelpKeys(option) {
  switch (option) {
    case TRIGGER_FIRST_ORDER:
      return { lead: 'lists.joinTriggerFirstOrderSends', body: 'lists.joinTriggerLatency' };
    case TRIGGER_SKUS:
      return { lead: 'lists.joinTriggerSkusSends', body: 'lists.joinTriggerLatency' };
    case TRIGGER_NO_PURCHASE:
      return { lead: 'lists.joinTriggerNoPurchaseSends', body: 'lists.joinTriggerNoPurchaseHelp' };
    case TRIGGER_POST_PURCHASE:
      return { lead: 'lists.joinTriggerNoPurchaseSends', body: 'lists.joinTriggerPostPurchaseHelp' };
    default:
      return null;
  }
}

// seriesPhrase is LIFECYCLE-SERIES-SPEC S10's standard phrase for a stored trigger: "Post-purchase
// series day 0" (first order), "No-purchase series day N" (no_purchase), "Post-purchase series day N"
// (post_purchase); '' for any other kind.
function seriesPhrase(joinTrigger) {
  const opt = triggerOption(joinTrigger);
  if (opt === TRIGGER_FIRST_ORDER) return 'Post-purchase series day 0';
  if (opt === TRIGGER_NO_PURCHASE) return `No-purchase series day ${joinTrigger.days}`;
  if (opt === TRIGGER_POST_PURCHASE) return `Post-purchase series day ${joinTrigger.days}`;
  return '';
}

// triggerWhat is the sub-line's kind phrase for a stored trigger ('' for none).
function triggerWhat(joinTrigger) {
  const opt = triggerOption(joinTrigger);
  if (opt === TRIGGER_SKUS) return 'joins on SKU purchase';
  return seriesPhrase(joinTrigger);
}

// triggerBadge is the badge beside the section title: null when the list has no trigger, else
// {state, key, params} -- pending (trigger set, no sync yet), live (params.time = synced_at, which
// the view formats) or invalid (params.error).
export function triggerBadge(joinTrigger, triggerSync) {
  if (triggerOption(joinTrigger) === TRIGGER_NONE) return null;
  if (isObj(triggerSync) && triggerSync.state === 'live') {
    const time = triggerSync.synced_at !== undefined ? triggerSync.synced_at : triggerSync.syncedAt;
    return { state: 'live', key: 'lists.triggerSyncLive', params: { time: time || '' } };
  }
  if (isObj(triggerSync) && triggerSync.state === 'invalid') {
    return { state: 'invalid', key: 'lists.triggerSyncInvalid', params: { error: triggerSync.error || '' } };
  }
  return { state: 'pending', key: 'lists.triggerSyncPending', params: {} };
}

// listTriggerItem is the Lists page sub-line item, or '' for a list without a trigger. The state
// word only -- never the error text.
export function listTriggerItem(list) {
  const opt = triggerOption(storedTrigger(list));
  if (opt === TRIGGER_NONE) return '';
  const what = triggerWhat(storedTrigger(list));
  const badge = triggerBadge(storedTrigger(list), storedTriggerSync(list));
  const state = { pending: 'pending sync', live: 'live', invalid: 'invalid' }[badge.state];
  return `${what} · ${state}`;
}

// withTriggerItem appends the item to the sub-line text with the page's ' · ' separator, or
// returns the text unchanged for a list without a trigger.
export function withTriggerItem(subline, list) {
  const item = listTriggerItem(list);
  return item ? `${subline} · ${item}` : subline;
}

// pickerLabel is the campaign list picker's dropdown label: the name, with the trigger suffix.
export function pickerLabel(list) {
  const l = list || {};
  const name = l.name || '';
  const opt = triggerOption(storedTrigger(l));
  if (opt === TRIGGER_SKUS) return `${name} — joins on SKU purchase`;
  const phrase = seriesPhrase(storedTrigger(l));
  return phrase ? `${name} — ${phrase}` : name;
}

// buildListFormRequest is the body the list form sends (F3): exactly the allow-listed form keys,
// the free tags and the brand (buildListRequest -- it throws when no brand is chosen), then
// join_trigger when the trigger section is shown (opts.joinTrigger defined -- edit only), and
// transactional only when the checkbox is rendered (opts.canTransactional -- a user without
// lists:transactional never sends the field, D5).
export function buildListFormRequest(form, brand, opts = {}) {
  const base = buildListRequest(form, brand);
  const out = {};
  LIST_REQUEST_KEYS.forEach((k) => {
    if (base[k] !== undefined) out[k] = base[k];
  });
  out.brand = base.brand;
  if (opts.joinTrigger !== undefined) out.join_trigger = opts.joinTrigger;
  if (opts.canTransactional) out.transactional = !!opts.transactional;
  return out;
}
