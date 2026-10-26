// Fork (on-demand brand-health refresh, integrations BRANDHEALTH-REFRESH-SPEC I5). Run:
// node --test src/healthRefresh.test.mjs. No dependency: Node's built-in runner, fake timers.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  CAP_MS, POLL_MS, STATE_MESSAGE, advanced, baselineOf, canRefresh, createRefresh, isActive,
} from './healthRefresh.mjs'; // eslint-disable-line import/extensions

// A fake clock: setTimer queues, tick() advances to the next timer and runs it.
function clock() {
  let t = 0;
  let q = [];
  let id = 0;
  return {
    now: () => t,
    setTimer: (fn, ms) => { id += 1; q.push({ id, at: t + ms, fn }); return id; },
    clearTimer: (x) => { q = q.filter((e) => e.id !== x); },
    pending: () => q.length,
    async tick() {
      q.sort((a, b) => a.at - b.at);
      const e = q.shift();
      if (!e) return false;
      t = e.at;
      await e.fn();
      return true;
    },
  };
}

const flush = () => new Promise((r) => { setImmediate(r); });

function harness({ start, docs }) {
  const c = clock();
  const states = [];
  let reads = 0;
  const r = createRefresh({
    start,
    read: async () => {
      const d = typeof docs === 'function' ? docs(reads) : docs[Math.min(reads, docs.length - 1)];
      reads += 1;
      if (d instanceof Error) throw d;
      return d;
    },
    onState: (s, doc) => states.push(doc ? [s, doc.computedAt] : s),
    now: c.now,
    setTimer: c.setTimer,
    clearTimer: c.clearTimer,
  });
  return {
    r, c, states, reads: () => reads,
  };
}

test('canRefresh: brands:manage AND the configured boolean, nothing else', () => {
  const can = (perms) => (p) => perms.includes(p);
  assert.equal(canRefresh({ health_refresh_enabled: true }, can(['brands:manage'])), true);
  assert.equal(canRefresh({ health_refresh_enabled: true }, can(['brands:get'])), false);
  assert.equal(canRefresh({ health_refresh_enabled: false }, can(['brands:manage'])), false);
  assert.equal(canRefresh({}, can(['brands:manage'])), false);
  assert.equal(canRefresh(null, can(['brands:manage'])), false);
  assert.equal(canRefresh({ health_refresh_enabled: 'true' }, can(['brands:manage'])), false);
  assert.equal(canRefresh({ health_refresh_enabled: true }, undefined), false);
});

test('advanced: strictly later computedAt; no baseline = first appearance', () => {
  assert.equal(advanced('2026-10-26T15:30:12Z', { computedAt: '2026-10-26T18:01:00Z' }), true);
  assert.equal(advanced('2026-10-26T15:30:12Z', { computedAt: '2026-10-26T15:30:12Z' }), false);
  assert.equal(advanced('2026-10-26T15:30:12Z', { computedAt: '2026-10-25T15:30:12Z' }), false);
  assert.equal(advanced('2026-10-26T15:30:12Z', null), false);
  assert.equal(advanced(null, null), false);
  assert.equal(advanced(null, {}), false);
  assert.equal(advanced(null, { computedAt: '2026-10-26T18:01:00Z' }), true);
  assert.equal(baselineOf(null), null);
  assert.equal(baselineOf({}), null);
  assert.equal(baselineOf({ computedAt: '2026-10-26T15:30:12Z' }), '2026-10-26T15:30:12Z');
});

test('started -> polls every 10 s -> done when computedAt advances', async () => {
  const old = { computedAt: '2026-10-26T15:30:12Z' };
  const fresh = { computedAt: '2026-10-26T18:01:05Z' };
  const h = harness({ start: async () => ({ started: true }), docs: [old, old, fresh] });
  await h.r.begin(baselineOf(old));
  assert.deepEqual(h.states, ['starting', 'running']);
  assert.ok(isActive(h.r.state));
  await h.c.tick();
  assert.equal(h.c.now(), POLL_MS);
  await h.c.tick();
  await h.c.tick();
  assert.deepEqual(h.states, ['starting', 'running', ['done', fresh.computedAt]]);
  assert.equal(h.reads(), 3);
  assert.equal(h.c.pending(), 0, 'no poll after done');
  assert.equal(isActive(h.r.state), false);
});

