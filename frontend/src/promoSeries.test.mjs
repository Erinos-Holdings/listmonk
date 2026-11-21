// Fork (promo series) -- integrations listmonk/PROMO-SERIES-SPEC T14 (I14, I15, D1, D5, D6, D7).
// Run: node --test src/promoSeries.test.mjs (Node's built-in runner, no dependency).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  applyStepChange, codeCounter, codeProblem, foldPromo, parseTags, pickerLabel, previewKey, previewQuery, promoCells,
  promoEditable, promoFormOf, promoNotes, purchasedArg, samePromo, settleForFold, silentDefaultOnStep, silentSkipForFold, tagProblem,
} from './promoSeries.mjs'; // eslint-disable-line import/extensions

test('the tag parser mirrors D1 for display: trim, drop empties, case-insensitive dedupe, the shape rules', () => {
  assert.deepEqual(parseTags(' BFCM-Deal-3, BFCM-BMSM-3 ,, bfcm-deal-3 '), { tags: ['BFCM-Deal-3', 'BFCM-BMSM-3'], problem: '' });
  assert.deepEqual(parseTags('BFCM*'), { tags: ['BFCM*'], problem: '' });
  assert.deepEqual(parseTags(''), { tags: [], problem: '' });
  assert.equal(tagProblem('abc*'), '');
  assert.equal(tagProblem('ab*'), 'prefix', 'the prefix floor is 3 characters');
  assert.equal(tagProblem('*'), 'prefix');
  assert.equal(tagProblem('a*b'), 'star', 'a * only last');
  assert.equal(tagProblem('ab**'), 'star');
  assert.equal(tagProblem('x'.repeat(64)), '');
  assert.equal(tagProblem('x'.repeat(65)), 'length');
  assert.equal(tagProblem('a\tb'), 'control');
  assert.equal(tagProblem('a,b'), 'comma');
  assert.equal(parseTags(Array.from({ length: 21 }, (_, i) => `tag-${i}`).join(',')).problem, 'count');
  assert.equal(parseTags(Array.from({ length: 20 }, (_, i) => `tag-${i}`).join(',')).problem, '');
  assert.equal(parseTags('ok, ab*').problem, 'prefix');
});

test('the Code: n/30 and the naming standard\'s rule (blank = no series)', () => {
  assert.equal(codeCounter(' BFCM-D3 '), '7/30');
  assert.equal(codeProblem(''), '');
  assert.equal(codeProblem('x'.repeat(30)), '');
  assert.equal(codeProblem('x'.repeat(31)), 'naming.nameTooLong');
  assert.equal(codeProblem('Á'.repeat(30)), '', 'characters, not bytes');
  assert.equal(codeProblem('a\nb'), 'naming.nameControl');
  assert.equal(codeProblem('CATALOG 01-button-outlook-word-columns-long'), 'naming.nameTooLong', 'no CATALOG exemption');
});

test('the form reads the served fields, never attribs', () => {
  const served = {
    promoCode: 'BFCM-D3', promoStep: 2, promoTags: ['a-1', 'BFCM*'], attribs: { promo_code: 'X' },
  };
  assert.deepEqual(promoFormOf(served), { promoCode: 'BFCM-D3', promoStep: 2, promoTagsText: 'a-1, BFCM*' });
  assert.deepEqual(promoFormOf({ promoCode: '', promoStep: null, promoTags: [] }), { promoCode: '', promoStep: '', promoTagsText: '' });
  assert.deepEqual(promoFormOf({}), { promoCode: '', promoStep: '', promoTagsText: '' });
});

test('foldPromo: the owned-key fold -- set, blank, junk keys, evergreen/opt-in', () => {
  const form = { promoCode: ' BFCM-D3 ', promoStep: '2', promoTagsText: 'BFCM-Deal-3, bfcm-deal-3, BFCM*' };
  const junk = {
    lang: 'en', promoCode: 'junk', promoStep: 1, promoTags: [],
  };
  const want = {
    lang: 'en', promo_code: 'BFCM-D3', promo_step: 2, promo_tags: ['BFCM-Deal-3', 'BFCM*'],
  };
  assert.deepEqual(foldPromo(junk, form), want);
  // Blank Code removes all three (a Step or tags never posted without a Code).
  assert.deepEqual(foldPromo({ promo_code: 'X', promo_step: 2, promo_tags: ['a-1'] }, { promoCode: ' ', promoStep: 2, promoTagsText: 'a-1' }), {});
  // Blank Step and tags: removed; an empty tag list is absent, never [].
  assert.deepEqual(foldPromo({ promo_step: 3, promo_tags: ['x-1'] }, { promoCode: 'C', promoStep: '', promoTagsText: ' , ' }), { promo_code: 'C' });
  // Not a non-evergreen regular campaign: all three removed whatever the form says.
  assert.deepEqual(foldPromo({ promo_code: 'X', silent_skip: true }, form, false), { silent_skip: true });
  assert.deepEqual(foldPromo(null, { promoCode: '' }), {});
  const src = { promo_code: 'X' };
  foldPromo(src, form);
  assert.deepEqual(src, { promo_code: 'X' }, 'input not mutated');
});

