import test from 'node:test';
import assert from 'node:assert/strict';
import { formatMountain } from './mountainTime.mjs'; // eslint-disable-line import/extensions

test('a UTC instant renders as Mountain wall time with no zone text', () => {
  // 16:36:30Z on 2026-10-26 is 10:36 AM MDT.
  assert.equal(formatMountain('2026-10-26T16:36:30Z'), 'Oct 26, 2026, 10:36 AM');
  // After the DST fall-back (2026-11-01) the same UTC hour is 9:36 AM MST.
  assert.equal(formatMountain('2026-11-03T16:36:30Z'), 'Nov 3, 2026, 9:36 AM');
});

test('no Z, no "UTC" in the output, ever', () => {
  const out = formatMountain('2026-10-26T16:36:30Z');
  assert.ok(!out.includes('Z'));
  assert.ok(!out.toUpperCase().includes('UTC'));
});

test('falsy or unparsable values yield the empty string', () => {
  assert.equal(formatMountain(''), '');
  assert.equal(formatMountain(null), '');
  assert.equal(formatMountain(undefined), '');
  assert.equal(formatMountain('not-a-date'), '');
});
