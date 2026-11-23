// Fork (brand roles, integrations listmonk/BRAND-ROLES-SPEC I10, I11 (the frontend half) and I12).
// Run: npm run test:brand-roles (node --test src/brandRoles.test.mjs src/brandRoleForm.test.mjs).
//   I10  the profile shape; accessPolicy reads list:analytics as neither get nor manage; the roster
//        includes analytics brands; the writing pickers exclude them; off the analytics pages an
//        analytics-only brand is disabled and a stored selection of one resolves to All; a campaign
//        of an analytics-only brand carries no campaign link.
//   I11  the launch filter is shown for an unlocked selector without brands:get; the launch rule's
//        agreement table (models/testdata/launch-rule.json, the Go handler test's too).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  LIST_ANALYTICS, analyticsListEntries, canList, canManageList, isAnalyticsEntry,
} from './accessPolicy.mjs'; // eslint-disable-line import/extensions
import {
  ALL, ANALYTICS_PAGES, analyticsOnlyBrands, brandDisabledOn, campaignLinkable, disabledBrands,
  filterByIds, isAnalyticsPage, launchDocsFrom, launchFilterShown, launchOf, resolveOnPage,
  rosterFrom, rosterLists, scopeListIds, scopeUnion, withoutBrands,
} from './brandScope.mjs'; // eslint-disable-line import/extensions
import { isUnlaunchedBrandDoc } from './health-sort.mjs'; // eslint-disable-line import/extensions

// A resolved list_role entry as /api/profile serves it.
const entry = (id, name, brand, permissions) => ({
  id, name, brand, permissions,
});

// Megan after gate G4: Shala at manage, every other brand at analytics -- the /api/profile shape
// (camelCased; brand and permissions keys unchanged).
const megan = {
  userRole: { id: 11, permissions: ['campaigns:get', 'campaigns:manage', 'subscribers:get'] },
  listRole: {
    id: 8,
    name: 'shala_younique',
    brands: [{ brand: 'shala', level: 'manage' }, { brand: null, level: 'analytics' }],
    lists: [
      entry(16, 'Shala news', 'shala', ['list:get', 'list:manage']),
      entry(15, 'Ruze news', 'ruze', [LIST_ANALYTICS]),
      entry(40, 'Ruze archived', 'ruze', [LIST_ANALYTICS]),
    ],
  },
};
// The minimal-lists store: the server's get|manage lists only (Shala's).
const store = { results: [{ id: 16, name: 'Shala news', brand: 'shala' }, { id: 17, name: 'Shala two', brand: 'shala' }] };
const brands = [{ slug: 'ruze', display_name: 'RUZE' }, { slug: 'shala', display_name: 'Shala' }];

test('I10 -- the profile shape and accessPolicy: list:analytics is neither get nor manage', () => {
  assert.deepEqual(megan.listRole.brands[1], { brand: null, level: 'analytics' });
  assert.ok(megan.listRole.lists.every((l) => typeof l.brand === 'string'));
  assert.equal(isAnalyticsEntry(megan.listRole.lists[1]), true);
  assert.equal(isAnalyticsEntry(megan.listRole.lists[0]), false);

  assert.equal(canManageList(megan, 16), true);
  assert.equal(canManageList(megan, 15), false, 'an analytics entry never manages');
  ['list:get', 'list:manage', LIST_ANALYTICS, 'anything'].forEach((perm) => {
    assert.equal(canList(megan, 15, perm), false, `$canList(15, ${perm})`);
  });
  assert.equal(canList(megan, 16, 'list:get'), true);
  assert.equal(canList(megan, 16, 'list:manage'), true);
  assert.equal(canList({ userRole: { id: 5, permissions: [] }, listRole: null }, 16, 'list:get'), false, 'no list role: never throws');
  assert.equal(canList({ userRole: { id: 1, permissions: [] }, listRole: null }, 15, 'list:get'), true, 'Super Admin');

  assert.deepEqual(analyticsListEntries(megan), [{ id: 15, brand: 'ruze' }, { id: 40, brand: 'ruze' }]);
  assert.deepEqual(analyticsListEntries(null), []);
  assert.deepEqual(analyticsListEntries({ userRole: { id: 5, permissions: [] }, listRole: null }), []);
});

test('I10 -- the roster includes analytics brands; the writing pickers never see their lists', () => {
  const entries = analyticsListEntries(megan);
  const roster = rosterFrom(rosterLists(store, entries), brands);
  assert.deepEqual(roster.brands.map((b) => b.slug), ['ruze', 'shala']);
  assert.deepEqual(analyticsOnlyBrands(store, entries), ['ruze']);

  // Ruze selected on an analytics page: its own ids (archived included) for the server to intersect.
  const ruze = { kind: 'brand', slug: 'ruze' };
  const ids = scopeListIds(rosterLists(store, entries), ruze);
  assert.deepEqual(ids, [15, 40]);
  // Every picker that writes filters the lists STORE by the scope: an analytics list never appears.
  assert.deepEqual(filterByIds(store, scopeUnion(ids, [])), []);
  assert.deepEqual(filterByIds(store, scopeUnion(scopeListIds(rosterLists(store, entries), { kind: 'brand', slug: 'shala' }), [])).map((l) => l.id), [16, 17]);
  // The store itself is untouched by the roster's union.
  assert.equal(store.results.length, 2);
  // A brand both held and in the entries (impossible under D2 precedence, but the union dedups by id).
  assert.deepEqual(rosterLists(store, [{ id: 16, brand: 'shala' }]).length, 2);
});

