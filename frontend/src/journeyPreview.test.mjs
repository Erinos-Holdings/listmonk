// Fork (journey preview, integrations listmonk/JOURNEY-PREVIEW-SPEC I1/I2). Run: yarn test
// (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  FRAME_WIDTH, frameScale, journeyColumns, listBrandOf, parseJourneyTag, pickBrand, stepPreviews, usageTarget,
} from './journeyPreview.mjs'; // eslint-disable-line import/extensions
import { GUIDE_SERIES, guideDays, seriesGuide } from './brandUsage.mjs'; // eslint-disable-line import/extensions

const [WELCOME, POST, RE] = GUIDE_SERIES.map((g) => g.family);

const lists = [
  { id: 6, tags: ['brand:thirstygirl'] },
  { id: 7, tags: ['brand:thirstygirl', 'other'] },
  { id: 8, tags: ['brand:ruze'] },
  { id: 9, tags: [] },
];

const camp = (id, tags, listIds, updatedAt = '2026-11-01T00:00:00Z') => ({
  id, tags, lists: listIds.map((l) => ({ id: l })), updatedAt,
});

test('I1: a valid journey tag maps to its (brand, family, day) cell', () => {
  const c = camp(1, ['journey', `journey:${WELCOME}:2`], [6]);
  const r = stepPreviews([c], lists);
  assert.deepEqual(r.brands, ['thirstygirl']);
  assert.deepEqual(r.byBrand.thirstygirl[WELCOME][2], { campaign: c, others: 0 });
});

test('I1: the brand is the one brand: tag across the lists; brandless lists add nothing', () => {
  const r = stepPreviews([camp(1, [`journey:${POST}:5`], [6, 7, 9])], lists);
  assert.ok(r.byBrand.thirstygirl[POST][5]);
});

test('I1: unknown family, non-standard day, malformed tags are ignored', () => {
  const r = stepPreviews([
    camp(1, ['journey:nosuch:2'], [6]),
    camp(2, [`journey:${WELCOME}:3`], [6]),
    camp(3, [`journey:${RE}:181`], [6]),
    camp(4, [`journey:${WELCOME}`], [6]),
    camp(5, [`journey:${WELCOME}:2:x`], [6]),
    camp(6, [`journey:${WELCOME}:two`], [6]),
    camp(7, ['journey'], [6]),
  ], lists);
  assert.deepEqual(r, { brands: [], byBrand: {} });
});

test('I1: two-brand and brandless campaigns are ignored', () => {
  const r = stepPreviews([
    camp(1, [`journey:${WELCOME}:0`], [6, 8]),
    camp(2, [`journey:${WELCOME}:0`], [9]),
    camp(3, [`journey:${WELCOME}:0`], []),
    camp(4, [`journey:${WELCOME}:0`], [999]),
  ], lists);
  assert.deepEqual(r, { brands: [], byBrand: {} });
});

test('I1: two campaigns on one cell -> the later updated_at wins, others = 1 (either order)', () => {
  const older = camp(10, [`journey:${RE}:365`], [6], '2026-10-01T00:00:00Z');
  const newer = camp(11, [`journey:${RE}:365`], [6], '2026-10-02T00:00:00Z');
  [[older, newer], [newer, older]].forEach((order) => {
    const cell = stepPreviews(order, lists).byBrand.thirstygirl[RE][365];
    assert.equal(cell.campaign.id, 11);
    assert.equal(cell.others, 1);
  });
  // snake_case rows (the raw API) are read too.
  const a = { ...older, updatedAt: undefined, updated_at: '2026-10-03T00:00:00Z' };
  assert.equal(stepPreviews([newer, a], lists).byBrand.thirstygirl[RE][365].campaign.id, 10);
});

test('I1: brands are listed once each, sorted', () => {
  const r = stepPreviews([
    camp(1, [`journey:${WELCOME}:0`], [8]),
    camp(2, [`journey:${WELCOME}:0`], [6]),
    camp(3, [`journey:${POST}:0`], [6]),
  ], lists);
  assert.deepEqual(r.brands, ['ruze', 'thirstygirl']);
});

test('I2: the page renders exactly the guide\'s series and days, in its order', () => {
  const cols = journeyColumns(stepPreviews([], lists), 'thirstygirl');
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
});

test('journeyColumns attaches the brand\'s cell to its step only', () => {
  const c = camp(1, [`journey:${POST}:14`], [6]);
  const cols = journeyColumns(stepPreviews([c], lists), 'thirstygirl');
  const filled = cols.flatMap((g) => g.steps.filter((s) => s.preview).map((s) => [g.family, s.days, s.preview.campaign.id]));
  assert.deepEqual(filled, [[POST, 14, 1]]);
  assert.ok(journeyColumns(stepPreviews([c], lists), 'ruze').every((g) => g.steps.every((s) => !s.preview)));
});

test('helpers: listBrandOf, pickBrand, frameScale, usageTarget', () => {
  assert.equal(listBrandOf({ tags: ['x', 'brand:ruze'] }), 'ruze');
  assert.equal(listBrandOf({ tags: ['brand:'] }), '');
  assert.equal(listBrandOf(null), '');
  assert.equal(pickBrand(['a', 'b'], 'b'), 'b');
  assert.equal(pickBrand(['a', 'b'], 'z'), 'a');
  assert.equal(pickBrand([], 'a'), null);
  assert.equal(frameScale(FRAME_WIDTH * 2), 1);
  assert.equal(frameScale(300), 300 / FRAME_WIDTH);
  assert.equal(frameScale(0), 1);
  assert.deepEqual(usageTarget({ kind: 'brand', slug: 'ruze' }), { name: 'journeyPreview', query: { brand: 'ruze' } });
  assert.deepEqual(usageTarget({ kind: 'all' }), { name: 'journeyPreview' });
  assert.deepEqual(usageTarget({ kind: 'none' }), { name: 'journeyPreview' });
});
