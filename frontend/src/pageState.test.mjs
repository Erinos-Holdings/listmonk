// Fork (page state) -- integrations listmonk/PAGE-STATE-SPEC I1, I5 (the shared validators) and I6.
// Run: node --test src/pageState.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  PAGE_STATE_PREFIX, RESTORE_PARAM, enumOf, isOwnRestore, keptListId, loadPageState, pageNumberOf, routeHasAny,
  savePageState, sessionStore, textOf,
} from './pageState.mjs'; // eslint-disable-line import/extensions

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');

const memStore = (init = {}) => {
  const m = { ...init };
  return {
    m,
    getItem: (k) => (Object.prototype.hasOwnProperty.call(m, k) ? m[k] : null),
    setItem: (k, v) => { m[k] = String(v); },
  };
};
const throwing = {
  getItem() { throw new Error('denied'); },
  setItem() { throw new Error('denied'); },
};

test('I1: load reads null for a missing store, a throwing accessor, bad JSON and non-objects', () => {
  assert.equal(loadPageState(null, 'campaigns'), null);
  assert.equal(loadPageState(undefined, 'campaigns'), null);
  assert.equal(loadPageState(throwing, 'campaigns'), null);
  assert.equal(loadPageState(memStore(), 'campaigns'), null);
  ['{bad', 'null', '[1,2]', '7', '"text"', 'true', ''].forEach((raw) => {
    assert.equal(loadPageState(memStore({ 'page.campaigns': raw }), 'campaigns'), null, raw);
  });
  assert.deepEqual(loadPageState(memStore({ 'page.campaigns': '{"query":"bfcm"}' }), 'campaigns'), { query: 'bfcm' });
});

test('I1: save never throws, writes one page.<name> JSON key, and ignores non-objects', () => {
  assert.doesNotThrow(() => savePageState(null, 'x', { a: 1 }));
  assert.doesNotThrow(() => savePageState(throwing, 'x', { a: 1 }));
  const cyclic = {};
  cyclic.self = cyclic;
  const s = memStore();
  assert.doesNotThrow(() => savePageState(s, 'x', cyclic));
  assert.doesNotThrow(() => savePageState(s, 'x', null));
  assert.doesNotThrow(() => savePageState(s, 'x', [1]));
  assert.doesNotThrow(() => savePageState(s, 'x', 3));
  assert.deepEqual(s.m, {});
  savePageState(s, 'lists', { query: 'a', page: 2 });
  assert.equal(PAGE_STATE_PREFIX, 'page.');
  assert.deepEqual(Object.keys(s.m), ['page.lists']);
  assert.deepEqual(loadPageState(s, 'lists'), { query: 'a', page: 2 });
});

test('I1: sessionStore is brandMatrix.mjs::sessionStore (null when absent or throwing)', () => {
  assert.equal(sessionStore(undefined), null);
  assert.equal(sessionStore({}), null);
  const w = {};
  Object.defineProperty(w, 'sessionStorage', { get() { throw new Error('blocked'); } });
  assert.equal(sessionStore(w), null);
  const st = memStore();
  assert.equal(sessionStore({ sessionStorage: st }), st);
});

test('I5: page numbers are positive integers, else 1', () => {
  [0, -2, 'x', 1.5, null, undefined, '', '0', '-1', '1.5', NaN, Infinity, {}].forEach((v) => {
    assert.equal(pageNumberOf(v), 1, String(v));
  });
  assert.equal(pageNumberOf(3), 3);
  assert.equal(pageNumberOf('4'), 4);
});

test('I5: enums, text and list ids validate', () => {
  assert.equal(enumOf('asc', ['asc', 'desc'], 'desc'), 'asc');
  assert.equal(enumOf('sideways', ['asc', 'desc'], 'desc'), 'desc');
  assert.equal(textOf(5), '');
  assert.equal(textOf('bfcm'), 'bfcm');
  assert.equal(keptListId(5, [4, 5]), 5);
  assert.equal(keptListId(6, [4, 5]), null);
  assert.equal(keptListId(5, null), null);
  assert.equal(keptListId('5', [5]), null);
  assert.equal(keptListId(0, [0]), null);
});

test('D5/D8: routeHasAny and the own-restore marker', () => {
  assert.equal(routeHasAny({}, ['status']), false);
  assert.equal(routeHasAny(null, ['status']), false);
  assert.equal(routeHasAny({ status: 'archived' }, ['status']), true);
  assert.equal(routeHasAny({ status: '' }, ['status']), true);
  assert.equal(routeHasAny({ lang: 'fr' }, ['status']), false);
  assert.equal(isOwnRestore({ [RESTORE_PARAM]: '1' }), true);
  assert.equal(isOwnRestore({}), false);
  assert.equal(isOwnRestore(undefined), false);
});

test('I6: Import keeps nothing -- Import.vue never references pageState', () => {
  const imp = read('./views/Import.vue');
  assert.doesNotMatch(imp, /pageState/);
  assert.doesNotMatch(imp, /PageState/);
});

test('Templates is unchanged: Templates.vue never references pageState (spec §3)', () => {
  assert.doesNotMatch(read('./views/Templates.vue'), /pageState/);
});
