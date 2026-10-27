// Fork (campaign review) -- integrations ALT-AUTOGEN-SPEC I9 (generateAndApply: phases in order; a
// failed call, a builder failure or nothing applied never saves and always returns; a rejected PUT
// notifies `failed`; no fixes with rejected > 0 is not "none needed"; alt_generated = keep + applied
// only; the PUT is reviewPayload; a throwing notify is ignored) and I11 (the window's message rule,
// token de-duplication, the three phases).
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  generateAndApply, generatedMap, acceptAlts, altsRowState, altsMessage, MSG_ALTS,
} from './altGenerate.mjs'; // eslint-disable-line import/extensions
import { altsStep } from './reviewChecklist.mjs'; // eslint-disable-line import/extensions
import { reviewPayload } from './reviewFixes.mjs'; // eslint-disable-line import/extensions
import { CAMPAIGN_UPDATE_KEYS } from './officialSweep.mjs'; // eslint-disable-line import/extensions
import { OPENER_POST_SCHEDULE_MS } from './reviewNavigate.mjs'; // eslint-disable-line import/extensions

const doc = {
  root: { type: 'EmailLayout', data: { childrenIds: ['a', 'b'] } },
  a: { type: 'Image', data: { style: {}, props: { url: 'https://e/a.jpg' } } },
  b: { type: 'Image', data: { style: {}, props: { url: 'https://e/b.jpg', alt: 'Human words' } } },
};
const raw = () => ({
  id: 7,
  name: 'C',
  subject: 'S',
  content_type: 'visual',
  body: '<p>old</p>',
  body_source: JSON.stringify(doc),
  attribs: { lang: 'en', alt_generated: { 'block:gone': { src: 'x', alt: 'y' } } },
  lists: [{ id: 15 }],
  media: [],
  headers: [],
  archive_meta: {},
});
const fixA = {
  kind: 'alt.set', block: 'a', from: '', to: 'Shop the drop', undecided: true, src: 'https://e/a.jpg',
};
const fixStale = {
  kind: 'alt.set', block: 'b', from: 'Not the current alt', to: 'X', src: 'https://e/b.jpg',
};
// The Lambda's answer time (ALT-AUTOGEN-SPEC A1.4).
const AT = '2026-10-27T18:00:00.000Z';
const t = (k, p) => (p ? `${k}${JSON.stringify(p)}` : k);

function harness(over = {}) {
  const calls = [];
  const notes = [];
  const api = {
    generateAlts: async (id) => {
      calls.push(['generateAlts', id]);
      return {
        status: 'ok', fixes: [fixA, fixStale], keep: { 'block:k': { src: 'k.jpg', alt: 'Kept' } }, rejected: 1, at: AT,
      };
    },
    getCampaignRaw: async (id) => {
      calls.push(['getCampaignRaw', id]);
      return raw();
    },
    updateCampaign: async (id, payload) => {
      calls.push(['updateCampaign', id, payload]);
      return {};
    },
    ...(over.api || {}),
  };
  const compile = over.compile || (async () => '<p>new</p>');
  const notify = over.notify || ((m) => notes.push(m));
  const deps = {
    id: 7, api, compile, notify,
  };
  return { calls, notes, deps };
}

test('I9: the phases in order; the PUT is reviewPayload with alt_generated = keep + applied only', async () => {
  const h = harness();
  const out = await generateAndApply(h.deps);
  assert.deepEqual(out, { saved: true, phase: 'done' });
  assert.deepEqual(h.notes.map((n) => n.phase), ['generating', 'done']);
  assert.deepEqual(h.notes[1], {
    phase: 'done', alts: [{ to: 'Shop the drop', block: 'a' }], skipped: 1, rejected: 1,
  });
  const put = h.calls.find((c) => c[0] === 'updateCampaign')[2];
  assert.deepEqual(Object.keys(put), [...CAMPAIGN_UPDATE_KEYS]);
  assert.equal(put.body, '<p>new</p>');
  // keep + the applied fix; the skipped (stale `from`) fix records nothing; the stored map is replaced by keep.
  // A1.4: the new entry carries the answer's `at`, and no `from` (the image was undecided).
  assert.deepEqual(put.attribs.alt_generated, {
    'block:k': { src: 'k.jpg', alt: 'Kept' },
    'block:a': { src: 'https://e/a.jpg', alt: 'Shop the drop', at: AT },
  });
  assert.equal(put.attribs.lang, 'en');
  assert.equal(JSON.parse(put.body_source).a.data.props.alt, 'Shop the drop');
  // The PUT is exactly reviewPayload over the fixed campaign.
  assert.deepEqual(Object.keys(put), Object.keys(reviewPayload(raw(), '')));
});

