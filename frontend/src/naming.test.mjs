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
  LANG_VALUES, NAME_MAX, WARNING_CODES, campaignBrandOf, chipsFor, clearsPrefill, cloneBaseOf, cloneName, duplicateOf,
  formatNameDate, formerNameOf, isExempt, joinListName, langMatches, langOf, listDisplayName, listPrefixAddon, nameCounter,
  namePlaceholder, namePrefill, nameWarnings, normalizeLang, splitListName,
} from './naming.mjs'; // eslint-disable-line import/extensions
import { LANG_PREF, langLabel, langParam } from './langScope.mjs'; // eslint-disable-line import/extensions

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
  // The base the Campaigns page searches for (review S4-10).
  assert.equal(cloneBaseOf('Clone3 Welcome'), 'Welcome');
  assert.equal(cloneBaseOf('Welcome'), 'Welcome');
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

// ---- NAME-GUARDRAILS-SPEC I1-I4b: the live naming warnings, the sample and the prefill. ----

const WX = JSON.parse(readFileSync(new URL('./testdata/name-warnings.json', import.meta.url), 'utf8'));
const wctx = (c) => ({ brands: WX.brands, ...(c.ctx || {}) });

test('I1/I2/I4b: nameWarnings answers every case of the shared table', () => {
  assert.ok(WX.cases.length >= 80);
  WX.cases.forEach((c) => {
    assert.deepEqual(nameWarnings(c.kind, c.name, wctx(c)), c.warnings, `${c.kind} ${JSON.stringify(c.name)}`);
  });
  // Every code is exercised firing at least once by the table.
  WARNING_CODES.forEach((code) => assert.ok(WX.cases.some((c) => c.warnings.includes(code)), code));
});

test('I1: codes come in the fixed order, each at most once', () => {
  WX.cases.forEach((c) => {
    const w = nameWarnings(c.kind, c.name, wctx(c));
    assert.deepEqual(w, WARNING_CODES.filter((code) => w.includes(code)));
  });
});

test('I1: no typing prefix of a correct name shows dateFormat', () => {
  const target = '11-14-26 Product Launch';
  for (let i = 1; i <= target.length; i += 1) {
    const w = nameWarnings('campaign', target.slice(0, i), { brands: WX.brands });
    assert.ok(!w.includes('dateFormat'), `${JSON.stringify(target.slice(0, i))} -> ${w}`);
  }
  const month = '11-2026 Winback';
  for (let i = 1; i <= month.length; i += 1) {
    assert.ok(!nameWarnings('campaign', month.slice(0, i), { brands: WX.brands }).includes('dateFormat'), month.slice(0, i));
  }
  // A list name being typed never shows listPrefix (S2-8).
  ['RUZE Sale', 'Ruze Pouches Emails'].forEach((full, k) => {
    const ctx = { brands: WX.brands, brandSlug: 'ruze', listType: k === 0 ? 'private' : 'public' };
    for (let i = 1; i <= full.length; i += 1) assert.ok(!nameWarnings('list', full.slice(0, i), ctx).includes('listPrefix'), full.slice(0, i));
  });
});

const DAYS = [new Date(2026, 10, 14), new Date(2026, 0, 5), new Date(2027, 8, 9), new Date(2028, 1, 29)];

test('formatNameDate: MM-DD-YY, local, zero-padded', () => {
  assert.equal(formatNameDate(new Date(2026, 10, 14)), '11-14-26');
  assert.equal(formatNameDate(new Date(2026, 0, 5)), '01-05-26');
  assert.equal(formatNameDate(new Date(2030, 8, 9, 23, 59)), '09-09-30');
});

test('I3: every placeholder yields no warning and fits the counter', () => {
  DAYS.forEach((today) => {
    const p = namePlaceholder('campaign', { today });
    assert.equal(p, `${formatNameDate(today)} Product Launch`);
    assert.deepEqual(nameWarnings('campaign', p, { brands: WX.brands }), []);
    assert.equal(nameCounter('campaign', p).over, false);
  });
  assert.equal(namePlaceholder('template', {}), 'Newsletter');
  assert.deepEqual(nameWarnings('template', 'Newsletter', { brands: WX.brands }), []);
  const lists = [
    [{ brandSlug: 'ruze', brandDisplay: 'Ruze Pouches', listType: 'private' }, 'RUZE Product Launch', 'RUZE '],
    [{ brandSlug: 'ruze', brandDisplay: 'Ruze Pouches', listType: 'public' }, 'Ruze Pouches Emails', 'Ruze Pouches '],
    [{ brandSlug: 'liyora', brandDisplay: 'LIYORÁ', listType: 'public' }, 'LIYORÁ Emails', 'LIYORÁ '],
    [{ brandSlug: '', brandDisplay: '', listType: 'private' }, 'BRAND Product Launch', ''],
    [{ brandSlug: '', brandDisplay: '', listType: 'public' }, 'Brand Emails', ''],
  ];
  lists.forEach(([ctx, want, prefix]) => {
    const p = namePlaceholder('list', ctx);
    assert.equal(p, want);
    assert.deepEqual(nameWarnings('list', p, { brands: WX.brands, brandSlug: ctx.brandSlug, listType: ctx.listType }), [], p);
    assert.equal(nameCounter('list', p, prefix).over, false);
  });
});

