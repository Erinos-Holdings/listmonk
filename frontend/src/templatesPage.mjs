// Fork (polish 2026-11-14, the user). The Templates page's rows: every global filter (the brand
// selector, the launch filter, the language toggle) and the page's own Footer | Other toggle, in
// one pure function. Tested by templatesPage.test.mjs.
//
// - brand: brandScope.mjs::filterTemplates (a brand's rows plus the brandless; No brand the brandless).
// - launch: a branded template whose brand does not match the launch filter is dropped; a brandless
//   template is no brand and always stays (as No brand stays offered under every launch value).
// - language: naming.mjs::langMatches (a language-less template shows under every language).
// - kind: footer = the official footer templates (an official name, officialSweep.mjs), other =
//   everything else.
// - Polish 2026-11-16 (the user): an official footer's brand is its NAME's (parseOfficialName), and
//   the corporate `Official Footer <LANG>` belongs to the parent brand (curated) -- so curated shows
//   its own footer plus the five corporate ones, any other brand only `Official <Brand> Footer`
//   (and its overrides), No brand no official footer. An unparseable official name stays brandless.

import { PARENT_BRAND, filterTemplates, matchesLaunch } from './brandScope.mjs'; // eslint-disable-line import/extensions
import { langMatches } from './naming.mjs'; // eslint-disable-line import/extensions
import { isOfficialName, parseOfficialName } from './officialSweep.mjs'; // eslint-disable-line import/extensions

export const TEMPLATE_KINDS = Object.freeze(['other', 'footer']);
export const TEMPLATE_KIND_PREF = 'templates.kind';

export function normalizeTemplateKind(v) {
  return TEMPLATE_KINDS.includes(v) ? v : 'other';
}

export function templateRows(templates, {
  selection, launch = 'all', docs = [], lang = 'all', kind = 'other',
} = {}) {
  const k = normalizeTemplateKind(kind);
  // The brand a row is filed under: an official footer's from its name, else the brand column.
  const brandOf = (t) => {
    const o = isOfficialName(t && t.name) ? parseOfficialName(t.name) : null;
    if (o) return o.kind === 'corporate' ? PARENT_BRAND : o.brand;
    return t && typeof t.brand === 'string' ? t.brand : '';
  };
  const filed = new Set(filterTemplates(
    (Array.isArray(templates) ? templates : []).map((t) => ({ id: t.id, brand: brandOf(t) })),
    selection,
  ).map((r) => r.id));
  return (Array.isArray(templates) ? templates : [])
    .filter((t) => filed.has(t.id))
    .filter((t) => brandOf(t) === '' || matchesLaunch(brandOf(t), launch, docs))
    .filter((t) => langMatches(t.lang, lang))
    .filter((t) => (isOfficialName(t && t.name) ? k === 'footer' : k === 'other'));
}
