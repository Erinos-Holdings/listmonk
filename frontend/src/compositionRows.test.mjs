// Fork (blocklist composition, integrations BLOCKLIST-COMPOSITION-SPEC I5). Run:
// node --test src/compositionRows.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  BUCKET_IDS, compositionRows, compositionState, share,
} from './compositionRows.mjs'; // eslint-disable-line import/extensions

const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));

const bucket = (key, count, domains = [], codes = []) => ({
  key, count, domains, codes,
});

const doc = {
  asOf: '2026-10-26',
  total: 8,
  buckets: [
    bucket('kickbox', 2),
    bucket('hard:block', 3, [{ domain: 'yahoo.com', n: 3 }], [{ code: '5.7.1', n: 3 }]),
    bucket('hard:other', 1, [{ domain: 'aol.com', n: 1 }], [{ code: '5.3.0', n: 1 }]),
    bucket('manual', 0),
    bucket('hard:future-class', 2),
  ],
};

test('share = count / total, one decimal; total 0 or not finite → null', () => {
  assert.equal(share(3, 8), '37.5%');
  assert.equal(share(1, 3), '33.3%');
  assert.equal(share(0, 8), '0.0%');
  [0, undefined, null, NaN, '8'].forEach((t) => assert.equal(share(1, t), null, String(t)));
});

test('rows in stored order; share of the stored total; counts formatted', () => {
  const rows = compositionRows(doc, (n) => `<${n}>`);
  assert.deepEqual(rows.map((r) => r.key), ['kickbox', 'hard:block', 'hard:other', 'manual', 'hard:future-class']);
  assert.deepEqual(rows.map((r) => r.share), ['25.0%', '37.5%', '12.5%', '0.0%', '25.0%']);
  assert.equal(rows[1].countText, '<3>');
  assert.deepEqual(rows[1].domains, [{ domain: 'yahoo.com', n: 3, nText: '<3>' }]);
  assert.deepEqual(rows[1].codes, [{ code: '5.7.1', n: 3, nText: '<3>' }]);
  assert.equal(rows[1].hasDetail, true);
  assert.equal(rows[0].hasDetail, false);
});

test('absent field (or not an object) → no rows; skipped → no rows, state skipped', () => {
  [undefined, null, 'x', 3, []].forEach((c) => {
    assert.deepEqual(compositionRows(c), [], String(c));
    assert.equal(compositionState(c), 'absent');
  });
  const skipped = {
    asOf: 'd', total: 60000, buckets: [], skipped: 'volume',
  };
  assert.equal(compositionState(skipped), 'skipped');
  assert.deepEqual(compositionRows(skipped), []);
  assert.equal(compositionState(doc), 'ok');
});

test('an unknown bucket key renders its raw key (fail-visible, never dropped)', () => {
  const row = compositionRows(doc).find((r) => r.key === 'hard:future-class');
  assert.ok(row);
  assert.equal(row.labelKey, null);
  assert.equal(row.helpKey, null);
  assert.equal(row.label, 'hard:future-class');
});

test('every known bucket has a label and a help key in en.json; Chain B vocabulary', () => {
  Object.values(BUCKET_IDS).forEach((id) => {
    assert.ok(en[`composition.bucket.${id}`], id);
    assert.ok(en[`composition.bucket.${id}Help`], `${id}Help`);
  });
  assert.equal(en['composition.bucket.hardBlock'], 'Blocks (5.7.x)');
  assert.match(en['composition.total'], /^Blocklisted/);
  // D6: the hard:block row's sub-text names the reversal path (text only, no action).
  assert.match(en['composition.bucket.hardBlockNote'], /reverse-listmonk-suppression\.ts/);
});
