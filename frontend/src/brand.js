// Shared brand helpers for the visual editor's brand color swatches (campaign and template
// editors both consume these; the tag-prefix constants move here from Campaign.vue in the
// template-editor changeset so the two editors import one copy).

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
