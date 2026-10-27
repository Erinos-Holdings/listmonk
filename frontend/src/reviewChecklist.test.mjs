// Fork (review navigation) -- integrations REVIEW-NAVIGATION-SPEC I5 (the early list is
// read-only by construction: every entry `decidable: false`, and only progress items are in it)
// and I14 (the progress contract is cumulative and additive: an older Lambda's `{stage, at}`
// renders without error), plus the §4.6 rows and the structure-key exclusion.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  STAGES, altsSection, altsStep, auxLayout, auxSections, checklist, earlyEntries, isContextConcern, openEntries, sectionCounts, sectionOf, withAltsStep,
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

// ------------------------------------------------------------------ polish pass (2026-10-06)

const P = 'campaigns.review.pill.';
const keys = (list) => list.map((s) => `${s.key}:${s.status}`);
const okStructure = { coverage: { verified: true, plan: { available: true } } };
const OK_REPORT = {
  items: [],
  context: [{ label: 'SES', value: 'ok' }, { label: 'Blocklists', value: 'none' }, { label: 'Recipients', value: '1204' }],
  ai: { status: 'ok', calls: [{ status: 'ok' }, { status: 'cached' }] },
};

test('auxSections: all ok -> Context, Structure, Provenance in that order, closed, Passed pills', () => {
  const s = auxSections({ report: OK_REPORT, structure: okStructure, t });
  assert.deepEqual(keys(s), ['context:ok', 'structure:ok', 'provenance:ok']);
  assert.ok(s.every((x) => x.open === false && x.pill === `${P}passed` && x.pillType === 'is-success'));
  assert.equal(s[0].highlight.size, 0);
});

test('auxSections: Structure -- verified ok, unavailable warn, else fail; the legacy block too', () => {
  const st = (coverage) => auxSections({ report: OK_REPORT, structure: { coverage }, t }).find((x) => x.key === 'structure');
  assert.equal(st({ verified: true }).status, 'ok');
  const un = st({ verified: false, plan: { available: false } });
  assert.deepEqual([un.status, un.pill, un.pillType, un.open], ['warn', `${P}unavailable`, 'is-warning', true]);
  const uv = st({ verified: false, plan: { available: true } });
  assert.deepEqual([uv.status, uv.pill, uv.pillType, uv.open], ['fail', `${P}unverified`, 'is-danger', true]);
  const legacy = (structure) => auxSections({ report: { ...OK_REPORT, structure }, structure: null, t }).find((x) => x.key === 'structure');
  assert.equal(legacy({ verified: { verified_at: 'x', test_id: 'y' } }).status, 'ok');
  assert.equal(legacy({ verified: null, differs: ['a'] }).status, 'fail');
  assert.equal(auxSections({ report: OK_REPORT, structure: null, t }).find((x) => x.key === 'structure'), undefined, 'no structure at all -> no section');
});

test('auxSections: Context concerns -- not ok, not none, not numeric; highlighted by index', () => {
  ['ok', 'OK', ' none ', '0', '12', '3.5', '-1'].forEach((v) => assert.equal(isContextConcern(v), false, v));
  ['issues', 'warn', 'unknown', 'c67 running, c88 running', ''].forEach((v) => assert.equal(isContextConcern(v), true, v));
  const report = {
    ...OK_REPORT,
    context: [{ label: 'SES', value: 'ok' }, { label: 'DNSBL', value: 'issues' }, { label: 'n', value: '4' }, { label: 'Running', value: 'c67, c88' }],
  };
  const c = auxSections({ report, structure: okStructure, t }).find((x) => x.key === 'context');
  assert.deepEqual([c.status, c.pill, c.open], ['warn', `${P}attention {"n":2}`, true]);
  assert.deepEqual([...c.highlight], [1, 3]);
});

