// Fork (polish 2026-11-14, the user). The blue filter bar across the top of a matrix or table while a
// GLOBAL filter (the navbar's brand selector, launch filter or language toggle) narrows it: it names
// each active filter and the columns the filter hides, so a missing Brand or Lang column is never a
// mystery. The pure half; GlobalFilterBar.vue only renders it. Tested by filterBar.test.mjs.
//
// A table says which filters apply to it (brand / launch / lang) and which columns it hides under
// one (brandColumn: the Brand column under a single-brand selection; langColumn: the Lang column
// under a language). A filter that does not apply to the table is never listed.

import { normalizeLang } from './naming.mjs'; // eslint-disable-line import/extensions
import { normalizeLaunch } from './brandScope.mjs'; // eslint-disable-line import/extensions

// filterBarState -> { items: [{ key, params }], hidden: [i18n key] } or null when nothing is active.
// selection: the resolved brand selection ({ kind: all | brand | none, slug }); launch: 'all' |
// 'launched' | 'unlaunched'; lang: the effective language ('all' while the toggle is hidden).
// applies: { brand, launch, lang } (brand and lang default true, launch false); columns:
// { brand, lang } -- the table has that column and hides it under the filter.
// eslint-disable-next-line import/prefer-default-export
export function filterBarState({
  selection, launch, lang, applies = {}, columns = {},
} = {}) {
  const ap = {
    brand: true, launch: false, lang: true, ...applies,
  };
  const items = [];
  const hidden = [];
  const sel = selection || { kind: 'all' };

  if (ap.brand && sel.kind === 'brand' && sel.slug) {
    items.push({ key: 'filterBar.brand', params: { brand: String(sel.slug).toLowerCase() } });
    if (columns.brand) hidden.push('filterBar.col.brand');
  } else if (ap.brand && sel.kind === 'none') {
    items.push({ key: 'filterBar.noBrand', params: {} });
  }

  const l = normalizeLaunch(launch);
  if (ap.launch && l !== 'all') {
    items.push({ key: `filterBar.${l}`, params: {} });
  }

  const g = normalizeLang(lang);
  if (ap.lang && g !== 'all') {
    items.push({ key: 'filterBar.lang', params: { lang: g.toUpperCase() } });
    if (columns.lang) hidden.push('filterBar.col.lang');
  }

  return items.length ? { items, hidden } : null;
}
