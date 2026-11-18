// Fork (global brand, integrations GLOBAL-BRAND-SPEC I2-I6, I12, I13). Run: yarn test:brand-scope
// (node --test src/brandScope.test.mjs). The selector's pure half: roster, resolution, the
// effective list set, the S4 notice, the picker unions and every Subscribers by-query body.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  ALL, NONE,
  storeLists, rosterFrom, resolveSelection, scopeListIds, listsScopeParams,
  scopeUnion, filterByIds, effectiveListIds, subscriberQueryBody, subscriberExportParams, gridFilterOf,
  campaignBrand, contextNotice, filterTemplates, formBrandDefault, templateNoticeBrand,
  mediaScopeContext, presetNoticeBrand, filterBrandRows,
  launchOf, matchesLaunch, applyLaunch, launchCounts, filterRowsByLaunch, launchApplies,
  selectionLoaded, brandKeyStep, canCreateCampaign,
  ESTABLISHED_BRANDS, BRAND_GROUPS, groupOf, groupMembers, groupsAvailable, offeredGroups, groupWaits,
  selectorOptions,
} from './brandScope.mjs'; // eslint-disable-line import/extensions

// Fork (integrations EDITOR-DEFAULTS-SPEC I1) -- New is enabled iff the selection is one brand; the
// navbar item reads the same predicate.
test('canCreateCampaign: brand only', () => {
  assert.equal(canCreateCampaign(ALL), false);
  assert.equal(canCreateCampaign(NONE), false);
  assert.equal(canCreateCampaign({ kind: 'brand', slug: 'ruze' }), true);
  // Unloaded: the store's brandSelection is ALL while pending; a raw pending scope or nothing at all is false too.
  assert.equal(canCreateCampaign({ pending: true }), false);
  assert.equal(canCreateCampaign(undefined), false);
  assert.equal(canCreateCampaign(null), false);
  assert.equal(canCreateCampaign({ kind: 'brand', slug: '' }), false);
  assert.equal(canCreateCampaign(resolveSelection({ kind: 'brand', slug: 'ruze' }, { brands: [{ slug: 'ruze' }], none: true }, false)), false);
  assert.equal(canCreateCampaign(resolveSelection({ kind: 'brand', slug: 'ruze' }, { brands: [{ slug: 'ruze' }, { slug: 'shala' }], none: true }).selection), true);
});

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

