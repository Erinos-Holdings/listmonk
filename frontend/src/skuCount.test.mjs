// Fork (SKU journey live count, integrations SKU-LIVE-COUNT-SPEC I11/I12). Run: node --test src/skuCount.test.mjs
//   I11 -- debounce 500 ms; only the latest answer shown; the display lines per response case (no
//          days, days, assumed languages, suggestions, 400, 502/503).
//   I12 -- the Active confirm counts the STORED trigger; joins > 0 -> the count text; 0 -> none;
//          a failure -> the unreadable text.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import {
  SKU_COUNT_DEBOUNCE_MS, skuCountRequest, storedCountRequest, requestKey, createSkuCounter, countLines, replaceSku,
  confirmOf, errorOf,
} from './skuCount.mjs'; // eslint-disable-line import/extensions

const RESP = {
  as_of: '2026-11-25T14:00:00.000Z',
  next_run_at: '2026-11-25T15:10:00.000Z',
  automations_running: 1,
  assumed_all_languages: false,
  skus: [{
    sku: 'joybelle-brushonglue', brand_sku: true, buyers: 1425, suggestions: [],
  }],
  window: {
    buyers: 1425, eligible: 168, skips: { consent: 1257 }, consent: { 'not-subscribed': 1200 },
  },
  due: null,
};

test('the request: SKU journeys only, at least one SKU, days null until a number is typed', () => {
  assert.equal(skuCountRequest({ option: 'no_purchase', skusText: 'a', daysText: '2' }), null);
  assert.equal(skuCountRequest({ option: 'replenish', skusText: ' \n ', daysText: '90' }), null);
  assert.deepEqual(skuCountRequest({
    listId: 106, brand: 'joybelle', option: 'replenish', skusText: 'JoyBelle-BrushOnGlue\n\n b ', daysText: ' 90 ',
  }), {
    list_id: 106, brand: 'joybelle', kind: 'replenish', skus: ['JoyBelle-BrushOnGlue', 'b'], days: 90,
  });
  assert.equal(skuCountRequest({ option: 'first_sku', skusText: 'a', daysText: '' }).days, null);
  assert.equal(skuCountRequest({ option: 'first_sku', skusText: 'a', daysText: 'x' }).days, null);
  assert.equal(skuCountRequest({ option: 'first_sku', skusText: 'a', daysText: '200' }).days, 200, 'out of range is the server\'s 400');
  assert.equal(skuCountRequest({ listId: undefined, option: 'first_sku', skusText: 'a' }).list_id, 0, 'a new list');
  assert.equal(requestKey(null), '');
  assert.notEqual(requestKey(skuCountRequest({ option: 'replenish', skusText: 'a', daysText: '9' })), requestKey(skuCountRequest({ option: 'replenish', skusText: 'a', daysText: '90' })));
});

// A manual clock: setTimer queues, tick(ms) runs what is due.
function clock() {
  let now = 0;
  let id = 0;
  const timers = new Map();
  return {
    setTimer: (fn, ms) => {
      id += 1;
      timers.set(id, { fn, at: now + ms });
      return id;
    },
    clearTimer: (t) => timers.delete(t),
    tick(ms) {
      now += ms;
      [...timers.entries()].filter(([, t]) => t.at <= now).forEach(([k, t]) => {
        timers.delete(k);
        t.fn();
      });
    },
    pending: () => timers.size,
  };
}

const flush = () => new Promise((r) => { setImmediate(r); });

test('I11: debounce 500 ms -- the request goes 500 ms after the LAST change, once', async () => {
  assert.equal(SKU_COUNT_DEBOUNCE_MS, 500);
  const c = clock();
  const sent = [];
  const states = [];
  const counter = createSkuCounter({
    send: async (req) => { sent.push(req); return RESP; }, onState: (s) => states.push(s.state), setTimer: c.setTimer, clearTimer: c.clearTimer,
  });
  const r = (sku) => skuCountRequest({ option: 'replenish', skusText: sku, daysText: '90' });
  counter.schedule(r('a'));
  c.tick(300);
  counter.schedule(r('ab'));
  c.tick(300);
  counter.schedule(r('abc'));
  c.tick(499);
  assert.equal(sent.length, 0);
  c.tick(1);
  await flush();
  assert.deepEqual(sent.map((q) => q.skus[0]), ['abc']);
  assert.deepEqual(states, ['counting', 'done']);
  // An unchanged request never re-asks; a cleared one goes idle at once.
  counter.schedule(r('abc'));
  c.tick(1000);
  assert.equal(sent.length, 1);
  counter.schedule(null);
  assert.equal(states[states.length - 1], 'idle');
  // now(): at once, no debounce (the stored trigger on opening the form).
  counter.now(r('x'));
  await flush();
  assert.equal(sent.length, 2);
});

