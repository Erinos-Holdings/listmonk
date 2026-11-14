// Fork (naming standard) -- integrations listmonk/NAMING-STANDARD-SPEC I11 and the client half of
// I12: naming.mjs -- the chips and their hiding (D1/D15), langMatches over the SHARED case table
// testdata/lang-filter.json (cmd/campaigns_test.go reads the same table against the server's
// query-campaigns lang filter), listDisplayName's exact prefix only (D6), cloneName (D7),
// duplicateOf (D8), the former name hidden when equal (D10) and the name counter (D3); plus
// langScope.mjs. Run: node --test src/naming.test.mjs (yarn test:naming).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  LANG_VALUES, NAME_MAX, campaignBrandOf, chipsFor, cloneName, duplicateOf, formerNameOf, isExempt, langMatches,
  langOf, listDisplayName, nameCounter, normalizeLang,
} from './naming.mjs'; // eslint-disable-line import/extensions
import {
  LANG_PREF, LANG_ROUTES, langLabel, langParam, langToggleShown,
} from './langScope.mjs'; // eslint-disable-line import/extensions

const ALL = { kind: 'all' };
const RUZE = { kind: 'brand', slug: 'ruze' };
const NONE = { kind: 'none' };

test('I12: langMatches agrees with the shared case table (no-language rows always included)', () => {
  const fx = JSON.parse(readFileSync(new URL('./testdata/lang-filter.json', import.meta.url), 'utf8'));
  assert.ok(fx.cases.length >= 10);
  fx.cases.forEach((c) => {
    assert.equal(langMatches(c.lang, c.toggle), c.match, JSON.stringify(c));
    // The toggle's 'all' is the table's ''.
    if (c.toggle === '') assert.equal(langMatches(c.lang, 'all'), c.match);
  });
  assert.equal(langMatches(undefined, 'fr'), true);
  assert.equal(langMatches('FR', 'fr'), true);
});

test('I11: chipsFor -- brand in capitals, language from attribs.lang or the lang column; each hidden by its own selection', () => {
  const camp = { brand: 'thirstygirl', attribs: { lang: 'en' } };
  assert.deepEqual(chipsFor(camp, { brandSelection: ALL, langSelection: 'all' }), { brand: 'THIRSTYGIRL', lang: 'EN' });
  assert.deepEqual(chipsFor(camp, { brandSelection: { kind: 'brand', slug: 'thirstygirl' }, langSelection: 'all' }), { brand: null, lang: 'EN' });
  assert.deepEqual(chipsFor(camp, { brandSelection: RUZE, langSelection: 'en' }), { brand: 'THIRSTYGIRL', lang: null });
  assert.deepEqual(chipsFor(camp, { brandSelection: NONE, langSelection: 'fr' }), { brand: 'THIRSTYGIRL', lang: 'EN' });
  // None when absent.
  assert.deepEqual(chipsFor({ attribs: {} }, { brandSelection: ALL, langSelection: 'all' }), { brand: null, lang: null });
  assert.deepEqual(chipsFor({ brand: '', attribs: { lang: '' } }, {}), { brand: null, lang: null });
  // A template: its brand and lang columns; a Matrix row: its lang field.
  assert.deepEqual(chipsFor({ brand: 'shala', lang: 'fr' }, { brandSelection: ALL, langSelection: 'all' }), { brand: 'SHALA', lang: 'FR' });
  assert.deepEqual(chipsFor({ brand: 'ruze', lang: 'it' }, { brandSelection: RUZE, langSelection: 'it' }), { brand: null, lang: null });
  assert.equal(langOf({ attribs: { lang: 'DE' } }), 'de');
});

test('campaignBrandOf: the row\'s own brand, else the one brand of its lists, else none', () => {
  const map = { 1: 'ruze', 2: 'ruze', 3: 'shala' };
  assert.equal(campaignBrandOf({ brand: 'bnkd', lists: [{ id: 3 }] }, map), 'bnkd');
  assert.equal(campaignBrandOf({ lists: [{ id: 1 }, { id: 2 }] }, map), 'ruze');
  assert.equal(campaignBrandOf({ lists: [{ id: 1 }, { id: 3 }] }, map), '');
  assert.equal(campaignBrandOf({ lists: [{ id: 9 }] }, map), '');
  assert.equal(campaignBrandOf({ lists: [] }, map), '');
});

