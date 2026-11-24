// Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC D7/D9/D10). The SMS Admin page's pure
// helpers: the short URL, when a target is a link, US Mountain 12-hour times, the link filter, the
// bodies the page sends (the whole record -- an omitted optional field is CLEARED by the admin API)
// and the error messages keyed on the HTTP status alone. The page reads counts, never computes them.
// No Vue, no DOM; tested by smsAdmin.test.mjs (npm test).
import { formatMountain } from './mountainTime.mjs'; // eslint-disable-line import/extensions

export const SHORT_DOMAIN = 'yq.link';

// The optional link fields, in the admin API's names (D6).
export const UTM_KEYS = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content'];
const OPTIONAL_KEYS = [...UTM_KEYS, 'brand', 'campaign', 'notes'];

export const LINK_FILTERS = ['active', 'inactive', 'all'];
export const DEFAULT_LINK_FILTER = 'active';

// shortUrl('tg-nov') -> 'https://yq.link/tg-nov'.
export function shortUrl(slug) {
  return `https://${SHORT_DOMAIN}/${slug}`;
}

// A target renders as a link only when it starts with https:// (otherwise plain text).
export function isLinkTarget(target) {
  return typeof target === 'string' && target.startsWith('https://');
}

// smsTime(iso) -> 'Nov 23, 2026, 7:05 PM' (US Mountain, 12-hour); '—' when absent or unparsable.
export function smsTime(iso) {
  return formatMountain(iso) || '—';
}

// The table's Active / Inactive / All filter (an absent `active` reads as active -- the admin
// API's default).
export function filterLinks(links, filter) {
  const rows = Array.isArray(links) ? links : [];
  if (filter === 'all') {
    return rows;
  }
  const want = filter !== 'inactive';
  return rows.filter((l) => (l && l.active !== false) === want);
}

// The modal's form from a stored record (empty strings for absent fields).
export function formFromLink(link) {
  const l = link || {};
  const form = { slug: l.slug || '', target: l.target || '', active: l.active !== false };
  OPTIONAL_KEYS.forEach((k) => { form[k] = typeof l[k] === 'string' ? l[k] : ''; });
  return form;
}

function optionalFields(form) {
  const out = {};
  OPTIONAL_KEYS.forEach((k) => { out[k] = typeof form[k] === 'string' ? form[k].trim() : ''; });
  return out;
}

// POST /api/sms/links -- slug empty = a generated code. No UTM defaults (Marketing types its own).
export function createBody(form) {
  return {
    slug: (form.slug || '').trim(),
    target: (form.target || '').trim(),
    ...optionalFields(form),
  };
}

// PUT /api/sms/links/:slug -- always the whole record, plus active.
export function updateBody(form) {
  return {
    target: (form.target || '').trim(),
    ...optionalFields(form),
    active: form.active !== false,
  };
}

// The message for a failed link write, keyed on the HTTP status alone: a 409 is the slug-taken
// message; anything else shows the server's text (the not-configured and unavailable notices
// included).
export function linkErrorMessage(status, serverMessage, t) {
  if (status === 409) {
    return t('sms.slugTaken');
  }
  return serverMessage || t('sms.adminUnavailable');
}
