// Fork (multi-brand overlap, integrations BRAND-OVERLAP-SPEC I1/I7). Run:
// node --test src/sesOverlap.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { overlapView, pct } from './sesOverlap.mjs'; // eslint-disable-line import/extensions

const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));

const detail = (over = {}) => ({
  active: 28068,
  multi: 1390,
  three: 253,
  share: 1390 / 28068,
  shareThree: 253 / 28068,
  thresholds: { warn: 0.12, issues: 0.15 },
  floor: 1000,
  ...over,
});

test('I7: every number comes from the stored detail', () => {
  const v = overlapView(detail());
  assert.deepEqual(v, {
    share: '5.0%',
    multi: '1,390',
    active: '28,068',
    shareThree: '0.9%',
    three: '253',
    warn: '12%',
    issues: '15%',
    fill: `${((1390 / 28068) / 0.15) * 100}%`,
    mark: '80%',
  });
});

test('I1: the thresholds line and the bar use the DOCUMENT\'s thresholds, not any constant', () => {
  const v = overlapView(detail({ share: 0.1, thresholds: { warn: 0.2, issues: 0.4 } }));
  assert.equal(v.warn, '20%');
  assert.equal(v.issues, '40%');
  assert.equal(v.fill, '25%');
  assert.equal(v.mark, '50%');
  const odd = overlapView(detail({ thresholds: { warn: 0.075, issues: 0.125 } }));
  assert.equal(odd.warn, '7.5%');
  assert.equal(odd.issues, '12.5%');
});

test('the bar is clamped to the issues line; absent thresholds → empty bar and dashes', () => {
  assert.equal(overlapView(detail({ share: 0.6 })).fill, '100%');
  const bare = overlapView(detail({ thresholds: undefined }));
  assert.equal(bare.fill, '0%');
  assert.equal(bare.mark, '0%');
  assert.equal(bare.warn, '—');
  assert.equal(bare.issues, '—');
});

test('a failed read (detail is a reason) or no detail → null', () => {
  assert.equal(overlapView({ reason: 'overlap read failed: boom' }), null);
  assert.equal(overlapView(null), null);
  assert.equal(overlapView('a string'), null);
});

test('below the floor the counts still show; a null share renders a dash', () => {
  const v = overlapView(detail({
    active: 0, multi: 0, three: 0, share: null, shareThree: null,
  }));
  assert.equal(v.share, '—');
  assert.equal(v.shareThree, '—');
  assert.equal(v.fill, '0%');
});

test('pct', () => {
  assert.equal(pct(0.0495), '5.0%');
  assert.equal(pct(0.12, 2, true), '12%');
  assert.equal(pct(undefined), null);
});

test('the three i18n strings exist with the placeholders the Vue fills', () => {
  assert.equal(en['dashboard.ses.overlap'], 'Multi-brand overlap');
  ['{share}', '{multi}', '{active}', '{shareThree}', '{three}'].forEach((p) => {
    assert.ok(en['dashboard.ses.overlapLine'].includes(p), p);
  });
  ['{warn}', '{issues}'].forEach((p) => assert.ok(en['dashboard.ses.overlapThresholds'].includes(p), p));
});
