// Fork (polish 2026-11-14, the user). The filter caption: a light-blue first row on a matrix or table
// while the navbar's brand selector or language toggle narrows it -- just the active values, e.g.
// "BNKD Wellness · English" (the brand's full display name, the language's English name -- polish
// 2026-11-14, the user: names, not codes).
// The launch filter is never named. Rendered as the table's <caption> (b-table's `caption` prop, or
// a <caption> in a hand-built matrix); the text comes from Vue.prototype.$filterCaption (main.js).
// The pure half; tested by filterBar.test.mjs.

import { normalizeLang } from './naming.mjs'; // eslint-disable-line import/extensions
import { brandLabelOf } from './brandScope.mjs'; // eslint-disable-line import/extensions

export const CAPTION_SEPARATOR = ' · ';

// The caption's language names, in English whatever the language (the picker labels in langs.js are
// each language's own name). English is the send language at the toggle; the caption keeps it bare.
export const CAPTION_LANG_NAMES = Object.freeze({
  en: 'English', es: 'Spanish', fr: 'French', de: 'German', it: 'Italian',
});
export const captionLangName = (code) => CAPTION_LANG_NAMES[(code || '').toLowerCase()] || '';

// SUBSCRIBERS-FILTERS-2-SPEC D4 (I8) -- the EN+ split's two parts, or null: only from a response
// for the CURRENT query (split.current === true) that carries lang_none_total (an integer between
// 0 and total). { en: total - none, none }; n + m = total, in the unit of total.
export function enSplitOf(split) {
  if (!split || split.current !== true) return null;
  const { total, langNoneTotal } = split;
  if (!Number.isInteger(total) || !Number.isInteger(langNoneTotal) || langNoneTotal < 0 || langNoneTotal > total) {
    return null;
  }
  return { en: total - langNoneTotal, none: langNoneTotal };
}

// SUBSCRIBERS-FILTERS-SPEC D8 (I13), amended by SUBSCRIBERS-FILTERS-2-SPEC D1/D4 (I8) -- the
// Subscribers page's own caption language, named from its route lang: '' / all -> '' (no
// language); en -> `English <n> + No language <m>` once the current query's answer carries the
// split (split = { current, total, langNoneTotal }; format = the number formatter, the page's
// $utils.formatNumber), else English+ (the send language); other -> the caller's label
// (labels.other, default Other); a code -> its English name. labels.none names the no-language
// part (default No language). CAPTION_LANG_NAMES is untouched: the global caption under en is
// "English".
export function subscriberCaptionLang(v, labels = {}, split = null, format = String) {
  const l = typeof v === 'string' ? v.toLowerCase() : '';
  if (!l || l === 'all') return '';
  if (l === 'en') {
    const s = enSplitOf(split);
    if (!s) return `${captionLangName('en')}+`;
    return `${captionLangName('en')} ${format(s.en)} + ${labels.none || 'No language'} ${format(s.none)}`;
  }
  if (l === 'other') return labels.other || 'Other';
  return captionLangName(l);
}

// filterCaption -> the caption text, '' when nothing applies. selection: the resolved brand selection
// ({ kind: all | brand | none, slug }); lang: the effective language ('all' while the toggle is
// hidden); brands: the GET /api/brands rows (display names); noBrand: the "No brand" label;
// applies: { brand, lang } -- whether that filter narrows this table (both default true);
// langText: a page's OWN language filter, named by the page (Subscribers' route lang) -- used in
// place of the global language when given; '' names no language.
export function filterCaption({
  selection, lang, brands = [], noBrand = 'No brand', applies = {}, langText,
} = {}) {
  const ap = { brand: true, lang: true, ...applies };
  const parts = [];
  const sel = selection || { kind: 'all' };
  if (ap.brand && sel.kind === 'brand' && sel.slug) {
    parts.push(brandLabelOf(sel.slug, brands));
  } else if (ap.brand && sel.kind === 'none') {
    parts.push(noBrand);
  }
  if (typeof langText === 'string') {
    if (ap.lang && langText) parts.push(langText);
    return parts.join(CAPTION_SEPARATOR);
  }
  const g = normalizeLang(lang);
  // A value with no English name (a subscriber-only language, which langEffective never passes)
  // names nothing.
  if (ap.lang && g !== 'all' && captionLangName(g)) {
    parts.push(captionLangName(g));
  }
  return parts.join(CAPTION_SEPARATOR);
}
