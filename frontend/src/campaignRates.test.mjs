// Fork (campaign list rates, integrations CAMPAIGN-RATES-SPEC I3). Run: npm run test:rates
// (node --test src/campaignRates.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { rateCell, rateTipKey } from './campaignRates.mjs'; // eslint-disable-line import/extensions

test('sent = 0: no rate, the count stands alone', () => {
  assert.deepEqual(rateCell(12, 0, 1), { pct: null, count: '12' });
});

test('count = 0 with sent > 0 is 0.0%', () => {
  assert.deepEqual(rateCell(0, 500, 1), { pct: '0.0%', count: '0' });
});

test('digits are honoured (1 and 2)', () => {
  assert.equal(rateCell(354, 1389, 1).pct, '25.5%');
  assert.equal(rateCell(2, 1389, 2).pct, '0.14%');
  assert.equal(rateCell(1, 3, 2).pct, '33.33%');
});

test('a ratio above 1 is not capped', () => {
  assert.equal(rateCell(31, 24, 1).pct, '129.2%');
});

test('non-finite sent yields no rate', () => {
  [undefined, null, NaN, Infinity, '100'].forEach((sent) => {
    assert.equal(rateCell(5, sent, 1).pct, null, String(sent));
  });
});

test('count formatting: default en-US separator, explicit formatter honoured', () => {
  assert.equal(rateCell(1234567, 2000000, 1).count, '1,234,567');
  assert.deepEqual(rateCell(1234, 10000, 1, (n) => `<${n}>`), { pct: '12.3%', count: '<1234>' });
});

test('rateTipKey: mode and sent select the key', () => {
  assert.equal(rateTipKey(true, 10), 'campaigns.rateHelpUnique');
  assert.equal(rateTipKey(false, 10), 'campaigns.rateHelpTotal');
  assert.equal(rateTipKey(undefined, 10), 'campaigns.rateHelpTotal');
  assert.equal(rateTipKey(true, 0), null);
  assert.equal(rateTipKey(false, 0), null);
  assert.equal(rateTipKey(true, undefined), null);
});