test('no prior document: the first document to appear is the advance', async () => {
  const fresh = { computedAt: '2026-10-26T18:01:05Z' };
  const h = harness({ start: async () => ({ started: true }), docs: [null, fresh] });
  await h.r.begin(baselineOf(null));
  await h.c.tick();
  await h.c.tick();
  assert.deepEqual(h.states.at(-1), ['done', fresh.computedAt]);
});

test('the 3-minute cap: capped, polling stops, button re-enabled', async () => {
  const old = { computedAt: '2026-10-26T15:30:12Z' };
  const h = harness({ start: async () => ({ started: true }), docs: [old] });
  await h.r.begin(old.computedAt);
  while (await h.c.tick()) { /* drain */ } // eslint-disable-line no-await-in-loop
  assert.equal(h.states.at(-1), 'capped');
  assert.equal(h.c.now(), CAP_MS);
  assert.equal(h.reads(), CAP_MS / POLL_MS);
  assert.equal(isActive(h.r.state), false);
  assert.equal(STATE_MESSAGE.capped, 'dashboard.ses.refreshCapped');
});

test('busy (the Lambda slot is taken): "already running" and the same poll', async () => {
  const old = { computedAt: '2026-10-26T15:30:12Z' };
  const fresh = { computedAt: '2026-10-26T15:31:40Z' };
  const h = harness({ start: async () => ({ started: false, busy: true }), docs: [fresh] });
  await h.r.begin(old.computedAt);
  assert.deepEqual(h.states, ['starting', 'busy']);
  assert.equal(STATE_MESSAGE.busy, 'dashboard.ses.refreshBusy');
  await h.c.tick();
  assert.deepEqual(h.states.at(-1), ['done', fresh.computedAt]);
});

test('a start error (400/502) is failed with no poll; a re-click is allowed', async () => {
  let calls = 0;
  const h = harness({ start: async () => { calls += 1; throw new Error('502'); }, docs: [null] });
  await h.r.begin(null);
  assert.deepEqual(h.states, ['starting', 'failed']);
  assert.equal(h.c.pending(), 0);
  await h.r.begin(null);
  assert.equal(calls, 2);
});

test('an unexpected success shape is failed, not a silent poll', async () => {
  const h = harness({ start: async () => ({ started: false }), docs: [null] });
  await h.r.begin(null);
  assert.equal(h.states.at(-1), 'failed');
  assert.equal(h.c.pending(), 0);
});

test('a read error is retried at the next tick', async () => {
  const fresh = { computedAt: '2026-10-26T18:01:05Z' };
  const h = harness({ start: async () => ({ started: true }), docs: [new Error('net'), fresh] });
  await h.r.begin('2026-10-26T15:30:12Z');
  await h.c.tick();
  assert.equal(h.states.at(-1), 'running');
  await h.c.tick();
  assert.deepEqual(h.states.at(-1), ['done', fresh.computedAt]);
});

test('a click while active is ignored (no second POST)', async () => {
  let calls = 0;
  const h = harness({ start: async () => { calls += 1; return { started: true }; }, docs: [null] });
  await h.r.begin(null);
  await h.r.begin(null);
  assert.equal(calls, 1);
});

test('stop() abandons the poll (component destroyed)', async () => {
  const h = harness({ start: async () => ({ started: true }), docs: [{ computedAt: '2026-10-26T18:01:05Z' }] });
  await h.r.begin(null);
  h.r.stop();
  assert.equal(h.c.pending(), 0);
  await flush();
  assert.deepEqual(h.states, ['starting', 'running']);
});

test('stop() during the start leaves no state change after it resolves', async () => {
  let resolve;
  const h = harness({ start: () => new Promise((r) => { resolve = r; }), docs: [null] });
  const p = h.r.begin(null);
  h.r.stop();
  resolve({ started: true });
  await p;
  assert.deepEqual(h.states, ['starting']);
  assert.equal(h.c.pending(), 0);
});

test('every state has its message key; idle has none', () => {
  ['starting', 'running', 'busy', 'done', 'capped', 'failed'].forEach((s) => {
    assert.match(STATE_MESSAGE[s], /^dashboard\.ses\.refresh/);
  });
  assert.equal(STATE_MESSAGE.idle, '');
});
