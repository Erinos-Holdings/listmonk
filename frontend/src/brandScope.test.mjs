// Fork (global brand, integrations GLOBAL-BRAND-SPEC I2-I6, I12, I13). Run: yarn test:brand-scope
// (node --test src/brandScope.test.mjs). The selector's pure half: roster, resolution, the
// effective list set, the S4 notice, the picker unions and every Subscribers by-query body.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  ALL, NONE,
  storeLists, rosterFrom, resolveSelection, scopeListIds, listsScopeParams,
  scopeUnion, filterByIds, effectiveListIds, subscriberQueryBody, subscriberExportParams,
  campaignBrand, contextNotice, filterTemplates, formBrandDefault, templateNoticeBrand,
} from './brandScope.mjs'; // eslint-disable-line import/extensions

const BRANDS = [
  { slug: 'younique', display_name: 'Younique' },
  { slug: 'ruze', display_name: 'Ruze' },
  { slug: 'shala', display_name: 'Shala' },
  { slug: 'thirstygirl', display_name: 'Thirsty Girl' },
];

const LISTS = {
  results: [
    { id: 5, brand: 'younique' },
    { id: 23, brand: 'younique' },
    { id: 9, brand: 'ruze' },
    { id: 16, brand: 'shala' },
    { id: 37, brand: '' },
    { id: 40, brand: '' },
  ],
};

const brand = (slug) => ({ kind: 'brand', slug });

test('storeLists accepts the store before and after a response', () => {
  assert.deepEqual(storeLists([]), []);
  assert.deepEqual(storeLists(undefined), []);
  assert.equal(storeLists(LISTS).length, 6);
  assert.deepEqual(storeLists([{ id: 1 }]), [{ id: 1 }]);
});

test('I2 rosterFrom: distinct brands of the permitted lists, by display name, plus none', () => {
  const r = rosterFrom(LISTS, BRANDS);
  assert.deepEqual(r.brands, [
    { slug: 'ruze', label: 'Ruze' },
    { slug: 'shala', label: 'Shala' },
    { slug: 'younique', label: 'Younique' },
  ]);
  assert.equal(r.none, true);
  // A brand with no permitted list is never offered (thirstygirl has a brands row, no list here).
  assert.ok(!r.brands.some((b) => b.slug === 'thirstygirl'));
});

test('I2 rosterFrom: a failed brands read leaves the roster slug-labelled, never empty', () => {
  const r = rosterFrom(LISTS, undefined);
  assert.deepEqual(r.brands.map((b) => b.label), ['ruze', 'shala', 'younique']);
  assert.equal(r.none, true);
});

test('I2 rosterFrom: a missing brands row labels by slug; no brandless list means none false', () => {
  const r = rosterFrom({ results: [{ id: 1, brand: 'zeta' }, { id: 2, brand: 'shala' }] }, BRANDS);
  assert.deepEqual(r.brands, [{ slug: 'shala', label: 'Shala' }, { slug: 'zeta', label: 'zeta' }]);
  assert.equal(r.none, false);
  assert.deepEqual(rosterFrom([], BRANDS), { brands: [], none: false });
});

test('I3 resolveSelection: a stored value in the roster is kept', () => {
  const roster = rosterFrom(LISTS, BRANDS);
  assert.deepEqual(resolveSelection(brand('ruze'), roster).selection, brand('ruze'));
  assert.deepEqual(resolveSelection(NONE, roster).selection, NONE);
  assert.deepEqual(resolveSelection(ALL, roster).selection, ALL);
  const r = resolveSelection(brand('ruze'), roster);
  assert.equal(r.pending, false);
  assert.equal(r.locked, false);
  assert.equal(r.hidden, false);
});

test('I3 resolveSelection: a stored value outside the roster resolves to all', () => {
  const roster = rosterFrom({ results: [{ id: 5, brand: 'younique' }, { id: 16, brand: 'shala' }] }, BRANDS);
  assert.deepEqual(resolveSelection(brand('ruze'), roster).selection, ALL);
  // none with no brandless list in the roster
  assert.deepEqual(resolveSelection(NONE, roster).selection, ALL);
  // garbage from localStorage
  [null, undefined, 'ruze', 42, {}, { kind: 'brand' }, { kind: 'brand', slug: '' }, { kind: 'x' }].forEach((s) => {
    assert.deepEqual(resolveSelection(s, roster).selection, ALL, JSON.stringify(s));
  });
});

