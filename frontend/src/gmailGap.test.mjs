// Fork (integrations GMAIL-GAP-SPEC I2/I4) -- the Gmail gap's arithmetic from the server's counts:
// the rates, the gap, every '—' reason, the pooling of a selection (journeys skipped) and the
// Campaign pane's summary. Run: node --test src/gmailGap.test.mjs. The threshold numbers here are
// FIXTURES (the stored line's shape), not the live values -- those live only in the integrations
// lib/ses-health.ts.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  REASONS, gapClass, gapCounts, gapHint, gapOf, gapReasonTip, gapSummary, gapText, poolCounts, ratePct,
} from './gmailGap.mjs'; // eslint-disable-line import/extensions

const T = { warn: 0.8, red: 0.4, minControl: 150 };

const row = (gmail, gp, yahoo, yp, extra = {}) => ({
  gmail, gmail_proxy: gp, yahoo, yahoo_proxy: yp, ...extra,
});

test('gapCounts: the four counts, or null when any is missing (a journey)', () => {
  assert.deepEqual(gapCounts(row(10, 1, 300, 30, { campaign_id: 4 })), row(10, 1, 300, 30));
  assert.equal(gapCounts(row(null, null, null, null)), null);
  assert.equal(gapCounts(row(10, 1, 300, undefined)), null);
  assert.equal(gapCounts(null), null);
});

test('gapOf: rates over each provider, gap = Gmail open rate / Yahoo open rate', () => {
  const g = gapOf(gapCounts(row(1000, 100, 400, 40)), T);
  assert.equal(g.gmailRate, 0.1);
  assert.equal(g.yahooRate, 0.1);
  assert.equal(g.gap, 1);
  assert.equal(g.reason, null);
  // c217's shape: almost no Gmail proxy opens -> a defined, near-zero gap.
  const bulked = gapOf(gapCounts(row(20446, 6, 5000, 480)), T);
  assert.equal(gapText(bulked.gap), '0.00');
});

test('gapOf: every undefined case has its reason', () => {
  assert.equal(gapOf(null, T).reason, REASONS.journey);
  assert.equal(gapOf(gapCounts(row(1000, 100, 149, 20)), T).reason, REASONS.floor);
  assert.equal(gapOf(gapCounts(row(1000, 100, 150, 20)), T).reason, null, 'at the floor is enough');
  assert.equal(gapOf(gapCounts(row(1000, 100, 400, 0)), T).reason, REASONS.noYahooOpens);
  assert.equal(gapOf(gapCounts(row(0, 0, 400, 40)), T).reason, REASONS.noGmail);
  // With no stored line the floor is unknown: a small Yahoo base still gives a (uncolored) gap.
  const noLine = gapOf(gapCounts(row(1000, 100, 20, 2)), null);
  assert.equal(noLine.reason, null);
  assert.equal(noLine.gap, 1);
  assert.equal(gapOf(gapCounts(row(1000, 100, 0, 0)), null).reason, REASONS.noYahoo);
  [REASONS.journey, REASONS.floor, REASONS.noYahooOpens, REASONS.noGmail, REASONS.noYahoo].forEach((r) => {
    assert.ok(gapReasonTip(r, T).key.startsWith('analytics.matrix.gap'), r);
  });
  assert.deepEqual(gapReasonTip(REASONS.floor, T), { key: 'analytics.matrix.gapFloor', params: { floor: '150' } });
  // The caller's formatter formats {floor}, once, here (no second patch at the call sites).
  assert.deepEqual(gapReasonTip(REASONS.floor, { ...T, minControl: 1500 }, (n) => `#${n}`).params, { floor: '#1500' });
  assert.equal(gapReasonTip(REASONS.floor, null, (n) => `#${n}`).params.floor, '');
  const s = gapSummary([row(1000, 100, 149, 20)], T, { fmt: (n) => `#${n}` });
  assert.deepEqual(s.tip, { key: 'analytics.matrix.gapFloor', params: { floor: '#150' } });
  assert.equal(gapReasonTip(null, T).key, 'analytics.matrix.gapHelp');
});

