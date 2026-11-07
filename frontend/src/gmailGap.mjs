// Fork (integrations GMAIL-GAP-SPEC D2-D5). The Gmail gap, computed from the server's counts:
// Gmail open rate (gmail_proxy / gmail -- Gmail recipients who opened through Gmail's image proxy)
// divided by Yahoo open rate (yahoo_proxy / yahoo -- the control). The server returns the four
// counts (NULL for a journey) and the stored line (`gap_thresholds`, the ses document's
// campaignThresholds.gmailGap); every rate, the gap, the pooling and the missing-value reason are
// here. The threshold and floor VALUES are never here (DASHBOARDS-POLISH D1): with no stored line
// the floor is unknown and the gap is shown uncolored whenever it is defined. Shared by the
// Campaign Matrix (campaignMatrix.mjs) and the Campaign pane's Gmail gap block. Pure: no Vue, no
// DOM; tested by gmailGap.test.mjs (yarn test).
import { gapTierOf, parseGapThresholds, tierClass } from './campaignTiers.mjs'; // eslint-disable-line import/extensions

export const GAP_DIGITS = 2;

export const COUNT_KEYS = Object.freeze(['gmail', 'gmail_proxy', 'yahoo', 'yahoo_proxy']);

// Why a gap is undefined (the hover names it), in the order they are tested.
export const REASONS = Object.freeze({
  journey: 'journey',
  floor: 'floor',
  noYahoo: 'noYahoo',
  noYahooOpens: 'noYahooOpens',
  noGmail: 'noGmail',
  // The Campaign pane only: the read failed or returned no rows (gapSummary).
  unavailable: 'unavailable',
});

const count = (v) => (typeof v === 'number' && Number.isFinite(v) && v >= 0 ? v : null);

// gapCounts is a row's four counts, or null when any is missing (a journey: the server's NULLs).
export function gapCounts(r) {
  if (!r || typeof r !== 'object') {
    return null;
  }
  const out = {};
  for (let i = 0; i < COUNT_KEYS.length; i += 1) {
    const v = count(r[COUNT_KEYS[i]]);
    if (v === null) {
      return null;
    }
    out[COUNT_KEYS[i]] = v;
  }
  return out;
}

// poolCounts sums the rows' counts, skipping rows without them (journeys); null when none has them
// -- several campaigns read as one audience (D3 totals, D5 selection).
export function poolCounts(rows) {
  const valued = (Array.isArray(rows) ? rows : []).map(gapCounts).filter(Boolean);
  if (!valued.length) {
    return null;
  }
  return valued.reduce((t, c) => {
    const out = {};
    COUNT_KEYS.forEach((k) => { out[k] = t[k] + c[k]; });
    return out;
  });
}

// gapOf computes { gmailRate, yahooRate, gap, reason } from counts (gapCounts' shape, or null) and
// the stored line. A rate is null when its denominator is 0. The gap is null -- with the reason --
// for a journey (no counts), a Yahoo base under the stored floor, no Yahoo recipients, no Yahoo
// opens, or no Gmail recipients (D3); otherwise reason is null.
export function gapOf(counts, thresholds) {
  if (!counts) {
    return {
      gmailRate: null, yahooRate: null, gap: null, reason: REASONS.journey,
    };
  }
  const t = parseGapThresholds(thresholds);
  const gmailRate = counts.gmail > 0 ? counts.gmail_proxy / counts.gmail : null;
  const yahooRate = counts.yahoo > 0 ? counts.yahoo_proxy / counts.yahoo : null;
  let reason = null;
  if (t && counts.yahoo < t.minControl) {
    reason = REASONS.floor;
  } else if (counts.yahoo === 0) {
    reason = REASONS.noYahoo;
  } else if (counts.yahoo_proxy === 0) {
    reason = REASONS.noYahooOpens;
  } else if (counts.gmail === 0) {
    reason = REASONS.noGmail;
  }
  return {
    gmailRate, yahooRate, gap: reason ? null : gmailRate / yahooRate, reason,
  };
}

// gapText is the gap to GAP_DIGITS decimals, or the missing mark.
export function gapText(gap, missing = '—') {
  return Number.isFinite(gap) ? gap.toFixed(GAP_DIGITS) : missing;
}

// ratePct is an open rate as a percentage to `digits` decimals (the Campaign pane's block uses 1, as
// scripts/gmail-gap.ts prints), or the missing mark when the rate is undefined.
export function ratePct(rate, digits = 1, missing = '—') {
  return Number.isFinite(rate) ? `${(rate * 100).toFixed(digits)}%` : missing;
}

// gapClass is the Gap cell's CSS class: the D4 tier of the DISPLAYED gap against the stored line
// ('' with no line, no gap, or above warn).
export function gapClass(gap, thresholds) {
  return tierClass(gapTierOf(gap, thresholds, GAP_DIGITS));
}

// gapReasonTip is the i18n { key, params } naming why the gap is '—', or the rule when it is shown.
// fmt formats the {floor} number (the caller passes its number formatter; default String).
export function gapReasonTip(reason, thresholds, fmt = String) {
  const t = parseGapThresholds(thresholds);
  switch (reason) {
    case REASONS.journey:
      return { key: 'analytics.matrix.gapJourney', params: {} };
    case REASONS.floor:
      return { key: 'analytics.matrix.gapFloor', params: { floor: t ? fmt(t.minControl) : '' } };
    case REASONS.noYahoo:
      return { key: 'analytics.matrix.gapNoYahoo', params: {} };
    case REASONS.noYahooOpens:
      return { key: 'analytics.matrix.gapNoYahooOpens', params: {} };
    case REASONS.noGmail:
      return { key: 'analytics.matrix.gapNoGmail', params: {} };
    default:
      return { key: 'analytics.matrix.gapHelp', params: {} };
  }
}

// gapHint is the Campaign pane's reading line as i18n { key, params }: the stored red line
// interpolated (`below {red} is bulking`), or -- with no stored line -- the wording without a number.
// The fork holds no threshold value.
export function gapHint(thresholds) {
  const t = parseGapThresholds(thresholds);
  return t
    ? { key: 'analytics.gmailGapHint', params: { red: t.red } }
    : { key: 'analytics.gmailGapHintNoLine', params: {} };
}

// gapSummary is the Campaign pane's block (D5): the selection's pooled counts, rates and gap, the
// gap's text and class and its hover. An all-journey selection is '—' with the journey reason. A
// read that failed (`failed`) or came back with no rows is '—' with the NEUTRAL unavailable hover,
// never the journey wording (reason 'unavailable'). fmt formats the hover's {floor}.
export function gapSummary(rows, thresholds, { failed = false, fmt = String } = {}) {
  if (failed || !Array.isArray(rows) || rows.length === 0) {
    return {
      counts: null,
      gmailRate: null,
      yahooRate: null,
      gap: null,
      reason: REASONS.unavailable,
      text: gapText(null),
      cls: '',
      tip: { key: 'analytics.gmailGapUnavailable', params: {} },
    };
  }
  const counts = poolCounts(rows);
  const g = gapOf(counts, thresholds);
  return {
    counts,
    ...g,
    text: gapText(g.gap),
    cls: gapClass(g.gap, thresholds),
    tip: gapReasonTip(g.reason, thresholds, fmt),
  };
}
