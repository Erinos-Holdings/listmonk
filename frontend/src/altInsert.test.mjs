// Fork (campaign review) -- integrations ALT-ON-INSERT-SPEC I7 (in-editor new entries carry the
// answer's lang), I8 (normLang agrees with the Lambda's campaignLang: the same table as integrations
// tests/lib/campaign-review/alt-generate.test.ts "ALT-ON-INSERT I8"), I10 (the triggers, the
// baseline, debounce, single flight, Inspect cancels), I11 (the stamp at the language change),
// I12 (the sequence never saves: the injected API's only method is generateAltsDraft) and I16 (D5's
// one rule for using an answer: each of (a)-(f) failing drops it whole; only (b), (d), (e) queue).
/* eslint-disable no-param-reassign -- the harness's page is mutated on purpose (the page changing while a request is out) */
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  createAltInsert, imagePairs, normLang, stampLang, parseAttribs, sameJSON, ALT_INSERT_DEBOUNCE_MS,
} from './altInsert.mjs'; // eslint-disable-line import/extensions

const AT = '2026-11-21T12:00:00.000Z';

const doc = (images) => {
  const d = { root: { type: 'EmailLayout', data: { childrenIds: Object.keys(images) } } };
  Object.entries(images).forEach(([id, p]) => {
    d[id] = { type: 'Image', data: { style: {}, props: typeof p === 'string' ? { url: p } : p } };
  });
  return JSON.stringify(d);
};

const flushMicro = () => new Promise((r) => { setImmediate(r); });

function fakeTimers() {
  let n = 0;
  const q = new Map();
  return {
    setTimeout: (fn, ms) => {
      n += 1;
      q.set(n, { fn, ms });
      return n;
    },
    clearTimeout: (id) => { q.delete(id); },
    pending: () => [...q.values()],
    async flush() {
      const items = [...q.entries()];
      items.forEach(([id]) => q.delete(id));
      // eslint-disable-next-line no-restricted-syntax
      for (const [, t] of items) {
        t.fn();
      }
      await flushMicro();
      await flushMicro();
    },
  };
}

function deferred() {
  let resolve;
  const p = new Promise((r) => { resolve = r; });
  return { p, resolve };
}

// A page: mutable state, the API (a Proxy recording every property touched -- I12), the builder.
function harness(over = {}) {
  const page = {
    mount: 0,
    contentType: 'visual',
    editable: true,
    status: 'draft',
    name: 'C',
    attribsStr: JSON.stringify({ lang: 'es', alt_generated: {} }, null, 4),
    lang: 'es',
    source: doc({ a: 'https://e/a.jpg' }),
    body: '<p>x</p>',
    saving: false,
    syncing: false,
    inspecting: false,
    ...(over.page || {}),
  };
  const requests = [];
  const touched = [];
  const toasts = [];
  const applied = [];
  let answer = over.answer || (() => ({
    status: 'ok',
    fixes: [{
      kind: 'alt.set', block: 'b', from: '', to: 'Botella', undecided: true, src: 'https://e/b.jpg',
    }],
    keep: {},
    rejected: 0,
    at: AT,
    lang: 'es',
  }));
  const api = new Proxy({}, {
    get(_t, prop) {
      touched.push(prop);
      if (prop === 'generateAltsDraft') {
        return async (snapshot) => {
          requests.push(JSON.parse(JSON.stringify(snapshot)));
          return answer(snapshot);
        };
      }
      return undefined;
    },
  });
  const timers = fakeTimers();
  const ai = createAltInsert({
    api,
    getState: () => page,
    setAttribsStr: (s) => { page.attribsStr = s; },
    applyAlts: over.applyAlts || ((fixes) => {
      applied.push(fixes);
      return { applied: fixes, skipped: [] };
    }),
    toast: (count) => toasts.push(count),
    setTimeout: timers.setTimeout,
    clearTimeout: timers.clearTimeout,
  });
  return {
    page, ai, timers, requests, touched, toasts, applied, setAnswer: (f) => { answer = f; },
  };
}

