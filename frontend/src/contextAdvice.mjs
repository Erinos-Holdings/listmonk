// Fork (campaign review, polish pass 2 -- integrations 2026-10-06) -- the Inspect window's Context
// section, pure half: each context line, whether it is a concern, its advice `note` and the admin
// page that helps with it. CampaignReview.vue renders the bullet, the note (plain text, no buttons)
// and the link.
//
// Integrations REVIEW-TRIM-SPEC D3: context is context -- the campaign-context rule (D7.1) and its
// buttons are retired; the review Lambda writes one sentence of advice as the line's `note` (the SES
// and brand-health lines when not ok). Nothing here is decided, so nothing here gates Done.
import { isContextConcern } from './reviewChecklist.mjs'; // eslint-disable-line import/extensions

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

// The report's context lines with, for each: `concern`, its `note` (the Lambda's advice, or null --
// an older report has none) and the page `href`.
export function contextRows(report, base) {
  const lines = (report && Array.isArray(report.context)) ? report.context : [];
  return lines.map((raw, index) => {
    const c = raw && typeof raw === 'object' ? raw : {};
    return {
      index,
      label: c.label,
      value: c.value,
      asOf: c.asOf || null,
      concern: isContextConcern(c.value),
      note: typeof c.note === 'string' && c.note.trim() !== '' ? c.note : null,
      href: contextHref(base, c.label),
      page: (contextPage(c.label) || {}).page || null,
    };
  });
}