test('I11: formerNameOf -- shown only when present and different from the name', () => {
  assert.equal(formerNameOf({ name: 'Welcome', former_name: 'Ruze_Welcome_EN' }), 'Ruze_Welcome_EN');
  assert.equal(formerNameOf({ name: 'Welcome', formerName: 'Ruze_Welcome_EN' }), 'Ruze_Welcome_EN');
  assert.equal(formerNameOf({ name: 'Welcome', former_name: 'Welcome' }), null);
  assert.equal(formerNameOf({ name: 'Welcome', former_name: null }), null);
  assert.equal(formerNameOf({ name: 'Welcome' }), null);
});

test('I11: listDisplayName -- under one brand, only that brand\'s exact SLUG or display-name prefix is hidden', () => {
  const brands = [{ slug: 'thirstygirl', display_name: 'Thirsty Girl' }, { slug: 'liyora', display_name: 'LIYORÁ' }];
  const tg = { kind: 'brand', slug: 'thirstygirl' };
  assert.equal(listDisplayName({ name: 'THIRSTYGIRL No-purchase D2', brand: 'thirstygirl' }, tg, brands), 'No-purchase D2');
  assert.equal(listDisplayName({ name: 'Thirsty Girl Emails', brand: 'thirstygirl' }, tg, brands), 'Emails');
  // Not the selected brand, not this list's brand, a near-miss case or spacing, all brands, none: whole.
  assert.equal(listDisplayName({ name: 'THIRSTYGIRL No-purchase D2', brand: 'thirstygirl' }, ALL, brands), 'THIRSTYGIRL No-purchase D2');
  assert.equal(listDisplayName({ name: 'THIRSTYGIRL x', brand: 'ruze' }, tg, brands), 'THIRSTYGIRL x');
  assert.equal(listDisplayName({ name: 'ThirstyGirl No-purchase D2', brand: 'thirstygirl' }, tg, brands), 'ThirstyGirl No-purchase D2');
  assert.equal(listDisplayName({ name: 'THIRSTYGIRLNo-purchase', brand: 'thirstygirl' }, tg, brands), 'THIRSTYGIRLNo-purchase');
  assert.equal(listDisplayName({ name: 'Why We Call It THIRSTYGIRL', brand: 'thirstygirl' }, tg, brands), 'Why We Call It THIRSTYGIRL');
  assert.equal(listDisplayName({ name: 'Liyora AirLuxe', brand: 'liyora' }, { kind: 'brand', slug: 'liyora' }, brands), 'Liyora AirLuxe');
  assert.equal(listDisplayName({ name: 'LIYORA AirLuxe', brand: 'liyora' }, { kind: 'brand', slug: 'liyora' }, brands), 'AirLuxe');
  assert.equal(listDisplayName({ name: 'LIYORÁ Emails', brand: 'liyora' }, { kind: 'brand', slug: 'liyora' }, brands), 'Emails');
  assert.equal(listDisplayName({ name: 'THIRSTYGIRL ', brand: 'thirstygirl' }, tg, brands), 'THIRSTYGIRL ');
  assert.equal(listDisplayName({ name: 'Render catalog (never send)', brand: '' }, NONE, brands), 'Render catalog (never send)');
});

test('I11: cloneName -- Clone<N> <base>, N one past the highest existing for that base', () => {
  assert.equal(cloneName('Welcome', []), 'Clone1 Welcome');
  assert.equal(cloneName('Welcome', ['Welcome', 'Clone1 Welcome']), 'Clone2 Welcome');
  assert.equal(cloneName('Welcome', ['Clone1 Welcome', 'Clone4 Welcome', 'Clone9 Other']), 'Clone5 Welcome');
  // Cloning a clone clones the base.
  assert.equal(cloneName('Clone2 Welcome', ['Clone1 Welcome', 'Clone2 Welcome']), 'Clone3 Welcome');
  // Never truncated (the server refuses over 30).
  const long = 'A name of exactly thirty chars';
  assert.equal(long.length, 30);
  assert.equal(cloneName(long, []), `Clone1 ${long}`);
  // Not Copy of.
  assert.ok(!cloneName('x', []).startsWith('Copy of'));
});

