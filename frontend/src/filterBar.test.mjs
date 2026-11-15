// Fork (polish 2026-11-14): filterBar.mjs -- the filter caption's text: the brand's display name and
// the two-letter language, nothing else. Run: node --test src/filterBar.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { filterCaption } from './filterBar.mjs'; // eslint-disable-line import/extensions

const BRANDS = [{ slug: 'bnkd', display_name: 'BNKD Wellness' }, { slug: 'ruze', display_name: 'RUZE' }];
const BNKD = { kind: 'brand', slug: 'bnkd' };

test('nothing active: no caption', () => {
  assert.equal(filterCaption({ selection: { kind: 'all' }, lang: 'all', brands: BRANDS }), '');
  assert.equal(filterCaption(), '');
  // A filter the table does not take is never named.
  assert.equal(filterCaption({
    selection: BNKD, lang: 'fr', brands: BRANDS, applies: { brand: false, lang: false },
  }), '');
});

test('brand display name, English language name, separated by a dot', () => {
  assert.equal(filterCaption({ selection: BNKD, lang: 'en', brands: BRANDS }), 'BNKD Wellness · English');
  assert.equal(filterCaption({ selection: BNKD, lang: 'all', brands: BRANDS }), 'BNKD Wellness');
  assert.equal(filterCaption({ selection: { kind: 'all' }, lang: 'es', brands: BRANDS }), 'Spanish');
  assert.equal(filterCaption({ selection: { kind: 'none' }, lang: 'de', noBrand: 'No brand' }), 'No brand · German');
  assert.equal(filterCaption({ selection: { kind: 'all' }, lang: 'fr' }), 'French');
  assert.equal(filterCaption({ selection: { kind: 'all' }, lang: 'it' }), 'Italian');
  // Language not applicable to the table: brand only.
  assert.equal(filterCaption({
    selection: BNKD, lang: 'en', brands: BRANDS, applies: { lang: false },
  }), 'BNKD Wellness');
  // A slug with no brands row falls back to the slug; an unknown language to none.
  assert.equal(filterCaption({ selection: { kind: 'brand', slug: 'newbrand' }, lang: 'pt' }), 'newbrand');
});

test("a page's own language (langText) replaces the global one", () => {
  // Subscribers: the route's lang, whatever the toggle says.
  assert.equal(filterCaption({
    selection: BNKD, lang: 'en', brands: BRANDS, langText: 'Spanish',
  }), 'BNKD Wellness · Spanish');
  // '' (route lang all): no language, even with the toggle on ES.
  assert.equal(filterCaption({
    selection: BNKD, lang: 'es', brands: BRANDS, langText: '',
  }), 'BNKD Wellness');
  assert.equal(filterCaption({ selection: { kind: 'all' }, lang: 'all', langText: 'No language' }), 'No language');
});

test('the global helper and every table use the caption', () => {
  const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');
  assert.match(read('./main.js'), /Vue\.prototype\.\$filterCaption = function/);
  // Every narrowed table carries it; the old bar component is gone.
  ['./components/CampaignMatrix.vue', './components/BrandMatrix.vue', './components/BrandUsage.vue',
    './views/Brands.vue', './views/Campaigns.vue', './views/Templates.vue', './views/Lists.vue',
    './views/Subscribers.vue', './views/Bounces.vue'].forEach((p) => {
    const vue = read(p);
    assert.match(vue, /\$filterCaption\(/, p);
    assert.doesNotMatch(vue, /global-filter-bar/, p);
  });
});
