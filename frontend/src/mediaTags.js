// Fork (media tags, MEDIA-TAGS-SPEC D2/D3). Media tags are bare lowercase slugs. The server is
// always strict (models.NormalizeMediaTags rejects an invalid tag with a 400); this file holds
// the two client-side halves:
//
//   - normalizeMediaTagsLenient: for CONTEXT tags derived from the editor (a campaign's brand
//     and Tags field, a template's brand) -- invalid entries are dropped silently, never sent.
//   - isValidMediaTag: the tag inputs' before-adding check for user-typed tags, the same rule
//     the server enforces, so a bad tag is refused in place instead of failing the save.
//
// MUST MATCH models.ReMediaTag / MediaTagMaxLen in the Go tree. Disposable at v7 (D12).

export const reMediaTag = /^[a-z0-9][a-z0-9_-]*$/;
export const MEDIA_TAG_MAX_LEN = 100;

export const foldMediaTag = (t) => String(t == null ? '' : t).trim().toLowerCase();

export const isValidMediaTag = (t) => t.length <= MEDIA_TAG_MAX_LEN && reMediaTag.test(t);

// Trim, lower-fold, drop empties and invalid entries, dedupe, sort.
export const normalizeMediaTagsLenient = (tags) => {
  const out = new Set();
  (tags || []).forEach((raw) => {
    const t = foldMediaTag(raw);
    if (t && isValidMediaTag(t)) {
      out.add(t);
    }
  });
  return [...out].sort();
};
