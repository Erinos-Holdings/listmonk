// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D2/D3. Resolve an OfficialFooter block's
// reference BY NAME from the context the host pushes, on every render. The block stores only
// its `kind`; the template `lang`/`brand` columns drive nothing -- the name is the marker.
//
// Import-free by construction: test/run.cjs compiles this file standalone (like postProcess.ts).
//
//   corporate -> Official_Footer_<LANG>
//   brand     -> Official_<Brand>_Footer_<LANG>, where slug(Brand) equals slug(context brand)
//
// LANG is the upper-cased context lang; an empty lang resolves as `en` (a campaign stored
// before the create-time default -- "no attribs.lang => the English send"). There is NO
// cross-language fallback: an absent name is `missing`. Brand `curated` has no brand footer by
// design (the corporate footer IS the Curated footer): a brand block resolves to `none`.

export const OFFICIAL_TEMPLATE_PREFIX = 'Official_';

export type TOfficialKind = 'corporate' | 'brand';

// One `Official_`-prefixed campaign_visual template, as the host pushes it.
export type TOfficialRef = {
  id: number;
  name: string;
  body_source: string | null | undefined;
};

// `brand: null` = the brand is not known yet (a campaign with no lists, or a derivation error):
// a brand block shows "Choose a list"; the corporate block still resolves (it needs lang only).
// `official: true` = an Official_ template is being edited (the Insert action is hidden).
export type TOfficialContext = {
  lang?: string | null;
  brand?: string | null;
  official?: boolean;
};

export type TOfficialStatus = 'ok' | 'missing' | 'none' | 'no-context' | 'duplicate';

export type TOfficialResolution = {
  status: TOfficialStatus;
  // The template name the block resolves to ('' when there is nothing to name).
  name: string;
  ref?: TOfficialRef;
};

export const CURATED_BRAND = 'curated';

// Lower-case, non-alphanumerics stripped: `AcmeCo` -> `acmeco`, `ACME` -> `acme`.
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

// Parse an official template name. null when it is not one of the two footer shapes.
export function parseOfficialName(name: string): { kind: TOfficialKind; brand: string; lang: string } | null {
  const corp = /^Official_Footer_([A-Za-z]+)$/.exec(name);
  if (corp) {
    return { kind: 'corporate', brand: '', lang: corp[1] };
  }
  const brand = /^Official_(.+)_Footer_([A-Za-z]+)$/.exec(name);
  if (brand) {
    return { kind: 'brand', brand: officialSlug(brand[1]), lang: brand[2] };
  }
  return null;
}

// The name a block of this kind would resolve to (brand kind: the context brand's slug stands
// in for the display casing, which only a matching template supplies).
function expectedName(kind: TOfficialKind, lang: string, brand: string): string {
  return kind === 'corporate' ? `Official_Footer_${lang}` : `Official_${brand}_Footer_${lang}`;
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
    if (brandSlug === CURATED_BRAND) {
      return { status: 'none', name: '' };
    }
  }

  const matches = (refs ?? []).filter((r) => {
    if (!r || typeof r.name !== 'string') {
      return false;
    }
    const p = parseOfficialName(r.name);
    if (!p || p.kind !== kind || p.lang !== lang) {
      return false;
    }
    return kind === 'corporate' || p.brand === brandSlug;
  });

  if (matches.length === 0) {
    return { status: 'missing', name: expectedName(kind, lang, brandSlug) };
  }
  const sorted = [...matches].sort((a, b) => a.id - b.id);
  return {
    status: sorted.length > 1 ? 'duplicate' : 'ok',
    name: sorted[0].name,
    ref: sorted[0],
  };
}