test('I4: namePrefill and clearsPrefill', () => {
  DAYS.forEach((today) => {
    const p = namePrefill('campaign', { today });
    assert.equal(p, `${formatNameDate(today)} `);
    assert.deepEqual(nameWarnings('campaign', p, { brands: WX.brands }), ['handleMissing']);
  });
  assert.equal(namePrefill('campaign', { today: new Date(2026, 2, 4) }), '03-04-26 ');
  // D8: no list prefill of any kind -- the private prefix is the addon.
  assert.equal(namePrefill('template', { today: new Date() }), '');
  assert.equal(namePrefill('list', { brandSlug: 'ruze', listType: 'private' }), '');
  assert.equal(namePrefill('list', { brandSlug: 'ruze', listType: 'public' }), '');
  assert.equal(namePrefill('list', { brandSlug: '', listType: 'private' }), '');

  const p = '11-14-26 ';
  assert.equal(clearsPrefill(p, p), true);
  assert.equal(clearsPrefill('', p), false);
  assert.equal(clearsPrefill(`${p}x`, p), false);
  assert.equal(clearsPrefill(p.trimEnd(), p), false);
  assert.equal(clearsPrefill('', ''), false);
  assert.equal(clearsPrefill('anything', ''), false);
});

test('I4 (D8): listPrefixAddon, splitListName and joinListName', () => {
  assert.equal(listPrefixAddon({ brandSlug: 'ruze', listType: 'private', isNew: true }), 'RUZE');
  const edit = (brandSlug, storedName) => listPrefixAddon({
    brandSlug, listType: 'private', isNew: false, storedName,
  });
  assert.equal(edit('ruze', 'RUZE Sale'), 'RUZE');
  assert.equal(edit('curated', 'RobbieList'), '');
  assert.equal(edit('ruze', 'RUZESale'), '');
  assert.equal(listPrefixAddon({ brandSlug: 'ruze', listType: 'public', isNew: true }), '');
  assert.equal(listPrefixAddon({ brandSlug: '', listType: 'private', isNew: true }), '');
  assert.equal(listPrefixAddon({ brandSlug: null, listType: 'private', isNew: true }), '');

  [['RUZE', 'RUZE Re-engage D180', 'ruze'], ['LIYORA', 'LIYORA AirLuxe', 'liyora'],
    ['THIRSTYGIRL', 'THIRSTYGIRL No-purchase D2', 'thirstygirl']].forEach(([a, n, slug]) => {
    const h = splitListName(n, a);
    assert.equal(`${a} ${h}`, n);
    assert.equal(joinListName(a, h), n);
    assert.ok(!nameWarnings('list', joinListName(a, h), { brands: WX.brands, brandSlug: slug, listType: 'private' }).includes('listPrefix'));
  });
  assert.equal(joinListName('RUZE', '  Promo '), 'RUZE Promo');
  assert.ok(!nameWarnings('list', joinListName('RUZE', '  Promo '), { brands: WX.brands, brandSlug: 'ruze', listType: 'private' }).includes('listPrefix'));
  // An empty handle joins to the bare prefix: no listPrefix, only handleMissing (shown after blur).
  assert.deepEqual(nameWarnings('list', joinListName('RUZE', ''), { brands: WX.brands, brandSlug: 'ruze', listType: 'private' }), ['handleMissing']);
  // No addon: the handle is the name.
  assert.equal(joinListName('', 'RobbieList'), 'RobbieList');
  assert.equal(splitListName('RobbieList', ''), 'RobbieList');
  assert.equal(splitListName('Ruze Sale', 'RUZE'), 'Ruze Sale');
});

// (Where the toggle shows and applies -- GLOBAL-LANG-SPEC, superseding the route list -- is
// langScope.test.mjs.)
test('langScope: values, the pref, its label and the Campaigns param', () => {
  assert.deepEqual([...LANG_VALUES], ['all', 'en', 'es', 'fr', 'de', 'it']);
  assert.equal(LANG_PREF, 'brand.lang');
  assert.equal(langLabel('all', 'All'), 'All');
  assert.equal(langLabel('fr', 'All'), 'FR');
  assert.equal(langParam('all'), '');
  assert.equal(langParam('de'), 'de');
  assert.equal(normalizeLang('pt'), 'all');
  assert.equal(normalizeLang(' IT '), 'it');
});
