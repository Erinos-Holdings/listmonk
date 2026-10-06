// Fork (review navigation) -- integrations REVIEW-NAVIGATION-SPEC I5 (the early list is
// read-only by construction: every entry `decidable: false`, and only progress items are in it)
// and I14 (the progress contract is cumulative and additive: an older Lambda's `{stage, at}`
// renders without error), plus the §4.6 rows and the structure-key exclusion.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  STAGES, checklist, earlyEntries, openEntries, sectionCounts, sectionOf,
} from './reviewChecklist.mjs'; // eslint-disable-line import/extensions

const t = (k, p) => (p ? `${k} ${JSON.stringify(p)}` : k);
const K = 'campaigns.review.checklist.';
const f = (id, n, extra = {}) => ({
  key: `${id}#${n}`, id, evidence: 'e', location: 'l', todo: 't', ...extra,
});
const D = (id, verdict, findings = []) => ({
  id, tier: 'D', title: id, verdict, acceptable: true, findings,
});
const A = (id, sev) => ({
  id, tier: 'A', title: id, verdict: 'finding', acceptable: true, findings: [f(id, 0, { severity: sev })],
});
const DITEMS = [
  D('D1.1', 'pass'),
  D('D2.4', 'fail', [f('D2.4', 1), f('D2.4', 2)]),
  D('D4.6', 'warn', [f('D4.6', 1)]),
  D('D4.2', 'fail', [{ ...f('D4.2', 0), key: 'R#abc' }]),
  D('D5.1', 'n/a'),
];
const REPORT = {
  items: [...DITEMS, A('A1.1', 'critical'), A('A2.4', 'high'), A('A4.5', 'medium')],
  ai: { status: 'ok', calls: [{}, {}, {}, {}] },
  provenance: { screenshots: 4 },
};

test('the stage order is the Lambda\'s', () => {
  assert.deepEqual(STAGES, ['reading', 'deterministic', 'screenshots', 'ai', 'writing']);
});

test('the section rule, and the structure key excluded from entries and counts', () => {
  assert.equal(sectionOf({ tier: 'D', verdict: 'fail' }, {}), 'blockers');
  assert.equal(sectionOf({ tier: 'D', verdict: 'warn' }, {}), 'advisory');
  assert.equal(sectionOf({ tier: 'A' }, { severity: 'critical' }), 'blockers');
  assert.equal(sectionOf({ tier: 'A' }, { severity: 'high' }), 'acknowledge');
  assert.equal(sectionOf({ tier: 'A' }, { severity: 'medium' }), 'advisory');
  assert.ok(openEntries(DITEMS).every((e) => !e.key.startsWith('R')));
  assert.deepEqual(sectionCounts(DITEMS), { blockers: 2, acknowledge: 0, advisory: 1 });
  assert.deepEqual(sectionCounts(REPORT.items), { blockers: 3, acknowledge: 1, advisory: 2 });
});

test('§4.6: the rows fill in stage by stage, from the cumulative progress', () => {
  const rows = (progress) => checklist({
    progress, report: null, phase: 'running', t,
  });
  let r = rows({ stage: 'reading', at: 'x' });
  assert.deepEqual(r.map((x) => x.state), ['current', 'pending', 'pending', 'pending', 'pending']);
  assert.equal(r[0].spinner, true);
  assert.equal(r[1].label, `${K}checksUnknown`);
  r = rows({ stage: 'deterministic', at: 'x', checks: 55 });
  assert.deepEqual(r.map((x) => x.state), ['done', 'current', 'pending', 'pending', 'pending']);
  assert.equal(r[0].label, `${K}readingDone`);
  assert.equal(r[1].label, `${K}deterministic {"checks":55}`);
  r = rows({
    stage: 'screenshots', at: 'x', checks: 55, items: DITEMS,
  });
  assert.equal(r[1].checked, true);
  assert.equal(r[1].label, `${K}deterministicDone {"checks":55,"b":2,"a":1}`);
  r = rows({
    stage: 'ai', at: 'x', checks: 55, items: DITEMS, screenshots: 4,
  });
  assert.equal(r[2].label, `${K}screenshotsDone {"n":4}`);
  assert.equal(rows({
    stage: 'ai', at: 'x', checks: 55, items: DITEMS, screenshots: 0,
  })[2].label, `${K}screenshotsNone`);
  r = rows({
    stage: 'writing', at: 'x', checks: 55, items: DITEMS, screenshots: 4, ai: { calls: 4, status: 'ok' },
  });
  assert.equal(r[3].label, `${K}aiDone {"calls":4}`);
  assert.equal(r[4].state, 'current');
  assert.equal(rows({
    stage: 'writing', at: 'x', ai: { calls: 4, status: 'partial' },
  })[3].label, `${K}aiStatus {"status":"partial"}`);
});

