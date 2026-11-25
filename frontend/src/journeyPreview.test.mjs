// Fork (journey preview, integrations listmonk/JOURNEY-PREVIEW-SPEC I2; USAGE-PREVIEW-LANG-SPEC I8).
// Run: yarn test (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  FRAME_WIDTH, STATUS_FILTERS, frameScale, isSystemGenerated, journeyColumns, parseJourneyTag, pickBrand, previewIds,
  previewTarget, statusMatches, stepCell, stepEntries, usageBrands,
} from './journeyPreview.mjs'; // eslint-disable-line import/extensions
import { GUIDE_SERIES, guideDays, seriesGuide } from './brandUsage.mjs'; // eslint-disable-line import/extensions

const [WELCOME, POST, RE] = GUIDE_SERIES.map((g) => g.family);
const ALL5 = ['en', 'es', 'fr', 'de', 'it'];

// A stored counted entry (integrations lib/brand-usage.ts CountedEntry): covers per the send rule.
const coversOf = (lang) => {
  if (lang === null) return ALL5;
  return ALL5.includes(lang) ? [lang] : [];
};
const ent = (id, bubble = 'green', lang = null) => ({
  id, bubble, lang, covers: coversOf(lang),
});
const camp = (id, extra = {}) => ({
  id, name: `c${id}`, subject: `s${id}`, updatedAt: '2026-11-01T00:00:00Z', tags: [], attribs: {}, ...extra,
});
const byId = (...cs) => new Map(cs.map((c) => [c.id, c]));

// A usage doc with counted entries on welcome day 2, post-purchase day 14 and Re-engagement 180.
const usage = {
  series: {
    welcome: { steps: [{ days: 0, counted: [] }, { days: 2, counted: [ent(5), ent(6, 'grey', 'es')] }, { days: 5 }] },
    postPurchase: { steps: [{ days: 0, counted: [] }, { days: 14, counted: [ent(7, 'amber', 'fr')] }] },
  },
  reengagement: { steps: [{ days: 180, counted: [ent(8, 'green', 'en')] }] },
  sku: { leaf: { counted: [ent(99)] } },
};

test('I8: steps come from the usage doc\'s counted entries (welcome, postPurchase, reengagement; never SKU)', () => {
  assert.deepEqual(stepEntries(usage, WELCOME, 2).map((e) => e.id), [5, 6]);
  assert.deepEqual(stepEntries(usage, POST, 14).map((e) => e.id), [7]);
  assert.deepEqual(stepEntries(usage, RE, 180).map((e) => e.id), [8]);
  assert.deepEqual(stepEntries(usage, WELCOME, 5), []);
  assert.deepEqual(previewIds(usage), [5, 6, 7, 8]);
  // A doc without `counted` (an older doc) yields no previews and never throws.
  const old = { series: { welcome: { steps: [{ days: 2, state: 'active', counts: { green: 1, amber: 0, grey: 0 } }] } } };
  assert.deepEqual(stepEntries(old, WELCOME, 2), []);
  assert.deepEqual(previewIds(old), []);
  [null, undefined, {}, { series: 'x' }, { reengagement: { steps: 'x' } }].forEach((u) => {
    assert.deepEqual(previewIds(u), []);
    journeyColumns(u, new Map()).forEach((g) => g.steps.forEach((s) => assert.equal(s.preview, null)));
  });
  // Malformed entries are skipped.
  assert.deepEqual(stepEntries({ reengagement: { steps: [{ days: 180, counted: [{ id: 'x', bubble: 'green' }, { id: 3, bubble: 'red' }, null, ent(4)] }] } }, RE, 180)
    .map((e) => e.id), [4]);
});

test('I8: ids missing from the campaigns read are dropped', () => {
  const cell = stepCell([ent(5), ent(6)], byId(camp(6)));
  assert.equal(cell.campaign.id, 6);
  assert.equal(cell.others, 0);
  assert.equal(stepCell([ent(5)], byId()), null);
  // A plain object keyed by id works too.
  assert.equal(stepCell([ent(5)], { 5: camp(5) }).campaign.id, 5);
});

