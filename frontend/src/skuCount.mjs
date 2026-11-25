// Fork (SKU journey live count, integrations lambda/integrations/komigo/SKU-LIVE-COUNT-SPEC D6/D7) --
// the pure half of the Lists form's count for a Replenish / First purchase of SKUs trigger: the
// request (from the form, or from the STORED trigger for the Active confirm), the debounce and
// latest-answer-only decision, and the response -> display lines. The view (ListForm.vue) only wires
// it; POST /api/lists/sku-count (cmd/lists_sku_count.go) proxies the integrations count Lambda, and
// the fork computes nothing.
import {
  TRIGGER_FIRST_SKU, TRIGGER_REPLENISH, isSkuJourneyTrigger, storedTrigger, textToDays, textToSkus, triggerOption,
} from './listTrigger.mjs'; // eslint-disable-line import/extensions

// D6 -- the count is requested this long after the last change of the SKU text, the days or the kind.
export const SKU_COUNT_DEBOUNCE_MS = 500;

// skuCountRequest is the route's body for the form's state, or null when no count applies (the
// option is not a SKU journey, or no SKU is parsed). `listId` 0 = a list being created (its brand is
// the form's); a stored list's brand is the server's to resolve (the body's is ignored there).
export function skuCountRequest({
  listId = 0, brand = '', option, skusText, daysText,
} = {}) {
  if (!isSkuJourneyTrigger(option)) return null;
  const skus = textToSkus(skusText);
  if (skus.length === 0) return null;
  const d = textToDays(daysText);
  return {
    list_id: Number(listId) > 0 ? Number(listId) : 0,
    brand: brand || '',
    kind: option === TRIGGER_FIRST_SKU ? TRIGGER_FIRST_SKU : TRIGGER_REPLENISH,
    skus,
    days: d === null || d === 0 ? null : d,
  };
}

// storedCountRequest is the Active confirm's request (D7): the list's STORED trigger -- the trigger
// the switch activates; unsaved form edits are not counted. null when the stored trigger is not a
// SKU journey.
export function storedCountRequest(list) {
  const l = list || {};
  const jt = storedTrigger(l);
  const option = triggerOption(jt);
  if (!isSkuJourneyTrigger(option)) return null;
  return skuCountRequest({
    listId: l.id, brand: l.brand || '', option, skusText: (jt.skus || []).join('\n'), daysText: jt.days == null ? '' : String(jt.days),
  });
}

// requestKey is the request's identity: an unchanged key never re-asks.
export function requestKey(req) {
  return req ? JSON.stringify([req.list_id, req.brand, req.kind, req.skus, req.days]) : '';
}

// createSkuCounter is the debounce and the latest-answer-only rule (D6): schedule(req) restarts the
// 500 ms timer; when it fires the request is sent with a new sequence number, and only the answer of
// the LATEST sent request is shown (an older answer arriving late is dropped). onState receives
// { state: 'idle' | 'counting' | 'done' | 'error', response?, error? }.
export function createSkuCounter({
  send, onState, debounceMs = SKU_COUNT_DEBOUNCE_MS, setTimer = setTimeout, clearTimer = clearTimeout,
}) {
  let timer = null;
  let seq = 0;
  let lastKey = '';
  const cancel = () => {
    if (timer !== null) clearTimer(timer);
    timer = null;
  };
  const fire = (req) => {
    seq += 1;
    const mine = seq;
    onState({ state: 'counting' });
    Promise.resolve()
      .then(() => send(req))
      .then((response) => {
        if (mine === seq) onState({ state: 'done', response });
      }, (error) => {
        if (mine === seq) onState({ state: 'error', error });
      });
  };
  return {
    schedule(req, { immediate = false } = {}) {
      const key = requestKey(req);
      if (!immediate && key === lastKey) return;
      lastKey = key;
      cancel();
      if (!req) {
        seq += 1; // any answer still in flight is now stale
        onState({ state: 'idle' });
        return;
      }
      if (immediate) {
        fire(req);
        return;
      }
      timer = setTimer(() => {
        timer = null;
        fire(req);
      }, debounceMs);
    },
    // now: the request at once, without the debounce (the stored trigger on opening the form).
    now(req) {
      this.schedule(req, { immediate: true });
    },
    cancel() {
      cancel();
      seq += 1;
    },
  };
}

