// Fork (campaign review, polish pass 2 -- integrations 2026-10-06) -- the Inspect window's Context
// section, pure half: each context line, whether it is a concern, the review Lambda's D7.1 finding
// for it (one finding per concern line, located at the line's label) and the admin page that helps
// with it. CampaignReview.vue renders the bullet, the link and the three buttons.
import { CONTEXT_RULE, isContextConcern } from './reviewChecklist.mjs'; // eslint-disable-line import/extensions

// The admin page for a context label: its router path (the window prefixes the router base) and
// the page's name key (i18n `campaigns.review.contextPage.<page>`). No page for any other label.
export function contextPage(label) {
  const l = String(label ?? '');
  if (/^brand health\b/i.test(l)) return { path: '/brands', page: 'brands' };
  if (/^ses account\b/i.test(l)) return { path: '/', page: 'dashboard' };
  if (/^broadcasts running\b/i.test(l)) return { path: '/campaigns', page: 'campaigns' };
  return null;
}

export function contextPath(label) {
  const p = contextPage(label);
  return p ? p.path : null;
}

// The link's href: the router base (`/admin`, trailing slash dropped) + the page path, or null.
export function contextHref(base, label) {
  const p = contextPath(label);
  if (p === null) return null;
  const b = String(base || '').replace(/\/+$/, '');
  return p === '/' ? `${b}/` : `${b}${p}`;
}

// The D7.1 item of a report, or null.
export function contextItem(report) {
  const items = (report && Array.isArray(report.items)) ? report.items : [];
  return items.find((i) => i && i.id === CONTEXT_RULE) || null;
}

// The report's context lines with, for each: `concern`, the D7.1 `finding` on it (by location =
// label; null when the rule passed or an older report has no D7.1) and the page `href`.
export function contextRows(report, base) {
  const lines = (report && Array.isArray(report.context)) ? report.context : [];
  const item = contextItem(report);
  const byLabel = new Map(((item && item.verdict !== 'pass' && item.findings) || []).map((f) => [f.location, f]));
  return lines.map((c, index) => ({
    index,
    label: c.label,
    value: c.value,
    asOf: c.asOf || null,
    concern: isContextConcern(c.value),
    finding: byLabel.get(c.label) || null,
    href: contextHref(base, c.label),
    page: (contextPage(c.label) || {}).page || null,
  }));
}

// The D7.1 finding keys the window must see decided before it is Done.
export function contextKeys(report) {
  const item = contextItem(report);
  if (!item || item.verdict === 'pass' || item.verdict === 'n/a') return [];
  return (item.findings || []).map((f) => f.key);
}