// Mount 0's baseline, then image b arrives.
const arrive = (h) => {
  h.ai.onBuilderChange({ mount: h.page.mount, source: h.page.source });
  h.page.source = doc({ a: 'https://e/a.jpg', b: 'https://e/b.jpg' });
  h.ai.onBuilderChange({ mount: h.page.mount, source: h.page.source });
};

const mapOf = (h) => JSON.parse(h.page.attribsStr).alt_generated;

// ---------------------------------------------------------------------------------------------

test('I8: normLang agrees with the Lambda\'s campaignLang (the table integrations alt-generate.test.ts lists)', () => {
  const PAIRS = [['', 'en'], ['en', 'en'], ['es', 'es'], ['fr', 'fr'], ['de', 'de'], ['it', 'it']];
  PAIRS.forEach(([given, want]) => assert.equal(normLang(given), want, JSON.stringify(given)));
  assert.equal(normLang(undefined), 'en');
  assert.equal(normLang(null), 'en');
});

test('imagePairs: (block id, trimmed url) over Image blocks with a url; not a document -> null', () => {
  const s = imagePairs(doc({ a: ' https://e/a.jpg ', b: '', c: { alt: 'x' } }));
  assert.deepEqual([...s], ['a\u0000https://e/a.jpg']);
  assert.equal(imagePairs('not json'), null);
  assert.equal(imagePairs('[]'), null);
});

test('stampLang / parseAttribs / sameJSON', () => {
  assert.deepEqual(stampLang({ x: { src: 's', alt: 'a' }, y: { src: 's', alt: 'b', lang: 'fr' } }, 'en'), {
    x: { src: 's', alt: 'a', lang: 'en' }, y: { src: 's', alt: 'b', lang: 'fr' },
  });
  assert.deepEqual(stampLang(null, 'en'), {});
  assert.deepEqual(parseAttribs(''), {});
  assert.equal(parseAttribs('{'), null);
  assert.equal(parseAttribs('[]'), null);
  assert.equal(sameJSON({ a: 1, b: { c: [1] } }, { b: { c: [1] }, a: 1 }), true);
  assert.equal(sameJSON({ a: 1 }, { a: 2 }), false);
});

test('I10: the first change after a mount is the baseline; a new (block, trimmed url) pair triggers after the debounce', async () => {
  const h = harness();
  h.ai.onBuilderChange({ mount: 0, source: h.page.source });
  assert.equal(h.timers.pending().length, 0, 'the baseline is never a trigger');
  // applyOfficial's re-emit: the same pairs.
  h.ai.onBuilderChange({ mount: 0, source: h.page.source });
  assert.equal(h.timers.pending().length, 0);
  // An alt edit, a whitespace-only url edit: no new pair.
  h.ai.onBuilderChange({ mount: 0, source: doc({ a: { url: '  https://e/a.jpg', alt: 'typed' } }) });
  assert.equal(h.timers.pending().length, 0);
  // A removed image: no trigger.
  h.ai.onBuilderChange({ mount: 0, source: doc({}) });
  assert.equal(h.timers.pending().length, 0);
  // It comes back (a new pair against the last seen set): a trigger.
  h.ai.onBuilderChange({ mount: 0, source: doc({ a: 'https://e/a.jpg' }) });
  assert.equal(h.timers.pending().length, 1);
  assert.equal(h.timers.pending()[0].ms, ALT_INSERT_DEBOUNCE_MS);
  await h.timers.flush();
  assert.equal(h.requests.length, 1);
});

test('I10: a replaced image (same block, new url) and a later resetDocument (a template import) trigger; a new mount\'s first change does not', async () => {
  const h = harness();
  h.ai.onBuilderChange({ mount: 0, source: doc({ a: 'https://e/a.jpg' }) });
  h.ai.onBuilderChange({ mount: 0, source: doc({ a: 'https://e/new.jpg' }) });
  assert.equal(h.timers.pending().length, 1);
  await h.timers.flush();
  // resetDocument of an imported template: whole new document, new pairs.
  h.ai.onBuilderChange({ mount: 0, source: doc({ t1: 'https://e/t1.jpg' }) });
  assert.equal(h.timers.pending().length, 1);
  await h.timers.flush();
  assert.equal(h.requests.length, 2);
  // editorKey changed (a reload after a save): its first change is a baseline.
  h.page.mount = 1;
  h.ai.onBuilderChange({ mount: 1, source: doc({ z: 'https://e/z.jpg', y: 'https://e/y.jpg' }) });
  assert.equal(h.timers.pending().length, 0);
});

