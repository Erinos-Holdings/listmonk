// Fork (brand picker, integrations BRAND-PICKER-SPEC D3). The list form's pure half: the brand is
// chosen from a dropdown and the server writes the brand:/from:/site: tags as the projection of
// the brands row, so the form shows and sends only the FREE tags. The server refuses a reserved
// tag (lists.reservedTag); the form refuses it first. Tested by listBrand.test.mjs.

// The brand's projection: written by the server, hidden by the form.
export const PROJECTION_TAG_PREFIXES = ['brand:', 'from:', 'site:'];

// Fork (integrations JOURNEY-UX-SPEC A1.7) -- every prefix a typed tag may not carry: the
// projection, and repermission: (the retired Sunset V1 queue tag, refused by the server too --
// cmd/lists_brand.go). repermission: is not projection, so a stored one is NOT hidden: it shows
// as a tag, and the save is refused until it is removed, rather than dropping it silently.
export const RESERVED_TAG_PREFIXES = [...PROJECTION_TAG_PREFIXES, 'repermission:'];

// The locked list (models.LockedListNames in the Go backend): no edit or delete control is
// offered for it; the server refuses both with 409 regardless.
export const LOCKED_LIST_NAMES = ['Render catalog (never send)'];

const trimTag = (tag) => String(tag == null ? '' : tag).trim();

// isReservedTag judges the way the server does: after trimming outer whitespace and the
// whitespace between a reserved prefix and its value (` brand: x` is reserved).
export function isReservedTag(tag) {
  const t = trimTag(tag);
  return RESERVED_TAG_PREFIXES.some((p) => t.startsWith(p));
}

// splitReservedTags separates a list's stored tags into the free tags the form shows and the
// projection tags it hides.
export function splitReservedTags(tags) {
  const free = [];
  const reserved = [];
  (Array.isArray(tags) ? tags : []).forEach((t) => {
    const proj = PROJECTION_TAG_PREFIXES.some((p) => trimTag(t).startsWith(p));
    (proj ? reserved : free).push(t);
  });
  return { free, reserved };
}

// buildListRequest is the body the form sends: the form's fields, the free tags only, and the
// chosen brand. It throws when no brand is chosen -- the form never sends brand "" (the untagged
// list exists only for the locked render catalog list and scripted creation).
export function buildListRequest(form, brand) {
  const slug = String(brand == null ? '' : brand).trim();
  if (!slug) {
    throw new Error('a brand is required');
  }
  const { free } = splitReservedTags(form.tags);
  const out = { ...form, tags: free, brand: slug };
  return out;
}

export function isLockedList(list) {
  return !!list && LOCKED_LIST_NAMES.includes(list.name);
}

// brandLabel is the dropdown option label: the display name alone (the slug when it is empty).
export function brandLabel(b) {
  return b.display_name || b.slug;
}