test('I11: duplicateOf -- another OPEN campaign, same trimmed case-insensitive name, brand and language', () => {
  const rows = [
    {
      id: 1, name: 'Welcome ', status: 'running', brand: 'ruze', attribs: { lang: 'en' },
    },
    {
      id: 2, name: 'welcome', status: 'finished', brand: 'ruze', attribs: { lang: 'en' },
    },
    {
      id: 3, name: 'Welcome', status: 'draft', brand: 'ruze', attribs: { lang: 'fr' },
    },
    {
      id: 4, name: 'Welcome', status: 'paused', brand: 'shala', attribs: { lang: 'en' },
    },
  ];
  assert.equal(duplicateOf({
    id: 9, name: ' WELCOME', brand: 'ruze', lang: 'en',
  }, rows).id, 1);
  assert.equal(duplicateOf({
    id: 1, name: 'Welcome', brand: 'ruze', lang: 'en',
  }, rows), null); // itself
  assert.equal(duplicateOf({
    id: 9, name: 'Welcome', brand: 'ruze', lang: 'de',
  }, rows), null);
  assert.equal(duplicateOf({
    id: 9, name: 'Welcome', brand: 'ruze', lang: 'fr',
  }, rows).id, 3);
  assert.equal(duplicateOf({
    id: 9, name: 'Welcome', brand: 'bnkd', lang: 'en',
  }, rows), null);
  assert.equal(duplicateOf({
    id: 9, name: '', brand: 'ruze', lang: 'en',
  }, rows), null);
  assert.equal(duplicateOf({
    id: 9, name: 'Welcome', brand: 'ruze', lang: 'en',
  }, [rows[1]]), null); // finished is not open
});

test('nameCounter / isExempt -- 30 on the handle; a list adds its prefix to maxlength; exempt names are unlimited', () => {
  assert.equal(NAME_MAX, 30);
  assert.deepEqual(nameCounter('campaign', 'Welcome'), {
    n: 7, max: 30, maxlength: 30, over: false,
  });
  assert.equal(nameCounter('campaign', 'x'.repeat(31)).over, true);
  assert.deepEqual(nameCounter('list', 'THIRSTYGIRL No-purchase D2', 'THIRSTYGIRL '), {
    n: 14, max: 30, maxlength: 42, over: false,
  });
  assert.equal(nameCounter('campaign', 'CATALOG 01-button-outlook-word-columns').maxlength, null);
  assert.equal(nameCounter('template', 'Official_footer_corporate_long_name_en').maxlength, null);
  assert.equal(isExempt('list', 'CATALOG x'), false);
  assert.equal(nameCounter('campaign', 'ÁÁÁ').n, 3);
});

test('langScope: values, the pref, the routes it shows on, its label and the Campaigns param', () => {
  assert.deepEqual([...LANG_VALUES], ['all', 'en', 'es', 'fr', 'de', 'it']);
  assert.equal(LANG_PREF, 'brand.lang');
  assert.deepEqual([...LANG_ROUTES], ['campaigns', 'templates', 'campaignAnalytics']);
  ['campaigns', 'templates', 'campaignAnalytics'].forEach((r) => assert.equal(langToggleShown(r), true));
  ['lists', 'subscribers', 'dashboard', 'media', 'campaign'].forEach((r) => assert.equal(langToggleShown(r), false));
  assert.equal(langLabel('all', 'All'), 'All');
  assert.equal(langLabel('fr', 'All'), 'FR');
  assert.equal(langParam('all'), '');
  assert.equal(langParam('de'), 'de');
  assert.equal(normalizeLang('pt'), 'all');
  assert.equal(normalizeLang(' IT '), 'it');
});
