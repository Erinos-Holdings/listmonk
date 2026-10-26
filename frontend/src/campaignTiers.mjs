// Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D9/I6). The Campaigns page's
// H. Bounces / Complaints tier coloring: amber at >= warn, red at >= red, none below warn. The
// threshold VALUES are never here -- they are read from the `ses` system_health document's
// `campaignThresholds` (written by the integrations BrandHealth Lambda from lib/ses-health.ts
// CAMPAIGN_RATE_THRESHOLDS, their one home). This module only compares a displayed rate against
// stored values (rendering, DASHBOARDS-POLISH D1). Absent, malformed or unfetchable thresholds
// mean no coloring and no error (fail-dark). Tested by campaignTiers.test.mjs
// (npm run test:campaign-tiers). No Vue, no DOM.

export const TIER_AMBER = 'amber';
export const TIER_RED = 'red';

const pair = (p) => (p && typeof p === 'object'
  && Number.isFinite(p.warn) && Number.isFinite(p.red)
  && p.warn > 0 && p.red >= p.warn ? { warn: p.warn, red: p.red } : null);

// One threshold set from a value (the document field), or null when anything is missing or out of
// shape -- both pairs are required, each 0 < warn <= red.
export function parseCampaignThresholds(v) {
  if (!v || typeof v !== 'object') {
    return null;
  }
  const hardBounce = pair(v.hardBounce);
  const complaint = pair(v.complaint);
  return hardBounce && complaint ? { hardBounce, complaint } : null;
}

// The newest document (docs are newest first, as GET /api/system/health/ses returns them) carrying
// a valid campaignThresholds; null when none does, or docs is not an array.
export function thresholdsFromDocs(docs) {
  if (!Array.isArray(docs)) {
    return null;
  }
  for (let i = 0; i < docs.length; i += 1) {
    const t = parseCampaignThresholds(docs[i] && docs[i].campaignThresholds);
    if (t) {
      return t;
    }
  }
  return null;
}

// The tier of count / sent for metric key ('hardBounce' | 'complaint'), compared as DISPLAYED --
// the percentage rounded to `digits` decimals, as the cell shows it -- so a cell never reads at a
// threshold yet uncolored. null with no thresholds, nothing sent, or below warn.
export function tierOf(count, sent, key, thresholds, digits = 2) {
  const t = thresholds && thresholds[key];
  if (!t || !Number.isFinite(sent) || sent <= 0) {
    return null;
  }
  const n = Number.isFinite(count) ? count : 0;
  const shown = Number(((n / sent) * 100).toFixed(digits));
  // Thresholds are fractions; compare in the displayed percent, rounded the same way so 0.1% vs
  // 0.001 never loses to floating point.
  const at = (frac) => shown >= Number((frac * 100).toFixed(digits + 2));
  if (at(t.red)) {
    return TIER_RED;
  }
  if (at(t.warn)) {
    return TIER_AMBER;
  }
  return null;
}

// The CSS class for a tier ('' = today's styling) -- the Dashboard's amber/red text classes.
export function tierClass(tier) {
  if (tier === TIER_RED) {
    return 'has-text-danger';
  }
  if (tier === TIER_AMBER) {
    return 'has-text-warning-dark';
  }
  return '';
}
