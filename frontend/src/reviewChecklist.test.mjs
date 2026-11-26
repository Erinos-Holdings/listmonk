// Fork (review navigation) -- integrations REVIEW-NAVIGATION-SPEC I14 (the progress contract is
// cumulative and additive: an older Lambda's `{stage, at}` renders without error), REVIEW-SETTLED-SPEC
// I6 (no finding and no Blocker/Advisory count before the report), plus the §4.6 rows and the
// structure-key exclusion.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import * as checklistModule from './reviewChecklist.mjs'; // eslint-disable-line import/extensions
import {
  STAGES, acceptState, altsSection, altsStep, auxLayout, auxSections, checklist, isContextConcern, isReviewDone, openEntries,
  sectionCounts, sectionOf, splitAccepted, structureAcknowledgeLabel, suggestionAcknowledgements, withAltsStep,
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
  items: [...DITEMS, A('A1.1', 'critical'), A('A2.4', 'high'), A('A4.3', 'medium')],
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
  // integrations REVIEW-CONVENTIONS-SPEC D5.7: the counts carry `suggestions` (0 here -- no A low).
  assert.deepEqual(sectionCounts(DITEMS), {
    blockers: 2, acknowledge: 0, advisory: 1, suggestions: 0,
  });
  assert.deepEqual(sectionCounts(REPORT.items), {
    blockers: 3, acknowledge: 1, advisory: 2, suggestions: 0,
  });
});

// integrations REVIEW-TRIM-2-SPEC D1: the current Lambda's progress -- no `screenshots` stage or key,
// so no Screenshots row. REVIEW-SETTLED-SPEC D5: a done rule-check row reads its count alone while
// the review runs, even when an older Lambda's progress carries the D items.
test('§4.6: the rows fill in stage by stage, from the cumulative progress (no Screenshots row)', () => {
  const rows = (progress) => checklist({
    progress, report: null, phase: 'running', t,
  });
  let r = rows({ stage: 'reading', at: 'x' });
  assert.deepEqual(r.map((x) => x.key), ['reading', 'deterministic', 'ai', 'writing']);
  assert.deepEqual(r.map((x) => x.state), ['current', 'pending', 'pending', 'pending']);
  assert.equal(r[0].spinner, true);
  assert.equal(r[1].label, `${K}checksUnknown`);
  r = rows({ stage: 'deterministic', at: 'x', checks: 55 });
  assert.deepEqual(r.map((x) => x.state), ['done', 'current', 'pending', 'pending']);
  assert.equal(r[0].label, `${K}readingDone`);
  assert.equal(r[1].label, `${K}deterministic {"checks":55}`);
  r = rows({
    stage: 'ai', at: 'x', checks: 55, items: DITEMS,
  });
  assert.deepEqual(r.map((x) => x.state), ['done', 'done', 'current', 'pending']);
  assert.equal(r[1].checked, true);
  assert.equal(r[1].label, `${K}deterministicCount {"checks":55}`);
  r = rows({
    stage: 'writing', at: 'x', checks: 55, items: DITEMS, ai: { calls: 3, status: 'ok' },
  });
  assert.equal(r[2].label, `${K}aiDone {"calls":3}`);
  assert.equal(r[3].state, 'current');
  assert.equal(rows({
    stage: 'writing', at: 'x', ai: { calls: 3, status: 'partial' },
  })[2].label, `${K}aiStatus {"status":"partial"}`);
});