test('I3 resolveSelection: a one-brand roster with no brandless list locks to that brand', () => {
  const roster = rosterFrom({ results: [{ id: 9, brand: 'ruze' }, { id: 10, brand: 'ruze' }] }, BRANDS);
  [ALL, NONE, brand('younique'), brand('ruze'), null].forEach((s) => {
    const r = resolveSelection(s, roster);
    assert.deepEqual(r.selection, brand('ruze'), JSON.stringify(s));
    assert.equal(r.locked, true);
    assert.equal(r.hidden, false);
  });
  // One brand plus a brandless list is NOT locked (S5: "and no brandless list").
  const r2 = resolveSelection(ALL, rosterFrom({ results: [{ id: 9, brand: 'ruze' }, { id: 37, brand: '' }] }, BRANDS));
  assert.equal(r2.locked, false);
  assert.deepEqual(r2.selection, ALL);
});

test('I3 resolveSelection: an empty roster resolves to all with the control hidden', () => {
  const r = resolveSelection(brand('ruze'), rosterFrom([], BRANDS));
  assert.deepEqual(r.selection, ALL);
  assert.equal(r.hidden, true);
  assert.equal(r.locked, false);
});

test('I13 resolveSelection: before the lists load the state is pending, never all', () => {
  const r = resolveSelection(brand('ruze'), rosterFrom([], BRANDS), false);
  assert.deepEqual(r, { pending: true });
  assert.equal(r.selection, undefined);
});

test('I4 scopeListIds: null for all; the brand\'s lists; the brandless lists for none', () => {
  assert.equal(scopeListIds(LISTS, ALL), null);
  assert.equal(scopeListIds(LISTS, undefined), null);
  assert.deepEqual(scopeListIds(LISTS, brand('younique')), [5, 23]);
  assert.deepEqual(scopeListIds(LISTS, NONE), [37, 40]);
});

test('I4 scopeListIds: never an empty array for a RESOLVED brand/none selection', () => {
  const roster = rosterFrom(LISTS, BRANDS);
  [brand('younique'), brand('ruze'), brand('shala'), NONE, brand('gone'), ALL].forEach((stored) => {
    const { selection } = resolveSelection(stored, roster);
    const ids = scopeListIds(LISTS, selection);
    assert.ok(ids === null || ids.length > 0, JSON.stringify(stored));
  });
});

test('listsScopeParams: tag=brand:<slug>, nobrand=true, nothing under all', () => {
  assert.deepEqual(listsScopeParams(ALL), {});
  assert.deepEqual(listsScopeParams(brand('ruze')), { tag: ['brand:ruze'] });
  assert.deepEqual(listsScopeParams(NONE), { nobrand: true });
});

test('I6 scopeUnion: scope united with every current-list array; null under all', () => {
  assert.equal(scopeUnion(null, [[{ id: 9 }]]), null);
  assert.deepEqual(scopeUnion([5, 23], []), [5, 23]);
  // campaign form: scope ∪ the campaign's current lists
  assert.deepEqual(scopeUnion([5, 23], [[{ id: 9, name: 'Ruze' }, { id: 23 }]]), [5, 23, 9]);
  // Manage-lists bulk by ids: scope ∪ the union of the checked rows' lists
  assert.deepEqual(scopeUnion([5, 23], [[{ id: 9 }], [{ id: 16 }, { id: 9 }], []]), [5, 23, 9, 16]);
  // ids as numbers too; junk ignored
  assert.deepEqual(scopeUnion([5], [[7, 0, null, 'x', { id: 8 }]]), [5, 7, 8]);
});

test('filterByIds narrows store rows; null ids is every row', () => {
  assert.equal(filterByIds(LISTS, null).length, 6);
  assert.deepEqual(filterByIds(LISTS, [9, 37]).map((l) => l.id), [9, 37]);
});

test('I12 effectiveListIds: the chosen list, else the scope, else null', () => {
  assert.deepEqual(effectiveListIds(9, [5, 23]), [9]);
  assert.deepEqual(effectiveListIds(null, [5, 23]), [5, 23]);
  assert.equal(effectiveListIds(null, null), null);
});

const QP = {
  search: 'amy', queryExp: '', listID: null, subStatus: 'confirmed', segment: 'active', lang: 'fr', page: 3,
};

test('I12 subscriberQueryBody: a chosen list wins', () => {
  assert.deepEqual(subscriberQueryBody({ ...QP, listID: 16 }, [5, 23]), {
    search: 'amy', query: '', list_ids: [16], subscription_status: 'confirmed', segment: 'active', lang: 'fr',
  });
});

test('I12 subscriberQueryBody: no list chosen carries the scope', () => {
  assert.deepEqual(subscriberQueryBody(QP, [5, 23]).list_ids, [5, 23]);
});

test('I12 subscriberQueryBody: no list and no scope (all) is null, as today', () => {
  const b = subscriberQueryBody({ ...QP, segment: null, lang: null }, null);
  assert.deepEqual(b, {
    search: 'amy', query: '', list_ids: null, subscription_status: 'confirmed',
  });
});

