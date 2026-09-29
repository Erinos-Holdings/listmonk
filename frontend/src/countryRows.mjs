// Fork (location stats, integrations LOCATION-STATS-SPEC D8). Row shaping for the Location table
// on Campaign Analytics: the localised country name (code as the fallback), the Unknown row, and
// the sort order. Pure: no Vue, no DOM, tested with Node's built-in runner
// (`npm run test:countries`). Display only -- the counts come from
// GET /api/campaigns/analytics/countries as they are.
//
// Default order: Views desc, then Clicks desc, then name A->Z. Unknown (country "") is ALWAYS
// last, whatever the column or direction. A column sort falls back to the default order on ties.

export const DEFAULT_SORT = Object.freeze({ field: 'views', order: 'desc' });

// regionNames returns an Intl.DisplayNames for region codes in the given locale, or null where
// the runtime has none (or the locale is not a valid tag).
export function regionNames(locale) {
  try {
    return new Intl.DisplayNames([locale, 'en'], { type: 'region' });
  } catch {
    try {
      return new Intl.DisplayNames(['en'], { type: 'region' });
    } catch {
      return null;
    }
  }
}

// countryName is the display name of a two-letter code, or the code itself when no name resolves.
export function countryName(code, names) {
  if (!names || !code) {
    return code;
  }
  try {
    const n = names.of(code);
    return typeof n === 'string' && n !== '' ? n : code;
  } catch {
    return code;
  }
}

const num = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);

function compareName(a, b, locale) {
  return a.name.localeCompare(b.name, locale) || a.country.localeCompare(b.country);
}

function compareDefault(a, b, locale) {
  return (b.views - a.views) || (b.clicks - a.clicks) || compareName(a, b, locale);
}

// sortCountryRows returns a new array sorted by field ('name', 'views' or 'clicks') in order
// ('asc' or 'desc'), Unknown last, ties broken by the default order.
export function sortCountryRows(rows, field = DEFAULT_SORT.field, order = DEFAULT_SORT.order, locale = 'en') {
  const dir = order === 'asc' ? 1 : -1;
  return [...rows].sort((a, b) => {
    if (a.unknown !== b.unknown) {
      return a.unknown ? 1 : -1;
    }
    let c = 0;
    if (field === 'name') {
      c = compareName(a, b, locale) * dir;
    } else if (field === 'views' || field === 'clicks') {
      c = (a[field] - b[field]) * dir;
    }
    return c || compareDefault(a, b, locale);
  });
}

// shapeCountryRows maps API rows to table rows {country, name, views, clicks, unknown}, sorted in
// the default order.
export function shapeCountryRows(rows, { locale = 'en', unknownLabel = 'Unknown' } = {}) {
  const names = regionNames(locale);
  const out = (Array.isArray(rows) ? rows : []).map((r) => {
    const country = typeof r.country === 'string' ? r.country.trim().toUpperCase() : '';
    const unknown = country === '';
    return {
      country,
      name: unknown ? unknownLabel : countryName(country, names),
      views: num(r.views),
      clicks: num(r.clicks),
      unknown,
    };
  });
  return sortCountryRows(out, DEFAULT_SORT.field, DEFAULT_SORT.order, locale);
}