// integrations GLOBAL-LANG-SPEC I4b -- the global language reaches the Subscribers page as the
// route's lang ALONE (no segment): the read query (gridFilterOf), the by-query body and the export
// carry exactly it.
test('GLOBAL-LANG I4b: a lang-only filter rides the read query, the by-query body and the export', () => {
  const qp = { ...QP, segment: null, lang: 'es' };
  assert.deepEqual(gridFilterOf(qp), { lang: 'es' });
  assert.deepEqual(gridFilterOf({ ...qp, lang: null }), {});
  assert.deepEqual(gridFilterOf({ ...QP }), { segment: 'active', lang: 'fr' });
  assert.equal(subscriberQueryBody(qp, null).lang, 'es');
  assert.equal('segment' in subscriberQueryBody(qp, null), false);
  assert.deepEqual(subscriberExportParams(qp, null, []).filter(([k]) => k === 'lang' || k === 'segment'), [['lang', 'es']]);
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

// Phase 2 (integrations GLOBAL-BRAND-PHASE2-SPEC I2, I6, I11).

test('Phase 2 I2 mediaScopeContext: brand = slug chip; none = Untagged; all/pending = All', () => {
  assert.deepEqual(mediaScopeContext(brand('ruze')), { context: ['ruze'], untagged: false });
  assert.deepEqual(mediaScopeContext(NONE), { context: [], untagged: true });
  assert.deepEqual(mediaScopeContext(ALL), { context: [], untagged: false });
  // Pending resolves to all in the store getter; an absent or malformed selection is all too.
  assert.deepEqual(mediaScopeContext(undefined), { context: [], untagged: false });
  assert.deepEqual(mediaScopeContext({ kind: 'brand', slug: '' }), { context: [], untagged: false });
});

test('Phase 2 I11 presetNoticeBrand: preview brand, else store brand of an existing list, else ""', () => {
  // A list to create (or an existing untagged list confirm will tag) carries its brand.
  assert.equal(presetNoticeBrand({ name: 'L', exists: false, brand: 'curated' }, LISTS), 'curated');
  assert.equal(presetNoticeBrand({
    name: 'L', id: 40, exists: true, will_tag: true, brand: 'curated',
  }, LISTS), 'curated');
  // An existing same-brand list is reused with no brand: the store row's brand.
  assert.equal(presetNoticeBrand({ name: 'L', id: 9, exists: true }, LISTS), 'ruze');
  assert.equal(presetNoticeBrand({ name: 'L', id: 37, exists: true }, LISTS), '');
  // An existing list the store does not hold (not visible, archived): underivable.
  assert.equal(presetNoticeBrand({ name: 'L', id: 999, exists: true }, LISTS), undefined);
  assert.equal(presetNoticeBrand({ name: 'L', id: 9, exists: true }, []), undefined);
  // A list to create with no list_brand: a brandless import list.
  assert.equal(presetNoticeBrand({ name: 'L', exists: false, tags: [] }, LISTS), '');
  // No preview: no notice.
  assert.equal(presetNoticeBrand(null, LISTS), undefined);

  // End to end with contextNotice: Loyalty Rewards (curated) under Ruze; nothing under Curated;
  // a brandless created list under a brand and under none.
  const B = [...BRANDS, { slug: 'curated', display_name: 'Curated' }];
  const n = (pl, sel) => contextNotice(presetNoticeBrand(pl, LISTS), sel, B, { noneLabel: 'No brand' });
  assert.deepEqual(n({ exists: false, brand: 'curated' }, brand('ruze')), { record: 'Curated', selected: 'Ruze' });
  assert.equal(n({ exists: false, brand: 'curated' }, brand('curated')), null);
  assert.deepEqual(n({ exists: false }, brand('ruze')), { record: 'No brand', selected: 'Ruze' });
  assert.equal(n({ exists: false }, NONE), null);
  assert.equal(n({ id: 999, exists: true }, brand('ruze')), null);
});

test('Phase 2 I6 filterBrandRows: a brand shows its row only; none and all every row', () => {
  const R = [{ brand: 'ruze' }, { brand: 'shala' }, { brand: 'younique' }];
  assert.deepEqual(filterBrandRows(R, brand('shala')), [{ brand: 'shala' }]);
  assert.deepEqual(filterBrandRows(R, brand('nope')), []);
  assert.equal(filterBrandRows(R, NONE).length, 3);
  assert.equal(filterBrandRows(R, ALL).length, 3);
  assert.deepEqual(filterBrandRows(undefined, brand('ruze')), []);
  // The record page's notice comes from contextNotice with the route's slug.
  assert.deepEqual(contextNotice('thirstygirl', brand('ruze'), BRANDS), { record: 'Thirsty Girl', selected: 'Ruze' });
  assert.deepEqual(contextNotice('ruze', NONE, BRANDS), { record: 'Ruze', selected: 'No brand' });
});

// Fork (launch filter, integrations LAUNCH-FILTER-SPEC I1-I5) ----------------------------------
// The health documents: shala unlaunched; ruze, younique launched; no document for thirstygirl.
const DOCS = [
  { brand: 'ruze', status: 'ok' },
  { brand: 'shala', status: 'unknown' },
  { brand: 'younique', status: 'warn' },
];

test('D3 selectionLoaded: under a narrowing filter the selection waits for the health documents', () => {
  // The store's brandScope composition, before and after the documents arrive (2026-10-28: a
  // stored unlaunched brand under Unlaunched resolved to All brands for one unscoped fetch).
  const roster = rosterFrom(LISTS, BRANDS);
  const scope = (launch, docs, healthLoaded) => resolveSelection(
    brand('shala'),
    roster,
    selectionLoaded(true, launch, healthLoaded),
    applyLaunch(roster, launch, docs),
  );
  // The defect, ungated: no documents yet, every brand reads as launched, shala is not offered.
  assert.deepEqual(resolveSelection(brand('shala'), roster, true, applyLaunch(roster, 'unlaunched', [])).selection, ALL);
  assert.deepEqual(scope('unlaunched', [], false), { pending: true });
  assert.deepEqual(scope('unlaunched', DOCS, true).selection, brand('shala'));
  assert.deepEqual(scope('launched', [], false), { pending: true });
  // A failed read releases the gate (no documents: every brand launched, as before).
  assert.deepEqual(scope('unlaunched', [], true).selection, ALL);
  // Under All nothing waits on the documents.
  assert.deepEqual(scope('all', [], false).selection, brand('shala'));
  assert.equal(selectionLoaded(false, 'all', true), false);
  assert.equal(selectionLoaded(true, 'bogus', false), true);
});

test('brandKeyStep: a page refetches exactly when the resolved key differs from the one it fetched', () => {
  const K1 = JSON.stringify(brand('ruze'));
  const K2 = JSON.stringify(ALL);
  // Mounted while pending: pending is ignored, the first resolution only records.
  assert.deepEqual(brandKeyStep(null, 'pending'), { seen: null, changed: false });
  assert.deepEqual(brandKeyStep(null, K1), { seen: K1, changed: false });
  // An ordinary selector change.
  assert.deepEqual(brandKeyStep(K1, K2), { seen: K2, changed: true });
  // The defect: a launch-filter switch while the documents load goes K1 -> pending -> K2. The old
  // guard skipped both transitions; the step keeps K1 through pending and fetches on K2.
  let s = brandKeyStep(K1, 'pending');
  assert.deepEqual(s, { seen: K1, changed: false });
  s = brandKeyStep(s.seen, K2);
  assert.deepEqual(s, { seen: K2, changed: true });
  // A pending round trip back to the same key does not refetch.
  s = brandKeyStep(brandKeyStep(K1, 'pending').seen, K1);
  assert.deepEqual(s, { seen: K1, changed: false });
});

test('I1 launchOf: unknown is unlaunched; ok/warn/issues, no status and no document are launched', () => {
  assert.equal(launchOf('shala', DOCS), 'unlaunched');
  assert.equal(launchOf('ruze', DOCS), 'launched');
  assert.equal(launchOf('younique', DOCS), 'launched');
  assert.equal(launchOf('x', [{ brand: 'x', status: 'issues' }]), 'launched');
  assert.equal(launchOf('x', [{ brand: 'x' }]), 'launched');
  assert.equal(launchOf('x', [{ brand: 'x', status: 2 }]), 'launched');
  // No document at all.
  assert.equal(launchOf('thirstygirl', DOCS), 'launched');
  assert.equal(launchOf('ruze', []), 'launched');
  assert.equal(launchOf('ruze', undefined), 'launched');
  // matchesLaunch: all (and an unknown value) matches everything.
  assert.equal(matchesLaunch('shala', 'all', DOCS), true);
  assert.equal(matchesLaunch('shala', 'bogus', DOCS), true);
  assert.equal(matchesLaunch('shala', 'launched', DOCS), false);
  assert.equal(matchesLaunch('shala', 'unlaunched', DOCS), true);
  assert.equal(matchesLaunch('thirstygirl', 'unlaunched', DOCS), false);
});

test('I2 applyLaunch: all unchanged; launched/unlaunched the matching brands in order; none kept', () => {
  const roster = rosterFrom(LISTS, BRANDS); // ruze, shala, younique + none
  const slugs = (r) => r.brands.map((b) => b.slug);
  assert.deepEqual(applyLaunch(roster, 'all', DOCS), roster);
  assert.deepEqual(slugs(applyLaunch(roster, 'launched', DOCS)), ['ruze', 'younique']);
  assert.deepEqual(slugs(applyLaunch(roster, 'unlaunched', DOCS)), ['shala']);
  ['all', 'launched', 'unlaunched', 'bogus', undefined].forEach((l) => {
    assert.equal(applyLaunch(roster, l, DOCS).none, true, String(l));
  });
  const noNone = rosterFrom({ results: [{ id: 9, brand: 'ruze' }, { id: 16, brand: 'shala' }] }, BRANDS);
  assert.equal(applyLaunch(noNone, 'unlaunched', DOCS).none, false);
  // An unknown value behaves as all.
  assert.deepEqual(applyLaunch(roster, 'bogus', DOCS), roster);
  assert.deepEqual(applyLaunch(roster, null, DOCS), roster);
  // No documents: every brand launched.
  assert.deepEqual(slugs(applyLaunch(roster, 'launched', [])), ['ruze', 'shala', 'younique']);
  assert.deepEqual(slugs(applyLaunch(roster, 'unlaunched', [])), []);
  // The input roster is not mutated.
  assert.deepEqual(slugs(roster), ['ruze', 'shala', 'younique']);
});

test('I3 resolveSelection offered: hidden/locked by the full roster, a stored brand by the offered', () => {
  const roster = rosterFrom(LISTS, BRANDS);
  const launched = applyLaunch(roster, 'launched', DOCS);
  // shala is in the roster but not offered: all, not locked, not hidden.
  const r = resolveSelection(brand('shala'), roster, true, launched);
  assert.deepEqual(r, {
    pending: false, selection: ALL, locked: false, hidden: false,
  });
  // A brand still offered is kept; none is kept (applyLaunch never drops it).
  assert.deepEqual(resolveSelection(brand('ruze'), roster, true, launched).selection, brand('ruze'));
  assert.deepEqual(resolveSelection(NONE, roster, true, launched).selection, NONE);
  // A two-brand roster narrowed to one offered brand is NOT locked.
  const two = rosterFrom({ results: [{ id: 9, brand: 'ruze' }, { id: 16, brand: 'shala' }] }, BRANDS);
  const twoOffered = applyLaunch(two, 'unlaunched', DOCS);
  assert.equal(twoOffered.brands.length, 1);
  const r2 = resolveSelection(brand('ruze'), two, true, twoOffered);
  assert.equal(r2.locked, false);
  assert.equal(r2.hidden, false);
  assert.deepEqual(r2.selection, ALL);
  // A one-brand roster is still locked whatever offered holds.
  const one = rosterFrom({ results: [{ id: 16, brand: 'shala' }] }, BRANDS);
  [applyLaunch(one, 'launched', DOCS), { brands: [], none: false }, undefined].forEach((o) => {
    const r3 = resolveSelection(ALL, one, true, o);
    assert.equal(r3.locked, true);
    assert.deepEqual(r3.selection, brand('shala'));
  });
  // Pending stays pending.
  assert.deepEqual(resolveSelection(brand('ruze'), roster, false, launched), { pending: true });
  // offered omitted = today's behaviour.
  assert.deepEqual(resolveSelection(brand('shala'), roster, true), resolveSelection(brand('shala'), roster, true, roster));
  assert.deepEqual(resolveSelection(brand('shala'), roster).selection, brand('shala'));
});

test('D4 launchApplies: only while the selector is shown and unlocked (full roster)', () => {
  assert.equal(launchApplies(rosterFrom(LISTS, BRANDS)), true);
  assert.equal(launchApplies(rosterFrom({ results: [{ id: 9, brand: 'ruze' }, { id: 16, brand: 'shala' }] }, BRANDS)), true);
  // One brand + a brandless list is not locked.
  assert.equal(launchApplies(rosterFrom({ results: [{ id: 9, brand: 'ruze' }, { id: 37, brand: '' }] }, BRANDS)), true);
  // Locked (one brand) and hidden (empty roster) do not apply.
  assert.equal(launchApplies(rosterFrom({ results: [{ id: 9, brand: 'ruze' }] }, BRANDS)), false);
  assert.equal(launchApplies(rosterFrom([], BRANDS)), false);
  assert.equal(launchApplies(undefined), false);
});

test('I4 launchCounts: unknown documents unlaunched, every other brand launched; sums to the roster', () => {
  const roster = rosterFrom(LISTS, BRANDS);
  assert.deepEqual(launchCounts(roster, DOCS), { launched: 2, unlaunched: 1 });
  assert.deepEqual(launchCounts(roster, []), { launched: 3, unlaunched: 0 });
  const withTg = rosterFrom({ results: [...LISTS.results, { id: 50, brand: 'thirstygirl' }] }, BRANDS);
  const c = launchCounts(withTg, DOCS);
  assert.deepEqual(c, { launched: 3, unlaunched: 1 });
  assert.equal(c.launched + c.unlaunched, withTg.brands.length);
  assert.deepEqual(launchCounts(undefined, DOCS), { launched: 0, unlaunched: 0 });
});

test('I5 filterRowsByLaunch: Brands rows by their own status; SES rows by their brand\'s document', () => {
  const bySlug = (r) => r.brand;
  const BR = [
    { brand: 'younique', status: 'warn' },
    { brand: 'shala', status: 'unknown' },
    { brand: 'ruze', status: 'ok' },
    { brand: 'zeta', status: 'unknown' },
  ];
  const b = (l) => filterRowsByLaunch(BR, l, bySlug, BR).map(bySlug);
  assert.deepEqual(b('all'), ['younique', 'shala', 'ruze', 'zeta']);
  assert.deepEqual(b('launched'), ['younique', 'ruze']);
  assert.deepEqual(b('unlaunched'), ['shala', 'zeta']);

  const SES = [
    { brand: 'unattributed', status: 'ok', sends: 4 },
    { brand: 'shala', status: 'ok', sends: 120 },
    { brand: 'ruze', status: 'unknown', sends: 0 },
    { brand: 'thirstygirl', status: 'unknown', sends: 0 },
  ];
  const s = (l) => filterRowsByLaunch(SES, l, bySlug, DOCS).map(bySlug);
  assert.deepEqual(s('all'), ['unattributed', 'shala', 'ruze', 'thirstygirl']);
  // shala's DOCUMENT is unknown: dropped under launched though the SES row shows sends.
  assert.deepEqual(s('launched'), ['unattributed', 'ruze', 'thirstygirl']);
  // No document (unattributed, thirstygirl): kept under all/launched, dropped under unlaunched.
  assert.deepEqual(s('unlaunched'), ['shala']);
  assert.deepEqual(filterRowsByLaunch(undefined, 'launched', bySlug, DOCS), []);
});

// Fork (brand groups, integrations BRAND-GROUPS-SPEC I1-I10) -----------------------------------
// The roster: the parent (curated), two established brands (joybelle, younique; joybelle
// UNLAUNCHED), shala established and launched here, two launched others (ruze, bnkd), one
// unlaunched other (zeta), and brandless lists.
const G_BRANDS = [
  { slug: 'curated', display_name: 'Curated' },
  { slug: 'joybelle', display_name: 'JoyBelle' },
  { slug: 'shala', display_name: 'Shala' },
  { slug: 'younique', display_name: 'Younique' },
  { slug: 'ruze', display_name: 'Ruze Pouches' },
  { slug: 'bnkd', display_name: 'BNKD Wellness' },
  { slug: 'zeta', display_name: 'Zeta' },
];
const G_LISTS = [
  { id: 1, brand: 'curated' },
  { id: 2, brand: 'joybelle' },
  { id: 3, brand: 'shala' },
  { id: 4, brand: 'younique' },
  { id: 5, brand: 'younique' },
  { id: 6, brand: 'ruze' },
  { id: 7, brand: 'bnkd' },
  { id: 8, brand: 'zeta' },
  { id: 9, brand: '' },
];
const G_DOCS = [
  { brand: 'curated', status: 'ok' },
  { brand: 'joybelle', status: 'unknown' },
  { brand: 'shala', status: 'ok' },
  { brand: 'ruze', status: 'warn' },
  { brand: 'zeta', status: 'unknown' },
  // younique, bnkd: no document -- launched.
];
const G_ROSTER = rosterFrom(G_LISTS, G_BRANDS);
const group = (g, slugs) => (slugs ? { kind: 'group', group: g, slugs } : { kind: 'group', group: g });
const INC = group('incubating', ['bnkd', 'ruze']);
const EST = group('established', ['joybelle', 'shala', 'younique']);

test('BRAND-GROUPS I11: the established list is the one constant, frozen', () => {
  assert.deepEqual([...ESTABLISHED_BRANDS], ['joybelle', 'shala', 'younique']);
  assert.ok(Object.isFrozen(ESTABLISHED_BRANDS));
  assert.deepEqual([...BRAND_GROUPS], ['incubating', 'established']);
});

test('BRAND-GROUPS I1 groupOf: parent none; established whatever its launch; launched other incubating; unlaunched other none', () => {
  const rows = [
    ['curated', null],
    ['joybelle', 'established'], // unlaunched
    ['shala', 'established'],
    ['younique', 'established'], // no document
    ['ruze', 'incubating'],
    ['bnkd', 'incubating'], // no document = launched
    ['zeta', null], // unlaunched
  ];
  rows.forEach(([slug, want]) => assert.equal(groupOf(slug, G_DOCS), want, slug));
  // No documents at all (a failed first read): every non-established, non-parent brand is incubating.
  assert.equal(groupOf('zeta', []), 'incubating');
  assert.equal(groupOf('curated', []), null);
});

test('BRAND-GROUPS I2 groupMembers: the full roster\'s brands of the group, sorted; never a brand outside the roster', () => {
  assert.deepEqual(groupMembers('incubating', G_ROSTER, G_DOCS), ['bnkd', 'ruze']);
  assert.deepEqual(groupMembers('established', G_ROSTER, G_DOCS), ['joybelle', 'shala', 'younique']);
  // A narrowed roster (a user permitted only ruze and shala lists): younique/joybelle/bnkd are absent.
  const narrow = rosterFrom([{ id: 6, brand: 'ruze' }, { id: 3, brand: 'shala' }], G_BRANDS);
  assert.deepEqual(groupMembers('incubating', narrow, G_DOCS), ['ruze']);
  assert.deepEqual(groupMembers('established', narrow, G_DOCS), ['shala']);
  assert.deepEqual(groupMembers('bogus', G_ROSTER, G_DOCS), []);
  assert.deepEqual(groupMembers('incubating', undefined, G_DOCS), []);
});

test('BRAND-GROUPS I2 offeredGroups: whole or absent under each launch value; the value never changes members', () => {
  const off = (launch, docs = G_DOCS) => offeredGroups(G_ROSTER, applyLaunch(G_ROSTER, launch, docs), docs, true);
  // All: both groups, whole -- Established includes the unlaunched joybelle.
  assert.deepEqual(off('all'), [
    { group: 'incubating', slugs: ['bnkd', 'ruze'] },
    { group: 'established', slugs: ['joybelle', 'shala', 'younique'] },
  ]);
  // Launched: joybelle (an unlaunched established brand) is filtered out, so Established is NOT
  // offered -- never offered without it. Incubating is whole.
  assert.deepEqual(off('launched'), [{ group: 'incubating', slugs: ['bnkd', 'ruze'] }]);
  // Unlaunched: neither (Incubating's members are launched by definition; Established is partial).
  assert.deepEqual(off('unlaunched'), []);
  // Every established brand launched (today's state): both under Launched, neither under Unlaunched.
  const docsAllLaunched = G_DOCS.map((d) => (d.brand === 'joybelle' ? { ...d, status: 'ok' } : d));
  assert.deepEqual(off('launched', docsAllLaunched).map((g) => g.group), ['incubating', 'established']);
  assert.deepEqual(off('unlaunched', docsAllLaunched), []);
  // Unavailable: none at all.
  assert.deepEqual(offeredGroups(G_ROSTER, G_ROSTER, G_DOCS, false), []);
  // An empty group is not offered (a roster with no established brand).
  const noEst = rosterFrom([{ id: 6, brand: 'ruze' }, { id: 7, brand: 'bnkd' }], G_BRANDS);
  assert.deepEqual(offeredGroups(noEst, noEst, G_DOCS, true).map((g) => g.group), ['incubating']);
});

test('BRAND-GROUPS D2 groupsAvailable: brands:get, a shown and unlocked selector, the documents loaded', () => {
  assert.equal(groupsAvailable(true, G_ROSTER, true), true);
  assert.equal(groupsAvailable(false, G_ROSTER, true), false);
  assert.equal(groupsAvailable(true, G_ROSTER, false), false);
  // Locked (one brand, no brandless list) and hidden (empty roster).
  assert.equal(groupsAvailable(true, { brands: [{ slug: 'ruze' }], none: false }, true), false);
  assert.equal(groupsAvailable(true, { brands: [], none: false }, true), false);
});

test('BRAND-GROUPS I3 resolveSelection: a stored group resolves to its offered members, else all', () => {
  const groups = offeredGroups(G_ROSTER, G_ROSTER, G_DOCS, true);
  const r = (stored, gs = groups, roster = G_ROSTER) => resolveSelection(stored, roster, true, roster, gs);
  assert.deepEqual(r(group('incubating')).selection, INC);
  assert.deepEqual(r(group('established')).selection, EST);
  // The stored slugs are ignored: resolution adds them.
  assert.deepEqual(r(group('incubating', ['zeta'])).selection, INC);
  // Unavailable (no groups offered) or not offered: all, pref untouched.
  assert.deepEqual(r(group('incubating'), []).selection, ALL);
  assert.deepEqual(r(group('established'), [groups[0]]).selection, ALL);
  // An empty group from the caller is never resolved to (slugs never empty).
  assert.deepEqual(r(group('incubating'), [{ group: 'incubating', slugs: [] }]).selection, ALL);
  // An unknown group name is invalid.
  assert.deepEqual(r(group('bogus'), [{ group: 'bogus', slugs: ['ruze'] }]).selection, ALL);
  // Locked and hidden resolve exactly as before.
  const locked = { brands: [{ slug: 'ruze', label: 'Ruze' }], none: false };
  assert.deepEqual(r(group('incubating'), groups, locked), {
    pending: false, selection: brand('ruze'), locked: true, hidden: false,
  });
  const hidden = { brands: [], none: false };
  assert.deepEqual(r(group('incubating'), groups, hidden), {
    pending: false, selection: ALL, locked: false, hidden: true,
  });
  // Omitting groups: today's behaviour (a stored group is all).
  assert.deepEqual(resolveSelection(group('incubating'), G_ROSTER).selection, ALL);
});

test('BRAND-GROUPS I4 selectionLoaded + groupWaits: a stored group waits for the documents, and only then', () => {
  // A brands:get user, unlocked selector: pending until the documents load, under every launch value.
  assert.equal(groupWaits(group('incubating'), true, G_ROSTER), true);
  assert.equal(selectionLoaded(true, 'all', false, true), false);
  assert.equal(selectionLoaded(true, 'all', true, true), true);
  assert.equal(selectionLoaded(true, 'launched', false, true), false);
  // No brands:get: never waits (the group resolves to all at once).
  assert.equal(groupWaits(group('incubating'), false, G_ROSTER), false);
  // A locked or hidden selector: never waits because of the group.
  assert.equal(groupWaits(group('incubating'), true, { brands: [{ slug: 'ruze' }], none: false }), false);
  assert.equal(groupWaits(group('incubating'), true, { brands: [], none: false }), false);
  // Not a group: never waits because of it.
  assert.equal(groupWaits(brand('ruze'), true, G_ROSTER), false);
  assert.equal(groupWaits(ALL, true, G_ROSTER), false);
  assert.equal(groupWaits(group('bogus'), true, G_ROSTER), false);
  // The store's composition: pending, then the group.
  const scope = (healthLoaded, can = true) => resolveSelection(
    group('established'),
    G_ROSTER,
    selectionLoaded(true, 'all', healthLoaded, groupWaits(group('established'), can, G_ROSTER)),
    G_ROSTER,
    offeredGroups(G_ROSTER, G_ROSTER, G_DOCS, groupsAvailable(can, G_ROSTER, healthLoaded)),
  );
  assert.deepEqual(scope(false), { pending: true });
  assert.deepEqual(scope(true).selection, EST);
  assert.deepEqual(scope(false, false).selection, ALL);
  // The lists gate still comes first.
  assert.equal(selectionLoaded(false, 'all', true, false), false);
});

test('BRAND-GROUPS I5 scopeListIds: exactly the members\' lists; null (never []) without slugs', () => {
  assert.deepEqual(scopeListIds(G_LISTS, INC), [6, 7]);
  assert.deepEqual(scopeListIds(G_LISTS, EST), [2, 3, 4, 5]);
  // No brandless list, no other brand's list.
  assert.ok(!scopeListIds(G_LISTS, INC).includes(9));
  assert.ok(!scopeListIds(G_LISTS, INC).includes(1));
  // The stored shape (missing or empty slugs) is no scope.
  assert.equal(scopeListIds(G_LISTS, group('incubating')), null);
  assert.equal(scopeListIds(G_LISTS, group('incubating', [])), null);
  assert.equal(scopeListIds(G_LISTS, group('incubating', [''])), null);
  // The by-query bodies and the export are built from that set.
  const qp = {
    search: '', queryExp: '', listID: 0, subStatus: '',
  };
  const ids = scopeListIds(G_LISTS, EST);
  assert.deepEqual(subscriberQueryBody(qp, ids).list_ids, [2, 3, 4, 5]);
  assert.deepEqual(subscriberExportParams(qp, ids), [['list_id', '2'], ['list_id', '3'], ['list_id', '4'], ['list_id', '5']]);
  // A group without slugs carries no list_ids at all (null), as All brands.
  assert.equal(subscriberQueryBody(qp, scopeListIds(G_LISTS, group('incubating'))).list_ids, null);
  assert.deepEqual(subscriberExportParams(qp, scopeListIds(G_LISTS, group('incubating'))), []);
});

test('BRAND-GROUPS I6 the resolved key differs exactly when the members differ', () => {
  const key = (docs) => {
    const gs = offeredGroups(G_ROSTER, G_ROSTER, docs, true);
    return JSON.stringify(resolveSelection(group('incubating'), G_ROSTER, true, G_ROSTER, gs).selection);
  };
  // zeta launches: Incubating gains a member, the key changes.
  const launchedZeta = G_DOCS.map((d) => (d.brand === 'zeta' ? { ...d, status: 'ok' } : d));
  assert.notEqual(key(G_DOCS), key(launchedZeta));
  // The same members (documents in another order): the same key -- slugs are sorted.
  assert.equal(key(G_DOCS), key([...G_DOCS].reverse()));
  assert.equal(key(G_DOCS), JSON.stringify(INC));
});

test('BRAND-GROUPS I8 listsScopeParams: brand=<members> for a group and nothing else', () => {
  assert.deepEqual(listsScopeParams(EST), { brand: ['joybelle', 'shala', 'younique'] });
  assert.deepEqual(listsScopeParams(group('established')), {});
  // Unchanged for the other kinds.
  assert.deepEqual(listsScopeParams(brand('ruze')), { tag: ['brand:ruze'] });
  assert.deepEqual(listsScopeParams(NONE), { nobrand: true });
  assert.deepEqual(listsScopeParams(ALL), {});
});

test('BRAND-GROUPS I7: every one-brand predicate is false under a group; every row rule follows the members', () => {
  // canCreateCampaign
  assert.equal(canCreateCampaign(INC), false);
  assert.equal(canCreateCampaign(group('incubating')), false);
  // formBrandDefault: as all, on both forms.
  assert.deepEqual(formBrandDefault(EST, 'template'), { value: null, locked: false });
  assert.deepEqual(formBrandDefault(EST, 'list'), { value: null, locked: false });
  // mediaScopeContext: All (media tags are AND-matched).
  assert.deepEqual(mediaScopeContext(EST), { context: [], untagged: false });
  // filterTemplates: the brandless rows plus the members'.
  const T = [{ id: 1, brand: '' }, { id: 2, brand: 'shala' }, { id: 3, brand: 'ruze' }, { id: 4, brand: 'younique' }, { id: 5 }];
  assert.deepEqual(filterTemplates(T, EST).map((t) => t.id), [1, 2, 4, 5]);
  assert.deepEqual(filterTemplates(T, INC).map((t) => t.id), [1, 3, 5]);
  assert.equal(filterTemplates(T, group('incubating')).length, 5);
  // templateNoticeBrand: a brandless template is in scope (undefined), a branded one keeps its brand.
  assert.equal(templateNoticeBrand('', EST), undefined);
  assert.equal(templateNoticeBrand('ruze', EST), 'ruze');
  // filterBrandRows: the members' rows.
  const R = [{ brand: 'ruze' }, { brand: 'shala' }, { brand: 'younique' }, { brand: 'bnkd' }];
  assert.deepEqual(filterBrandRows(R, INC), [{ brand: 'ruze' }, { brand: 'bnkd' }]);
  assert.deepEqual(filterBrandRows(R, EST), [{ brand: 'shala' }, { brand: 'younique' }]);
  assert.equal(filterBrandRows(R, group('established')).length, 4);
  // contextNotice: null for a member; else the notice named by the group's label.
  assert.equal(contextNotice('shala', EST, G_BRANDS), null);
  assert.deepEqual(contextNotice('ruze', EST, G_BRANDS), { record: 'Ruze Pouches', selected: 'Established' });
  assert.deepEqual(contextNotice('', INC, G_BRANDS, { noneLabel: 'Sans marque', groupLabels: { incubating: 'Incubation' } }), { record: 'Sans marque', selected: 'Incubation' });
  assert.equal(contextNotice('ruze', group('established'), G_BRANDS), null);
  assert.equal(contextNotice(undefined, EST, G_BRANDS), null);
});

test('BRAND-GROUPS I10 selectorOptions: groups, parent, rest and the divider rule', () => {
  const offered = G_ROSTER.brands;
  const groups = offeredGroups(G_ROSTER, G_ROSTER, G_DOCS, true);
  const o = selectorOptions(offered, groups, true);
  assert.deepEqual(o.groups.map((g) => g.group), ['incubating', 'established']);
  assert.equal(o.parent.slug, 'curated');
  assert.deepEqual(o.rest.map((b) => b.label), ['BNKD Wellness', 'JoyBelle', 'Ruze Pouches', 'Shala', 'Younique', 'Zeta']);
  assert.equal(o.divider, true);
  // Groups given out of order keep BRAND_GROUPS order; empty ones are dropped.
  assert.deepEqual(selectorOptions(offered, [groups[1], { group: 'incubating', slugs: [] }]).groups.map((g) => g.group), ['established']);
  // Groups absent, parent present: the divider as before.
  assert.equal(selectorOptions(offered, []).divider, true);
  // Groups present, parent absent: still a divider.
  const noParent = offered.filter((b) => b.slug !== 'curated');
  const np = selectorOptions(noParent, groups);
  assert.equal(np.parent, null);
  assert.equal(np.divider, true);
  // Neither groups nor parent: no divider.
  assert.equal(selectorOptions(noParent, []).divider, false);
  // Nothing below: no divider, unless No brand is shown.
  const onlyParent = offered.filter((b) => b.slug === 'curated');
  assert.equal(selectorOptions(onlyParent, groups).divider, false);
  assert.equal(selectorOptions(onlyParent, groups, true).divider, true);
  assert.deepEqual(selectorOptions(undefined, undefined), {
    groups: [], parent: null, rest: [], divider: false,
  });
});
