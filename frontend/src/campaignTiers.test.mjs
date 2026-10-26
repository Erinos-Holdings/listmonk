// Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC I6). Run:
// node --test src/campaignTiers.test.mjs. No dependency: Node's built-in runner. The threshold
// values below are FIXTURES (the document's shape), not the live values -- those live only in the
// integrations lib/ses-health.ts.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  TIER_AMBER, TIER_RED, parseCampaignThresholds, thresholdsFromDocs, tierClass, tierOf,
} from './campaignTiers.mjs'; // eslint-disable-line import/extensions

const T = { hardBounce: { warn: 0.02, red: 0.05 }, complaint: { warn: 0.001, red: 0.003 } };

test('amber at >= warn, red at >= red, none below warn', () => {
  assert.equal(tierOf(19, 1000, 'hardBounce', T), null);
  assert.equal(tierOf(20, 1000, 'hardBounce', T), TIER_AMBER);
  assert.equal(tierOf(49, 1000, 'hardBounce', T), TIER_AMBER);
  assert.equal(tierOf(50, 1000, 'hardBounce', T), TIER_RED);
  assert.equal(tierOf(900, 1000, 'hardBounce', T), TIER_RED);
  assert.equal(tierOf(0, 1000, 'complaint', T), null);
  assert.equal(tierOf(1, 1000, 'complaint', T), TIER_AMBER);
  assert.equal(tierOf(3, 1000, 'complaint', T), TIER_RED);
});

test('compared as displayed (2 decimals): a cell that reads 0.10% is amber', () => {
  // 0.0996% displays as 0.10%.
  assert.equal(tierOf(996, 1000000, 'complaint', T), TIER_AMBER);
  // 0.094% displays as 0.09%.
  assert.equal(tierOf(94, 100000, 'complaint', T), null);
});

test('no coloring when nothing was sent', () => {
  assert.equal(tierOf(5, 0, 'hardBounce', T), null);
  assert.equal(tierOf(5, undefined, 'hardBounce', T), null);
});

test('absent or malformed thresholds -> no coloring (fail-dark)', () => {
  [null, undefined, {}, 'x', { hardBounce: T.hardBounce },
    { hardBounce: { warn: 0.05, red: 0.02 }, complaint: T.complaint },
    { hardBounce: { warn: '0.02', red: 0.05 }, complaint: T.complaint },
    { hardBounce: { warn: 0, red: 0.05 }, complaint: T.complaint }].forEach((bad) => {
    const t = parseCampaignThresholds(bad);
    assert.equal(t, null, JSON.stringify(bad));
    assert.equal(tierOf(900, 1000, 'hardBounce', t), null);
  });
  assert.deepEqual(parseCampaignThresholds(T), T);
});

test('a failed or forbidden fetch (no docs) and docs without the field -> null; the newest valid wins', () => {
  assert.equal(thresholdsFromDocs(undefined), null);
  assert.equal(thresholdsFromDocs(null), null);
  assert.equal(thresholdsFromDocs({ message: 'forbidden' }), null);
  assert.equal(thresholdsFromDocs([]), null);
  assert.equal(thresholdsFromDocs([{ kind: 'ses' }, { kind: 'ses', campaignThresholds: 'bad' }]), null);
  const older = { hardBounce: { warn: 0.03, red: 0.06 }, complaint: T.complaint };
  assert.deepEqual(thresholdsFromDocs([{ kind: 'ses' }, { campaignThresholds: T }, { campaignThresholds: older }]), T);
});

test('tier classes; none below warn keeps today\'s styling', () => {
  assert.equal(tierClass(TIER_RED), 'has-text-danger');
  assert.equal(tierClass(TIER_AMBER), 'has-text-warning-dark');
  assert.equal(tierClass(null), '');
});
