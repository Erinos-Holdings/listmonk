// Fork (multi-brand overlap, integrations BRAND-OVERLAP-SPEC D3/I1/I7). Pure shaping for the
// "Multi-brand overlap" box on the Dashboard's SES account pane. The integrations BrandHealth
// Lambda computes every count, share, status, threshold and floor (lib/brand-overlap.ts owns them)
// and stores them on the ses document's `overlap` sub-input; this module only formats the stored
// numbers and scales the bar to the stored thresholds. No threshold value lives here.
// Tested by sesOverlap.test.mjs (node --test).

const defaultFormat = new Intl.NumberFormat('en-US').format;

const isObj = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);
const isNum = (v) => typeof v === 'number' && Number.isFinite(v);

// pct formats a stored fraction as a percentage: `digits` decimals, trailing zeros dropped when
// `trim` (a threshold reads "12%", a share "4.9%"); null for a non-number.
export function pct(v, digits = 1, trim = false) {
  if (!isNum(v)) {
    return null;
  }
  const s = (v * 100).toFixed(digits);
  return `${trim ? String(Number(s)) : s}%`;
}

const clampPct = (v) => `${Math.min(100, Math.max(0, v * 100))}%`;

// overlapView returns the box's numbers from the stored `overlap.detail`, or null when the detail
// carries no counts (a failed read: the Vue shows the reason instead):
// { share, multi, active, shareThree, three, warn, issues, fill, mark }. Every value is a display
// string; `fill` is the share over the stored issues line (clamped), `mark` the stored warn line
// over it; both '0%' when the stored thresholds are absent.
export function overlapView(detail, fmt = defaultFormat) {
  if (!isObj(detail) || !isNum(detail.active) || !isNum(detail.multi)) {
    return null;
  }
  const t = isObj(detail.thresholds) ? detail.thresholds : {};
  const warn = isNum(t.warn) ? t.warn : null;
  const issues = isNum(t.issues) && t.issues > 0 ? t.issues : null;
  const share = isNum(detail.share) ? detail.share : null;
  return {
    share: pct(share) ?? '—',
    multi: fmt(detail.multi),
    active: fmt(detail.active),
    shareThree: pct(detail.shareThree) ?? '—',
    three: isNum(detail.three) ? fmt(detail.three) : '—',
    warn: pct(warn, 2, true) ?? '—',
    issues: pct(issues, 2, true) ?? '—',
    fill: issues !== null && share !== null ? clampPct(share / issues) : '0%',
    mark: issues !== null && warn !== null ? clampPct(warn / issues) : '0%',
  };
}
