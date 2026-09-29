// Fork (brand analytics, integrations BRAND-ANALYTICS-SPEC I7/I8). Run: npm run test:access
// (node --test src/accessPolicy.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  isAnalyticsOnly,
  canManageList,
  canViewBrand,
  routeRedirect,
  defaultFromDate,
} from './accessPolicy.mjs'; // eslint-disable-line import/extensions

const profile = (id, permissions, lists) => ({
  userRole: { id, permissions },
  listRole: lists === undefined ? null : { lists },
});

// The user-role census of 2026-09-29 (spec §1 D1): the live permission sets of every role, read
// as listmonk_agent (GET /api/roles/users, /api/roles/lists) by the spec thread. Role 11's list
// role is shala_younique (lists 16 Shala, 23 Younique).
const RUZE = 15;
const OTHER = 99;
const census = {
  1: profile(1, []),
  2: profile(2, ['lists:get_all', 'lists:manage_all', 'subscribers:get', 'subscribers:get_all', 'subscribers:manage',
    'subscribers:import', 'campaigns:get', 'campaigns:get_all', 'campaigns:get_analytics', 'campaigns:manage',
    'campaigns:manage_all', 'bounces:get', 'media:get', 'media:manage', 'templates:get', 'templates:manage', 'brands:get']),
  3: profile(3, ['subscribers:get', 'subscribers:get_all', 'subscribers:manage', 'subscribers:import', 'tx:send',
    'campaigns:get', 'campaigns:get_all', 'campaigns:get_analytics', 'campaigns:manage', 'campaigns:manage_all',
    'campaigns:send', 'bounces:get', 'bounces:manage', 'webhooks:post_bounce', 'media:get', 'media:manage', 'templates:get',
    'templates:manage', 'lists:get_all', 'users:get', 'users:manage', 'roles:get', 'roles:manage', 'settings:get',
    'lists:manage_all', 'subscribers:sql_query', 'brands:get']),
  4: profile(4, ['subscribers:get', 'subscribers:get_all', 'subscribers:manage', 'lists:get_all', 'lists:manage_all']),
  5: profile(5, ['lists:get_all', 'campaigns:get_all', 'campaigns:get_analytics', 'brands:get']),
  6: profile(6, ['lists:get_all', 'subscribers:get', 'subscribers:get_all', 'subscribers:sql_query', 'campaigns:get',
    'campaigns:get_all', 'campaigns:get_analytics', 'bounces:get', 'media:get', 'templates:get', 'users:get', 'roles:get',
    'settings:get']),
  7: profile(7, ['lists:get_all', 'lists:manage_all', 'subscribers:get', 'subscribers:get_all', 'subscribers:manage',
    'campaigns:get', 'campaigns:get_all', 'campaigns:get_analytics', 'campaigns:manage_all', 'templates:manage',
    'media:manage', 'settings:maintain', 'campaigns:review', 'campaigns:review_structure']),
  11: profile(
    11,
    ['campaigns:get', 'campaigns:manage', 'campaigns:get_analytics', 'subscribers:get', 'subscribers:manage',
      'templates:get', 'media:get', 'media:manage'],
    [{ id: 16, permissions: ['list:get', 'list:manage'] }, { id: 23, permissions: ['list:get', 'list:manage'] }],
  ),
  13: profile(13, ['subscribers:get', 'subscribers:get_all', 'subscribers:sql_query', 'subscribers:manage', 'lists:get_all',
    'lists:manage_all', 'campaigns:get_all']),
  14: profile(14, ['brands:manage']),
  15: profile(15, ['campaigns:get_all', 'lists:get_all', 'templates:get', 'media:get', 'brands:get', 'campaigns:review']),
  18: profile(18, ['campaigns:get_analytics'], [{ id: RUZE, permissions: ['list:get'] }]),
};

test('I7 isAnalyticsOnly: of every current role, only 18 (Brand Analytics)', () => {
  Object.entries(census).forEach(([id, p]) => {
    assert.equal(isAnalyticsOnly(p), id === '18', `role ${id}`);
  });
  // Super Admin is never restricted, whatever its permission list says.
  assert.equal(isAnalyticsOnly(profile(1, ['campaigns:get_analytics'])), false);
  // get_analytics alongside either campaigns read -> not analytics-only.
  assert.equal(isAnalyticsOnly(profile(20, ['campaigns:get_analytics', 'campaigns:get'])), false);
  assert.equal(isAnalyticsOnly(profile(20, ['campaigns:get_analytics', 'campaigns:get_all'])), false);
  // No get_analytics -> not analytics-only (nothing to show them).
  assert.equal(isAnalyticsOnly(profile(20, [])), false);
  // Derived from grants, not the role id: another role with the same grant is analytics-only.
  assert.equal(isAnalyticsOnly(profile(42, ['campaigns:get_analytics'], null)), true);
});

test('I7 canManageList truth table', () => {
  assert.equal(canManageList(census[1], RUZE), true, 'super admin');
  assert.equal(canManageList(profile(20, ['lists:manage_all']), RUZE), true, 'lists:manage_all');
  assert.equal(canManageList(profile(20, [], [{ id: RUZE, permissions: ['list:get', 'list:manage'] }]), RUZE), true, 'list:manage');
  assert.equal(canManageList(profile(20, [], [{ id: OTHER, permissions: ['list:manage'] }]), RUZE), false, 'list:manage elsewhere');
  assert.equal(canManageList(census[18], RUZE), false, 'list:get only');
  assert.equal(canManageList(profile(20, ['lists:get_all']), RUZE), false, 'lists:get_all alone');
  // Every current role: the lists:manage_all holders and Super Admin manage any list; role 11
  // manages its own lists only; nobody else manages the Ruze list.
  const managers = ['1', '2', '3', '4', '7', '13'];
  Object.entries(census).forEach(([id, p]) => {
    assert.equal(canManageList(p, RUZE), managers.includes(id), `role ${id}`);
  });
  assert.equal(canManageList(census[11], 16), true, 'role 11 on its own list');
});

