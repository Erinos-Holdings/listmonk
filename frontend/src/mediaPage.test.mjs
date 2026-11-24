// Fork (page state) -- integrations listmonk/PAGE-STATE-SPEC I3/I4/I5 for the /media page.
// Run: node --test src/mediaPage.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { restoreMediaPage, saveMediaPage } from './mediaPage.mjs'; // eslint-disable-line import/extensions

const KEY = 'brand:ruze|launch:all';

test('PAGE-STATE I3: search, chips, Untagged, All and page round-trip; uploads and editors never', () => {
  const saved = saveMediaPage({
    query: 'logo',
    page: 2,
    chips: [{ tag: 'ruze', on: true, context: true }, {
      tag: 'bfcm', on: false, context: false, extra: 1,
    }],
    untaggedOn: true,
    allOn: false,
    brandKey: KEY,
    form: { files: [1] },
    showUploadForm: true,
    editingId: 4,
    uploadTags: ['x'],
  });
  const restored = restoreMediaPage(JSON.parse(JSON.stringify(saved)), KEY);
  assert.deepEqual(restored, {
    query: 'logo',
    page: 2,
    chips: [{ tag: 'ruze', on: true, context: true }, { tag: 'bfcm', on: false, context: false }],
    untaggedOn: true,
    allOn: false,
  });
});

test('PAGE-STATE I4/D7: kept under another brand selection, nothing restores (the filter re-seeds)', () => {
  const saved = saveMediaPage({ query: 'logo', brandKey: KEY });
  assert.equal(restoreMediaPage(saved, 'brand:bnkd|launch:all'), null);
  assert.equal(restoreMediaPage(saved, undefined), null);
  assert.equal(restoreMediaPage(null, KEY), null);
  assert.equal(restoreMediaPage({ query: 'x' }, KEY), null);
});

test('PAGE-STATE I5: bad chips, flags and pages default', () => {
  const r = restoreMediaPage({
    brandKey: KEY, chips: [null, { tag: '' }, { tag: 7 }, { tag: 'ok', on: 'yes' }], untaggedOn: 'true', allOn: 1, page: 'x',
  }, KEY);
  assert.deepEqual(r.chips, [{ tag: 'ok', on: false, context: false }]);
  assert.equal(r.untaggedOn, false);
  assert.equal(r.allOn, false);
  [0, -2, 'x', 1.5].forEach((p) => assert.equal(restoreMediaPage({ brandKey: KEY, page: p }, KEY).page, 1, String(p)));
  assert.deepEqual(restoreMediaPage({ brandKey: KEY, chips: 'ruze' }, KEY).chips, []);
});

test('PAGE-STATE: the picker (modal mode) neither saves nor restores', () => {
  const vue = readFileSync(new URL('./views/Media.vue', import.meta.url), 'utf8');
  assert.match(vue, /if \(!this\.isModal\) \{\s*savePageState\(/);
  assert.match(vue, /this\.isModal \? null : restoreMediaPage\(/);
});