test('auxSections: Provenance by ai.status; the not-ok/cached call lines highlighted', () => {
  const prov = (ai) => auxSections({ report: { ...OK_REPORT, ai }, structure: okStructure, t }).find((x) => x.key === 'provenance');
  const partial = prov({ status: 'partial', calls: [{ status: 'ok' }, { status: 'invalid' }, { status: 'cached' }, { status: 'skipped' }] });
  assert.deepEqual([partial.status, partial.pill, partial.pillType], ['warn', `${P}aiPartial`, 'is-warning']);
  assert.deepEqual([...partial.highlight], [1, 3]);
  const down = prov({ status: 'unavailable', calls: [{ status: 'unavailable' }] });
  assert.deepEqual([down.status, down.pill, down.pillType, down.open], ['fail', `${P}aiUnavailable`, 'is-danger', true]);
});

test('auxSections: ordered fail, then warn, then ok (stable within a status)', () => {
  const report = {
    ...OK_REPORT,
    context: [{ label: 'DNSBL', value: 'issues' }],
    ai: { status: 'unavailable', calls: [] },
  };
  const s = auxSections({ report, structure: { coverage: { verified: false, plan: { available: true } } }, t });
  assert.deepEqual(keys(s), ['structure:fail', 'provenance:fail', 'context:warn']);
  const s2 = auxSections({ report: { ...OK_REPORT, context: [{ label: 'x', value: 'warn' }] }, structure: okStructure, t });
  assert.deepEqual(keys(s2), ['context:warn', 'structure:ok', 'provenance:ok']);
});

test('auxSections: an old report with no coverage, no ai and no context renders Provenance only; no report -> none', () => {
  const s = auxSections({ report: { items: [] }, structure: null, t });
  assert.deepEqual(keys(s), ['provenance:ok']);
  assert.deepEqual(auxSections({ report: null, structure: null, t }), []);
  assert.deepEqual(auxSections({ report: undefined, structure: undefined, t }), []);
});

test('the item templates: no Evidence line; every btn-goto-block is an <a> wrapping an <img> or a where-badge', () => {
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  assert.doesNotMatch(tpl, /campaigns\.review\.evidence/);
  const early = /<!-- EARLY FINDINGS -->([\s\S]*?)<!-- \/EARLY FINDINGS -->/.exec(tpl)[1];
  const reportList = /<section v-for="sec in sections"([\s\S]*?)<\/section>/.exec(tpl)[1];
  // A called-out finding's card shows the rule's failure title (an older report falls back to title);
  // the Passed list keeps the title.
  [early, reportList].forEach((part) => {
    assert.match(part, /\{\{ e\.item\.failTitle \|\| e\.item\.title \}\}/);
    assert.doesNotMatch(part.replace(/e\.item\.failTitle \|\| e\.item\.title/g, ''), /e\.item\.title/);
  });
  const passedList = /<ul class="passed-list[\s\S]*?<\/ul>/.exec(tpl)[0];
  assert.match(passedList, /\{\{ i\.title \}\}/);
  assert.doesNotMatch(passedList, /failTitle/);
  [early, reportList].forEach((part) => {
    const anchors = [...part.matchAll(/<a\b([^>]*)>([\s\S]*?)<\/a>/g)].filter((m) => /btn-goto-block/.test(m[1]));
    assert.equal(anchors.length, 2, 'one image link and one badge link per item template');
    anchors.forEach((m) => assert.ok(/<img\b/.test(m[2]) || /class="where-badge"/.test(m[1]), m[0]));
    // btn-goto-block appears nowhere but on those anchors.
    assert.equal((part.match(/btn-goto-block/g) || []).length, anchors.length);
  });
});

// ------------------------------------------------------------------ polish pass 2 (2026-10-06)

test('checklist: "Running N checks" from the first PATCH; the AI row names the questions when known', () => {
  const rows = (progress, phase = 'running', report = null) => checklist({
    progress, report, phase, t,
  });
  assert.equal(rows({ stage: 'reading', checks: 69 })[1].label, `${K}deterministic {"checks":69}`);
  // `questions` arrives with `checks` on the first PATCH: the AI row names them from the start.
  assert.equal(rows({ stage: 'reading', checks: 69, questions: 29 })[3].label, `${K}aiQuestions {"questions":29}`);
  const ai = rows({
    stage: 'ai', checks: 69, items: DITEMS, screenshots: 4, questions: 29,
  });
  assert.equal(ai[3].label, `${K}aiQuestions {"questions":29}`);
  const done = rows({
    stage: 'writing', checks: 69, items: DITEMS, screenshots: 4, questions: 29, ai: { calls: 4, status: 'ok' },
  });
  assert.equal(done[3].label, `${K}aiQuestionsDone {"questions":29,"calls":4}`);
  // An older Lambda (no `questions`): today's labels.
  assert.equal(rows({ stage: 'ai', checks: 55 })[3].label, `${K}ai`);
  assert.equal(rows({ stage: 'writing', ai: { calls: 4, status: 'ok' } })[3].label, `${K}aiDone {"calls":4}`);
  assert.equal(rows({ stage: 'writing', questions: 29, ai: { calls: 4, status: 'partial' } })[3].label, `${K}aiStatus {"status":"partial"}`);
});