test('I11: only the latest answer is shown -- an older answer arriving late is dropped', async () => {
  const c = clock();
  const resolvers = [];
  const shown = [];
  const counter = createSkuCounter({
    send: () => new Promise((res, rej) => { resolvers.push({ res, rej }); }), onState: (s) => shown.push(s), setTimer: c.setTimer, clearTimer: c.clearTimer,
  });
  counter.schedule(skuCountRequest({ option: 'replenish', skusText: 'old', daysText: '' }));
  c.tick(500);
  counter.schedule(skuCountRequest({ option: 'replenish', skusText: 'new', daysText: '' }));
  c.tick(500);
  await flush();
  assert.equal(resolvers.length, 2);
  resolvers[1].res({ ...RESP, as_of: 'new' });
  await flush();
  resolvers[0].res({ ...RESP, as_of: 'old' }); // late
  await flush();
  const done = shown.filter((s) => s.state === 'done');
  assert.deepEqual(done.map((s) => s.response.as_of), ['new']);
  // A late failure of an old request is dropped too.
  counter.schedule(skuCountRequest({ option: 'replenish', skusText: 'n3', daysText: '' }));
  c.tick(500);
  await flush();
  counter.cancel();
  resolvers[2].rej(new Error('late'));
  await flush();
  assert.equal(shown.filter((s) => s.state === 'error').length, 0);
});

const keys = (lines) => lines.map((l) => l.key || `text:${l.text}`);

test('I11: the display lines per response case', () => {
  assert.deepEqual(countLines({ state: 'idle' }), []);
  assert.deepEqual(keys(countLines({ state: 'counting' })), ['lists.skuCountCounting']);

  // No days: the window line only.
  const noDays = countLines({ state: 'done', response: RESP });
  assert.deepEqual(noDays, [{ key: 'lists.skuCountWindow', params: { buyers: 1425, eligible: 168 }, kind: 'strong' }]);

  // Days: ONE line — who is due at those days, bought days–reach-back ago (the user, 2026-11-25: no
  // window line, no age table, no skip counts once days are entered).
  const due = {
    ...RESP,
    due: {
      days: 90,
      joins: 5,
      by_age: [{ from_days: 90, to_days: 120, joins: 2 }, { from_days: 120, to_days: 150, joins: 3 }, { from_days: 150, to_days: 180, joins: 0 }],
      by_lang: { en: 5 },
      skips: { 'not-due': 160, 'already-sent': 0, consent: 3 },
      consent: {},
    },
  };
  const d = countLines({ state: 'done', response: due });
  assert.deepEqual(keys(d), ['lists.skuCountDue']);
  assert.deepEqual(d[0].params, {
    n: 5, time: '2026-11-25T15:10:00.000Z', from: 90, to: 180,
  });
  // The range follows the days: 60 reads 60–180.
  const due60 = { ...due.due, days: 60, by_age: [{ from_days: 60, to_days: 90, joins: 1 }, { from_days: 150, to_days: 180, joins: 0 }] };
  const d60 = countLines({ state: 'done', response: { ...due, due: due60 } });
  assert.deepEqual([d60[0].params.from, d60[0].params.to], [60, 180]);

  // Assumed languages.
  assert.deepEqual(keys(countLines({ state: 'done', response: { ...RESP, assumed_all_languages: true } })), ['lists.skuCountWindow', 'lists.skuCountAssumed']);

  // Suggestions: a non-brand SKU (not counted) and a brand SKU with 0 buyers; a click replaces it.
  const sugg = {
    ...RESP,
    window: null,
    skus: [
      {
        sku: 'jb-11009-01', brand_sku: false, buyers: null, suggestions: [{ sku: 'JoyBelle-BrushOnGlue', brand_sku: 'JB-11009-01', title: 'Brush-On Nail Glue' }],
      },
      {
        sku: 'joybelle-glue-3', brand_sku: true, buyers: 0, suggestions: [{ sku: 'JoyBelle-Glue-2', brand_sku: '', title: '' }],
      },
    ],
  };
  const s = countLines({ state: 'done', response: sugg }, { brandName: 'JoyBelle' });
  assert.deepEqual(keys(s), ['lists.skuCountNotBrand', 'lists.skuCountSuggest', 'lists.skuCountNoBuyers', 'lists.skuCountSuggestNoTitle']);
  assert.deepEqual(s[0].params, { sku: 'jb-11009-01', brand: 'JoyBelle' });
  assert.deepEqual([s[1].kind, s[1].from, s[1].to, s[1].params], ['suggest', 'jb-11009-01', 'JoyBelle-BrushOnGlue', { sku: 'JoyBelle-BrushOnGlue', title: 'Brush-On Nail Glue' }]);
  assert.equal(replaceSku('a\n JB-11009-01 \nc', s[1].from, s[1].to), 'a\nJoyBelle-BrushOnGlue\nc');

  // 400: its text as a red line; 502 / 503 / a network error: "Count unavailable right now."
  const err = (status, message) => ({ state: 'error', error: { response: { status, data: { message } } } });
  assert.deepEqual(countLines(err(400, 'trigger.days "180" must be a whole number from 1 to 179')), [{
    key: '', text: 'trigger.days "180" must be a whole number from 1 to 179', params: {}, kind: 'danger',
  }]);
  [err(502, 'x'), err(503, 'y'), { state: 'error', error: new Error('network') }].forEach((e) => {
    assert.deepEqual(keys(countLines(e)), ['lists.skuCountUnavailable']);
  });
  assert.deepEqual(errorOf(new Error('x')), { status: 0, message: '' });
});

