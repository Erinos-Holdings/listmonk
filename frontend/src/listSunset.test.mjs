// Fork (sunset v2) -- integrations SUNSET-V2-SPEC J7 (the fork half): the Lists page renders the
// stored countdown only when it is non-null -- queue 1 "oldest waiting N d", queue 2 "oldest
// archives in N d" -- and nothing for a list without one.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { listSunsetItem, withSunsetItem } from './listSunset.mjs'; // eslint-disable-line import/extensions

test('queue 1: oldest waiting', () => {
  assert.equal(listSunsetItem({ oldestDays: 12, archiveInDays: null, asOf: '2026-11-01T15:00:00Z' }), 'oldest waiting 12 d');
  assert.equal(listSunsetItem({ oldestDays: 0, asOf: '2026-11-01T15:00:00Z' }), 'oldest waiting 0 d');
});

test('queue 2: oldest archives in', () => {
  assert.equal(listSunsetItem({ oldestDays: 20, archiveInDays: 65, asOf: '2026-11-01T15:00:00Z' }), 'oldest archives in 65 d');
  assert.equal(listSunsetItem({ oldestDays: 20, archiveInDays: 0, asOf: '2026-11-01T15:00:00Z' }), 'oldest archives in 0 d');
});

test('nothing for a list without a sunset (null, absent, malformed)', () => {
  [null, undefined, '', [], {}, { oldestDays: 'x' }, { oldestDays: 1.5 }].forEach((v) => {
    assert.equal(listSunsetItem(v), '', JSON.stringify(v));
  });
});

test('the sub-line gains the item with the page separator, or stays as it was', () => {
  assert.equal(withSunsetItem('#41 · Shala', { oldestDays: 3, archiveInDays: 200 }), '#41 · Shala · oldest archives in 200 d');
  assert.equal(withSunsetItem('#16 · Shala', null), '#16 · Shala');
  assert.equal(withSunsetItem('#16', undefined), '#16');
});
