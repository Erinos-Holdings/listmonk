// Fork (brand picker, integrations BRAND-PICKER-SPEC I15). Run: yarn test:list-brand
// (node --test src/listBrand.test.mjs). The list form never sends a reserved tag and never sends
// brand "".
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  isReservedTag,
  splitReservedTags,
  buildListRequest,
  isLockedList,
  brandLabel,
} from './listBrand.mjs'; // eslint-disable-line import/extensions

test('isReservedTag judges after trimming, like the server', () => {
  ['brand:x', ' brand: x', 'from:A <a@b.test>', '  site:https://x.test ', 'site:'].forEach((t) => {
    assert.equal(isReservedTag(t), true, t);
  });
  ['repermission:12', 'holiday-2026', 'brandx', 'my brand:x', '', null].forEach((t) => {
    assert.equal(isReservedTag(t), false, String(t));
  });
});

test('splitReservedTags hides the projection and keeps the free tags in order', () => {
  const { free, reserved } = splitReservedTags(['b', 'brand:acme', 'from:Acme <hello@acme.test>', 'a', 'site:https://s.test', 'repermission:3']);
  assert.deepEqual(free, ['b', 'a', 'repermission:3']);
  assert.deepEqual(reserved, ['brand:acme', 'from:Acme <hello@acme.test>', 'site:https://s.test']);
  assert.deepEqual(splitReservedTags(undefined), { free: [], reserved: [] });
});

test('buildListRequest sends the free tags only, and the brand', () => {
  const form = {
    name: 'L', type: 'public', optin: 'single', status: 'active', tags: ['x', ' brand: other', 'from:X <x@x.test>', 'site:https://x.test'],
  };
  const req = buildListRequest(form, 'acme');
  assert.deepEqual(req.tags, ['x']);
  assert.equal(req.brand, 'acme');
  assert.equal(req.name, 'L');
  assert.equal(req.tags.some(isReservedTag), false);
  // The form's own tags are not mutated.
  assert.equal(form.tags.length, 4);
});

test('buildListRequest never sends brand ""', () => {
  [undefined, null, '', '   '].forEach((b) => {
    assert.throws(() => buildListRequest({ name: 'L', tags: [] }, b), /brand is required/);
  });
});

test('the locked list and the option label', () => {
  assert.equal(isLockedList({ name: 'Render catalog (never send)' }), true);
  assert.equal(isLockedList({ name: 'Render catalog' }), false);
  assert.equal(isLockedList(null), false);
  assert.equal(brandLabel({ slug: 'curated', display_name: 'Curated' }), 'Curated — curated');
});