test('silentDefaultOnStep (I15): the D6 table', () => {
  const rows = [
    // [draft, prev, next, silentOn, touched, want]
    [true, '', 2, false, false, true],
    [true, '', 1, false, false, false],
    [true, '', 3, false, false, false],
    [true, 1, 2, false, false, true],
    [true, 2, 3, true, false, false],
    [true, 2, '', true, false, false],
    [true, 3, 2, false, false, true],
    // The switch differs from the default of the step being left: a choice, kept.
    [true, 1, 2, true, false, null],
    [true, 2, 3, false, false, null],
    [true, '', 2, true, false, null],
    // Touched this session: never overridden.
    [true, '', 2, false, true, null],
    [true, 2, 1, true, true, null],
    // Not a draft (scheduled, paused, finished): nothing.
    [false, '', 2, false, false, null],
    // Not a change.
    [true, 2, '2', true, false, null],
  ];
  rows.forEach(([draft, prevStep, nextStep, silentOn, touched, want]) => {
    const args = {
      draft, prevStep, nextStep, silentOn, touched,
    };
    assert.equal(silentDefaultOnStep(args), want, JSON.stringify(args));
  });
});

test('Stage 4 V3: emptying the Code resets the Step as picking blank does -- a new campaign posts no silent_skip and no promo key', () => {
  const ctx = { draft: true, touched: false }; // a new campaign: a draft, the switch not shown
  let form = {
    promoCode: '', promoStep: '', promoTagsText: '', silentSkip: false,
  };
  form = { ...form, promoCode: 'BFCM-D3' };
  form = { ...form, ...applyStepChange(form, 2, ctx) };
  assert.deepEqual([form.promoStep, form.silentSkip], [2, true], 'step 2 turns 60-day engaged on');
  assert.deepEqual(settleForFold(form, ctx), { promoStep: 2, silentSkip: true }, 'with a Code the fold reads the form');
  // Saved with the Code emptied: the fold reads a blank Step and its default (off).
  form = { ...form, promoCode: '' };
  const r = settleForFold(form, ctx);
  assert.deepEqual(r, { promoStep: '', silentSkip: false });
  const attribs = foldPromo({ lang: 'en' }, form, true);
  const silent = silentSkipForFold({ showSmartSending: true, formSilent: r.silentSkip, storedSilent: undefined });
  assert.equal(silent, false, 'no silent_skip is posted');
  assert.deepEqual(attribs, { lang: 'en' }, 'no promo key is posted');
  // Stage 4b W1 -- nothing is reset WHILE TYPING: the form keeps its Step through an empty Code, so
  // a Code emptied and retyped saves with the Step and the switch it had.
  assert.deepEqual([form.promoStep, form.silentSkip], [2, true], 'the form itself is untouched');
  form = { ...form, promoCode: 'BFCM-D4' };
  assert.deepEqual(settleForFold(form, ctx), { promoStep: 2, silentSkip: true });
  assert.deepEqual(foldPromo({}, form, true), { promo_code: 'BFCM-D4', promo_step: 2 });
  // No Step set: the form's values, whatever the Code.
  assert.deepEqual(settleForFold({ promoCode: '', promoStep: '', silentSkip: true }, ctx), { promoStep: '', silentSkip: true });
  // A switch the user touched is kept even when the Step counts as blank.
  assert.deepEqual(settleForFold({ promoCode: ' ', promoStep: 2, silentSkip: true }, { draft: true, touched: true }), { promoStep: '', silentSkip: true });
});

test('silentSkipForFold (D6, S1): the form value whenever the Smart Sending row applies, a new campaign included', () => {
  assert.equal(silentSkipForFold({ showSmartSending: true, formSilent: true, storedSilent: false }), true, 'a new campaign whose Step set it');
  assert.equal(silentSkipForFold({ showSmartSending: true, formSilent: false, storedSilent: true }), false);
  assert.equal(silentSkipForFold({ showSmartSending: false, formSilent: true, storedSilent: false }), false, 'hidden: the stored state');
  assert.equal(silentSkipForFold({ showSmartSending: false, formSilent: false, storedSilent: true }), true);
  // A new campaign created without a Step posts exactly what it posts today (off).
  assert.equal(silentSkipForFold({ showSmartSending: true, formSilent: false, storedSilent: undefined }), false);
});