test('I8: single language -- eligibility by covers only; exact lang before language-less; never a fallback', () => {
  const cs = byId(
    camp(1, { updatedAt: '2026-11-05T00:00:00Z' }), // language-less, newest
    camp(2, { updatedAt: '2026-11-01T00:00:00Z' }), // es, older
    camp(3, { updatedAt: '2026-11-09T00:00:00Z' }), // en
  );
  const entries = [ent(1, 'green', null), ent(2, 'green', 'es'), ent(3, 'green', 'en')];
  // ES: the exact es entry wins over the newer language-less one; the en entry is not eligible.
  const es = stepCell(entries, cs, { lang: 'es' });
  assert.equal(es.campaign.id, 2);
  assert.equal(es.others, 1);
  assert.equal(es.noneInLang, false);
  // FR: only the language-less one covers it.
  assert.equal(stepCell(entries, cs, { lang: 'fr' }).campaign.id, 1);
  // EN (= EN+): the en entry.
  assert.equal(stepCell(entries, cs, { lang: 'en' }).campaign.id, 3);
  // A step whose entries cover no FR: "None in this language", never the English one.
  const none = stepCell([ent(3, 'green', 'en'), ent(2, 'green', 'es')], cs, { lang: 'fr' });
  assert.deepEqual(none, {
    campaign: null, others: 0, otherLangs: 0, noneInLang: true,
  });
  // A `pt` entry covers no usage language (counted only under All).
  const ptEntry = ent(1, 'green', 'pt');
  assert.deepEqual(ptEntry.covers, []);
  const pt = stepCell([ptEntry], cs, { lang: 'en' });
  assert.equal(pt.noneInLang, true);
  assert.equal(stepCell([ptEntry], cs, { lang: 'all' }).campaign.id, 1);
  // Ties: the latest updated_at, then the higher id.
  const tie = byId(camp(10, { updatedAt: '2026-11-01T00:00:00Z' }), camp(11, { updatedAt: '2026-11-01T00:00:00Z' }), camp(12, { updatedAt: '2026-10-01T00:00:00Z' }));
  assert.equal(stepCell([ent(10, 'green', 'es'), ent(11, 'green', 'es'), ent(12, 'green', 'es')], tie, { lang: 'es' }).campaign.id, 11);
  // snake_case updated_at is read too.
  const snake = byId(camp(20, { updatedAt: undefined, updated_at: '2026-12-01T00:00:00Z' }), camp(21));
  assert.equal(stepCell([ent(20, 'green', 'es'), ent(21, 'green', 'es')], snake, { lang: 'es' }).campaign.id, 20);
});

test('I8: All -- en first, then latest, then id; "+N other languages" counts distinct other langs', () => {
  const cs = byId(
    camp(1, { updatedAt: '2026-11-01T00:00:00Z' }),
    camp(2, { updatedAt: '2026-11-09T00:00:00Z' }),
    camp(3, { updatedAt: '2026-11-08T00:00:00Z' }),
    camp(4, { updatedAt: '2026-11-07T00:00:00Z' }),
    camp(5, { updatedAt: '2026-11-06T00:00:00Z' }),
    camp(6, { updatedAt: '2026-11-05T00:00:00Z' }),
  );
  const entries = [ent(1, 'green', 'en'), ent(2, 'green', 'fr'), ent(3, 'green', 'fr'), ent(4, 'green', 'es'), ent(5, 'green', 'en'), ent(6, 'green', null)];
  const all = stepCell(entries, cs, { lang: 'all' });
  // en first: of the two en entries the latest (5) is shown.
  assert.equal(all.campaign.id, 5);
  // fr and es are other languages (fr once); the other en and the language-less one are extras.
  assert.equal(all.otherLangs, 2);
  assert.equal(all.others, 2);
  // No en entry: the latest overall.
  const noEn = stepCell([ent(2, 'green', 'fr'), ent(4, 'green', 'es')], cs);
  assert.equal(noEn.campaign.id, 2);
  assert.equal(noEn.otherLangs, 1);
  assert.equal(noEn.others, 0);
  // One entry: nothing to note.
  assert.deepEqual(stepCell([ent(6)], cs), {
    campaign: cs.get(6), others: 0, otherLangs: 0, noneInLang: false,
  });
  // No entry at all -> the guide-text placeholder.
  assert.equal(stepCell([], cs), null);
  assert.equal(stepCell([], cs, { lang: 'es' }), null);
});

