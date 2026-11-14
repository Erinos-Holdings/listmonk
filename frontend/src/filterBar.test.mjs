// Fork (polish 2026-11-14): filterBar.mjs -- which global filters the blue bar names and which
// columns it says are hidden. Run: node --test src/filterBar.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { filterBarState } from './filterBar.mjs'; // eslint-disable-line import/extensions

const ALL = { kind: 'all' };
const RUZE = { kind: 'brand', slug: 'ruze' };
const NONE = { kind: 'none' };

test('nothing active: no bar', () => {
  assert.equal(filterBarState({ selection: ALL, launch: 'all', lang: 'all' }), null);
  assert.equal(filterBarState(), null);
  // A filter the table does not take is never listed.
  assert.equal(filterBarState({ selection: ALL, launch: 'launched', lang: 'all' }), null);
  assert.equal(filterBarState({ selection: RUZE, lang: 'fr', applies: { brand: false, lang: false } }), null);
});

test('each filter, in order, with its hidden column', () => {
  const s = filterBarState({
    selection: RUZE,
    launch: 'launched',
    lang: 'es',
    applies: { launch: true },
    columns: { brand: true, lang: true },
  });
  assert.deepEqual(s.items, [
    { key: 'filterBar.brand', params: { brand: 'ruze' } },
    { key: 'filterBar.launched', params: {} },
    { key: 'filterBar.lang', params: { lang: 'ES' } },
  ]);
  assert.deepEqual(s.hidden, ['filterBar.col.brand', 'filterBar.col.lang']);
  // No column declared: none listed as hidden.
  assert.deepEqual(filterBarState({ selection: RUZE, lang: 'es' }).hidden, []);
  // No brand hides no Brand column (several brandless rows may still differ).
  const n = filterBarState({ selection: NONE, lang: 'all', columns: { brand: true } });
  assert.deepEqual(n, { items: [{ key: 'filterBar.noBrand', params: {} }], hidden: [] });
  // Unknown launch / lang values resolve to all.
  assert.equal(filterBarState({
    selection: ALL, launch: 'x', lang: 'pt', applies: { launch: true },
  }), null);
});

test('every key the helper emits is in en.json', () => {
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  ['filterBar.label', 'filterBar.brand', 'filterBar.noBrand', 'filterBar.launched', 'filterBar.unlaunched',
    'filterBar.lang', 'filterBar.hidden', 'filterBar.col.brand', 'filterBar.col.lang']
    .forEach((k) => assert.ok(en[k], k));
});
