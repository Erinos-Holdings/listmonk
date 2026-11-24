// Fork (page state, integrations listmonk/PAGE-STATE-SPEC §3; supersedes MEDIA-TAGS-SPEC D5's
// "never persisted"). The /media page's session state (pageState.mjs): the search, the tag chips,
// Untagged, All and the page. The picker (Media.vue in modal mode) keeps nothing. Media reads no
// route query, so no link is overridden (D5). The state is kept with the brand key it was made
// under and restored only under that same key: a brand change re-seeds the filter row on the page
// (GLOBAL-BRAND-PHASE2-SPEC D7), so a selection changed in between re-seeds it too (D7). No Vue;
// tested by mediaPage.test.mjs.
import { pageNumberOf, textOf } from './pageState.mjs'; // eslint-disable-line import/extensions

export const MEDIA_PAGE = 'media';

const chipOf = (c) => (c && typeof c.tag === 'string' && c.tag.trim() !== ''
  ? { tag: c.tag, on: c.on === true, context: c.context === true }
  : null);

const chipsOf = (chips) => (Array.isArray(chips) ? chips.map(chipOf).filter(Boolean) : []);

// D3/D6 -- the kept fields of a fetch, with the brand key (store getter brandKey) they apply under.
export function saveMediaPage(s) {
  const v = s || {};
  return {
    query: textOf(v.query),
    page: pageNumberOf(v.page),
    chips: chipsOf(v.chips),
    untaggedOn: v.untaggedOn === true,
    allOn: v.allOn === true,
    brandKey: typeof v.brandKey === 'string' ? v.brandKey : null,
  };
}

// D7 -- the state to apply under `brandKey`, or null (seed the filter from the selection as today):
// null when nothing is stored or it was kept under another brand key.
export function restoreMediaPage(stored, brandKey) {
  if (!stored || typeof stored !== 'object' || typeof brandKey !== 'string' || stored.brandKey !== brandKey) {
    return null;
  }
  const s = saveMediaPage(stored);
  return {
    query: s.query, page: s.page, chips: s.chips, untaggedOn: s.untaggedOn, allOn: s.allOn,
  };
}
