// Fork (2026-11-02, user): the parent brand -- the selector's order and the Brands › Usage band
// (Loyalty Rewards block placement, months, EN alignment, the row out of the sort).
// Run: yarn test (node --test src/*.test.mjs).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { PARENT_BRAND, selectorBrands } from './brandScope.mjs'; // eslint-disable-line import/extensions
import {
  LOYALTY_LANGS, LOYALTY_PLACEHOLDER, SKIPPED_FIELDS, bodyColumns, loyaltyLayout, loyaltyMonths, loyaltyShift, splitParent,
} from './brandUsage.mjs'; // eslint-disable-line import/extensions

test('the parent brand is curated', () => {
  assert.equal(PARENT_BRAND, 'curated');
});

test('selectorBrands puts the parent first and keeps the rest in roster order', () => {
  const brands = [{ slug: 'joybelle', label: 'JoyBelle' }, { slug: 'curated', label: 'Curated' }, { slug: 'ruze', label: 'Ruze' }];
  const o = selectorBrands(brands);
  assert.deepEqual(o.parent, { slug: 'curated', label: 'Curated' });
  assert.deepEqual(o.rest.map((b) => b.slug), ['joybelle', 'ruze']);
});

test('selectorBrands without the parent (no permitted parent lists) has no parent', () => {
  const o = selectorBrands([{ slug: 'ruze', label: 'Ruze' }]);
  assert.equal(o.parent, null);
  assert.deepEqual(o.rest.map((b) => b.slug), ['ruze']);
  assert.deepEqual(selectorBrands(undefined), { parent: null, rest: [] });
});

test('splitParent takes the parent row out of the sorted rows', () => {
  const s = splitParent([{ brand: 'ruze' }, { brand: 'curated' }, { brand: 'shala' }]);
  assert.deepEqual(s.parent, { brand: 'curated' });
  assert.deepEqual(s.rest.map((r) => r.brand), ['ruze', 'shala']);
  assert.equal(splitParent([{ brand: 'ruze' }]).parent, null);
});

test('loyaltyLayout puts EN over SKU and FR..IT over their Skipped columns, spanning the whole row', () => {
  const cols = bodyColumns();
  const l = loyaltyLayout(cols);
  assert.ok(l);
  assert.deepEqual(l.cells.map((c) => c.lang), [...LOYALTY_LANGS]);
  assert.deepEqual(LOYALTY_LANGS, ['en', 'fr', 'es', 'de', 'it']);
  assert.deepEqual(l.cells.map((c) => c.col.field), ['sku', ...SKIPPED_FIELDS]);
  assert.equal(l.lead + l.cells.length + l.tail, 2 + cols.length);
  // Each cell carries its body column's flags, so it pads as the column below it.
  l.cells.forEach((c) => assert.equal(c.col, cols.find((x) => x.field === c.col.field)));
});

test('loyaltyLayout refuses a column order where SKU and the Skipped leaves are not adjacent', () => {
  const cols = bodyColumns().filter((c) => c.field !== SKIPPED_FIELDS[0]);
  assert.equal(loyaltyLayout(cols), null);
  assert.equal(loyaltyLayout([]), null);
});

test('loyaltyMonths is this month and next in Mountain time', () => {
  assert.deepEqual(loyaltyMonths(new Date('2026-11-02T18:00:00Z')), [
    { key: '2026-11', label: 'Nov 2026' }, { key: '2026-12', label: 'Dec 2026' },
  ]);
  // 03:00Z on Dec 1 is still Nov 30 in Denver; the year rolls in December.
  assert.deepEqual(loyaltyMonths(new Date('2026-12-01T03:00:00Z')).map((m) => m.key), ['2026-11', '2026-12']);
  assert.deepEqual(loyaltyMonths(new Date('2026-12-15T12:00:00Z')).map((m) => m.label), ['Dec 2026', 'Jan 2027']);
});

test('the placeholder is one red 0 (an expected campaign, never a dash)', () => {
  assert.deepEqual(LOYALTY_PLACEHOLDER, [{ color: 'red', n: 0 }]);
});

test('loyaltyShift puts EN one mean FR..IT step left of FR', () => {
  // FR..IT 40px apart; EN naturally at 120 (FR at 200) -> 160 wanted, shift 40.
  assert.equal(loyaltyShift([120, 200, 240, 280, 320]), 40);
  // Already shifted by 40 and measured at 160: stays 40 (idempotent).
  assert.equal(loyaltyShift([160, 200, 240, 280, 320], 40), 40);
  // Unmeasurable input: no shift.
  assert.equal(loyaltyShift([1, 2]), 0);
  assert.equal(loyaltyShift([NaN, 2, 3, 4, 5]), 0);
});