test('D7.1 (campaign context) is excluded from the sections, their counts and the early list', () => {
  const d71 = D('D7.1', 'warn', [f('D7.1', 1)]);
  assert.ok(openEntries([...DITEMS, d71]).every((e) => e.item.id !== 'D7.1'));
  assert.deepEqual(sectionCounts([...DITEMS, d71]), sectionCounts(DITEMS));
  assert.ok(earlyEntries({ progress: { stage: 'screenshots', items: [...DITEMS, d71] }, phase: 'running' }).every((e) => e.item.id !== 'D7.1'));
  assert.equal(isContextConcern("1204 (listmonk's cached count)"), false);
  assert.equal(isContextConcern('88 Fall drop'), true);
});

test('the window: a proposal box with Copy in both card templates; the context list at card size and D7.1 in it; AI glyphs in Passed', () => {
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  const early = /<!-- EARLY FINDINGS -->([\s\S]*?)<!-- \/EARLY FINDINGS -->/.exec(tpl)[1];
  const reportList = /<section v-for="sec in sections"([\s\S]*?)<\/section>/.exec(tpl)[1];
  [early, reportList].forEach((part) => {
    const box = /<div v-if="e\.finding\.proposal" class="proposal"[\s\S]*?<\/div>/.exec(part);
    assert.ok(box, 'a proposal box');
    assert.match(box[0], /\{\{ e\.finding\.proposal \}\}/);
    assert.match(box[0], /btn-copy-proposal[\s\S]*copyText\(e\.finding\.proposal\)/);
    // An icon-only Copy (no text label), named for screen readers and on hover.
    const copy = /<button type="button" class="proposal-copy"[\s\S]*?<\/button>/.exec(box[0]);
    assert.ok(copy, 'an icon-only button.proposal-copy');
    assert.match(copy[0], /:aria-label="\$t\('campaigns\.review\.copyProposal'\)"/);
    assert.match(copy[0], /:title="\$t\('campaigns\.review\.copyProposal'\)"/);
    assert.match(copy[0], /<b-icon icon="file-multiple-outline" size="is-small" \/>/);
    assert.doesNotMatch(copy[0], /structureCopy/);
  });
  // Fix for me: light green when a fix exists (fixType), disabled without one; "I fixed it" by its key.
  assert.match(reportList, /:disabled="!e\.finding\.fix" data-cy="btn-fix-for-me"\s+:type="fixType\(e\)"/);
  const context = /data-cy="section-context"([\s\S]*?)<\/b-collapse>/.exec(tpl)[1];
  const list = /<ul class="context-list"[^>]*>/.exec(context)[0];
  assert.doesNotMatch(list, /is-size-7/);
  assert.doesNotMatch(context, /has-text-warning\b/, 'never yellow text');
  assert.match(context, /stage\(c\.finding\.key, 'D7\.1', 'accept'\)/);
  assert.match(context, /<b-button size="is-small" disabled data-cy="btn-fix-for-me">/);
  assert.match(context, /<b-button size="is-small" disabled data-cy="btn-ill-fix-it">/);
  assert.match(context, /target="_blank" rel="noopener"/);
  // D7.1 out of the cards; its keys in Done.
  const entries = /entries\(\) \{([\s\S]*?)\n {4}\},/.exec(src)[1];
  assert.match(entries, /isContextItem\(item\)/);
  assert.match(src, /contextKeys\(this\.report\)\.every\(decided\)/);
  // Passed: the count in a green tag; an A item carries the AI glyph, a D item none.
  const passed = /data-cy="section-passed"([\s\S]*?)<\/b-collapse>/.exec(tpl)[1];
  assert.match(passed, /<b-tag type="is-success">\{\{ passed\.length \}\}<\/b-tag>/);
  assert.match(passed, /<b-icon v-if="i\.tier === 'A'" icon="creation" size="is-small"/);
});

