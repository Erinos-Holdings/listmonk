// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D2/D3, amended by integrations BRAND-FOOTER-SPEC
// D1/D2. Resolve an OfficialFooter block's reference BY NAME from the context the host pushes, on
// every render. The block stores only its `kind`; the template `lang`/`brand` columns drive
// nothing -- the name is the marker.
//
// Import-free by construction: test/run.cjs compiles this file standalone (like postProcess.ts).
//
// The name grammar (BRAND-FOOTER-SPEC D1): an official name starts with `Official ` (the word and
// ONE space) and has exactly one of three shapes, LANG = two upper-case letters:
//   corporate        -> Official Footer <LANG>
//   brand, neutral   -> Official <Brand> Footer          (one footer for every language)
//   brand, override  -> Official <Brand> Footer <LANG>   (that language only)
// where slug(Brand) equals slug(context brand). Corporate is checked first.
//
// LANG is the upper-cased context lang; an empty lang resolves as `en` (a campaign stored before
// the create-time default -- "no attribs.lang => the English send"). Corporate has no fallback.
// Brand: the override for the context language, else the neutral footer, else `missing`. The
// neutral footer is not a cross-language fallback: it declares it holds no language (a neutral
// footer may carry no text -- the fork's template save and the seed writers enforce that). Brand
// `curated` is an ordinary brand here (CURATED-FOOTER-SPEC D1). Shared case table:
// test/fixtures/official-names.json (BRAND-FOOTER-SPEC D5).

export const OFFICIAL_TEMPLATE_PREFIX = 'Official ';

export type TOfficialKind = 'corporate' | 'brand';

// One official campaign_visual template, as the host pushes it.
export type TOfficialRef = {
  id: number;
  name: string;
  body_source: string | null | undefined;
};

// `brand: null` = the brand is not known yet (a campaign with no lists, or a derivation error):
// a brand block shows "Choose a list"; the corporate block still resolves (it needs lang only).
// `official: true` = an official template is being edited (the Insert action is hidden).
// Fork (integrations EDITOR-DEFAULTS-SPEC D2/D4) -- `campaign: true` = the campaign editor (a null
// body_source opens with the footer pair; the template editor never sets it); `linkDefault` = the
// campaign's own brand's default Image/Button link (frontend/src/linkDefaults.mjs), null for none.
// Neither is read by the resolution or the compile.
export type TOfficialContext = {
  lang?: string | null;
  brand?: string | null;
  official?: boolean;
  campaign?: boolean;
  linkDefault?: string | null;
};

export type TOfficialStatus = 'ok' | 'missing' | 'no-context' | 'duplicate';

export type TOfficialResolution = {
  status: TOfficialStatus;
  // The template name the block resolves to ('' when there is nothing to name).
  name: string;
  ref?: TOfficialRef;
};

// Lower-case, non-alphanumerics stripped: `AcmeCo` -> `acmeco`, `Thirsty Girl` -> `thirstygirl`.
// Applied to BOTH the name's brand segment and the context brand, so `acme-co` and
// `AcmeCo` are one brand.
export function officialSlug(s: string | null | undefined): string {
  return String(s ?? '').toLowerCase().replace(/[^a-z0-9]/g, '');
}

export function officialLang(lang: string | null | undefined): string {
  const l = String(lang ?? '').trim();
  return (l === '' ? 'en' : l).toUpperCase();
}

export function isOfficialName(name: string | null | undefined): boolean {
  return String(name ?? '').startsWith(OFFICIAL_TEMPLATE_PREFIX);
}

const RE_CORPORATE = /^Official Footer ([A-Z]{2})$/;
const RE_OVERRIDE = /^Official (.+) Footer ([A-Z]{2})$/;
const RE_NEUTRAL = /^Official (.+) Footer$/;

// The brand segment's slug, or null when the segment is not usable (a leading/trailing space, or
// nothing left after the slug rule).
function brandSlugOf(segment: string): string | null {
  if (segment !== segment.trim()) {
    return null;
  }
  const s = officialSlug(segment);
  return s === '' ? null : s;
}

// Parse an official template name (D1). null when it is none of the three shapes. `lang` is ''
// for a language-neutral brand footer; `brand` is the segment's slug ('' for corporate).
export function parseOfficialName(name: string): { kind: TOfficialKind; brand: string; lang: string } | null {
  const n = String(name ?? '');
  const corp = RE_CORPORATE.exec(n);
  if (corp) {
    return { kind: 'corporate', brand: '', lang: corp[1] };
  }
  const over = RE_OVERRIDE.exec(n);
  if (over) {
    const b = brandSlugOf(over[1]);
    return b === null ? null : { kind: 'brand', brand: b, lang: over[2] };
  }
  const neutral = RE_NEUTRAL.exec(n);
  if (neutral) {
    const b = brandSlugOf(neutral[1]);
    return b === null ? null : { kind: 'brand', brand: b, lang: '' };
  }
  return null;
}

function pick(matches: TOfficialRef[]): TOfficialResolution {
  const sorted = [...matches].sort((a, b) => a.id - b.id);
  return {
    status: sorted.length > 1 ? 'duplicate' : 'ok',
    name: sorted[0].name,
    ref: sorted[0],
  };
}

export function resolveOfficial(
  kind: TOfficialKind,
  context: TOfficialContext | null | undefined,
  refs: TOfficialRef[] | null | undefined
): TOfficialResolution {
  if (!context) {
    return { status: 'no-context', name: '' };
  }
  const lang = officialLang(context.lang);
  let brandSlug = '';
  if (kind === 'brand') {
    if (context.brand === null || context.brand === undefined || String(context.brand).trim() === '') {
      return { status: 'no-context', name: '' };
    }
    brandSlug = officialSlug(context.brand);
  }

  const parsed: { r: TOfficialRef; lang: string }[] = [];
  for (const r of refs ?? []) {
    if (!r || typeof r.name !== 'string') {
      continue;
    }
    const p = parseOfficialName(r.name);
    if (p && p.kind === kind && (kind === 'corporate' || p.brand === brandSlug)) {
      parsed.push({ r, lang: p.lang });
    }
  }

  const exact = parsed.filter((x) => x.lang === lang).map((x) => x.r);
  if (exact.length) {
    return pick(exact);
  }
  if (kind === 'corporate') {
    return { status: 'missing', name: `Official Footer ${lang}` };
  }
  const neutral = parsed.filter((x) => x.lang === '').map((x) => x.r);
  if (neutral.length) {
    return pick(neutral);
  }
  // The name to create: the neutral footer (the context brand's slug stands in for the display
  // casing, which only a matching template supplies).
  return { status: 'missing', name: `Official ${brandSlug} Footer` };
}