test('§4.6: complete -> every row checked, the writing row counts the report with the window\'s rule', () => {
  const r = checklist({
    progress: {
      stage: 'writing', at: 'x', checks: 55, items: DITEMS, screenshots: 4, ai: { calls: 4, status: 'ok' },
    },
    report: REPORT,
    phase: 'complete',
    t,
  });
  assert.ok(r.every((x) => x.checked && !x.spinner));
  assert.equal(r[4].label, `${K}writingDone {"b":3,"ack":1,"adv":2}`);
  assert.equal(r[1].label, `${K}deterministicDone {"checks":55,"b":2,"a":1}`);
});

test('a sectionsOf passed in is the one used (the window\'s own rule)', () => {
  const r = checklist({
    progress: { stage: 'screenshots', items: DITEMS, checks: 5 },
    phase: 'running',
    sectionsOf: () => ({ blockers: 9, acknowledge: 8, advisory: 7 }),
    t,
  });
  assert.equal(r[1].label, `${K}deterministicDone {"checks":5,"b":9,"a":7}`);
});

test('I14: an older Lambda\'s {stage, at} (and no progress at all) renders, never throws', () => {
  [{ stage: 'ai', at: 'x' }, { stage: 'bogus' }, null, undefined, 'x', {}].forEach((progress) => {
    const r = checklist({
      progress, report: null, phase: 'running', t,
    });
    assert.equal(r.length, 5);
    assert.ok(r.every((x) => typeof x.label === 'string' && x.label));
  });
  const old = checklist({
    progress: { stage: 'ai', at: 'x' }, report: null, phase: 'running', t,
  });
  assert.deepEqual(old.map((x) => x.state), ['done', 'done', 'done', 'current', 'pending']);
  assert.equal(old[1].label, `${K}checksUnknown`, 'pending label when the count is unknown');
  assert.equal(old[2].label, `${K}screenshots`);
  // A complete report from an older Lambda: the final counts from the report itself.
  const done = checklist({
    progress: { stage: 'writing', at: 'x' }, report: REPORT, phase: 'complete', t,
  });
  assert.equal(done[1].label, `${K}deterministicDone {"checks":5,"b":2,"a":1}`);
  assert.equal(done[2].label, `${K}screenshotsDone {"n":4}`);
  assert.equal(done[3].label, `${K}aiDone {"calls":4}`);
  assert.equal(done[4].label, `${K}writingDone {"b":3,"ack":1,"adv":2}`);
  assert.equal(checklist({
    progress: null, report: { items: 'nope' }, phase: 'complete', t,
  }).length, 5);
});

test('halted (failed/stale): the rows stay as at the last PATCH and nothing spins', () => {
  const r = checklist({
    progress: { stage: 'screenshots', checks: 55, items: DITEMS }, report: REPORT, phase: 'halted', t,
  });
  assert.deepEqual(r.map((x) => x.state), ['done', 'done', 'current', 'pending', 'pending']);
  assert.ok(r.every((x) => !x.spinner));
});

test('I5: the early list is the progress D items only, from screenshots on, every entry undecidable', () => {
  assert.deepEqual(earlyEntries({ progress: { stage: 'deterministic', items: DITEMS }, phase: 'running' }), []);
  const early = earlyEntries({ progress: { stage: 'screenshots', items: [...DITEMS, A('A1.1', 'critical')] }, phase: 'running' });
  assert.deepEqual(early.map((e) => e.key), ['D2.4#1', 'D2.4#2', 'D4.6#1'], 'D only, structure key excluded');
  assert.ok(early.every((e) => e.decidable === false));
  assert.deepEqual(early.map((e) => e.section), ['blockers', 'blockers', 'advisory']);
  assert.deepEqual(earlyEntries({ progress: { stage: 'ai', items: DITEMS }, phase: 'complete' }), [], 'the report replaces them');
  assert.deepEqual(earlyEntries({ progress: { stage: 'ai' }, phase: 'running' }), [], 'an older Lambda');
  assert.deepEqual(earlyEntries({ progress: null }), []);
});

test('I5: the window stages only report entries -- its `entries` reads the report, the early list is separate', () => {
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const entries = /entries\(\) \{([\s\S]*?)\n {4}\},/.exec(src);
  assert.ok(entries, 'CampaignReview.vue has an entries() computed');
  assert.match(entries[1], /this\.report/);
  assert.doesNotMatch(entries[1], /progress/);
  // The early findings render without any of the three buttons.
  const early = /<!-- EARLY FINDINGS -->([\s\S]*?)<!-- \/EARLY FINDINGS -->/.exec(src);
  assert.ok(early, 'the early findings block is marked');
  assert.doesNotMatch(early[1], /btn-fix-for-me|btn-ill-fix-it|btn-accept|stage\(/);
});