test('I12: the Active confirm counts the STORED trigger -- never the unsaved form', () => {
  const stored = {
    id: 106, brand: 'joybelle', join_trigger: { kind: 'replenish', skus: ['joybelle-brushonglue'], days: 90 },
  };
  assert.deepEqual(storedCountRequest(stored), {
    list_id: 106, brand: 'joybelle', kind: 'replenish', skus: ['joybelle-brushonglue'], days: 90,
  });
  // The camel-cased row (the API client) reads the same.
  assert.deepEqual(storedCountRequest({ id: 106, brand: 'joybelle', joinTrigger: { kind: 'first_sku', skus: ['a'], days: 30 } }).kind, 'first_sku');
  // Not a SKU journey: no count, no confirm.
  assert.equal(storedCountRequest({ id: 5, join_trigger: { kind: 'first_order' } }), null);
  assert.equal(storedCountRequest({}), null);
});

test('I12: joins > 0 -> the count text; 0 -> none; a failure -> the unreadable text; a stored non-brand SKU is named', () => {
  const due = (joins, skus = RESP.skus) => ({
    response: {
      ...RESP,
      skus,
      due: {
        days: 90, joins, by_age: [], by_lang: {}, skips: {}, consent: {},
      },
    },
  });
  assert.deepEqual(confirmOf(due(12)), { key: 'lists.skuCountConfirm', params: { n: 12, time: '2026-11-25T15:10:00.000Z' }, notCounted: [] });
  assert.equal(confirmOf(due(0)), null);
  assert.equal(confirmOf({ response: { ...RESP, due: null } }), null);
  assert.deepEqual(confirmOf({ error: new Error('502') }), { key: 'lists.skuCountConfirmUnreadable', params: {} });
  const withOther = due(3, [...RESP.skus, {
    sku: 'ruze-x', brand_sku: false, buyers: null, suggestions: [],
  }]);
  assert.deepEqual(confirmOf(withOther).notCounted, ['ruze-x']);
  // A stored non-brand SKU ALWAYS opens the confirm, whatever the joins.
  const other = {
    sku: 'ruze-x', brand_sku: false, buyers: null, suggestions: [],
  };
  assert.deepEqual(confirmOf(due(0, [...RESP.skus, other])), {
    key: 'lists.skuCountConfirm', params: { n: 0, time: '2026-11-25T15:10:00.000Z' }, notCounted: ['ruze-x'],
  });
  // Every stored SKU non-brand: due is null -> "no count" plus the not-counted line.
  assert.deepEqual(confirmOf({
    response: {
      ...RESP, window: null, due: null, skus: [other],
    },
  }), {
    key: 'lists.skuCountConfirmNoCount', params: {}, notCounted: ['ruze-x'],
  });
});

test('every key the count and the confirm render exists in en.json; the D8 preview keys are gone', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  ['lists.skuCountCounting', 'lists.skuCountWindow', 'lists.skuCountNoBuyers', 'lists.skuCountNotBrand', 'lists.skuCountSuggest',
    'lists.skuCountSuggestNoTitle', 'lists.skuCountDue', 'lists.skuCountAssumed',
    'lists.skuCountUnavailable', 'lists.skuCountNotConfigured', 'lists.skuCountInvalid', 'lists.skuCountNoBrand',
    'lists.skuCountConfirm', 'lists.skuCountConfirmNoCount', 'lists.skuCountConfirmNotCounted', 'lists.skuCountConfirmUnreadable',
  ].forEach((k) => assert.equal(typeof en[k], 'string', k));
  assert.deepEqual(Object.keys(en).filter((k) => k.startsWith('lists.skuPreview')), []);
  // The age table and skip line were dropped from the form (2026-11-25).
  assert.equal(en['lists.skuCountAge'], undefined);
  assert.equal(en['lists.skuCountSkips'], undefined);
  // The view wires the module; it renders no stored preview.
  const form = readFileSync(new URL('./views/ListForm.vue', import.meta.url), 'utf8');
  assert.equal(/preview/i.test(form.replace(/journeyPreview|previewHtml/g, '')), false, 'ListForm.vue still mentions a preview');
  assert.match(form, /from '\.\.\/skuCount\.mjs'/);
});
