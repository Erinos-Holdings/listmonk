// Fork (integrations EDITOR-DEFAULTS-SPEC D4/D5/I6) -- the ONE rule for the default link a freshly
// inserted Image or Button block gets in the campaign editor. Pure; Campaign.vue pushes the result
// to the builder as `linkDefault` on the official context, and the builder's
// src/linkDefaults.ts::applyLinkDefault fills an EMPTY link with it (never a set one).
//
//   brand, fromEmail -- the CAMPAIGN's derived brand and From (Campaign.vue brandDerivation, as
//     officialContext.brand pushes it: null with no lists or a derivation error; `curated` for
//     unmapped lists), never the navbar selector's.
//   healthDocs -- the store's brand-health documents (GET /api/brands/health; each carries
//     `brand` and the hourly `usage` object). `usage.attribution` is the Usage pane's
//     Configurations > Shop links switch (integrations lib/brand-usage.ts: the registry siteUrl,
//     trimmed, is non-empty) -- read as stored, the fork computes nothing about it (D5).
//
// Returns:
//   null -- no brand, or no document for it (not loaded, a user without brands:get, a brand with
//     no document yet), or a shop-link brand whose slug is not a template identifier. Never a
//     guess: the review is the backstop.
//   the bare `{{ .Subscriber.Attribs.shop_links.<brand>.url }}` (attributes.js shopLinkExpr; no
//     `or` fallback -- runbook rule) when usage.attribution is true, for EVERY brand, corporate
//     ones included;
//   otherwise keyed on the From address's domain: curatedfor.you -> https://curatedfor.you (the
//     explore-by-brand landing page), curatedby.you -> https://curatedby.you, any other ->
//     https://curatedfor.you/<brand>.
// By construction no other brand's link, shop link or slug can be the default.
// eslint-disable-next-line import/extensions, import/no-relative-packages
import { shopLinkExpr } from '../email-builder/src/attributes.js';

const CORPORATE_DEFAULTS = Object.freeze({
  'curatedfor.you': 'https://curatedfor.you',
  'curatedby.you': 'https://curatedby.you',
});

// The domain of a From value (`Name <addr@domain>` or a bare address), lower-cased; '' when none.
function fromDomain(fromEmail) {
  const s = String(fromEmail || '');
  const m = /<([^>]+)>/.exec(s);
  const addr = (m ? m[1] : s).trim().toLowerCase();
  const at = addr.lastIndexOf('@');
  return at >= 0 ? addr.slice(at + 1).replace(/^www\./, '') : '';
}

// eslint-disable-next-line import/prefer-default-export
export function campaignLinkDefault(brand, fromEmail, healthDocs) {
  if (typeof brand !== 'string' || brand.trim() === '') {
    return null;
  }
  const slug = brand.trim().toLowerCase();
  const doc = (Array.isArray(healthDocs) ? healthDocs : []).find((d) => d && typeof d.brand === 'string' && d.brand.toLowerCase() === slug);
  if (!doc) {
    return null;
  }
  if (doc.usage && doc.usage.attribution === true) {
    return shopLinkExpr(slug);
  }
  const domain = fromDomain(fromEmail);
  if (Object.prototype.hasOwnProperty.call(CORPORATE_DEFAULTS, domain)) {
    return CORPORATE_DEFAULTS[domain];
  }
  return `https://curatedfor.you/${encodeURIComponent(slug)}`;
}
