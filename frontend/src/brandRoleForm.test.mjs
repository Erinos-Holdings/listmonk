// Fork (brand roles, integrations listmonk/BRAND-ROLES-SPEC I12) -- the Brand role form's pure half.
// Run: npm run test:brand-roles.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  ALL_BRANDS, brandRolePayload, grantLabels, manageOptions, moveTo, normalizeReadPicks, picksFromRole, readOptions,
} from './brandRoleForm.mjs'; // eslint-disable-line import/extensions

const brands = [{ slug: 'joybelle' }, { slug: 'ruze' }, { slug: 'shala' }, { slug: 'younique' }];

test('I12 -- a brand is never offered at both levels', () => {
  assert.deepEqual(manageOptions(brands, ['ruze']), ['joybelle', 'shala', 'younique']);
  assert.deepEqual(readOptions(brands, ['shala', 'younique']), [ALL_BRANDS, 'joybelle', 'ruze']);
  assert.deepEqual(readOptions(brands, []), [ALL_BRANDS, 'joybelle', 'ruze', 'shala', 'younique']);
  // All brands is never a read/write option.
  assert.ok(!manageOptions(brands, [ALL_BRANDS]).includes(ALL_BRANDS));
  assert.deepEqual(manageOptions(brands, [ALL_BRANDS]), ['joybelle', 'ruze', 'shala', 'younique']);
  // Moving a brand to the other level removes it from the first.
  assert.deepEqual(moveTo('analytics', 'shala', ['shala'], []), { manage: [], read: ['shala'] });
  assert.deepEqual(moveTo('manage', 'ruze', [], ['ruze', 'joybelle']), { manage: ['ruze'], read: ['joybelle'] });
  assert.deepEqual(moveTo('manage', ALL_BRANDS, [], []), { manage: [], read: [] });
});

test('I12 -- All brands clears the redundant read-only picks; manage picks are kept', () => {
  assert.deepEqual(normalizeReadPicks(['ruze', ALL_BRANDS, 'joybelle']), [ALL_BRANDS]);
  assert.deepEqual(normalizeReadPicks(['ruze', 'ruze']), ['ruze']);
  assert.deepEqual(moveTo('analytics', ALL_BRANDS, ['shala'], ['ruze']), { manage: ['shala'], read: [ALL_BRANDS] });
});

test('I12 -- the payload has brands and no lists', () => {
  const body = brandRolePayload('shala_younique', ['shala', 'younique', 'joybelle'], [ALL_BRANDS, 'ruze']);
  assert.deepEqual(Object.keys(body).sort(), ['brands', 'name']);
  assert.equal('lists' in body, false);
  assert.deepEqual(body.brands, [
    { brand: 'shala', level: 'manage' },
    { brand: 'younique', level: 'manage' },
    { brand: 'joybelle', level: 'manage' },
    { brand: null, level: 'analytics' },
  ]);
  // A brand at both levels is sent at manage only; All brands is never sent at manage.
  assert.deepEqual(brandRolePayload('x', ['ruze', ALL_BRANDS], ['ruze']).brands, [{ brand: 'ruze', level: 'manage' }]);
  assert.deepEqual(brandRolePayload('x', [], []).brands, []);
  // The form's request bodies come from the helper (RoleForm.vue never builds a lists field).
  const form = readFileSync(new URL('./views/RoleForm.vue', import.meta.url), 'utf8');
  assert.doesNotMatch(form, /form\.lists\s*=/);
  assert.equal((form.match(/brandRolePayload\(/g) || []).length, 2);
});

test('I12 -- a stored role reads back into the two pick arrays and the listing labels', () => {
  const role = { brands: [{ brand: 'shala', level: 'manage' }, { brand: null, level: 'analytics' }, { brand: 'ruze', level: 'analytics' }] };
  assert.deepEqual(picksFromRole(role), { manage: ['shala'], read: [ALL_BRANDS] });
  assert.deepEqual(picksFromRole({ brands: [{ brand: 'ruze', level: 'analytics' }] }), { manage: [], read: ['ruze'] });
  assert.deepEqual(picksFromRole(null), { manage: [], read: [] });
  const labels = grantLabels({ brands: [{ brand: 'shala', level: 'manage' }, { brand: null, level: 'analytics' }] }, (s) => s.toUpperCase(), 'All brands');
  assert.deepEqual(labels, { manage: ['SHALA'], analytics: ['All brands'] });
});
