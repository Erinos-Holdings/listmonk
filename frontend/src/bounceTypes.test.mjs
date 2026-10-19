// Fork (bounce types, integrations BOUNCE-TYPES-SPEC I3). Run: npm test
// (node --test src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { BOUNCE_TYPE_ORDER, bounceSplitText } from './bounceTypes.mjs'; // eslint-disable-line import/extensions

const labels = { hard: 'hard', soft: 'soft', complaint: 'complaints' };

test('I3 non-zero types only, in the order hard, soft, complaint', () => {
  assert.deepEqual(BOUNCE_TYPE_ORDER, ['hard', 'soft', 'complaint']);
  assert.equal(bounceSplitText([{ type: 'hard', count: 3 }, { type: 'soft', count: 1 }], labels), '3 hard, 1 soft');
  // Response order does not matter; a zero type is left out.
  assert.equal(
    bounceSplitText([{ type: 'complaint', count: 2 }, { type: 'soft', count: 0 }, { type: 'hard', count: 5 }], labels),
    '5 hard, 2 complaints',
  );
  assert.equal(bounceSplitText([{ type: 'complaint', count: 1 }], labels), '1 complaints');
});

test('I3 counts are locale-formatted by the given formatter', () => {
  const de = new Intl.NumberFormat('de-DE').format;
  assert.equal(bounceSplitText([{ type: 'hard', count: 1234 }, { type: 'soft', count: 5678 }], labels, de), '1.234 hard, 5.678 soft');
  // The default formatter is en-US.
  assert.equal(bounceSplitText([{ type: 'hard', count: 1234 }], labels), '1,234 hard');
  // Labels come from the caller (translations).
  assert.equal(bounceSplitText([{ type: 'soft', count: 2 }], { hard: 'h', soft: 'weich', complaint: 'c' }), '2 weich');
});

test('I3 empty string when every type is zero or the rows are absent or malformed', () => {
  assert.equal(bounceSplitText([{ type: 'hard', count: 0 }, { type: 'soft', count: 0 }], labels), '');
  assert.equal(bounceSplitText([], labels), '');
  assert.equal(bounceSplitText(undefined, labels), '');
  assert.equal(bounceSplitText(null, labels), '');
  assert.equal(bounceSplitText({ type: 'hard', count: 1 }, labels), '');
  assert.equal(bounceSplitText([null, 'hard', 3, { type: 'bogus', count: 4 }], labels), '');
  assert.equal(
    bounceSplitText([{ type: 'hard', count: '3' }, { type: 'soft', count: NaN }, { type: 'complaint', count: -1 }, { type: 'hard' }], labels),
    '',
  );
  // A malformed row beside a good one: only the good one shows.
  assert.equal(bounceSplitText([{ type: 'hard', count: Infinity }, { type: 'soft', count: 2 }], labels), '2 soft');
});