// REVIEW-TRIM-2-SPEC I4: no Screenshots row when the progress/report carries no `screenshots`; the
// row still renders for an older progress or report that does.
test('REVIEW-TRIM-2-SPEC I4: the Screenshots row only where a `screenshots` key is carried', () => {
  const keysOf = (args) => checklist({ t, ...args }).map((x) => x.key);
  const NEW_REPORT = { ...REPORT, provenance: { images: { footerExcluded: 1 }, placement: { unplaced: [] } } };
  assert.ok(!keysOf({ progress: { stage: 'ai', checks: 5, items: DITEMS }, phase: 'running' }).includes('screenshots'));
  assert.ok(!keysOf({ progress: { stage: 'writing' }, report: NEW_REPORT, phase: 'complete' }).includes('screenshots'));
  // An older Lambda: the `ai` PATCH carries `screenshots`, and its report's provenance does.
  const old = checklist({
    progress: {
      stage: 'ai', at: 'x', checks: 55, items: DITEMS, screenshots: 4,
    },
    report: null,
    phase: 'running',
    t,
  });
  assert.equal(old.find((x) => x.key === 'screenshots').label, `${K}screenshotsDone {"n":4}`);
  assert.equal(checklist({
    progress: {
      stage: 'ai', at: 'x', checks: 55, items: DITEMS, screenshots: 0,
    },
    report: null,
    phase: 'running',
    t,
  }).find((x) => x.key === 'screenshots').label, `${K}screenshotsNone`);
  assert.ok(keysOf({ progress: { stage: 'writing' }, report: REPORT, phase: 'complete' }).includes('screenshots'));
  // An older halted progress at `stage: 'screenshots'` still indexes (legacy STAGES key): the rows before it are done.
  const halted = checklist({
    progress: { stage: 'screenshots', checks: 55, items: DITEMS }, report: null, phase: 'halted', t,
  });
  assert.deepEqual(halted.map((x) => [x.key, x.state]), [['reading', 'done'], ['deterministic', 'done'], ['ai', 'pending'], ['writing', 'pending']]);
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
  assert.equal(r[4].label, `${K}writingDone {"b":3,"ack":1,"adv":2,"sug":0}`);
  assert.equal(r[1].label, `${K}deterministicDone {"checks":55,"b":2,"a":1}`);
});

// Integrations REVIEW-SETTLED-SPEC I6: running or halted, the done rule-check row reads
// `deterministicCount` even with `progress.items` present; complete, its counts are the REPORT's D
// items, never `progress.items`.
test('I6: no Blocker/Advisory count before the report; complete counts the report, not progress.items', () => {
  const progress = {
    stage: 'ai', at: 'x', checks: 55, items: DITEMS,
  };
  ['running', 'halted'].forEach((phase) => {
    const r = checklist({
      progress, report: REPORT, phase, t,
    });
    assert.equal(r.find((x) => x.key === 'deterministic').label, `${K}deterministicCount {"checks":55}`, phase);
    assert.ok(r.every((x) => !/Blockers|"b":/.test(x.label)), phase);
  });
  // Complete: progress.items says 2 blockers, 1 advisory; the report's D items one advisory only.
  const report = { ...REPORT, items: [D('D1.1', 'pass'), D('D4.6', 'warn', [f('D4.6', 1)]), A('A2.4', 'high')] };
  const done = checklist({
    progress, report, phase: 'complete', t,
  });
  assert.equal(done.find((x) => x.key === 'deterministic').label, `${K}deterministicDone {"checks":55,"b":0,"a":1}`);
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.equal(en[`${K}deterministicCount`], '{checks} checks');
});

test('I14: an older Lambda\'s {stage, at} (and no progress at all) renders, never throws', () => {
  [{ stage: 'ai', at: 'x' }, { stage: 'bogus' }, null, undefined, 'x', {}].forEach((progress) => {
    const r = checklist({
      progress, report: null, phase: 'running', t,
    });
    // REVIEW-TRIM-2-SPEC D1: no `screenshots` key carried → no Screenshots row.
    assert.equal(r.length, 4);
    assert.ok(r.every((x) => typeof x.label === 'string' && x.label));
  });
  const old = checklist({
    progress: { stage: 'ai', at: 'x' }, report: null, phase: 'running', t,
  });
  assert.deepEqual(old.map((x) => x.state), ['done', 'done', 'current', 'pending']);
  assert.equal(old[1].label, `${K}checksUnknown`, 'pending label when the count is unknown');
  // A complete report from an older Lambda: the final counts from the report itself.
  const done = checklist({
    progress: { stage: 'writing', at: 'x' }, report: REPORT, phase: 'complete', t,
  });
  assert.equal(done[1].label, `${K}deterministicDone {"checks":5,"b":2,"a":1}`);
  assert.equal(done[2].label, `${K}screenshotsDone {"n":4}`);
  assert.equal(done[3].label, `${K}aiDone {"calls":4}`);
  assert.equal(done[4].label, `${K}writingDone {"b":3,"ack":1,"adv":2,"sug":0}`);
  assert.equal(checklist({
    progress: null, report: { items: 'nope' }, phase: 'complete', t,
  }).length, 4);
});