test('I9: a failed call, a non-ok status, or a throw notifies failed, never saves, always returns', async () => {
  const answers = [
    async () => { throw new Error('502'); },
    async () => ({ status: 'timeout', fixes: [], keep: {} }),
    async () => ({ status: 'unavailable', fixes: [fixA], keep: {} }),
    async () => null,
  ];
  await Promise.all(answers.map(async (generateAlts) => {
    const h = harness({ api: { generateAlts } });
    const out = await generateAndApply(h.deps);
    assert.deepEqual(out, { saved: false, phase: 'failed' });
    assert.deepEqual(h.notes.map((n) => n.phase), ['generating', 'failed']);
    assert.equal(h.calls.some((c) => c[0] === 'updateCampaign'), false);
  }));
});

test('I9: no fixes -> done with no save; rejected > 0 is not "none needed"', async () => {
  const none = harness({ api: { generateAlts: async () => ({ status: 'none', fixes: [], keep: {} }) } });
  await generateAndApply(none.deps);
  assert.deepEqual(none.notes[1], { phase: 'done', alts: [], rejected: 0 });
  assert.equal(none.calls.some((c) => c[0] === 'getCampaignRaw' || c[0] === 'updateCampaign'), false);
  const step = (m) => altsStep({
    live: altsRowState(null, altsMessage(7, 'r', 2, m), null), report: null, reportJob: null, running: false, t,
  });
  assert.equal(step(none.notes[1]).state, 'done');
  assert.equal(step(none.notes[1]).label, 'campaigns.review.checklist.altsChecked');

  const rejAnswer = {
    status: 'ok', fixes: [], keep: {}, rejected: 2,
  };
  const rej = harness({ api: { generateAlts: async () => rejAnswer } });
  await generateAndApply(rej.deps);
  assert.deepEqual(rej.notes[1], { phase: 'done', alts: [], rejected: 2 });
  // A1.1: a `done` with rejected > 0 is the warning state with the note, never "checked".
  assert.equal(step(rej.notes[1]).state, 'warn');
  assert.match(step(rej.notes[1]).label, /altsStillNeed\{"n":2\}/);
});

test('I9: nothing applied -> done with skipped, no save', async () => {
  const h = harness({ api: { generateAlts: async () => ({ status: 'ok', fixes: [fixStale], keep: {} }) } });
  const out = await generateAndApply(h.deps);
  assert.deepEqual(out, { saved: false, phase: 'done' });
  assert.deepEqual(h.notes[1], {
    phase: 'done', alts: [], skipped: 1, rejected: 0,
  });
  assert.equal(h.calls.some((c) => c[0] === 'updateCampaign'), false);
});

test('I9: a builder failure never saves; a rejected PUT notifies failed', async () => {
  await Promise.all([async () => { throw new Error('no EmailBuilder'); }, async () => null].map(async (compile) => {
    const h = harness({ compile });
    assert.deepEqual(await generateAndApply(h.deps), { saved: false, phase: 'failed' });
    assert.equal(h.calls.some((c) => c[0] === 'updateCampaign'), false);
  }));
  const put = harness({ api: { updateCampaign: async () => { throw new Error('Unschedule the campaign first'); } } });
  assert.deepEqual(await generateAndApply(put.deps), { saved: false, phase: 'failed' });
  assert.deepEqual(put.notes.map((n) => n.phase), ['generating', 'failed']);
});

test('I9: a notify that throws (or rejects) is ignored', async () => {
  const h = harness({ notify: () => { throw new Error('window closed'); } });
  assert.deepEqual(await generateAndApply(h.deps), { saved: true, phase: 'done' });
  const r = harness({ notify: () => Promise.reject(new Error('closed')) });
  assert.deepEqual(await generateAndApply(r.deps), { saved: true, phase: 'done' });
});

test('generatedMap: keep + applied alt.set entries only, keyed block:/index:', () => {
  assert.deepEqual(generatedMap({ 'block:k': { src: 's', alt: 'K' } }, [
    {
      kind: 'alt.set', block: 'a', to: '', src: 'a.jpg',
    },
    {
      kind: 'alt.set', index: 2, to: 'Two', src: 'two.jpg',
    },
    { kind: 'subject.trim', from: ' a', to: 'a' },
  ]), {
    'block:k': { src: 's', alt: 'K' },
    'block:a': { src: 'a.jpg', alt: '' },
    'index:2': { src: 'two.jpg', alt: 'Two' },
  });
  assert.deepEqual(generatedMap(null, []), {});
});

