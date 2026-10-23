// Fork (list join triggers, integrations LIST-JOIN-TRIGGER-SPEC D7). The pure half of the Lists
// form's "Who joins automatically" section, the Lists page sub-line item and the campaign list
// picker label. The server stores join_trigger ({kind: 'first_order'}, {kind: 'skus', skus} or
// {kind: 'no_purchase', days} -- NO-PURCHASE-STEPS-SPEC D7) and
// the reconciler's write-back trigger_sync ({synced_at, state: 'live'|'invalid', slug?, error?});
// this module only maps them to and from the form and formats them -- the fork stores and
// renders, never computes. Tested by listTrigger.test.mjs.

import { buildListRequest } from './listBrand.mjs'; // eslint-disable-line import/extensions

export const TRIGGER_NONE = 'none';
export const TRIGGER_FIRST_ORDER = 'first_order';
export const TRIGGER_SKUS = 'skus';
// NO-PURCHASE-STEPS-SPEC D1/D7 -- a "still deciding" step: {kind: 'no_purchase', days: N}, N 1-365.
export const TRIGGER_NO_PURCHASE = 'no_purchase';
export const DAYS_MIN = 1;
export const DAYS_MAX = 365;

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
  return TRIGGER_NONE;
}

// storedDays is the days input's value for a stored join_trigger: its days for a no_purchase
// trigger, else '' (the field starts empty for every other kind).
export function storedDays(joinTrigger) {
  if (triggerOption(joinTrigger) !== TRIGGER_NO_PURCHASE) return '';
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
  if (option === TRIGGER_NO_PURCHASE) return { kind: TRIGGER_NO_PURCHASE, days: textToDays(daysText) };
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
  const opt = [TRIGGER_FIRST_ORDER, TRIGGER_SKUS, TRIGGER_NO_PURCHASE].includes(option) ? option : TRIGGER_FIRST_ORDER;
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
    default:
      return null;
  }
}

// triggerWhat is the sub-line's kind phrase for a stored trigger ('' for none).
function triggerWhat(joinTrigger) {
  const opt = triggerOption(joinTrigger);
  if (opt === TRIGGER_FIRST_ORDER) return 'joins on first order';
  if (opt === TRIGGER_SKUS) return 'joins on SKU purchase';
  if (opt === TRIGGER_NO_PURCHASE) return `joins on day ${joinTrigger.days} no-purchase`;
  return '';
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
  if (opt === TRIGGER_FIRST_ORDER) return `${name} — joins on first purchase`;
  if (opt === TRIGGER_SKUS) return `${name} — joins on SKU purchase`;
  if (opt === TRIGGER_NO_PURCHASE) return `${name} — joins when still deciding (day ${storedTrigger(l).days})`;
  return name;
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