test('halted (failed/stale): the rows stay as at the last PATCH and nothing spins', () => {
  const r = checklist({
    progress: { stage: 'ai', checks: 55, items: DITEMS }, report: REPORT, phase: 'halted', t,
  });
  assert.deepEqual(r.map((x) => x.state), ['done', 'done', 'current', 'pending']);
  assert.ok(r.every((x) => !x.spinner));
});

// REVIEW-SETTLED-SPEC D5 / I6: the window builds no findings while a review runs -- its `entries`
// reads the report only, and the early findings (their block, computed and export) are gone.
test('I6: the window\'s findings come from the report only; no early findings', () => {
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const entries = /entries\(\) \{([\s\S]*?)\n {4}\},/.exec(src);
  assert.ok(entries, 'CampaignReview.vue has an entries() computed');
  assert.match(entries[1], /this\.report/);
  assert.doesNotMatch(entries[1], /progress/);
  assert.doesNotMatch(src, /EARLY FINDINGS|earlySections|earlyEntries|is-early/);
  assert.equal(checklistModule.earlyEntries, undefined);
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
  const reportList = /<section v-for="sec in sections"([\s\S]*?)<\/section>/.exec(tpl)[1];
  // A called-out finding's card shows the rule's failure title (an older report falls back to title);
  // the Passed list keeps the title.
  assert.match(reportList, /\{\{ e\.item\.failTitle \|\| e\.item\.title \}\}/);
  assert.doesNotMatch(reportList.replace(/e\.item\.failTitle \|\| e\.item\.title/g, ''), /e\.item\.title/);
  const passedList = /<ul class="passed-list[\s\S]*?<\/ul>/.exec(tpl)[0];
  assert.match(passedList, /\{\{ i\.title \}\}/);
  assert.doesNotMatch(passedList, /failTitle/);
  const anchors = [...reportList.matchAll(/<a\b([^>]*)>([\s\S]*?)<\/a>/g)].filter((m) => /btn-goto-block/.test(m[1]));
  assert.equal(anchors.length, 2, 'one image link and one badge link per item template');
  anchors.forEach((m) => assert.ok(/<img\b/.test(m[2]) || /class="where-badge"/.test(m[1]), m[0]));
  // btn-goto-block appears nowhere but on those anchors.
  assert.equal((reportList.match(/btn-goto-block/g) || []).length, anchors.length);
});

// ------------------------------------------------------------------ polish pass 2 (2026-10-06)

test('checklist: "Running N checks" from the first PATCH; the AI row names the questions when known', () => {
  // REVIEW-TRIM-2-SPEC D1: rows are read by key (the Screenshots row is present only with the key).
  const rows = (progress, phase = 'running', report = null) => {
    const list = checklist({
      progress, report, phase, t,
    });
    const by = Object.fromEntries(list.map((x) => [x.key, x]));
    return by;
  };
  assert.equal(rows({ stage: 'reading', checks: 69 }).deterministic.label, `${K}deterministic {"checks":69}`);
  // `questions` arrives with `checks` on the first PATCH: the AI row names them from the start.
  assert.equal(rows({ stage: 'reading', checks: 69, questions: 29 }).ai.label, `${K}aiQuestions {"questions":29}`);
  const ai = rows({
    stage: 'ai', checks: 69, items: DITEMS, questions: 29,
  });
  assert.equal(ai.ai.label, `${K}aiQuestions {"questions":29}`);
  const done = rows({
    stage: 'writing', checks: 69, items: DITEMS, questions: 29, ai: { calls: 3, status: 'ok' },
  });
  assert.equal(done.ai.label, `${K}aiQuestionsDone {"questions":29,"calls":3}`);
  // An older Lambda (no `questions`): today's labels.
  assert.equal(rows({ stage: 'ai', checks: 55 }).ai.label, `${K}ai`);
  assert.equal(rows({ stage: 'writing', ai: { calls: 4, status: 'ok' } }).ai.label, `${K}aiDone {"calls":4}`);
  assert.equal(rows({ stage: 'writing', questions: 29, ai: { calls: 4, status: 'partial' } }).ai.label, `${K}aiStatus {"status":"partial"}`);
});