// ------------------------------------------------------------------ integrations ALT-AUTOGEN-SPEC A1

const live = (phase, extra = {}) => ({
  token: 'r#1', run: 'r', seq: 1, phase, alts: [], skipped: 0, rejected: 0, baseJob: 'job-old', ...extra,
});
const withAlts = (jobId, alts) => ({ ...REPORT, jobId, ...(alts ? { alts } : {}) });
const step = (args) => altsStep({ running: false, t, ...args });
// One report.alts item (A1.3): `block:<id>`, `<id>.jpg`.
const altItem = (id, category, alt, from) => ({
  key: `block:${id}`,
  file: `${id}.jpg`,
  category,
  alt,
  ...(from !== undefined ? { from } : {}),
});

test('A1-I4: a newer live run supersedes a shown older report, until a report of a later job is shown', () => {
  const old = withAlts('job-old', {
    items: [], approved: 3, modified: 0, generated: 0, unfixed: 2, thisInspect: 0,
  });
  // The run began while job-old's report was shown: the live message drives the step.
  assert.deepEqual(step({ live: live('generating'), report: old, reportJob: 'job-old' }), {
    key: 'alts', state: 'current', checked: false, spinner: true, label: `${K}altsRunning`,
  });
  assert.equal(step({ live: live('done', { alts: ['a', 'b'] }), report: old, reportJob: 'job-old' }).label, `${K}altsDone {"n":2}`);
  // No report shown (the generation save changed the hash; the new job runs): still the live run.
  const running = step({
    live: live('done', { alts: ['a'] }), report: null, reportJob: null, running: true,
  });
  assert.equal(running.label, `${K}altsDone {"n":1}`);
  // The new job's report arrives: the report decides (rule 3).
  const fresh = withAlts('job-new', {
    items: [], approved: 4, modified: 1, generated: 0, unfixed: 0, thisInspect: 1,
  });
  assert.equal(step({ live: live('done', { alts: [] }), report: fresh, reportJob: 'job-new' }).label, `${K}altsDone {"n":1}`);
  // A run whose base was not known yet (the window had not loaded): the live run applies.
  assert.equal(step({ live: live('generating', { baseJob: undefined }), report: fresh, reportJob: 'job-new' }).state, 'current');
});

test('A1-I4: live phases -- done with 0 is "checked", a done with rejected > 0 and a failed are the warning state', () => {
  assert.equal(step({ live: live('done'), report: null }).label, `${K}altsChecked`);
  const rej = step({ live: live('done', { alts: ['a'], rejected: 2 }), report: null });
  assert.equal(rej.state, 'warn');
  assert.equal(rej.checked, false);
  assert.equal(rej.label, `${K}altsFailed · ${K}altsStillNeed {"n":2}`);
  assert.deepEqual(step({ live: live('failed'), report: null }), {
    key: 'alts', state: 'warn', checked: false, spinner: false, label: `${K}altsFailed`,
  });
});

test('A1-I4: no live message -- a running job is "checked"; from a report, failed iff unfixed > 0, else N = thisInspect; no step without alts', () => {
  assert.equal(step({ live: null, report: null, running: true }).label, `${K}altsChecked`);
  const r = (alts) => step({ live: null, report: withAlts('j', alts), reportJob: 'j' });
  assert.equal(r({
    items: [], approved: 1, modified: 0, generated: 0, unfixed: 1, thisInspect: 0,
  }).label, `${K}altsFailed · ${K}altsStillNeed {"n":1}`);
  assert.equal(r({
    items: [], approved: 1, modified: 1, generated: 2, unfixed: 0, thisInspect: 3,
  }).label, `${K}altsDone {"n":3}`);
  assert.equal(r({
    items: [], approved: 1, modified: 0, generated: 0, unfixed: 0, thisInspect: 0,
  }).label, `${K}altsChecked`);
  assert.equal(r(null), null);
  assert.equal(step({ live: null, report: null }), null);
});