// errorOf is the HTTP status and the server's message of a failed request (axios's shape).
export function errorOf(error) {
  const r = error && error.response;
  const status = r && Number(r.status);
  const msg = r && r.data && typeof r.data.message === 'string' ? r.data.message : '';
  return { status: Number.isFinite(status) ? status : 0, message: msg };
}

// countLines is the block under the days field (D6) for a counter state, as display lines
// { key, params, kind } -- kind 'plain', 'strong', 'danger' or 'suggest' (a suggestion carries
// { from, to }: clicking it replaces `from` with `to` in the SKU text). `brandName` names the brand in
// "not a <brand> SKU". A 400 shows its text as a red line; any other failure says the count is
// unavailable (never blocks saving).
export function countLines(st, { brandName = '' } = {}) {
  if (!st || st.state === 'idle') return [];
  if (st.state === 'counting') return [{ key: 'lists.skuCountCounting', params: {}, kind: 'plain' }];
  if (st.state === 'error') {
    const e = errorOf(st.error);
    if (e.status === 400 && e.message) {
      return [{
        key: '', text: e.message, params: {}, kind: 'danger',
      }];
    }
    return [{ key: 'lists.skuCountUnavailable', params: {}, kind: 'plain' }];
  }
  const r = st.response || {};
  const out = [];
  // With days entered the one number is who is due at those days (bought days–180 ago); the
  // window line is the no-days (0-day) answer only (the user, 2026-11-25).
  if (r.window && !r.due) {
    out.push({ key: 'lists.skuCountWindow', params: { buyers: r.window.buyers, eligible: r.window.eligible }, kind: 'strong' });
  }
  (r.skus || []).forEach((s) => {
    if (!s.brand_sku) out.push({ key: 'lists.skuCountNotBrand', params: { sku: s.sku, brand: brandName }, kind: 'plain' });
    else if (s.buyers === 0) out.push({ key: 'lists.skuCountNoBuyers', params: { sku: s.sku }, kind: 'plain' });
    (s.suggestions || []).forEach((g) => {
      out.push({
        key: g.title ? 'lists.skuCountSuggest' : 'lists.skuCountSuggestNoTitle', params: { sku: g.sku, title: g.title }, kind: 'suggest', from: s.sku, to: g.sku,
      });
    });
  });
  if (r.due) {
    const to = (r.due.by_age || []).reduce((m, b) => Math.max(m, b.to_days), r.due.days);
    out.push({
      key: 'lists.skuCountDue',
      params: {
        n: r.due.joins, time: r.next_run_at, from: r.due.days, to,
      },
      kind: 'strong',
    });
  }
  if (r.assumed_all_languages) out.push({ key: 'lists.skuCountAssumed', params: {}, kind: 'plain' });
  return out;
}

// replaceSku replaces the SKU line `from` (compared trimmed, lower-cased) with `to` in the text.
export function replaceSku(skusText, from, to) {
  const norm = (s) => String(s).trim().toLowerCase();
  return String(skusText == null ? '' : skusText).split(/\r?\n/).map((line) => (norm(line) === norm(from) ? to : line)).join('\n');
}

// confirmOf is the Active confirm (D7) for a FRESH count of the stored trigger: joins > 0 -> the
// count and the next run; a stored non-brand SKU ALWAYS opens the confirm, whatever the joins (the
// sweep still plans every stored SKU, so a 0 or null count is not safe) -- the count, or "no count"
// when due is null, plus "<sku> is not counted"; joins 0 with every SKU a brand SKU -> null (no
// confirm); a failed request -> the unreadable text, asking to continue. That confirmation IS the
// approval.
export function confirmOf(result) {
  if (!result || result.error !== undefined) return { key: 'lists.skuCountConfirmUnreadable', params: {} };
  const r = result.response || {};
  const joins = r.due ? Number(r.due.joins) || 0 : 0;
  const notCounted = (r.skus || []).filter((s) => !s.brand_sku).map((s) => s.sku);
  if (joins <= 0 && notCounted.length === 0) return null;
  if (!r.due) return { key: 'lists.skuCountConfirmNoCount', params: {}, notCounted };
  return {
    key: 'lists.skuCountConfirm', params: { n: joins, time: r.next_run_at }, notCounted,
  };
}