test('I10 -- off the analytics pages an analytics-only brand is disabled and a stored one resolves to All', () => {
  assert.deepEqual([...ANALYTICS_PAGES].sort(), ['brand', 'brands', 'campaignAnalytics', 'dashboard']);
  ['subscribers', 'campaigns', 'campaign', 'bounces', 'lists', 'media', 'templates', 'import', ''].forEach((page) => {
    assert.equal(isAnalyticsPage(page), false, page);
    assert.equal(brandDisabledOn('ruze', ['ruze'], page), true, page);
    assert.equal(brandDisabledOn('shala', ['ruze'], page), false, page);
  });
  ANALYTICS_PAGES.forEach((page) => {
    assert.equal(brandDisabledOn('ruze', ['ruze'], page), false, page);
    assert.deepEqual(disabledBrands(['ruze'], page), []);
  });
  assert.deepEqual(disabledBrands(['ruze', 'bnkd'], 'subscribers'), ['ruze', 'bnkd']);

  const entries = analyticsListEntries(megan);
  const roster = rosterFrom(rosterLists(store, entries), brands);
  const stored = { kind: 'brand', slug: 'ruze' };
  const off = resolveOnPage(stored, roster, true, roster, [], disabledBrands(['ruze'], 'subscribers'));
  assert.deepEqual(off.selection, ALL, 'Subscribers: a stored Ruze resolves to All');
  assert.equal(off.locked, false);
  assert.equal(stored.slug, 'ruze', 'the stored value is not rewritten');
  const on = resolveOnPage(stored, roster, true, roster, [], disabledBrands(['ruze'], 'dashboard'));
  assert.deepEqual(on.selection, stored, 'Dashboard: Ruze stays selected');
  // A Shala selection is unaffected on every page.
  assert.deepEqual(resolveOnPage({ kind: 'brand', slug: 'shala' }, roster, true, roster, [], ['ruze']).selection, { kind: 'brand', slug: 'shala' });

  // A selector locked to its one (analytics-only) brand: All off the analytics pages, still locked.
  const elizRoster = rosterFrom(rosterLists([], [{ id: 15, brand: 'ruze' }]), brands);
  const elizOff = resolveOnPage(ALL, elizRoster, true, elizRoster, [], ['ruze']);
  assert.equal(elizOff.locked, true);
  assert.deepEqual(elizOff.selection, ALL);
  assert.deepEqual(resolveOnPage(ALL, elizRoster, true, elizRoster, [], []).selection, { kind: 'brand', slug: 'ruze' });
  assert.deepEqual(resolveOnPage(stored, roster, false, roster, [], ['ruze']), { pending: true });

  assert.deepEqual(withoutBrands(roster, ['ruze']).brands.map((b) => b.slug), ['shala']);
});

test('I10 -- a campaign of an analytics-only brand carries no campaign link', () => {
  assert.equal(campaignLinkable('ruze', ['ruze']), false);
  assert.equal(campaignLinkable('shala', ['ruze']), true);
  assert.equal(campaignLinkable('', ['ruze']), true);
  assert.equal(campaignLinkable(undefined, []), true);
  // The Brands page's send rows read it.
  const view = readFileSync(new URL('./views/Brands.vue', import.meta.url), 'utf8');
  assert.match(view, /v-if="s\.id && campaignLinks" :to="\{ name: 'campaign'/);
});

test('I11 -- the launch filter is shown for an unlocked selector, without brands:get', () => {
  assert.equal(launchFilterShown({ pending: false, hidden: false, locked: false }), true);
  assert.equal(launchFilterShown({ pending: false, hidden: true, locked: false }), false);
  assert.equal(launchFilterShown({ pending: false, hidden: false, locked: true }), false);
  assert.equal(launchFilterShown(null), false);
  const vue = readFileSync(new URL('./components/LaunchFilter.vue', import.meta.url), 'utf8');
  assert.doesNotMatch(vue, /\$can\(/, 'LaunchFilter.vue carries no permission condition');
  assert.match(vue, /launchFilterShown\(this\.brandScope\)/);
});

test('I11 -- the launch rule agrees with the server statement on every status value', () => {
  const cases = JSON.parse(readFileSync(new URL('../../models/testdata/launch-rule.json', import.meta.url), 'utf8'));
  assert.ok(cases.length >= 4);
  cases.forEach(({ status, unlaunched }) => {
    assert.equal(isUnlaunchedBrandDoc({ brand: 'x', status }), unlaunched, status);
    // The endpoint's bit, read back through launchDocsFrom + launchOf, is the same answer.
    assert.equal(launchOf('x', launchDocsFrom([{ brand: 'x', unlaunched }])) === 'unlaunched', unlaunched, status);
  });
  assert.equal(isUnlaunchedBrandDoc(undefined), false, 'no document = launched (the server: no row = false)');
  assert.deepEqual(launchDocsFrom([{ brand: 'a', unlaunched: false }, { brand: 'b', unlaunched: true }, null]), [{ brand: 'b', status: 'unknown' }]);
});