test('the purchased share: the served one while the form equals the stored values, else the preview', () => {
  const c = { promoCode: 'BFCM-D3', promoStep: 2, promoTags: ['BFCM-Deal-3'] };
  const same = { promoCode: 'BFCM-D3 ', promoStep: '2', promoTagsText: 'BFCM-Deal-3' };
  assert.equal(samePromo(same, c), true);
  assert.equal(purchasedArg(same, c, { skipped: 9 }), null);
  const edited = { ...same, promoTagsText: 'BFCM-Deal-3, BFCM*' };
  assert.equal(samePromo(edited, c), false);
  assert.equal(purchasedArg(edited, c, null), 0, 'until the preview answers');
  assert.equal(purchasedArg(edited, c, { skipped: 9 }), 9);
  assert.deepEqual(previewQuery(edited), { code: 'BFCM-D3', step: '2', tags: 'BFCM-Deal-3,BFCM*' });
  assert.deepEqual(previewQuery({ promoCode: 'C', promoStep: '', promoTagsText: '' }), { code: 'C', step: '', tags: '' });
});

test('the preview key: a saved not-finished broadcast with a Code and valid tags only', () => {
  const c = {
    id: 7, status: 'draft', evergreen: false, updatedAt: 't',
  };
  const form = { promoCode: 'C', promoStep: 1, promoTagsText: 'BFCM*' };
  assert.equal(previewKey(c, form, true), '7:t:C:1:BFCM*');
  assert.equal(previewKey(c, form, false), null);
  assert.equal(previewKey({ ...c, id: 0 }, form, true), null, 'not saved');
  assert.equal(previewKey({ ...c, evergreen: true }, form, true), null);
  assert.equal(previewKey({ ...c, status: 'finished' }, form, true), null);
  assert.equal(previewKey(c, { ...form, promoTagsText: '' }, true), null);
  assert.equal(previewKey(c, { ...form, promoTagsText: 'a*' }, true), null, 'a malformed tag');
  assert.equal(previewKey(c, { ...form, promoCode: '' }, true), null);
});

test('the notes and the editable rule', () => {
  assert.deepEqual(promoNotes({ promoCode: 'C', promoStep: 2, promoTagsText: '' }, null), { noTags: true, stale: false });
  assert.deepEqual(promoNotes({ promoCode: 'C', promoStep: 3, promoTagsText: '' }, null), { noTags: true, stale: false });
  assert.deepEqual(promoNotes({ promoCode: 'C', promoStep: 1, promoTagsText: '' }, null), { noTags: false, stale: false });
  assert.deepEqual(promoNotes({ promoCode: '', promoStep: 2, promoTagsText: '' }, null), { noTags: false, stale: false });
  assert.deepEqual(promoNotes({ promoCode: 'C', promoStep: 2, promoTagsText: 'x-1' }, { stale: true }), { noTags: false, stale: true });
  assert.deepEqual(promoNotes({ promoCode: 'C', promoStep: 1, promoTagsText: 'x-1' }, { stale: true }), { noTags: false, stale: false });
  assert.equal(promoEditable('draft', true), true);
  assert.equal(promoEditable('paused', true), true);
  assert.equal(promoEditable('scheduled', true), false);
  assert.equal(promoEditable('draft', false), false);
  assert.equal(promoEditable(undefined, true, true), true, 'a new campaign');
});

test('the picker label and the Campaigns list cells (D7)', () => {
  assert.equal(pickerLabel('Black Friday', 'BFCM-D3', 2), 'Black Friday · BFCM-D3 S2');
  assert.equal(pickerLabel('Black Friday', 'BFCM-D3', null), 'Black Friday · BFCM-D3');
  assert.equal(pickerLabel('Black Friday', '', 2), 'Black Friday');
  assert.equal(pickerLabel('Black Friday', null, null), 'Black Friday');
  assert.deepEqual(promoCells({ promoCode: 'BFCM-D3', promoStep: 3 }), { code: 'BFCM-D3', step: '3' });
  assert.deepEqual(promoCells({ promoCode: 'X', promoStep: 2, evergreen: true }), { code: '', step: '' }, 'blank for a journey');
  assert.deepEqual(promoCells({}), { code: '', step: '' });
});
