// Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC I6). Run:
// node --test src/campaignTiers.test.mjs. No dependency: Node's built-in runner. The threshold
// values below are FIXTURES (the document's shape), not the live values -- those live only in the
// integrations lib/ses-health.ts.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  TIER_AMBER, TIER_RED, gapTierOf, parseCampaignThresholds, parseGapThresholds, thresholdsFromDocs, tierClass, tierOf,
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

// Fork (integrations GMAIL-GAP-SPEC I3) -- the gap line: optional in the document, inverse direction,
// compared on the displayed two-decimal value; no line -> no color. FIXTURE values.
const G = { warn: 0.8, red: 0.4, minControl: 150 };

test('GMAIL-GAP I3: gmailGap is optional -- with it parsed, without it the bounce tiers still work', () => {
  assert.deepEqual(parseCampaignThresholds({ ...T, gmailGap: G }), { ...T, gmailGap: G });
  assert.deepEqual(parseCampaignThresholds(T), T);
  // A malformed gmailGap is dropped, never fatal to the bounce tiers.
  [{ warn: 0.4, red: 0.8, minControl: 150 }, { warn: 0.8, red: 0.4 }, { warn: '0.8', red: 0.4, minControl: 1 },
    { warn: 0.8, red: 0, minControl: 1 }, { warn: 0.8, red: 0.4, minControl: -1 }, 'x', null].forEach((bad) => {
    assert.deepEqual(parseCampaignThresholds({ ...T, gmailGap: bad }), T, JSON.stringify(bad));
    assert.equal(parseGapThresholds(bad), null, JSON.stringify(bad));
  });
  // A gmailGap alone is not a bounce threshold set.
  assert.equal(parseCampaignThresholds({ gmailGap: G }), null);
  assert.deepEqual(thresholdsFromDocs([{ campaignThresholds: { ...T, gmailGap: G } }]), { ...T, gmailGap: G });
});

test('GMAIL-GAP I3: red at or below red, amber at or below warn, none above -- inverse of the bounces', () => {
  assert.equal(gapTierOf(0.1, G), TIER_RED);
  assert.equal(gapTierOf(0.4, G), TIER_RED);
  assert.equal(gapTierOf(0.41, G), TIER_AMBER);
  assert.equal(gapTierOf(0.8, G), TIER_AMBER);
  assert.equal(gapTierOf(0.81, G), null);
  assert.equal(gapTierOf(3.5, G), null);
  // On the displayed value: 0.4049 reads 0.40 (red), 0.8049 reads 0.80 (amber), 0.8051 reads 0.81.
  assert.equal(gapTierOf(0.4049, G), TIER_RED);
  assert.equal(gapTierOf(0.8049, G), TIER_AMBER);
  assert.equal(gapTierOf(0.8051, G), null);
  // No (valid) line or no gap -> no color.
  assert.equal(gapTierOf(0.1, null), null);
  assert.equal(gapTierOf(0.1, { warn: 0.8 }), null);
  assert.equal(gapTierOf(null, G), null);
  assert.equal(gapTierOf(Number.NaN, G), null);
});

test('tier classes; none below warn keeps today\'s styling', () => {
  assert.equal(tierClass(TIER_RED), 'has-text-danger');
  assert.equal(tierClass(TIER_AMBER), 'has-text-warning-dark');
  assert.equal(tierClass(null), '');
});