test('poolCounts: sums the rows carrying counts, skips journeys; null when none does', () => {
  assert.deepEqual(poolCounts([row(10, 1, 300, 30), row(null, null, null, null), row(5, 4, 100, 10)]), row(15, 5, 400, 40));
  assert.equal(poolCounts([row(null, null, null, null)]), null);
  assert.equal(poolCounts([]), null);
  assert.equal(poolCounts(undefined), null);
});

test('gapSummary: the pooled selection reads as one audience; all journeys read —', () => {
  const s = gapSummary([row(1000, 23, 500, 108), row(500, 2, 100, 10)], T);
  assert.deepEqual(s.counts, row(1500, 25, 600, 118));
  assert.equal(s.text, gapText((25 / 1500) / (118 / 600)));
  assert.equal(s.text, '0.08');
  assert.equal(s.cls, 'has-text-danger');
  assert.equal(s.tip.key, 'analytics.matrix.gapHelp');
  const j = gapSummary([row(null, null, null, null)], T);
  assert.equal(j.text, '—');
  assert.equal(j.counts, null);
  assert.equal(j.cls, '');
  assert.equal(j.tip.key, 'analytics.matrix.gapJourney');
});

test('gapSummary: a failed or empty read is — with the NEUTRAL unavailable hover, never the journey one', () => {
  [
    gapSummary([row(1000, 100, 400, 40)], T, { failed: true }),
    gapSummary([], T),
    gapSummary(null, null),
  ].forEach((s) => {
    assert.equal(s.text, '—');
    assert.equal(s.counts, null);
    assert.equal(s.cls, '');
    assert.equal(s.reason, REASONS.unavailable);
    assert.deepEqual(s.tip, { key: 'analytics.gmailGapUnavailable', params: {} });
  });
  // A good read is not unavailable by default.
  assert.equal(gapSummary([row(1000, 100, 400, 40)], T).reason, null);
});

test('gapHint: the stored red line interpolated; the no-number wording without a line', () => {
  assert.deepEqual(gapHint(T), { key: 'analytics.gmailGapHint', params: { red: 0.4 } });
  assert.deepEqual(gapHint(null), { key: 'analytics.gmailGapHintNoLine', params: {} });
  assert.deepEqual(gapHint({ warn: 0.8 }), { key: 'analytics.gmailGapHintNoLine', params: {} });
  // The i18n strings: the hint carries the {red} placeholder and no number; the no-line one no number.
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.ok(en['analytics.gmailGapHint'].includes('{red}'));
  ['analytics.gmailGapHint', 'analytics.gmailGapHintNoLine'].forEach((k) => {
    assert.ok(en[k], k);
    assert.ok(!/\d\.\d/.test(en[k]), `${k} holds no threshold literal`);
  });
  assert.equal(en['analytics.gmailGapUnavailable'], 'The Gmail gap could not be read.');
});

test('gapClass: red at or below red, amber at or below warn, on the displayed value; none without a line', () => {
  assert.equal(gapClass(0.4, T), 'has-text-danger');
  assert.equal(gapClass(0.404, T), 'has-text-danger', 'displays 0.40');
  assert.equal(gapClass(0.405, T), 'has-text-warning-dark', 'displays 0.41 (0.405 is stored above the half)');
  assert.equal(gapClass(0.8, T), 'has-text-warning-dark');
  assert.equal(gapClass(0.8049, T), 'has-text-warning-dark', 'displays 0.80');
  assert.equal(gapClass(0.81, T), '');
  assert.equal(gapClass(3.5, T), '');
  assert.equal(gapClass(0.1, null), '');
  assert.equal(gapClass(null, T), '');
});

test('ratePct and gapText', () => {
  assert.equal(ratePct(0.0234), '2.3%');
  assert.equal(ratePct(null), '—');
  assert.equal(gapText(0.1), '0.10');
  assert.equal(gapText(null), '—');
  assert.equal(gapText(Number.NaN, 'x'), 'x');
});
