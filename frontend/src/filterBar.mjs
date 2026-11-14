// Fork (polish 2026-11-14, the user). The filter caption: a light-blue first row on a matrix or table
// while the navbar's brand selector or language toggle narrows it -- just the active values, e.g.
// "BNKD Wellness · EN" (the brand's full display name, the language's two-letter code upper-cased).
// The launch filter is never named. Rendered as the table's <caption> (b-table's `caption` prop, or
// a <caption> in a hand-built matrix); the text comes from Vue.prototype.$filterCaption (main.js).
// The pure half; tested by filterBar.test.mjs.

import { normalizeLang } from './naming.mjs'; // eslint-disable-line import/extensions
import { brandLabelOf } from './brandScope.mjs'; // eslint-disable-line import/extensions

export const CAPTION_SEPARATOR = ' · ';

// filterCaption -> the caption text, '' when nothing applies. selection: the resolved brand selection
// ({ kind: all | brand | none, slug }); lang: the effective language ('all' while the toggle is
// hidden); brands: the GET /api/brands rows (display names); noBrand: the "No brand" label;
// applies: { brand, lang } -- whether that filter narrows this table (both default true).
// eslint-disable-next-line import/prefer-default-export
export function filterCaption({
  selection, lang, brands = [], noBrand = 'No brand', applies = {},
} = {}) {
  const ap = { brand: true, lang: true, ...applies };
  const parts = [];
  const sel = selection || { kind: 'all' };
  if (ap.brand && sel.kind === 'brand' && sel.slug) {
    parts.push(brandLabelOf(sel.slug, brands));
  } else if (ap.brand && sel.kind === 'none') {
    parts.push(noBrand);
  }
  const g = normalizeLang(lang);
  if (ap.lang && g !== 'all') {
    parts.push(g.toUpperCase());
  }
  return parts.join(CAPTION_SEPARATOR);
}