test('A1-I4: the step is first in the list; alone before the job exists', () => {
  const rows = checklist({ progress: { stage: 'reading' }, report: null, t });
  const s = step({ live: live('generating'), report: null });
  assert.deepEqual(withAltsStep(s, rows).map((x) => x.key), ['alts', ...STAGES]);
  assert.deepEqual(withAltsStep(s, []).map((x) => x.key), ['alts']);
  assert.deepEqual(withAltsStep(null, rows).map((x) => x.key), STAGES);
});

test('A1-I5: auxLayout puts alts directly before passed; none without alts', () => {
  const aux = [{ key: 'structure', status: 'fail' }, { key: 'context', status: 'warn' }, { key: 'provenance', status: 'ok' }];
  assert.deepEqual(auxLayout(aux, true).map((s) => s.key), ['structure', 'context', 'alts', 'passed', 'provenance']);
  assert.deepEqual(auxLayout(aux, false).map((s) => s.key), ['structure', 'context', 'passed', 'provenance']);
  assert.deepEqual(auxLayout([], true).map((s) => s.key), ['alts', 'passed']);
});

test('A1-I5: the section -- total = approved + modified + generated, groups in order, empty omitted, absent without alts, the blank new alt worded the same in both groups', () => {
  const tt = (k) => k;
  const report = withAlts('j', {
    items: [
      altItem('g', 'generated', ''),
      altItem('a', 'approved', 'Shop'),
      altItem('u', 'unfixed', 'photo'),
      altItem('m', 'modified', '', 'photo'),
      altItem('n', 'modified', 'New words', ''),
      altItem('d', 'approved', ''),
    ],
    approved: 2,
    modified: 2,
    generated: 1,
    unfixed: 1,
    thisInspect: 0,
  });
  const s = altsSection(report, tt);
  assert.equal(s.total, 5);
  assert.deepEqual(s.groups.map((g) => [g.key, g.count]), [['approved', 2], ['modified', 2], ['generated', 1]]);
  const blankNew = 'campaigns.review.altTags.blankNew';
  assert.equal(s.groups[1].rows[0].alt, blankNew);
  assert.equal(s.groups[2].rows[0].alt, blankNew);
  assert.deepEqual(s.groups[1].rows[1], {
    key: 'block:n', file: 'n.jpg', alt: 'New words', from: 'campaigns.review.altTags.blank',
  });
  assert.equal(s.groups[0].rows[1].alt, 'campaigns.review.altTags.blank');
  assert.equal(s.groups[1].rows[0].from, 'photo');
  // Empty groups omitted.
  const onlyApproved = altsSection(withAlts('j', {
    items: [altItem('a', 'approved', 'Shop')], approved: 1, modified: 0, generated: 0, unfixed: 0, thisInspect: 0,
  }), tt);
  assert.deepEqual(onlyApproved.groups.map((g) => g.key), ['approved']);
  // Absent: no alts, or no images in scope.
  assert.equal(altsSection(withAlts('j', null), tt), null);
  assert.equal(altsSection(withAlts('j', {
    items: [], approved: 0, modified: 0, generated: 0, unfixed: 0, thisInspect: 0,
  }), tt), null);
  assert.equal(altsSection(null, tt), null);
});

test('A1: the window -- the alt step lives inside review-checklist (no row above it); the Alt tags section is built like Passed', () => {
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  assert.doesNotMatch(tpl, /review-alts/);
  assert.match(src, /withAltsStep\(this\.altsStepRow, rows\)/);
  assert.match(src, /auxLayout\(aux, !!this\.altTags\)/);
  const alts = /data-cy="section-alts"([\s\S]*?)<\/b-collapse>/.exec(tpl)[1];
  assert.match(alts, /class="expand-btn"/);
  assert.match(alts, /<b-tag type="is-success">\{\{ altTags\.total \}\}<\/b-tag>/);
  assert.match(tpl, /<b-collapse v-if="s\.key === 'alts'" :key="s\.key" :open="false" class="review-section"/);
});