// Integrations REVIEW-TRIM-SPEC D3/D9: the campaign-context rule is retired -- no special case; a
// stored report's D7.1 warn item (the interim between the fork and Lambda deploys) is an ordinary
// advisory entry, which never gates Done (D8).
test('D7.1 is no longer special: an older report\'s D7.1 warn item is an ordinary advisory entry', () => {
  assert.equal(checklistModule.CONTEXT_RULE, undefined);
  assert.equal(checklistModule.isContextItem, undefined);
  const d71 = D('D7.1', 'warn', [f('D7.1', 1)]);
  const e = openEntries([...DITEMS, d71]).filter((x) => x.item.id === 'D7.1');
  assert.equal(e.length, 1);
  assert.equal(e[0].section, 'advisory');
  assert.equal(isContextConcern("1204 (listmonk's cached count)"), false);
  assert.equal(isContextConcern('88 Fall drop'), true);
});

// Integrations REVIEW-TRIM-SPEC I10 (D8): the Done rule, pure.
test('I10: advisory entries do not gate Done; blockers, acknowledgements and the structure term do', () => {
  const entries = openEntries(REPORT.items);
  const keysOf = (section) => entries.filter((x) => x.section === section).map((x) => x.key);
  const advisory = keysOf('advisory');
  const gating = [...keysOf('blockers'), ...keysOf('acknowledge')];
  assert.ok(advisory.length > 0 && gating.length > 0);
  const done = (decidedKeys, extra = {}) => isReviewDone({
    report: REPORT, verdict: 'blocked', entries, decided: (k) => decidedKeys.includes(k), ...extra,
  });
  // Every gating entry decided, no advisory one: Done.
  assert.equal(done(gating), true);
  // An advisory entry left undecided never holds Done; a blocker or an acknowledgement does.
  assert.equal(done([...gating.slice(1)]), false);
  assert.equal(done(advisory), false);
  // The structure term: a structure admin must decide D4.2's structure finding; others are not held.
  assert.equal(done(gating, { structureAcknowledge: true, structureKey: 'R#abc' }), false);
  assert.equal(done([...gating, 'R#abc'], { structureAcknowledge: true, structureKey: 'R#abc' }), true);
  assert.equal(done(gating, { structureAcknowledge: false, structureKey: 'R#abc' }), true);
  // A pass verdict is Done; a running review or no report never is.
  assert.equal(done([], { verdict: 'pass' }), true);
  assert.equal(done(gating, { running: true }), false);
  assert.equal(isReviewDone({
    report: null, verdict: 'pass', entries: [], decided: () => true,
  }), false);
});

test('I10: the author-fix D4.2 button label; any other structure state reads Acknowledge', () => {
  const plan = (next, available = true) => ({ available, next, stages: { 1: [], 2: [] } });
  assert.equal(structureAcknowledgeLabel({ verified: false, plan: plan(null) }), 'campaigns.review.acknowledgeAuthorFix');
  assert.equal(structureAcknowledgeLabel({ verified: false, plan: plan(1) }), 'campaigns.review.acknowledge');
  assert.equal(structureAcknowledgeLabel({ verified: false, plan: plan(null, false) }), 'campaigns.review.acknowledge');
  assert.equal(structureAcknowledgeLabel({ verified: true, plan: plan(null) }), 'campaigns.review.acknowledge');
  assert.equal(structureAcknowledgeLabel(null), 'campaigns.review.acknowledge');
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  const ack = /data-cy="btn-structure-acknowledge"[\s\S]*?<\/b-button>/.exec(tpl)[0];
  assert.match(ack, /\{\{ \$t\(structureAckLabel\) \}\}/);
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.equal(en['campaigns.review.acknowledgeAuthorFix'], 'Acknowledge (author fix expected)');
});