test('I8: the status filter -- by the stored bubble; System-generated by seed or journey tag', () => {
  assert.deepEqual(STATUS_FILTERS.map((f) => f.value), ['all', 'running', 'paused', 'draft', 'system']);
  const seeded = camp(1, { attribs: { journey_seed: 'abc' } });
  const tagged = camp(2, { tags: ['journey', `journey:${WELCOME}:2`] });
  const badTag = camp(3, { tags: ['journey', 'journey:nosuch:2'] });
  const plain = camp(4, { attribs: { journey_seed: null } });
  assert.equal(isSystemGenerated(seeded), true);
  assert.equal(isSystemGenerated(camp(9, { attribs: { journey_seed: '' } })), true);
  assert.equal(isSystemGenerated(tagged), true);
  assert.equal(isSystemGenerated(badTag), false);
  assert.equal(isSystemGenerated(plain), false);
  assert.equal(isSystemGenerated(null), false);
  // Running / Paused / Draft keep the circle's own colour: an inactive row's running automation is
  // stored amber, so it is under Paused.
  assert.equal(statusMatches(ent(1, 'green'), plain, 'running'), true);
  assert.equal(statusMatches(ent(1, 'amber'), plain, 'running'), false);
  assert.equal(statusMatches(ent(1, 'amber'), plain, 'paused'), true);
  assert.equal(statusMatches(ent(1, 'grey'), plain, 'draft'), true);
  assert.equal(statusMatches(ent(1, 'grey'), seeded, 'system'), true);
  assert.equal(statusMatches(ent(1, 'green'), seeded, 'system'), true);
  assert.equal(statusMatches(ent(1, 'grey'), plain, 'system'), false);
  assert.equal(statusMatches(ent(1, 'grey'), plain, 'all'), true);
  assert.equal(statusMatches(ent(1, 'grey'), plain, 'nosuch'), true);
  // Filters, not partitions: a seeded draft is under Draft/scheduled AND System-generated.
  assert.equal(statusMatches(ent(1, 'grey'), seeded, 'draft'), true);
  // Applied before the pick; nothing left -> the placeholder.
  const cs = byId(seeded, plain);
  assert.equal(stepCell([ent(1, 'grey'), ent(4, 'green')], cs, { status: 'running' }).campaign.id, 4);
  assert.equal(stepCell([ent(1, 'grey'), ent(4, 'green')], cs, { status: 'system' }).campaign.id, 1);
  assert.equal(stepCell([ent(1, 'grey'), ent(4, 'green')], cs, { status: 'paused' }), null);
  assert.equal(stepCell([ent(1, 'grey'), ent(4, 'green')], cs, { status: 'paused', lang: 'es' }), null);
  // Stage 4 F1: "None in this language" is decided BEFORE the status filter. An ES automation that
  // the status filter hides -> the plain placeholder (an es automation exists), whatever other
  // languages survive the filter; no ES entry at all -> "None in this language", whatever the filter.
  const en = camp(30);
  const es = camp(31);
  const both = byId(en, es);
  assert.equal(stepCell([ent(30, 'green', 'en'), ent(31, 'amber', 'es')], both, { status: 'running', lang: 'es' }), null);
  assert.equal(stepCell([ent(31, 'amber', 'es')], both, { status: 'running', lang: 'es' }), null);
  assert.equal(stepCell([ent(30, 'green', 'en')], both, { status: 'paused', lang: 'es' }).noneInLang, true);
  assert.equal(stepCell([ent(30, 'green', 'en')], both, { status: 'running', lang: 'es' }).noneInLang, true);
});

test('I2: the page renders exactly the guide\'s series and days, in its order', () => {
  const cols = journeyColumns(null, new Map());
  const guide = seriesGuide();
  assert.deepEqual(cols.map((c) => c.family), guide.map((g) => g.family));
  cols.forEach((c, i) => {
    assert.deepEqual(c.steps.map((s) => s.days), guide[i].steps.map((s) => s.days));
    assert.deepEqual(c.steps.map((s) => s.days), [...guideDays(c.family)]);
    assert.deepEqual(c.steps.map((s) => s.title), guide[i].steps.map((s) => s.title));
    c.steps.forEach((s) => assert.equal(s.preview, null));
  });
});

test('I2: every guide step accepts its own tag (the tag grammar covers the guide)', () => {
  seriesGuide().forEach((g) => g.steps.forEach((s) => {
    assert.deepEqual(parseJourneyTag(`journey:${g.family}:${s.days}`), { family: g.family, day: s.days });
  }));
  ['journey:nosuch:2', `journey:${WELCOME}:3`, `journey:${RE}:181`, `journey:${WELCOME}`, `journey:${WELCOME}:2:x`,
    `journey:${WELCOME}:two`, 'journey', null].forEach((t) => assert.equal(parseJourneyTag(t), null, String(t)));
});

test('journeyColumns attaches each step\'s cell under the language and status', () => {
  const cs = byId(camp(5), camp(6), camp(7), camp(8));
  const filled = (opts) => journeyColumns(usage, cs, opts).flatMap((g) => g.steps.filter((s) => s.preview)
    .map((s) => [g.family, s.days, s.preview.campaign ? s.preview.campaign.id : 'none']));
  // Under All, no en entry on day 2: the same updated_at, so the higher id (6).
  assert.deepEqual(filled({}), [[WELCOME, 2, 6], [POST, 14, 7], [RE, 180, 8]]);
  assert.deepEqual(filled({ lang: 'es' }), [[WELCOME, 2, 6], [POST, 14, 'none'], [RE, 180, 'none']]);
  assert.deepEqual(filled({ status: 'paused' }), [[POST, 14, 7]]);
});

test('helpers: usageBrands, pickBrand, frameScale, previewTarget', () => {
  assert.deepEqual(usageBrands([{ brand: 'ruze', usage: {} }, { brand: 'x', usage: null }, { brand: '', usage: {} }, null, { brand: 'tg', usage: { v: 1 } }]), ['ruze', 'tg']);
  assert.equal(pickBrand(['a', 'b'], 'b'), 'b');
  assert.equal(pickBrand(['a', 'b'], 'z'), 'a');
  assert.equal(pickBrand([], 'a'), null);
  assert.equal(frameScale(FRAME_WIDTH * 2), 1);
  assert.equal(frameScale(300), 300 / FRAME_WIDTH);
  assert.equal(frameScale(0), 1);
  assert.deepEqual(previewTarget('ruze'), { name: 'journeyPreview', query: { brand: 'ruze' } });
});
