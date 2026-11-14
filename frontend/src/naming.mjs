// Fork (naming standard, integrations listmonk/NAMING-STANDARD-SPEC D1-D3/D6-D10/D15 §4.4). The pure
// half of the naming standard on screen: the brand/language chips of a title cell, the language
// toggle's predicate, the Lists brand-prefix hiding, the clone name, the duplicate warning, the
// former-name hover and the name input's counter. No Vue, no store -- the views only call it.
// Tested with `node --test src/naming.test.mjs` (yarn test:naming).
//
// The server's name rule is models/naming.go (NameProblem); this module mirrors only what the form
// shows (the 30 limit and the exempt prefixes) and never refuses -- the server does.

export const NAME_MAX = 30;
export const CATALOG_PREFIX = 'CATALOG ';
export const OFFICIAL_PREFIX = 'Official_';

// D2 -- the language toggle's values, in display order. 'all' applies no filter.
export const LANG_VALUES = Object.freeze(['all', 'en', 'es', 'fr', 'de', 'it']);

export function normalizeLang(v) {
  const s = typeof v === 'string' ? v.trim().toLowerCase() : '';
  return LANG_VALUES.includes(s) ? s : 'all';
}

const str = (v) => (v === null || v === undefined ? '' : String(v));

// A campaign row's language (attribs.lang), a template row's (its lang column); '' when absent.
export function langOf(row) {
  if (!row) return '';
  if (row.attribs && typeof row.attribs === 'object') return str(row.attribs.lang).trim().toLowerCase();
  return str(row.lang).trim().toLowerCase();
}

// D2 -- the ONE predicate (pinned with the server's query-campaigns lang filter by
// testdata/lang-filter.json): 'all' (or '') matches everything; a language matches that language
// AND every row with no language -- a campaign with no language shows under every setting.
export function langMatches(rowLang, toggle) {
  const t = normalizeLang(toggle);
  const l = str(rowLang).trim().toLowerCase();
  return t === 'all' || l === '' || l === t;
}

// The brand of a campaign row: its own `brand` field when the API row carries one (the Campaign
// Matrix rows do), else the single brand of its lists in the lists store (listBrands: { [id]: slug }).
// Several brands or none -> ''.
export function campaignBrandOf(row, listBrands = {}) {
  if (row && typeof row.brand === 'string') return row.brand;
  const ids = (row && Array.isArray(row.lists) ? row.lists : []).map((l) => (l && typeof l === 'object' ? l.id : l));
  const brands = [...new Set(ids.map((id) => (listBrands && listBrands[id]) || '').filter((b) => b !== ''))];
  return brands.length === 1 ? brands[0] : '';
}

// D1/D15 -- the title cell's chips: { brand, lang }, each the upper-cased value or null. The brand
// chip is hidden when the brand selector is that brand; the language chip when the toggle is that
// language. row.brand is the campaign's list brand (campaignBrandOf) or the template's brand column.
export function chipsFor(row, { brandSelection, langSelection } = {}) {
  const brand = str(row && row.brand).trim();
  const lang = langOf(row);
  const brandShown = brand !== '' && !(brandSelection && brandSelection.kind === 'brand' && brandSelection.slug === brand);
  const langShown = lang !== '' && normalizeLang(langSelection) !== lang;
  return { brand: brandShown ? brand.toUpperCase() : null, lang: langShown ? lang.toUpperCase() : null };
}

// D10 -- the former name shown on hover, or null when there is none or it equals the name. The API's
// `former_name` (raw reads) or `formerName` (the camelCased stores).
export function formerNameOf(row) {
  const f = row && (row.former_name !== undefined ? row.former_name : row.formerName);
  if (typeof f !== 'string' || f === '' || f === str(row.name)) return null;
  return f;
}

// D6 -- a list's name on the Lists page: under a single-brand selection, a list OF THAT BRAND loses
// exactly `<SLUG-UPPERCASED> ` or `<display_name> ` from its start -- never a heuristic; anything else
// is shown whole. brands: the GET /api/brands rows ({ slug, display_name }).
export function listDisplayName(list, brandSelection, brands = []) {
  const name = str(list && list.name);
  const slug = str(list && list.brand);
  if (!brandSelection || brandSelection.kind !== 'brand' || slug === '' || brandSelection.slug !== slug) return name;
  const row = (Array.isArray(brands) ? brands : []).find((b) => b && b.slug === slug);
  const prefixes = [`${slug.toUpperCase()} `, row && row.display_name ? `${row.display_name} ` : null].filter(Boolean);
  const hit = prefixes.find((p) => name.startsWith(p) && name.length > p.length);
  return hit ? name.slice(hit.length) : name;
}

const RE_CLONE = /^Clone(\d+) /;

// D7 -- `Clone<N> <base>`: base = the name minus any leading `Clone<k> `; N = 1 + the highest k among
// the existing `Clone<k> <base>` names (1 when none). No truncation: over 30 the server refuses.
export function cloneName(name, existingNames = []) {
  const base = str(name).replace(RE_CLONE, '');
  let max = 0;
  (Array.isArray(existingNames) ? existingNames : []).forEach((n) => {
    const m = RE_CLONE.exec(str(n));
    if (m && str(n).slice(m[0].length) === base) max = Math.max(max, Number(m[1]));
  });
  return `Clone${max + 1} ${base}`;
}

export const OPEN_STATUSES = Object.freeze(['draft', 'scheduled', 'paused', 'running']);
const fold = (s) => str(s).trim().toLowerCase();

// D8 -- the other OPEN campaign with the same name (trimmed, case-insensitive), brand and language,
// or null. A warning only: saving is never blocked. candidate/rows carry { id, name, status, brand,
// lang or attribs.lang }.
export function duplicateOf(candidate, rows = []) {
  const name = fold(candidate && candidate.name);
  if (!name) return null;
  const brand = str(candidate.brand);
  const lang = langOf(candidate);
  return (Array.isArray(rows) ? rows : []).find((r) => r && r.id !== candidate.id
    && OPEN_STATUSES.includes(r.status)
    && fold(r.name) === name
    && str(r.brand) === brand
    && langOf(r) === lang) || null;
}

// D3 -- a code-named record is exempt from the limit (a `CATALOG ` campaign, an `Official_` template).
export function isExempt(kind, name) {
  const n = str(name);
  if (kind === 'campaign') return n.startsWith(CATALOG_PREFIX);
  if (kind === 'template') return n.startsWith(OFFICIAL_PREFIX);
  return false;
}

// D3/D6 -- the name input's counter and maxlength: { n, max, maxlength, over } over the handle (a
// list's name minus its brand prefix; prefix is that text, '' for none). Exempt names are not
// limited (maxlength null).
export function nameCounter(kind, name, prefix = '') {
  const full = str(name);
  const handle = prefix && full.startsWith(prefix) ? full.slice(prefix.length) : full;
  const n = Array.from(handle.trim()).length;
  const exempt = isExempt(kind, full);
  return {
    n,
    max: NAME_MAX,
    maxlength: exempt ? null : NAME_MAX + Array.from(prefix || '').length,
    over: !exempt && n > NAME_MAX,
  };
}
