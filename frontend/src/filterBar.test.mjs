// Fork (polish 2026-11-14): filterBar.mjs -- the filter caption's text: the brand's display name and
// the two-letter language, nothing else. Run: node --test src/filterBar.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { CAPTION_LANG_NAMES, filterCaption, subscriberCaptionLang } from './filterBar.mjs'; // eslint-disable-line import/extensions

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

// SUBSCRIBERS-FILTERS-SPEC I13 -- the Subscribers page's own caption language; the global one is
// untouched.
test("Subscribers' caption language names the route language", () => {
  const labels = { none: 'No language', other: 'Other' };
  assert.equal(subscriberCaptionLang('en', labels), 'English+');
  // SUBSCRIBERS-FILTERS-2-SPEC D1: en_only / none are no longer toggle values (the guard drops a
  // route carrying one) and their caption branches are deleted -- neither names a language.
  assert.equal(subscriberCaptionLang('en_only', labels), '');
  assert.equal(subscriberCaptionLang('none', labels), '');
  assert.equal(subscriberCaptionLang('other', labels), 'Other');
  assert.equal(subscriberCaptionLang('fr', labels), 'French');
  assert.equal(subscriberCaptionLang('it', labels), 'Italian');
  assert.equal(subscriberCaptionLang('', labels), '');
  assert.equal(subscriberCaptionLang(null, labels), '');
  assert.equal(subscriberCaptionLang('all', labels), '');
  // The global caption under en still reads "English" (CAPTION_LANG_NAMES untouched).
  assert.equal(CAPTION_LANG_NAMES.en, 'English');
  assert.equal(filterCaption({ selection: BNKD, lang: 'en', brands: BRANDS }), 'BNKD Wellness · English');
  // A subscriber-only value (never passed by langEffective) names nothing in the global caption.
  assert.equal(filterCaption({ selection: BNKD, lang: 'none', brands: BRANDS }), 'BNKD Wellness');
  // Subscribers passes it as its langText.
  const subs = readFileSync(new URL('./views/Subscribers.vue', import.meta.url), 'utf8');
  assert.match(subs, /\$filterCaption\(\{ langText: captionLang \}\)/);
  assert.match(subs, /captionLang\(\) \{[\s\S]*?return subscriberCaptionLang\(this\.queryParams\.lang, \{/);
  // Other views read the folded selection.
  assert.match(readFileSync(new URL('./main.js', import.meta.url), 'utf8'), /lang: this\.\$store\.getters\.langEffective,/);
});

// SUBSCRIBERS-FILTERS-2-SPEC D4 (I8) -- under EN+ the split renders only from a response for the
// current query; otherwise English+.
test('EN+ caption split: only from the current query\'s answer', () => {
  const labels = { none: 'No language', other: 'Other' };
  const fmt = (n) => n.toLocaleString('en-US');
  const ok = { current: true, total: 602, langNoneTotal: 219 };
  assert.equal(subscriberCaptionLang('en', labels, ok, fmt), 'English 383 + No language 219');
  const big = { current: true, total: 174441, langNoneTotal: 8404 };
  assert.equal(subscriberCaptionLang('en', labels, big, fmt), 'English 166,037 + No language 8,404');
  // Zero parts render.
  assert.equal(subscriberCaptionLang('en', labels, { current: true, total: 219, langNoneTotal: 219 }, fmt), 'English 0 + No language 219');
  assert.equal(subscriberCaptionLang('en', labels, { current: true, total: 0, langNoneTotal: 0 }, fmt), 'English 0 + No language 0');
  // Not for the current query (busy, or another query's answer), absent or malformed: English+.
  assert.equal(subscriberCaptionLang('en', labels, { ...ok, current: false }, fmt), 'English+');
  assert.equal(subscriberCaptionLang('en', labels, { current: true, total: 602 }, fmt), 'English+');
  assert.equal(subscriberCaptionLang('en', labels, { current: true, total: 602, langNoneTotal: 700 }, fmt), 'English+');
  assert.equal(subscriberCaptionLang('en', labels, null, fmt), 'English+');
  assert.equal(subscriberCaptionLang('en', labels), 'English+');
  // Every other language is unchanged, split or not.
  assert.equal(subscriberCaptionLang('fr', labels, ok, fmt), 'French');
  assert.equal(subscriberCaptionLang('other', labels, ok, fmt), 'Other');
  assert.equal(subscriberCaptionLang('', labels, ok, fmt), '');
  // The page's wiring: current = the answered filter under en and not busy; the store's
  // langNoneTotal (camelCased lang_none_total); the formatter is $utils.formatNumber.
  const subs = readFileSync(new URL('./views/Subscribers.vue', import.meta.url), 'utf8');
  assert.match(subs, /current: !this\.byQueryBusy && !!answered && answered\.lang === 'en',/);
  assert.match(subs, /langNoneTotal: this\.subscribers\.langNoneTotal,/);
  assert.match(subs, /split, \(n\) => this\.\$utils\.formatNumber\(n\)\)/);
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