test('I12 subscriberExportParams: the three cases, plus the checked ids', () => {
  assert.deepEqual(subscriberExportParams({ ...QP, listID: 16 }, [5, 23]), [
    ['search', 'amy'], ['list_id', '16'], ['subscription_status', 'confirmed'], ['segment', 'active'], ['lang', 'fr'],
  ]);
  assert.deepEqual(subscriberExportParams(QP, [5, 23], [101, 102]), [
    ['search', 'amy'], ['list_id', '5'], ['list_id', '23'], ['subscription_status', 'confirmed'],
    ['segment', 'active'], ['lang', 'fr'], ['id', '101'], ['id', '102'],
  ]);
  assert.deepEqual(subscriberExportParams({
    search: '', queryExp: 'x = 1', listID: null, subStatus: null,
  }, null), [['query', 'x = 1']]);
});

test('campaignBrand: the brand of its lists in the store; undefined when none is there', () => {
  assert.equal(campaignBrand([{ id: 23, name: 'Younique' }], LISTS), 'younique');
  assert.equal(campaignBrand([{ id: 37 }], LISTS), '');
  assert.equal(campaignBrand([{ id: 999 }], LISTS), undefined);
  assert.equal(campaignBrand([], LISTS), undefined);
  assert.equal(campaignBrand([{ id: 0, name: 'deleted' }], LISTS), undefined);
});

test('I5 contextNotice: null under all and for a matching record', () => {
  assert.equal(contextNotice('thirstygirl', ALL, BRANDS), null);
  assert.equal(contextNotice('ruze', brand('ruze'), BRANDS), null);
  assert.equal(contextNotice('', NONE, BRANDS), null);
});

test('I5 contextNotice: null when the record brand cannot be derived', () => {
  assert.equal(contextNotice(undefined, brand('ruze'), BRANDS), null);
  assert.equal(contextNotice(null, NONE, BRANDS), null);
});

test('I5 contextNotice: a record outside the scope gets display-name labels', () => {
  assert.deepEqual(contextNotice('thirstygirl', brand('ruze'), BRANDS), { record: 'Thirsty Girl', selected: 'Ruze' });
  assert.deepEqual(contextNotice('', brand('ruze'), BRANDS, { noneLabel: 'No brand' }), { record: 'No brand', selected: 'Ruze' });
  assert.deepEqual(contextNotice('ruze', NONE, BRANDS, { noneLabel: 'No brand' }), { record: 'Ruze', selected: 'No brand' });
  assert.deepEqual(contextNotice('zeta', brand('ruze'), BRANDS), { record: 'zeta', selected: 'Ruze' });
});

test('D12 templateNoticeBrand: a brandless template is every brand\'s -- no notice under a named brand', () => {
  const notice = (tb, sel) => contextNotice(templateNoticeBrand(tb, sel), sel, BRANDS, { noneLabel: 'No brand' });
  // Brandless template under a named brand: in scope, no notice.
  assert.equal(templateNoticeBrand('', brand('ruze')), undefined);
  assert.equal(notice('', brand('ruze')), null);
  assert.equal(notice(undefined, brand('ruze')), null);
  // A template of a DIFFERENT named brand gets the notice.
  assert.deepEqual(notice('shala', brand('ruze')), { record: 'Shala', selected: 'Ruze' });
  // Same brand: none.
  assert.equal(notice('ruze', brand('ruze')), null);
  // Under none a branded template still gets its notice; a brandless one does not.
  assert.deepEqual(notice('ruze', NONE), { record: 'Ruze', selected: 'No brand' });
  assert.equal(notice('', NONE), null);
  // Under all: never.
  assert.equal(notice('shala', ALL), null);
});

test('filterTemplates: a brand shows its rows plus the brandless rows; none the brandless only', () => {
  const T = [{ id: 1, brand: '' }, { id: 2, brand: 'ruze' }, { id: 3, brand: 'shala' }, { id: 4 }];
  assert.deepEqual(filterTemplates(T, ALL).map((t) => t.id), [1, 2, 3, 4]);
  assert.deepEqual(filterTemplates(T, brand('ruze')).map((t) => t.id), [1, 2, 4]);
  assert.deepEqual(filterTemplates(T, NONE).map((t) => t.id), [1, 4]);
});

test('formBrandDefault: brand presets and locks; none locks the template form only', () => {
  assert.deepEqual(formBrandDefault(ALL), { value: null, locked: false });
  assert.deepEqual(formBrandDefault(brand('ruze'), 'list'), { value: 'ruze', locked: true });
  assert.deepEqual(formBrandDefault(brand('ruze'), 'template'), { value: 'ruze', locked: true });
  assert.deepEqual(formBrandDefault(NONE, 'list'), { value: null, locked: false });
  assert.deepEqual(formBrandDefault(NONE, 'template'), { value: '', locked: true });
});
