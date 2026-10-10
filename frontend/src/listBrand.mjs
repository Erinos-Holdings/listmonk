// Fork (brand picker, integrations BRAND-PICKER-SPEC D3). The list form's pure half: the brand is
// chosen from a dropdown and the server writes the brand:/from:/site: tags as the projection of
// the brands row, so the form shows and sends only the FREE tags. The server refuses a reserved
// tag (lists.reservedTag); the form refuses it first. Tested by listBrand.test.mjs.

export const RESERVED_TAG_PREFIXES = ['brand:', 'from:', 'site:'];

// The locked list (models.LockedListNames in the Go backend): no edit or delete control is
// offered for it; the server refuses both with 409 regardless.
export const LOCKED_LIST_NAMES = ['Render catalog (never send)'];

// isReservedTag judges the way the server does: after trimming outer whitespace and the
// whitespace between a reserved prefix and its value (` brand: x` is reserved).
export function isReservedTag(tag) {
  const t = String(tag == null ? '' : tag).trim();
  return RESERVED_TAG_PREFIXES.some((p) => t.startsWith(p));
}

// splitReservedTags separates a list's stored tags into the free tags the form shows and the
// reserved ones it hides.
export function splitReservedTags(tags) {
  const free = [];
  const reserved = [];
  (Array.isArray(tags) ? tags : []).forEach((t) => {
    (isReservedTag(t) ? reserved : free).push(t);
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

// brandLabel is the dropdown option label: `display_name — slug`.
export function brandLabel(b) {
  return `${b.display_name} — ${b.slug}`;
}
