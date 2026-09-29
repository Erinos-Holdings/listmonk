// Fork (inspect scope) -- integrations INSPECT-SCOPE-SPEC I10 (the permission -> buttons mapping;
// structure keys excluded from the generic actions; no declineMatrix string remains) and I11 (the
// brief is deterministic, carries every §2.5 field, both Notify buttons use the one builder, and
// nothing is POSTed).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  buildStructureBrief, isStructureKey, structureButtons, structureOf, STRUCTURE_PERMISSION,
} from './structureBrief.mjs'; // eslint-disable-line import/extensions

const coverage = {
  fingerprint: 'f'.repeat(64),
  verified: false,
  itemCount: 12,
  items: [
    {
      id: 'composition:Columns[Text;Text;-]',
      kind: 'composition',
      label: 'Columns 2 · [Text, Text]',
      scope: 'full',
      reason: 'never verified',
      missing: { 1: ['gmailcom-lm_chrcurrent_win10', 'gmailcom-dm_chrcurrent_win10'], 2: ['yahoocom-lm_ffcurrent_win10'] },
    },
    {
      id: 'block:Text|font=-|size=l|weight=normal|color=-|backgroundColor=#003b4d',
      kind: 'block',
      label: 'Text, large, backgroundColor #003b4d',
      scope: 'dark',
      reason: 'colour-only difference',
      missing: { 1: ['gmailcom-dm_chrcurrent_win10'], 2: [] },
    },
  ],
  plan: {
    available: true,
    next: 1,
    stages: { 1: ['gmailcom-lm_chrcurrent_win10', 'gmailcom-dm_chrcurrent_win10'], 2: ['yahoocom-lm_ffcurrent_win10'] },
    ids: ['gmailcom-lm_chrcurrent_win10', 'gmailcom-dm_chrcurrent_win10'],
    count: 2,
    remaining: 1,
    total: 3,
  },
  recordsUsed: [{
    id: 1, verified_at: '2026-09-25T23:57:50Z', test_id: 'CkaoTvXc', stage: 'all', campaign_id: 108,
  }],
  canaryChanged: ['Text'],
  unavailable: [],
  clientLabels: {
    'gmailcom-lm_chrcurrent_win10': 'Gmail web (Chrome, Win10)',
    'gmailcom-dm_chrcurrent_win10': 'Gmail web (Chrome, Win10) — dark',
    'yahoocom-lm_ffcurrent_win10': 'Yahoo.com (Windows 10)',
  },
  commands: [
    'AWS_PROFILE=erinos CLIENTS=plan STAGE=1 npx tsx scripts/inspect-listmonk-campaign.ts 110',
    'AWS_PROFILE=erinos npx tsx scripts/check-structure-coverage.ts 110',
  ],
};
const report = {
  rubricVersion: '2026-09-25.3',
  items: [
    { id: 'D4.1', tier: 'D', verdict: 'n/a' },
    {
      id: 'D4.2', tier: 'D', verdict: 'fail', acceptable: true, findings: [{ key: 'R#0123456789abcdef', id: 'D4.2' }], coverage,
    },
  ],
};
const campaign = { id: 110, name: 'RUZE_FR' };
const review = { id: 42, updated_at: '2026-09-28T20:00:00Z' };
const input = {
  campaign, review, report, origin: 'https://email.curatedfor.you',
};

test('I10: the permission decides the buttons and the CLI visibility', () => {
  assert.equal(STRUCTURE_PERMISSION, 'campaigns:review_structure');
  assert.deepEqual(structureButtons(true), { notify: 'claude', acknowledge: true, showCli: true });
  assert.deepEqual(structureButtons(false), { notify: 'robbie', acknowledge: false, showCli: false });
});

test('I10: structure keys (R#…, legacy R) are recognised for exclusion from the generic actions', () => {
  assert.equal(isStructureKey('R#0123456789abcdef'), true);
  assert.equal(isStructureKey('R'), true);
  assert.equal(isStructureKey('D4.2#abc'), false);
  assert.equal(isStructureKey('A'), false);
  assert.equal(isStructureKey('Rx'), false);
});

test('I10: a report without coverage renders the legacy section (structureOf is null)', () => {
  assert.equal(structureOf({ items: [{ id: 'D4.2', verdict: 'warn', findings: [] }] }), null);
  assert.equal(structureOf(null), null);
  assert.equal(buildStructureBrief({ ...input, report: { items: [] } }), '');
  assert.equal(structureOf(report).finding.key, 'R#0123456789abcdef');
});

test('I11: the brief is deterministic and carries every §2.5 field', () => {
  const a = buildStructureBrief(input);
  assert.equal(a, buildStructureBrief(JSON.parse(JSON.stringify(input))));
  assert.equal(a.split('\n')[0], 'Structure unverified for campaign 110 — run the plan or adjudicate a rendering-engine fix?');
  [
    'RUZE_FR (id 110)',
    'https://email.curatedfor.you/admin/campaigns/110',
    'Review: 42 · completed 2026-09-28T20:00:00Z · rubric 2026-09-25.3',
    `Fingerprint: ${'f'.repeat(64)}`,
    'Canary keys changed since the records: Text',
    'Columns 2 · [Text, Text] — never verified — light + dark',
    'Text, large, backgroundColor #003b4d — colour-only difference — dark only',
    'Gmail web (Chrome, Win10) — dark [gmailcom-dm_chrcurrent_win10]',
    'Yahoo.com (Windows 10) [yahoocom-lm_ffcurrent_win10]',
    'Plan: next stage 1, 2 client(s) = 2 credit(s); remaining stage 1; total 3 credit(s) to finish',
    'AWS_PROFILE=erinos CLIENTS=plan STAGE=1 npx tsx scripts/inspect-listmonk-campaign.ts 110',
    '#1 · 2026-09-25 · test CkaoTvXc · stage all',
  ].forEach((want) => assert.ok(a.includes(want), `brief lacks ${want}\n---\n${a}`));
  const unavailable = buildStructureBrief({
    ...input,
    report: {
      ...report,
      items: [{
        ...report.items[1],
        coverage: {
          ...coverage, unavailable: ['canary'], plan: { available: false, reason: 'count unavailable' },
        },
      }],
    },
  });
  assert.ok(unavailable.includes('Inputs unavailable: canary') && unavailable.includes('Plan: count unavailable'));
});

test('I11: both Notify buttons use the one builder and nothing is POSTed; no declineMatrix string remains (I10)', () => {
  const vue = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const src = readFileSync(new URL('./structureBrief.mjs', import.meta.url), 'utf8');
  assert.ok(!/fetch\(|XMLHttpRequest|\$api|axios/.test(src), 'the brief builder performs no I/O');
  assert.equal((vue.match(/buildStructureBrief\(/g) || []).length, 1, 'one call site builds the brief for both Notify buttons');
  assert.ok(!/declineMatrix/.test(vue));
  const en = readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8');
  assert.ok(!/declineMatrix/.test(en));
  // The notify handler only opens the modal: no $api call in it.
  const at = vue.indexOf('openStructureBrief() {');
  assert.ok(at > 0);
  const notify = vue.slice(at, vue.indexOf('copyBrief() {'));
  assert.ok(!/\$api/.test(notify), notify);
});
