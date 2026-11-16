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

import { filterTemplates, matchesLaunch } from './brandScope.mjs'; // eslint-disable-line import/extensions
import { langMatches } from './naming.mjs'; // eslint-disable-line import/extensions
import { isOfficialName } from './officialSweep.mjs'; // eslint-disable-line import/extensions

export const TEMPLATE_KINDS = Object.freeze(['other', 'footer']);
export const TEMPLATE_KIND_PREF = 'templates.kind';

export function normalizeTemplateKind(v) {
  return TEMPLATE_KINDS.includes(v) ? v : 'other';
}

export function templateRows(templates, {
  selection, launch = 'all', docs = [], lang = 'all', kind = 'other',
} = {}) {
  const k = normalizeTemplateKind(kind);
  const brandOf = (t) => (t && typeof t.brand === 'string' ? t.brand : '');
  return filterTemplates(Array.isArray(templates) ? templates : [], selection)
    .filter((t) => brandOf(t) === '' || matchesLaunch(brandOf(t), launch, docs))
    .filter((t) => langMatches(t.lang, lang))
    .filter((t) => (isOfficialName(t && t.name) ? k === 'footer' : k === 'other'));
}
