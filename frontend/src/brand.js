// Shared brand helpers for the visual editor's brand color swatches, imported by both the
// campaign editor (Campaign.vue, which derives the brand from selected lists) and the
// template editor (TemplateForm.vue, which picks it from a dropdown).

// --- List-scoped From address and brand tag -------------------------------------------------
// A list carries its brand mapping as two tags, `brand:<slug>` and `from:<address>`. Both the
// campaign's From address and its `brand` SES message tag are derived from them, so neither is
// hand-typed. This is the convenience half; cmd/campaigns.go enforces the same rules server-side
// and is the actual control (this file is disposable at listmonk v7, that one is not).
//
// MUST MATCH `brandTagPrefix`/`fromTagPrefix` in cmd/campaigns_brand.go — nothing checks that
// they agree; if they diverge, the editors derive one mapping and the backend enforces another.
export const BRAND_TAG_PREFIX = 'brand:';
export const FROM_TAG_PREFIX = 'from:';

// SES message tag values accept only alphanumerics, - and _. List tags are stored verbatim with no
// normalisation anywhere, so `brand:Thirsty Girl` would otherwise reach SES and be rejected at SEND
// time, on a campaign nobody touched, weeks after someone edited a list tag. The campaign editor
// surfaces a failing slug as a loud derivation error; the template editor's dropdown roster simply
// omits it (the roster is a convenience list, not the enforcement point).
//
// MUST MATCH `reBrandSlug` in cmd/campaigns_brand.go — same coupling as the prefixes above.
export const reBrandSlug = /^[A-Za-z0-9_-]+$/;

// The catalog theme's color roles, in the order the swatch row renders them. Font entries
// (fontBody, fontHeading) are deliberately excluded — web fonts are unreliable in email
// clients and the builder has its own stack.
export const BRAND_THEME_ROLES = Object.freeze(['bg', 'fg', 'accent']);

// Build the swatch-row palette the email builder's setBrandPalettes() takes from a
// GET /api/brands/:slug/theme payload. The theme arrives as an unordered map (Go marshals
// map keys alphabetically), so role order is imposed here. Returns null when the theme
// carries no known color role — the caller renders no row (soft-fail, by design).
export const brandThemePalette = (slug, theme) => {
  const colors = BRAND_THEME_ROLES.filter((r) => theme && theme[r])
    .map((r) => ({ role: r, value: theme[r] }));
  if (colors.length === 0) {
    return null;
  }

  // The label is the lowercase-folded slug — the proxy's canonical casing.
  return { label: slug.toLowerCase(), colors };
};
