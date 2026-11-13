// Fork (campaign lists, integrations CAMPAIGN-LISTS-SPEC I7). Run: node --test src/listSelection.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { nextListSelection } from './listSelection.mjs'; // eslint-disable-line import/extensions

const A = { id: 1, name: 'A' };
const B = { id: 2, name: 'B' };

test('I7: single mode replaces the selection; otherwise a pick appends', () => {
  assert.deepEqual(nextListSelection([A], B, true), [B]);
  assert.deepEqual(nextListSelection([], B, true), [B]);
  assert.deepEqual(nextListSelection(undefined, B, true), [B]);
  assert.deepEqual(nextListSelection([A], B, false), [A, B]);
  assert.deepEqual(nextListSelection([A], B, undefined), [A, B]);
  assert.deepEqual(nextListSelection(null, A, false), [A]);
  // The input array is not mutated.
  const sel = [A];
  nextListSelection(sel, B, false);
  assert.deepEqual(sel, [A]);
});

// I7 wiring (source reads): ListSelector picks through the helper; only the campaign form passes
// `single` -- the Subscriber form, the bulk list and Import do not.
test('I7: ListSelector uses the helper; only Campaign.vue is single', () => {
  const sel = readFileSync(new URL('./components/ListSelector.vue', import.meta.url), 'utf8');
  assert.match(sel, /nextListSelection\(this\.selectedItems, l, this\.\$props\.single\)/);
  const tag = (file) => {
    const src = readFileSync(new URL(`./views/${file}`, import.meta.url), 'utf8');
    const m = src.match(/<list-selector[\s\S]*?\/>/);
    return m ? m[0] : '';
  };
  assert.match(tag('Campaign.vue'), /\ssingle\s/);
  ['SubscriberForm.vue', 'SubscriberBulkList.vue', 'Import.vue'].forEach((f) => {
    assert.ok(tag(f), f);
    assert.doesNotMatch(tag(f), /\ssingle[\s/>]/, f);
  });
});