// Integrations REVIEW-TRIM-SPEC D7: an item's `override` permission.
test('D7: acceptState -- cannot, override (a permission the user lacks), ok; the window never stages an override accept', () => {
  const can = (perms) => (p) => perms.includes(p);
  const d215 = { ...D('D2.15', 'fail', [f('D2.15', 1)]), override: 'campaigns:review_structure' };
  assert.equal(acceptState(d215, can([])), 'override');
  assert.equal(acceptState(d215, can(['campaigns:review_structure'])), 'ok');
  assert.equal(acceptState({ ...d215, acceptable: false }, can(['campaigns:review_structure'])), 'cannot');
  assert.equal(acceptState(D('D2.3', 'fail'), can([])), 'ok');
  assert.equal(acceptState(d215, undefined), 'override');
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  const reportList = /<section v-for="sec in sections"([\s\S]*?)<\/section>/.exec(tpl)[1];
  const tip = /<b-tooltip v-else-if="acceptStateOf\(e\.item\) === 'override'" :label="\$t\('campaigns\.review\.overrideAdminOnly'\)"[\s\S]*?<\/b-tooltip>/.exec(reportList);
  assert.ok(tip, 'the override tooltip');
  assert.match(tip[0], /<b-button size="is-small" disabled data-cy="btn-accept-override">/);
  assert.doesNotMatch(tip[0], /stage\(/);
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.equal(en['campaigns.review.overrideAdminOnly'], 'Only a review admin (campaigns:review_structure) can override a missing shop-link attribution.');
});

test('the window: a proposal box with Copy in the card template; the context list at card size; AI glyphs in Passed', () => {
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  const reportList = /<section v-for="sec in sections"([\s\S]*?)<\/section>/.exec(tpl)[1];
  const box = /<div v-if="e\.finding\.proposal" class="proposal"[\s\S]*?<\/div>/.exec(reportList);
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
  // Fix for me: light green when a fix exists (fixType), disabled without one; "I fixed it" by its key.
  assert.match(reportList, /:disabled="!e\.finding\.fix" data-cy="btn-fix-for-me"\s+:type="fixType\(e\)"/);
  const context = /data-cy="section-context"([\s\S]*?)<\/b-collapse>/.exec(tpl)[1];
  const list = /<ul class="context-list"[^>]*>/.exec(context)[0];
  assert.doesNotMatch(list, /is-size-7/);
  assert.doesNotMatch(context, /has-text-warning\b/, 'never yellow text');
  // REVIEW-TRIM-SPEC D3: no buttons in the Context section; the Open link stays.
  assert.doesNotMatch(context, /<b-button/);
  assert.match(context, /target="_blank" rel="noopener"/);
  // REVIEW-TRIM-SPEC D8: the window's Done is the pure rule.
  const entries = /entries\(\) \{([\s\S]*?)\n {4}\},/.exec(src)[1];
  assert.doesNotMatch(entries, /isContextItem/);
  assert.match(src, /return isReviewDone\(\{/);
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
  // The new job's report arrives: the report decides (rule 3) -- N = its alts section row count
  // (integrations REVIEW-TRIM-2-SPEC D5: Approved + Modified + Generated).
  const fresh = withAlts('job-new', {
    items: [altItem('a', 'approved', 'A'), altItem('m', 'modified', 'M', 'm'), altItem('g', 'generated', 'G')],
    approved: 1,
    modified: 1,
    generated: 1,
    unfixed: 0,
    thisInspect: 1,
  });
  assert.equal(step({ live: live('done', { alts: [] }), report: fresh, reportJob: 'job-new' }).label, `${K}altsDone {"n":3}`);
  // A run whose base was not known yet (the window had not loaded): the live run applies.
  assert.equal(step({ live: live('generating', { baseJob: undefined }), report: fresh, reportJob: 'job-new' }).state, 'current');
});

test('A1-I4: live phases -- done with 0 is "No images need alt tags", a done with rejected > 0 and a failed are the warning state', () => {
  // Post-release fix (2026-11-05): 0 APPLIED in the live phase is "reviewed", never "no images".
  assert.equal(step({ live: live('done'), report: null }).label, `${K}altsReviewed`);
  const rej = step({ live: live('done', { alts: ['a'], rejected: 2 }), report: null });
  assert.equal(rej.state, 'warn');
  assert.equal(rej.checked, false);
  assert.equal(rej.label, `${K}altsFailed · ${K}altsStillNeed {"n":2}`);
  assert.deepEqual(step({ live: live('failed'), report: null }), {
    key: 'alts', state: 'warn', checked: false, spinner: false, label: `${K}altsFailed`,
  });
});

// integrations REVIEW-TRIM-2-SPEC I6: the alt row reads "{n} alt tags generated/reviewed" with n =
// the report's alts row count; zero reads "No images need alt tags"; a running job with no live
// message shows no step (no count is known -- "Alt tags checked" is retired).
test('REVIEW-TRIM-2-SPEC I6: no live message -- a running job has no step; from a report, failed iff unfixed > 0, else N = the alts row count; no step without alts', () => {
  assert.equal(step({ live: null, report: null, running: true }), null);
  const r = (alts) => step({ live: null, report: withAlts('j', alts), reportJob: 'j' });
  assert.equal(r({
    items: [], approved: 1, modified: 0, generated: 0, unfixed: 1, thisInspect: 0,
  }).label, `${K}altsFailed · ${K}altsStillNeed {"n":1}`);
  // thisInspect (1) is not the count: the rows are (2 approved + 1 modified + 1 generated; unfixed never a row).
  assert.equal(r({
    items: [altItem('a', 'approved', 'A'), altItem('b', 'approved', 'B'), altItem('m', 'modified', 'M', 'x'), altItem('g', 'generated', 'G'), altItem('u', 'unfixed', '')],
    approved: 2,
    modified: 1,
    generated: 1,
    unfixed: 0,
    thisInspect: 1,
  }).label, `${K}altsDone {"n":4}`);
  assert.equal(r({
    items: [], approved: 0, modified: 0, generated: 0, unfixed: 0, thisInspect: 0,
  }).label, `${K}altsNone`);
  // The label itself: one string for both phases. integrations ALT-COPY-SPEC S4-4: never "reviewed"
  // — a kept hand-written alt is not checked against its image.
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.equal(en[`${K}altsDone`], '{n} alt tags generated or kept');
  assert.equal(en[`${K}altsNone`], 'No images need alt tags');
  assert.equal(en[`${K}altsReviewed`], 'Alt tags up to date');
  assert.equal(en[`${K}altsChecked`], undefined);
  assert.equal(r(null), null);
  assert.equal(step({ live: null, report: null }), null);
  // A locked campaign (sent, scheduled… or a render sheet): done, never "Inspect again", even with unfixed alts.
  const locked = r({
    items: [], approved: 1, modified: 0, generated: 0, unfixed: 1, thisInspect: 0, locked: true,
  });
  assert.equal(locked.label, `${K}altsLocked`);
  assert.equal(locked.state, 'done');
});

test('A1-I4: the step is first in the list; alone before the job exists', () => {
  const rows = checklist({ progress: { stage: 'reading' }, report: null, t });
  const s = step({ live: live('generating'), report: null });
  // REVIEW-TRIM-2-SPEC D1: no Screenshots row without the key.
  const shown = STAGES.filter((k) => k !== 'screenshots');
  assert.deepEqual(withAltsStep(s, rows).map((x) => x.key), ['alts', ...shown]);
  assert.deepEqual(withAltsStep(s, []).map((x) => x.key), ['alts']);
  assert.deepEqual(withAltsStep(null, rows).map((x) => x.key), shown);
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

// integrations EDITOR-SYNC-SPEC I5 -- a card is in Accepted exactly when its latest recorded
// disposition is `accept` (and the item is acceptable).
test('I5: splitAccepted -- accept moves a card to Accepted; fixme / fixed / none stay open; order preserved', () => {
  const item = { id: 'A5.4', tier: 'A', acceptable: true };
  const e = (key, it = item) => ({ key, item: it, finding: { key } });
  const entries = [e('k1'), e('k2'), e('k3'), e('k4'), e('k5')];
  const latest = {
    k1: { item_key: 'k1', action: 'accept' },
    k2: { item_key: 'k2', action: 'fixme' },
    k3: { item_key: 'k3', action: 'fixed' },
    k5: { item_key: 'k5', action: 'accept' },
  };
  const { open, accepted } = splitAccepted(entries, latest);
  assert.deepEqual(accepted.map((x) => x.key), ['k1', 'k5']);
  assert.deepEqual(open.map((x) => x.key), ['k2', 'k3', 'k4']);
  // No dispositions at all: everything open, in order.
  assert.deepEqual(splitAccepted(entries, {}).open.map((x) => x.key), ['k1', 'k2', 'k3', 'k4', 'k5']);
  assert.deepEqual(splitAccepted(entries, null).accepted, []);
  assert.deepEqual(splitAccepted(null, latest), { open: [], accepted: [] });
});

test('I5: splitAccepted -- a later fixme after an earlier accept is open (latest holds the newest of a newest-first log)', () => {
  const log = [
    { item_key: 'k', action: 'fixme' },
    { item_key: 'k', action: 'accept' },
  ];
  // The window's `latest` rule: the first row per key of the newest-first log.
  const latest = {};
  log.forEach((d) => { if (!latest[d.item_key]) latest[d.item_key] = d; });
  const entries = [{ key: 'k', item: { id: 'A5.4', acceptable: true }, finding: { key: 'k' } }];
  const r = splitAccepted(entries, latest);
  assert.deepEqual(r.open.map((x) => x.key), ['k']);
  assert.deepEqual(r.accepted, []);
});

test('I5: splitAccepted -- an accept on an acceptable:false item stays open (the verdict still blocks it, S6)', () => {
  const entries = [
    { key: 'a', item: { id: 'D2.15', acceptable: false }, finding: { key: 'a' } },
    { key: 'b', item: { id: 'A5.4', acceptable: true }, finding: { key: 'b' } },
  ];
  const latest = { a: { action: 'accept' }, b: { action: 'accept' } };
  const r = splitAccepted(entries, latest);
  assert.deepEqual(r.open.map((x) => x.key), ['a']);
  assert.deepEqual(r.accepted.map((x) => x.key), ['b']);
});

test('D3: the window -- the three sections are built from open, a fourth Accepted section from accepted', () => {
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  // integrations LOGO-APPROVAL-SPEC D10: through logoAwareSplit (a D3.5 approval card stays open).
  assert.match(src, /logoAwareSplit\(this\.entries, this\.latest, splitAccepted, this\.brandRows\)/);
  assert.match(src, /campaigns\.review\.sectionAccepted/);
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  assert.match(tpl, /data-cy="tag-accepted"/);
  assert.match(tpl, /:disabled="sec\.key === 'accepted'"/);
});

// ------------------------------------------------------------------ integrations
// REVIEW-CONVENTIONS-SPEC I9 / I11 -- the Suggestions section (A findings of severity low).
const SUG_REPORT = {
  items: [...REPORT.items, A('A4.7', 'low'), A('A6.2', 'low')],
  ai: { status: 'ok', calls: [{}, {}, {}] },
};

test('I9: sectionOf -- A low is a suggestion; every other mapping unchanged; D never a suggestion', () => {
  assert.equal(sectionOf({ tier: 'A' }, { severity: 'low' }), 'suggestions');
  assert.equal(sectionOf({ tier: 'A' }, { severity: 'critical' }), 'blockers');
  assert.equal(sectionOf({ tier: 'A' }, { severity: 'high' }), 'acknowledge');
  assert.equal(sectionOf({ tier: 'A' }, { severity: 'medium' }), 'advisory');
  assert.equal(sectionOf({ tier: 'A' }, {}), 'advisory');
  ['warn', 'n/a', 'pass'].forEach((verdict) => assert.equal(sectionOf({ tier: 'D', verdict }, { severity: 'low' }), 'advisory'));
  assert.equal(sectionOf({ tier: 'D', verdict: 'fail' }, { severity: 'low' }), 'blockers');
});

test('I9: sectionCounts counts suggestions; the writing-done line carries {sug}', () => {
  assert.deepEqual(sectionCounts(SUG_REPORT.items), {
    blockers: 3, acknowledge: 1, advisory: 2, suggestions: 2,
  });
  const r = checklist({
    progress: { stage: 'writing', at: 'x' }, report: SUG_REPORT, phase: 'complete', t,
  });
  assert.equal(r[r.length - 1].label, `${K}writingDone {"b":3,"ack":1,"adv":2,"sug":2}`);
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.match(en['campaigns.review.checklist.writingDone'], /\{sug\} Suggestions$/);
  assert.equal(en['campaigns.review.sectionSuggestions'], 'Suggestions');
  assert.equal(en['campaigns.review.acknowledgeSuggestions'], 'Acknowledge suggestions');
  assert.match(en['campaigns.review.acknowledgedBy'], /^Acknowledged by \{user\}, \{date\}$/);
});

test('I9: isReviewDone is false while a suggestion is undecided and true once decided', () => {
  const entries = openEntries(SUG_REPORT.items);
  const keysOf = (section) => entries.filter((x) => x.section === section).map((x) => x.key);
  const gating = [...keysOf('blockers'), ...keysOf('acknowledge')];
  const sug = keysOf('suggestions');
  assert.deepEqual(sug, ['A4.7#0', 'A6.2#0']);
  const done = (decidedKeys) => isReviewDone({
    report: SUG_REPORT, verdict: 'blocked', entries, decided: (k) => decidedKeys.includes(k),
  });
  assert.equal(done(gating), false, 'an undecided suggestion holds Done');
  assert.equal(done([...gating, sug[0]]), false);
  assert.equal(done([...gating, ...sug]), true);
  // Advisory still never gates.
  assert.ok(keysOf('advisory').length > 0);
});

test('I9: an acknowledged suggestion is NOT moved to Accepted (splitAccepted); another A accept still is', () => {
  const entries = openEntries(SUG_REPORT.items);
  const latest = {
    'A4.7#0': { action: 'accept', username: 'kay' },
    'A2.4#0': { action: 'accept', username: 'kay' },
  };
  const { open, accepted } = splitAccepted(entries, latest);
  assert.ok(open.some((e) => e.key === 'A4.7#0'));
  assert.deepEqual(accepted.map((e) => e.key), ['A2.4#0']);
});

test('I11: Acknowledge suggestions stages accept for exactly the undecided suggestion keys', () => {
  const entries = openEntries(SUG_REPORT.items);
  const all = suggestionAcknowledgements(entries, () => false);
  assert.deepEqual(all, [
    { key: 'A4.7#0', rubric_id: 'A4.7', action: 'accept' },
    { key: 'A6.2#0', rubric_id: 'A6.2', action: 'accept' },
  ]);
  // Never an advisory, blocker or acknowledge key.
  const sections = new Map(entries.map((e) => [e.key, e.section]));
  assert.ok(all.every((s) => sections.get(s.key) === 'suggestions'));
  // A decided suggestion is not staged again.
  assert.deepEqual(suggestionAcknowledgements(entries, (k) => k === 'A4.7#0').map((s) => s.key), ['A6.2#0']);
  assert.deepEqual(suggestionAcknowledgements(entries, () => true), []);
  // No suggestion: nothing.
  assert.deepEqual(suggestionAcknowledgements(openEntries(REPORT.items), () => false), []);
  assert.deepEqual(suggestionAcknowledgements(null), []);
});

test('D5: the window -- a fourth Suggestions section with no per-card buttons and one Acknowledge suggestions button', () => {
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  assert.match(src, /key: 'suggestions', label: this\.\$t\('campaigns\.review\.sectionSuggestions'\)/);
  assert.match(tpl, /<div v-if="sec\.key !== 'suggestions'" class="buttons mt-2">/);
  assert.match(tpl, /v-if="sec\.key === 'suggestions' && suggestionsOpen\.length"/);
  assert.match(tpl, /data-cy="btn-acknowledge-suggestions"/);
  assert.match(tpl, /data-cy="review-acknowledged"/);
  assert.match(src, /suggestionAcknowledgements\(this\.entries/);
});