test('I7 canViewBrand', () => {
  assert.equal(canViewBrand(census[1]), true);
  assert.equal(canViewBrand(census[2]), true);
  assert.equal(canViewBrand(census[18]), false);
  assert.equal(canViewBrand(census[14]), false, 'brands:manage alone is not brands:get');
  assert.equal(canViewBrand(null), false);
});

test('I7 routeRedirect: analytics-only users only', () => {
  const p = census[18];
  ['campaigns', 'campaign', 'campaignReview', 'media', 'templates', 'brands', 'brand'].forEach((r) => {
    assert.deepEqual(routeRedirect(p, r, { id: 1 }), { name: 'dashboard' }, r);
  });
  assert.deepEqual(routeRedirect(p, 'forms', {}), { name: 'lists' });
  assert.deepEqual(routeRedirect(p, 'list', { id: String(RUZE) }), { name: 'lists' });
  ['dashboard', 'lists', 'campaignAnalytics', 'userProfile', '404_page', undefined].forEach((r) => {
    assert.equal(routeRedirect(p, r, {}), null, String(r));
  });

  // An analytics-only user who may read brands keeps the brand pages.
  const withBrands = profile(18, ['campaigns:get_analytics', 'brands:get'], [{ id: RUZE, permissions: ['list:get'] }]);
  assert.equal(routeRedirect(withBrands, 'brands', {}), null);
  assert.equal(routeRedirect(withBrands, 'brand', { brand: 'ruze' }), null);

  // Every other current role: never redirected, on any guarded route.
  Object.entries(census).filter(([id]) => id !== '18').forEach(([id, prof]) => {
    ['campaigns', 'campaign', 'campaignReview', 'media', 'templates', 'brands', 'brand', 'forms', 'list'].forEach((r) => {
      assert.equal(routeRedirect(prof, r, {}), null, `role ${id} ${r}`);
    });
  });

  // Profile not loaded yet -> no decision.
  assert.equal(routeRedirect(null, 'campaigns', {}), null);
  assert.equal(routeRedirect(undefined, 'campaigns', {}), null);
});

test('I7 listRole null and partial profiles never throw', () => {
  const noListRole = profile(18, ['campaigns:get_analytics'], null);
  assert.equal(isAnalyticsOnly(noListRole), true);
  assert.equal(canManageList(noListRole, RUZE), false);
  assert.equal(canViewBrand(noListRole), false);
  assert.deepEqual(routeRedirect(noListRole, 'list', {}), { name: 'lists' });

  [null, undefined, {}, { userRole: null, listRole: null }, { userRole: {}, listRole: {} },
    { userRole: { id: 5 }, listRole: { lists: null } }].forEach((p) => {
    assert.doesNotThrow(() => {
      isAnalyticsOnly(p);
      canManageList(p, RUZE);
      canViewBrand(p);
      routeRedirect(p, 'campaigns', {});
    }, JSON.stringify(p));
    assert.equal(isAnalyticsOnly(p), false);
    assert.equal(canManageList(p, RUZE), false);
  });
});

// Local-time constructors keep I8 independent of the machine's time zone.
const local = (y, m, d, h = 0, min = 0) => new Date(y, m - 1, d, h, min, 0, 0);

test('I8 defaultFromDate: earliest started_at, start of day', () => {
  const out = defaultFromDate([
    { id: 1, startedAt: local(2026, 9, 12, 15, 30).toISOString(), createdAt: local(2026, 9, 1).toISOString() },
    { id: 2, startedAt: local(2026, 9, 10, 8, 5).toISOString(), createdAt: local(2026, 9, 9).toISOString() },
  ]);
  assert.deepEqual(out, local(2026, 9, 10));
});

test('I8 defaultFromDate: created_at fallback for a campaign that never started', () => {
  const out = defaultFromDate([{ id: 1, startedAt: null, createdAt: local(2026, 8, 3, 22, 10).toISOString() }]);
  assert.deepEqual(out, local(2026, 8, 3));
});

test('I8 defaultFromDate: mixed -- a never-started campaign created earliest wins', () => {
  const out = defaultFromDate([
    { id: 1, startedAt: local(2026, 9, 12, 9).toISOString(), createdAt: local(2026, 9, 1).toISOString() },
    { id: 2, startedAt: null, createdAt: local(2026, 9, 5, 18).toISOString() },
    { id: 3, started_at: local(2026, 9, 7, 1).toISOString(), created_at: local(2026, 9, 6).toISOString() },
  ]);
  assert.deepEqual(out, local(2026, 9, 5));
  // started_at, not created_at, counts for a started campaign even when it was created earlier.
  assert.deepEqual(defaultFromDate([
    { id: 1, startedAt: local(2026, 9, 12, 9).toISOString(), createdAt: local(2026, 9, 1).toISOString() },
  ]), local(2026, 9, 12));
});

test('I8 defaultFromDate: empty selection -> null', () => {
  assert.equal(defaultFromDate([]), null);
  assert.equal(defaultFromDate(null), null);
  assert.equal(defaultFromDate(undefined), null);
  assert.equal(defaultFromDate([{ id: 1, startedAt: null, createdAt: null }]), null);
  assert.equal(defaultFromDate([{ id: 1, startedAt: 'not a date' }]), null);
});