test('I10: debounced (two triggers, one request); single flight with at most one queued run; Inspect cancels the debounce', async () => {
  const h = harness();
  const gate = deferred();
  h.setAnswer(() => gate.p.then(() => ({
    status: 'none', fixes: [], keep: {}, rejected: 0, at: AT, lang: 'es',
  })));
  h.ai.onBuilderChange({ mount: 0, source: doc({}) });
  h.ai.onBuilderChange({ mount: 0, source: doc({ a: 'https://e/a.jpg' }) });
  h.ai.onBuilderChange({ mount: 0, source: doc({ a: 'https://e/a.jpg', b: 'https://e/b.jpg' }) });
  assert.equal(h.timers.pending().length, 1, 'the second trigger replaced the first timer');
  await h.timers.flush();
  assert.equal(h.requests.length, 1);
  // Two triggers while the request is out: exactly one more run after it settles.
  h.ai.onBuilderChange({ mount: 0, source: doc({ a: 'https://e/a.jpg', b: 'https://e/b.jpg', c: 'https://e/c.jpg' }) });
  await h.timers.flush();
  const four = {
    a: 'https://e/a.jpg', b: 'https://e/b.jpg', c: 'https://e/c.jpg', d: 'https://e/d.jpg',
  };
  h.ai.onBuilderChange({ mount: 0, source: doc(four) });
  await h.timers.flush();
  assert.equal(h.requests.length, 1, 'one request at a time');
  gate.resolve();
  await flushMicro();
  await flushMicro();
  await flushMicro();
  assert.equal(h.requests.length, 2, 'exactly one queued run');
  await flushMicro();
  assert.equal(h.requests.length, 2);
  // Inspect click: the pending debounce is cancelled.
  h.ai.onBuilderChange({ mount: 0, source: doc({ e: 'https://e/e.jpg' }) });
  assert.equal(h.timers.pending().length, 1);
  h.ai.onInspect();
  assert.equal(h.timers.pending().length, 0);
  await h.timers.flush();
  assert.equal(h.requests.length, 2);
});

test('I10: nothing triggers while the campaign is not visual, not editable, or its attribs text does not parse', async () => {
  [{ contentType: 'richtext' }, { editable: false }, { attribsStr: '{ not json' }].forEach((page) => {
    const h = harness({ page });
    arrive(h);
    h.ai.onLangChange('es', 'fr');
    assert.equal(h.timers.pending().length, 0, JSON.stringify(page));
  });
});

test('I10: a language change triggers; the snapshot is the form\'s source, body, attribs {lang, alt_generated}, status, name', async () => {
  const h = harness();
  h.ai.onBuilderChange({ mount: 0, source: h.page.source });
  h.page.lang = 'fr';
  h.ai.onLangChange('es', 'fr');
  assert.equal(h.timers.pending().length, 1);
  await h.timers.flush();
  assert.deepEqual(h.requests[0], {
    content_type: 'visual',
    body_source: h.page.source,
    body: '<p>x</p>',
    attribs: { lang: 'fr', alt_generated: {} },
    status: 'draft',
    name: 'C',
  });
});

test('I11: the stamp is written at the change with the language being left; only lang-less entries; a second change stamps nothing', () => {
  const map = {
    'block:a': { src: 'https://e/a.jpg', alt: 'Shop the drop' },
    'block:b': { src: 'https://e/b.jpg', alt: 'Acheter', lang: 'fr' },
  };
  // Not editable: no request follows, the stamp is still written.
  const h = harness({ page: { editable: false, lang: '', attribsStr: JSON.stringify({ preheader: 'P', alt_generated: map }) } });
  h.ai.onLangChange('', 'es');
  assert.deepEqual(JSON.parse(h.page.attribsStr), {
    preheader: 'P',
    alt_generated: { 'block:a': { src: 'https://e/a.jpg', alt: 'Shop the drop', lang: 'en' }, 'block:b': map['block:b'] },
  });
  assert.equal(h.timers.pending().length, 0, 'no request follows');
  const after = h.page.attribsStr;
  h.ai.onLangChange('es', 'de');
  assert.equal(h.page.attribsStr, after, 'the first language left wins');
  // The same normalised language ('' and en): no stamp.
  const same = harness({ page: { attribsStr: JSON.stringify({ alt_generated: { 'block:a': { src: 's', alt: 'a' } } }) } });
  const before = same.page.attribsStr;
  same.ai.onLangChange('', 'en');
  assert.equal(same.page.attribsStr, before);
  assert.equal(same.timers.pending().length, 0);
  // Unparseable attribs text: no stamp and no run.
  const bad = harness({ page: { attribsStr: '{"alt_generated": ' } });
  bad.ai.onLangChange('en', 'es');
  assert.equal(bad.page.attribsStr, '{"alt_generated": ');
  assert.equal(bad.timers.pending().length, 0);
});

