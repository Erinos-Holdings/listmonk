// Fork (list join triggers, integrations LIST-JOIN-TRIGGER-SPEC D7). The pure half of the Lists
// form's "Who joins automatically" section, the Lists page sub-line item and the campaign list
// picker label. The server stores join_trigger ({kind: 'first_order'} or {kind: 'skus', skus}) and
// the reconciler's write-back trigger_sync ({synced_at, state: 'live'|'invalid', slug?, error?});
// this module only maps them to and from the form and formats them -- the fork stores and
// renders, never computes. Tested by listTrigger.test.mjs.

import { buildListRequest } from './listBrand.mjs'; // eslint-disable-line import/extensions

export const TRIGGER_NONE = 'none';
export const TRIGGER_FIRST_ORDER = 'first_order';
export const TRIGGER_SKUS = 'skus';

// The keys the list form sends (F3) -- never the spread GET row, whose sunset, health,
// subscriber_grid, trigger_* and transactional keys must not ride back to the server.
export const LIST_REQUEST_KEYS = ['name', 'type', 'optin', 'status', 'tags', 'description'];

const isObj = (v) => !!v && typeof v === 'object' && !Array.isArray(v);

// triggerOption is the radio value for a stored join_trigger.
export function triggerOption(joinTrigger) {
  if (!isObj(joinTrigger)) return TRIGGER_NONE;
  if (joinTrigger.kind === TRIGGER_FIRST_ORDER) return TRIGGER_FIRST_ORDER;
  if (joinTrigger.kind === TRIGGER_SKUS) return TRIGGER_SKUS;
  return TRIGGER_NONE;
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

// buildJoinTrigger is the join_trigger the form sends for a radio value and the textarea text:
// null for none (clears), the document otherwise.
export function buildJoinTrigger(option, skusText) {
  if (option === TRIGGER_FIRST_ORDER) return { kind: TRIGGER_FIRST_ORDER };
  if (option === TRIGGER_SKUS) return { kind: TRIGGER_SKUS, skus: textToSkus(skusText) };
  return null;
}

// triggerBadge is the badge beside the section title: null when the list has no trigger, else
// {state, key, params} -- pending (trigger set, no sync yet), live (params.time = synced_at, which
// the view formats) or invalid (params.error).
export function triggerBadge(joinTrigger, triggerSync) {
  if (triggerOption(joinTrigger) === TRIGGER_NONE) return null;
  if (isObj(triggerSync) && triggerSync.state === 'live') {
    return { state: 'live', key: 'lists.triggerSyncLive', params: { time: triggerSync.synced_at || '' } };
  }
  if (isObj(triggerSync) && triggerSync.state === 'invalid') {
    return { state: 'invalid', key: 'lists.triggerSyncInvalid', params: { error: triggerSync.error || '' } };
  }
  return { state: 'pending', key: 'lists.triggerSyncPending', params: {} };
}

// listTriggerItem is the Lists page sub-line item, or '' for a list without a trigger. The state
// word only -- never the error text.
export function listTriggerItem(list) {
  const l = list || {};
  const opt = triggerOption(l.join_trigger);
  if (opt === TRIGGER_NONE) return '';
  const what = opt === TRIGGER_FIRST_ORDER ? 'joins on first order' : 'joins on SKU purchase';
  const badge = triggerBadge(l.join_trigger, l.trigger_sync);
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
  const opt = triggerOption(l.join_trigger);
  if (opt === TRIGGER_FIRST_ORDER) return `${name} — joins on first purchase`;
  if (opt === TRIGGER_SKUS) return `${name} — joins on SKU purchase`;
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
