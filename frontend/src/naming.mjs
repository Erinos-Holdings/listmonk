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

// D7 -- a name minus any leading `Clone<k> ` (the base the clone number counts over; the Campaigns
// page searches for it to find every existing clone, review S4-10).
export function cloneBaseOf(name) {
  return str(name).replace(RE_CLONE, '');
}

// D7 -- `Clone<N> <base>`: base = the name minus any leading `Clone<k> `; N = 1 + the highest k among
// the existing `Clone<k> <base>` names (1 when none). No truncation: over 30 the server refuses.
export function cloneName(name, existingNames = []) {
  const base = cloneBaseOf(name);
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

// ---------------------------------------------------------------------------------------------------
// Fork (name guardrails, integrations listmonk/NAME-GUARDRAILS-SPEC §3/§3.1). Live naming WARNINGS
// on the three name inputs -- guidance only, never a refusal (D1), deterministic (D2). This is the
// rule's ONE home: no mirror in Go or in integrations lib/naming.ts. Pinned by the case table
// testdata/name-warnings.json (I1). Ambiguities resolve toward silence: no shape beyond §3's table.

// The codes, in the fixed order they are reported (§3 table).
export const WARNING_CODES = Object.freeze([
  'underscore', 'stepCode', 'language', 'brandFirst', 'dateFormat', 'datePosition', 'listPrefix', 'handleMissing',
]);

// Folded = lower-cased, diacritics removed (NFD, combining marks stripped).
const foldName = (s) => str(s).normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();

// ⟨…⟩: not adjacent to a letter or digit on either side (not regex \b, which treats `_` as a word
// character). The leading side is a CONSUMED boundary, never a lookbehind (review S4-2: this module
// is in the entry chunk, and an engine without lookbehind would fail to boot the whole admin UI).
const NB = '(?:^|[^\\p{L}\\p{N}])';
const NA = '(?![\\p{L}\\p{N}])';
const bounded = (p, flags = 'u') => new RegExp(`${NB}(?:${p})${NA}`, flags);

const RE_STEP = bounded('[A-Za-z]{2}\\d{1,2}');
// Row 3 (a): `_` + a code (any case), or `-` + an UPPER-CASE code; then `_`, `-`, a space or the end.
const RE_LANG_A_DASH = /-(EN|ES|FR|DE|IT)(?=[_\- ]|$)/;
const RE_LANG_A_UNDER = /_(?:en|es|fr|de|it)(?=[_\- ]|$)/i;
const LANG_CODES = ['EN', 'ES', 'FR', 'DE', 'IT'];

// Row 5's non-standard date shapes.
const RE_DATE_BAD = [
  bounded('\\d{6}'),
  bounded('\\d{8}'),
  bounded('\\d{1,2}[/._]\\d{1,2}[/._]\\d{2,4}'),
  bounded('\\d{4}-\\d{2}-\\d{2}'),
  bounded('\\d{1,2}-\\d{1,2}-\\d{4}'),
  new RegExp(`^(?:\\d-\\d{1,2}-\\d{2}|\\d{2}-\\d-\\d{2})${NA}`, 'u'),
];
const MONTHS = 'january|february|march|april|may|june|july|august|september|october|november|december'
  + '|jan|feb|mar|apr|jun|jul|aug|sep|oct|nov|dec';
const RE_MONTH = new RegExp(`${NB}(?:${MONTHS})${NA} \\d`, 'iu');

// A MM-DD-YY / MM-YYYY shaped token: additionally not adjacent to `-` or a digit (rows 5-6). The
// leading boundary is consumed (S4-2); capture 1 is the token itself, so its index is derived.
const RE_STD_TOKEN = /(?:^|[^\p{L}\p{N}-])((\d{2})-(\d{2})(?:-(\d{2}))?)(?![\p{L}\p{N}-])/gu;
const RE_STD_TOKEN_4 = /(?:^|[^\p{L}\p{N}-])((\d{2})-(\d{4}))(?![\p{L}\p{N}-])/gu;
const tokenIndex = (m) => m.index + m[0].length - m[1].length;

const daysIn = (month, year) => new Date(Date.UTC(year, month, 0)).getUTCDate();

// Is MM-DD-YY (dd given) or MM-YYYY (dd null) a real calendar date? YY -> 20YY.
function validStd(mm, dd, yy) {
  const m = Number(mm);
  if (m < 1 || m > 12) return false;
  if (dd === null) return true;
  const d = Number(dd);
  return d >= 1 && d <= daysIn(m, 2000 + Number(yy));
}

// Every standard-shaped token: { index, valid }.
function stdTokens(s) {
  const days = Array.from(s.matchAll(RE_STD_TOKEN)).filter((m) => m[4] !== undefined)
    .map((m) => ({ index: tokenIndex(m), valid: validStd(m[2], m[3], m[4]) }));
  const months = Array.from(s.matchAll(RE_STD_TOKEN_4)).map((m) => ({ index: tokenIndex(m), valid: validStd(m[2], null, null) }));
  return days.concat(months);
}

// The leading date token (§3, S2-4): one numeric shape anchored at the start, ending at a non-digit.
// Returns its length, 0 when none. Month names are never a leading token.
const RE_LEADING_SHAPE = /^(?:\d{8}|\d{6}|\d{4}-\d{2}-\d{2}|\d{1,2}-\d{1,2}-\d{4}|\d{1,2}[/._]\d{1,2}[/._]\d{2,4}|\d-\d{1,2}-\d{2}|\d{2}-\d-\d{2})(?!\d)/;
const RE_LEADING_STD = /^(?:(\d{2})-(\d{2})-(\d{2})|(\d{2})-(\d{4}))(?!\d)/;
function leadingDateLength(s) {
  const std = RE_LEADING_STD.exec(s);
  if (std) {
    const valid = std[1] !== undefined ? validStd(std[1], std[2], std[3]) : validStd(std[4], null, null);
    // A valid standard date is a token when followed by a space or the end; an impossible one always.
    if (!valid || /^(?: |$)/.test(s.slice(std[0].length))) return std[0].length;
  }
  const m = RE_LEADING_SHAPE.exec(s);
  return m ? m[0].length : 0;
}

// The name minus its leading date token and the separators after it.
const afterLeadingDate = (s) => s.slice(leadingDateLength(s)).replace(/^[ _-]+/, '');

const brandRow = (brands, slug) => (Array.isArray(brands) ? brands : []).find((b) => b && b.slug === slug) || null;

// The list's expected brand text: { prefix, expected, bare } -- private `<SLUG> ` (expected = the
// prefix), public `<display_name> ` / `<display_name> Emails`; null when the list has no brand (or a
// public list's display name is unknown).
function listBrandText(ctx) {
  const slug = str(ctx && ctx.brandSlug);
  if (slug === '') return null;
  const isPublic = ctx.listType === 'public';
  let bare = slug.toUpperCase();
  if (isPublic) {
    const row = brandRow(ctx.brands, slug);
    bare = row ? str(row.display_name) : '';
    if (bare === '') return null;
  }
  return {
    prefix: `${bare} `, expected: isPublic ? `${bare} Emails` : `${bare} `, bare, public: isPublic,
  };
}

// §3 -- the warning codes for a name being typed. kind: campaign | template | list;
// ctx = { brands, brandSlug, listType }.
export function nameWarnings(kind, name, ctx = {}) {
  const raw = str(name);
  const trimmed = raw.trim();
  if (trimmed === '' || isExempt(kind, trimmed)) return [];
  const c = ctx || {};
  const isList = kind === 'list';
  const lt = isList ? listBrandText(c) : null;

  // The text the checks run on: campaign/template -- trimmed, minus one `Clone<k> `; list -- the
  // handle (the trimmed name minus its exact brand prefix, else the whole name).
  let s;
  if (isList) s = lt && trimmed.startsWith(lt.prefix) ? trimmed.slice(lt.prefix.length) : trimmed;
  else s = cloneBaseOf(trimmed);

  const out = new Set();
  if (s.includes('_')) out.add('underscore');
  if (RE_STEP.test(s)) out.add('stepCode');

  if (!isList) {
    const tokens = s.split(' ').filter((t) => t !== '');
    const last = tokens[tokens.length - 1];
    if (RE_LANG_A_UNDER.test(s) || RE_LANG_A_DASH.test(s) || LANG_CODES.includes(last)) out.add('language');

    const rest = foldName(afterLeadingDate(s));
    const hit = (Array.isArray(c.brands) ? c.brands : []).some((b) => [b && b.slug, b && b.display_name]
      .map(foldName).filter((p) => p !== '')
      .some((p) => rest.startsWith(p) && /^(?:[ _-]|$)/.test(rest.slice(p.length))));
    if (hit) out.add('brandFirst');
  }

  const tokens = stdTokens(s);
  if (RE_DATE_BAD.some((re) => re.test(s)) || RE_MONTH.test(s) || tokens.some((t) => !t.valid)) out.add('dateFormat');
  // Silent whenever a VALID standard token is at the start, whatever follows it (S4-4).
  if (!tokens.some((t) => t.valid && t.index === 0) && tokens.some((t) => t.valid && t.index !== 0)) out.add('datePosition');

  if (isList && lt) {
    const okPrefix = lt.public ? trimmed === lt.expected : raw.trimStart().startsWith(lt.prefix);
    if (!okPrefix && !lt.expected.startsWith(trimmed)) out.add('listPrefix');
    if (trimmed === lt.bare) out.add('handleMissing');
  } else if (!isList && afterLeadingDate(cloneBaseOf(trimmed)) === '') {
    out.add('handleMissing');
  }

  return WARNING_CODES.filter((code) => out.has(code));
}

// §3.1 -- the one date formatter: MM-DD-YY, local date, zero-padded.
export function formatNameDate(date) {
  const d = date instanceof Date ? date : new Date(date);
  const p = (n) => String(n).padStart(2, '0');
  return `${p(d.getMonth() + 1)}-${p(d.getDate())}-${p(d.getFullYear() % 100)}`;
}

// §3.1 (D3) -- the grey sample in the empty field. ctx = { today, brandSlug, brandDisplay, listType }.
export function namePlaceholder(kind, ctx = {}) {
  const c = ctx || {};
  if (kind === 'campaign') return `${formatNameDate(c.today || new Date())} Product Launch`;
  if (kind === 'template') return 'Newsletter';
  if (c.listType === 'public') return `${str(c.brandDisplay) || 'Brand'} Emails`;
  const slug = str(c.brandSlug);
  return `${slug ? slug.toUpperCase() : 'BRAND'} Product Launch`;
}

// §3.1 (D5) -- the new form's starting name: a campaign's `MM-DD-YY `; '' otherwise (a private
// list's prefix is the D8 addon, never prefilled text). ctx = { today }.
export function namePrefill(kind, ctx = {}) {
  const c = ctx || {};
  if (kind === 'campaign') return `${formatNameDate(c.today || new Date())} `;
  return '';
}

// §3.1 (D8) -- a private list's brand prefix as a fixed addon before the handle input:
// `<SLUG-UPPERCASED>` for a private list with a brand that is new or whose STORED name already
// starts with `<SLUG-UPPERCASED> `; '' otherwise (the plain input and its listPrefix warning).
export function listPrefixAddon({
  brandSlug, listType, isNew, storedName,
} = {}) {
  const slug = str(brandSlug);
  if (listType !== 'private' || slug === '') return '';
  const up = slug.toUpperCase();
  return isNew || str(storedName).startsWith(`${up} `) ? up : '';
}

// §3.1 (D8) -- the handle: the name minus exactly `<addon> `; the whole name otherwise.
export function splitListName(name, addon) {
  const n = str(name);
  const p = `${str(addon)} `;
  return str(addon) !== '' && n.startsWith(p) ? n.slice(p.length) : n;
}

// §3.1 (D8) -- the saved name: `<addon> <handle trimmed>`, or the handle alone without an addon.
export function joinListName(addon, handle) {
  return str(addon) ? `${str(addon)} ${str(handle).trim()}` : str(handle);
}

// §3.1 (D5) -- does one Backspace clear the field? Only on the exact, untouched prefill.
export function clearsPrefill(value, prefill) {
  return typeof prefill === 'string' && prefill !== '' && value === prefill;
}