test('I7: an applied answer writes generatedMap(keep, applied, at, lang) -- the new entries carry the answer\'s lang -- and toasts the count', async () => {
  const h = harness({ page: { attribsStr: JSON.stringify({ lang: 'es', preheader: 'P', alt_generated: {} }) } });
  h.setAnswer(() => ({
    status: 'ok',
    fixes: [{
      kind: 'alt.set', block: 'b', from: '', to: 'Botella', undecided: true, src: 'https://e/b.jpg',
    }],
    keep: { 'block:a': { src: 'https://e/a.jpg', alt: 'Kept', lang: 'es' } },
    rejected: 0,
    at: AT,
    lang: 'es',
  }));
  arrive(h);
  await h.timers.flush();
  assert.equal(h.applied.length, 1);
  assert.deepEqual(JSON.parse(h.page.attribsStr), {
    lang: 'es',
    preheader: 'P',
    alt_generated: {
      'block:a': { src: 'https://e/a.jpg', alt: 'Kept', lang: 'es' },
      'block:b': {
        src: 'https://e/b.jpg', alt: 'Botella', at: AT, lang: 'es',
      },
    },
  });
  assert.deepEqual(h.toasts, [1]);
});

test('no applied fix: the map is written only when keep differs; a stale bundle (applyAlts null) does nothing', async () => {
  const keepSame = harness({ page: { attribsStr: JSON.stringify({ alt_generated: { 'block:a': { src: 's', alt: 'A' } } }) } });
  keepSame.setAnswer(() => ({
    status: 'none', fixes: [], keep: { 'block:a': { src: 's', alt: 'A' } }, rejected: 0, at: AT, lang: 'es',
  }));
  const before = keepSame.page.attribsStr;
  arrive(keepSame);
  await keepSame.timers.flush();
  assert.equal(keepSame.page.attribsStr, before);
  assert.deepEqual(keepSame.toasts, []);

  const dropped = harness({ page: { attribsStr: JSON.stringify({ alt_generated: { 'block:gone': { src: 's', alt: 'A' } } }) } });
  dropped.setAnswer(() => ({
    status: 'none', fixes: [], keep: {}, rejected: 0, at: AT, lang: 'es',
  }));
  arrive(dropped);
  await dropped.timers.flush();
  assert.deepEqual(mapOf(dropped), {});
  assert.deepEqual(dropped.toasts, []);

  const skippedAll = harness({ applyAlts: (fixes) => ({ applied: [], skipped: fixes }) });
  const b0 = skippedAll.page.attribsStr;
  arrive(skippedAll);
  await skippedAll.timers.flush();
  assert.equal(skippedAll.page.attribsStr, b0);
  assert.deepEqual(skippedAll.toasts, []);

  const stale = harness({ applyAlts: () => null });
  const b1 = stale.page.attribsStr;
  arrive(stale);
  await stale.timers.flush();
  assert.equal(stale.page.attribsStr, b1);
  assert.deepEqual(stale.toasts, []);
});