test('A1-I2: generatedMap writes from (omitted when undecided) and the answer\'s at (never the browser clock) on new entries', () => {
  const realNow = Date.now;
  Date.now = () => 0; // a browser clock that must never reach the map
  try {
    const kept = {
      src: 's', alt: 'K', from: 'old', at: '2026-10-01T00:00:00.000Z',
    };
    const out = generatedMap({ 'block:k': kept }, [
      {
        kind: 'alt.set', block: 'u', from: '', to: 'New', undecided: true, src: 'u.jpg',
      },
      {
        kind: 'alt.set', block: 'e', from: '', to: 'Was blank', src: 'e.jpg',
      },
      {
        kind: 'alt.set', index: 1, from: 'photo', to: 'Shop', src: 'one.jpg',
      },
    ], AT);
    assert.deepEqual(out, {
      'block:k': kept,
      'block:u': { src: 'u.jpg', alt: 'New', at: AT },
      'block:e': {
        src: 'e.jpg', alt: 'Was blank', from: '', at: AT,
      },
      'index:1': {
        src: 'one.jpg', alt: 'Shop', from: 'photo', at: AT,
      },
    });
    // An answer without `at` (an older Lambda): no `at` at all.
    assert.deepEqual(generatedMap({}, [{
      kind: 'alt.set', block: 'a', from: 'x', to: 'y', src: 'a.jpg',
    }]), { 'block:a': { src: 'a.jpg', alt: 'y', from: 'x' } });
  } finally {
    Date.now = realNow;
  }
});

test('I11: the window accepts only a same-origin lm-review:alts for its campaign from its opener', () => {
  const opener = { name: 'opener' };
  const data = altsMessage(7, 'run1', 1, { phase: 'generating' });
  const ok = {
    origin: 'https://e', ourOrigin: 'https://e', source: opener, opener, data, campaignId: 7,
  };
  assert.equal(acceptAlts(ok), true);
  assert.equal(acceptAlts({ ...ok, origin: 'https://evil' }), false);
  assert.equal(acceptAlts({ ...ok, source: { name: 'other' } }), false);
  assert.equal(acceptAlts({ ...ok, opener: null }), false);
  assert.equal(acceptAlts({ ...ok, campaignId: 8 }), false);
  assert.equal(acceptAlts({ ...ok, data: { ...data, type: 'lm-review:opener' } }), false);
  assert.equal(acceptAlts({ ...ok, data: { ...data, phase: 'weird' } }), false);
  assert.equal(acceptAlts({ ...ok, data: { ...data, token: 'no-seq' } }), false);
  assert.equal(data.type, MSG_ALTS);
});

test('I11: de-duplicates by token; a late re-post of an earlier phase never overwrites; a new run replaces', () => {
  const g = altsMessage(7, 'r1', 1, { phase: 'generating' });
  const d = altsMessage(7, 'r1', 2, {
    phase: 'done', alts: [{ to: 'Shop', block: 'a' }, { to: '', block: 's2' }], skipped: 0, rejected: 0,
  });
  let s = altsRowState(null, g);
  assert.equal(s.phase, 'generating');
  const same = altsRowState(s, g);
  assert.equal(same, s); // the repeated post changes nothing
  s = altsRowState(s, d);
  assert.equal(s.phase, 'done');
  assert.equal(altsRowState(s, g), s); // the late re-post of `generating`
  const next = altsRowState(s, altsMessage(7, 'r2', 1, { phase: 'generating' }));
  assert.equal(next.run, 'r2');
  assert.equal(next.phase, 'generating');
});

test('I11 / A1.1: the three phases render as the checklist\'s alt step', () => {
  const step = (m) => altsStep({
    live: altsRowState(null, altsMessage(7, 'r', 1, m), null), report: null, reportJob: null, running: false, t,
  });
  assert.deepEqual(step({ phase: 'generating' }), {
    key: 'alts', state: 'current', checked: false, spinner: true, label: 'campaigns.review.checklist.altsRunning',
  });
  assert.equal(step({ phase: 'failed' }).state, 'warn');
  assert.equal(step({ phase: 'failed' }).label, 'campaigns.review.checklist.altsFailed');
  const done = step({ phase: 'done', alts: [{ to: 'Shop the drop', block: 'a' }, { to: '', block: 's2' }], skipped: 1 });
  assert.equal(done.state, 'done');
  assert.equal(done.checked, true);
  assert.equal(done.label, 'campaigns.review.checklist.altsDone{"n":2}');
});

test('A1.1: the live state records the shown report\'s job at the run\'s first message and keeps it for the run', () => {
  const s1 = altsRowState(null, altsMessage(7, 'r1', 1, { phase: 'generating' }), 'job-a');
  assert.equal(s1.baseJob, 'job-a');
  const s2 = altsRowState(s1, altsMessage(7, 'r1', 2, { phase: 'done', alts: [] }), 'job-b');
  assert.equal(s2.baseJob, 'job-a');
  const s3 = altsRowState(s2, altsMessage(7, 'r2', 1, { phase: 'generating' }), 'job-b');
  assert.equal(s3.baseJob, 'job-b');
  assert.equal(altsRowState(null, altsMessage(7, 'r', 1, { phase: 'generating' })).baseJob, undefined);
});

test('D9: the alts row shares the opener re-post schedule (one constant)', () => {
  assert.deepEqual([...OPENER_POST_SCHEDULE_MS], [0, 300, 1000, 2500, 5000]);
});