// I16: one case per clause. `during` changes the page while the request is out.
const CLAUSES = [
  ['a', 'the editor mount changed', (p) => { p.mount = 9; }, false],
  ['b', 'the page is saving', (p) => { p.saving = true; }, true],
  ['b', 'the page is syncing', (p) => { p.syncing = true; }, true],
  ['b', 'the page is inspecting', (p) => { p.inspecting = true; }, true],
  ['c', 'the campaign is no longer editable', (p) => { p.editable = false; }, false],
  ['c', 'the attribs text no longer parses', (p) => { p.attribsStr = '{'; }, false],
  ['d', 'the form\'s map changed', (p) => { p.attribsStr = JSON.stringify({ lang: 'es', alt_generated: { 'block:x': { src: 's', alt: 'typed' } } }); }, true],
  ['e', 'the form\'s language changed', (p) => { p.lang = 'fr'; }, true],
];

CLAUSES.forEach(([clause, what, during, queues]) => {
  test(`I16 (${clause}): ${what} -> the answer is dropped whole${queues ? ', one run queued for when the page settles' : ', nothing queued'}`, async () => {
    const h = harness();
    const gate = deferred();
    h.setAnswer(() => gate.p.then(() => ({
      status: 'ok',
      fixes: [{
        kind: 'alt.set', block: 'b', from: '', to: 'Botella', undecided: true, src: 'https://e/b.jpg',
      }],
      keep: {},
      rejected: 0,
      at: AT,
      lang: 'es',
    })));
    arrive(h);
    await h.timers.flush();
    assert.equal(h.requests.length, 1);
    during(h.page);
    const attribsAtDrop = h.page.attribsStr;
    gate.resolve();
    await flushMicro();
    await flushMicro();
    assert.equal(h.applied.length, 0, 'no fix applied');
    assert.equal(h.page.attribsStr, attribsAtDrop, 'no map written');
    assert.deepEqual(h.toasts, [], 'no toast');
    assert.equal(h.timers.pending().length, queues ? 1 : 0);
    if (queues) {
      // Still unsettled: the poll waits. Then the page settles: one run.
      h.setAnswer(() => ({
        status: 'none', fixes: [], keep: {}, rejected: 0, at: AT, lang: normLang(h.page.lang),
      }));
      Object.assign(h.page, { saving: false, syncing: false, inspecting: false });
      await h.timers.flush();
      assert.equal(h.requests.length, 2, 'one queued run');
      assert.equal(h.timers.pending().length, 0);
    }
  });
});

test('I16 (f): a failed or non-ok answer -> dropped whole, nothing queued', async () => {
  const answers = [
    () => Promise.reject(new Error('502')),
    () => null,
    () => ({
      status: 'timeout', fixes: [], keep: {}, rejected: 0, lang: 'es',
    }),
    () => ({
      status: 'unavailable',
      fixes: [{
        kind: 'alt.set', block: 'b', from: '', to: 'x', undecided: true, src: 'https://e/b.jpg',
      }],
      keep: {},
      rejected: 0,
      lang: 'es',
    }),
  ];
  // eslint-disable-next-line no-restricted-syntax
  for (const answer of answers) {
    const h = harness({ answer });
    const before = h.page.attribsStr;
    arrive(h);
    // eslint-disable-next-line no-await-in-loop
    await h.timers.flush();
    assert.equal(h.requests.length, 1);
    assert.equal(h.applied.length, 0);
    assert.equal(h.page.attribsStr, before);
    assert.deepEqual(h.toasts, []);
    assert.equal(h.timers.pending().length, 0);
  }
});

test('I16: a settled page with every clause holding uses the answer', async () => {
  const h = harness();
  arrive(h);
  await h.timers.flush();
  assert.equal(h.applied.length, 1);
  assert.deepEqual(h.toasts, [1]);
});

test('I12: the in-editor sequence never saves -- the only API it touches is generateAltsDraft, on every path', async () => {
  const all = [];
  const run = async (over, during) => {
    const h = harness(over);
    arrive(h);
    if (during) during(h.page);
    h.ai.onLangChange('es', 'fr');
    h.page.lang = 'fr';
    await h.timers.flush();
    await h.timers.flush();
    h.ai.onInspect();
    h.ai.destroy();
    all.push(...h.touched);
  };
  await run({});
  await run({ answer: () => Promise.reject(new Error('x')) });
  await run({ applyAlts: () => null });
  await run({}, (p) => { p.saving = true; });
  assert.ok(all.length > 0);
  assert.deepEqual([...new Set(all)], ['generateAltsDraft']);
});
