// Fork (journey preview, integrations listmonk/JOURNEY-PREVIEW-SPEC D3, its D1 source superseded by
// USAGE-PREVIEW-LANG-SPEC D3/D4). The pure half of the journey preview page
// (views/JourneyPreview.vue): which campaign previews which journey step, per brand. No Vue, no
// DOM; tested by journeyPreview.test.mjs (yarn test).
//
// USAGE-PREVIEW-LANG D3 -- the page previews exactly what the Brands > Usage circles count: a step's
// candidates are the COUNTED entries the integrations BrandHealth Lambda stores on that step's leaf
// (lib/brand-usage.ts `counted` -- { id, bubble, lang, covers }, ascending id) in the brand's usage
// doc (GET /api/dashboard/brand-matrix). The fork applies no language or status rule of its own: a
// language reaches an entry only through its stored `covers`, a status filter only through its
// stored `bubble`. The `journey:<family>:<day>` tag (parseJourneyTag) is no longer the page's source
// -- it stays the seed script's handle and feeds the System-generated filter.
import { GUIDE_SERIES, guideDays, seriesGuide } from './brandUsage.mjs'; // eslint-disable-line import/extensions
import { SERIES_POST_PURCHASE, SERIES_REENGAGEMENT, SERIES_WELCOME } from './listTrigger.mjs'; // eslint-disable-line import/extensions

export const JOURNEY_TAG = 'journey';
const TAG_PREFIX = `${JOURNEY_TAG}:`;

// The email canvas the preview iframe renders at (px) and its fixed height (D3: the sandboxed
// preview document is an opaque origin, so its height cannot be read).
export const FRAME_WIDTH = 600;
export const FRAME_HEIGHT = 1200;

// parseJourneyTag: `journey:<family>:<day>` -> { family, day }, null when malformed (an unknown
// family or a day that is not one of the family's standard days).
export function parseJourneyTag(tag) {
  if (typeof tag !== 'string' || !tag.startsWith(TAG_PREFIX)) {
    return null;
  }
  const parts = tag.slice(TAG_PREFIX.length).split(':');
  if (parts.length !== 2 || !/^\d+$/.test(parts[1])) {
    return null;
  }
  const [family] = parts;
  const day = Number(parts[1]);
  if (!GUIDE_SERIES.some((g) => g.family === family) || !guideDays(family).includes(day)) {
    return null;
  }
  return { family, day };
}

const isObj = (v) => !!v && typeof v === 'object' && !Array.isArray(v);
const posInt = (v) => typeof v === 'number' && Number.isInteger(v) && v > 0;
const BUBBLES = Object.freeze(['green', 'amber', 'grey']);

// The stored steps of a guide family: usage.series.welcome / .postPurchase steps, or
// usage.reengagement.steps; [] when the doc does not carry them.
function familySteps(usage, family) {
  if (!isObj(usage)) {
    return [];
  }
  let s = null;
  if (family === SERIES_REENGAGEMENT) {
    s = usage.reengagement;
  } else if (family === SERIES_WELCOME || family === SERIES_POST_PURCHASE) {
    s = isObj(usage.series) ? usage.series[family] : null;
  }
  return isObj(s) && Array.isArray(s.steps) ? s.steps : [];
}

// D3 -- stepEntries: the counted entries of one guide step (family, days) in a brand's usage doc,
// each well-formed { id, bubble, lang, covers }; [] when the doc carries no `counted` there (an
// older doc yields no previews, never a throw).
export function stepEntries(usage, family, days) {
  const st = familySteps(usage, family).find((x) => isObj(x) && x.days === days);
  const counted = st && Array.isArray(st.counted) ? st.counted : [];
  return counted.filter((e) => isObj(e) && posInt(e.id) && BUBBLES.includes(e.bubble)).map((e) => ({
    id: e.id,
    bubble: e.bubble,
    lang: typeof e.lang === 'string' ? e.lang : null,
    covers: Array.isArray(e.covers) ? e.covers.filter((l) => typeof l === 'string') : [],
  }));
}

// D3 -- every campaign id a brand's usage doc counts on the guide's steps, ascending (the one
// campaigns read).
export function previewIds(usage) {
  const ids = new Set();
  GUIDE_SERIES.forEach((g) => guideDays(g.family).forEach((d) => stepEntries(usage, g.family, d).forEach((e) => ids.add(e.id))));
  return [...ids].sort((a, b) => a - b);
}

// D3 -- the status dropdown's options (filters, not partitions). `bubble` is the stored circle
// colour an option keeps; `system` the System-generated predicate.
export const STATUS_FILTERS = Object.freeze([
  Object.freeze({ value: 'all', label: 'journeyPreview.statusAll' }),
  Object.freeze({ value: 'running', label: 'journeyPreview.statusRunning', bubble: 'green' }),
  Object.freeze({ value: 'paused', label: 'journeyPreview.statusPaused', bubble: 'amber' }),
  Object.freeze({ value: 'draft', label: 'journeyPreview.statusDraft', bubble: 'grey' }),
  Object.freeze({ value: 'system', label: 'journeyPreview.statusSystem' }),
]);

// D3 -- System-generated: the campaign carries attribs.journey_seed (any value present) or a valid
// journey:<family>:<day> tag, whatever its status.
export function isSystemGenerated(campaign) {
  if (!isObj(campaign)) {
    return false;
  }
  const a = isObj(campaign.attribs) ? campaign.attribs : {};
  if (a.journey_seed !== undefined && a.journey_seed !== null) {
    return true;
  }
  return Array.isArray(campaign.tags) && campaign.tags.some((t) => parseJourneyTag(t) !== null);
}

// D3 -- whether an entry (and its campaign) passes the status filter. An unknown filter is All.
export function statusMatches(entry, campaign, filter) {
  const f = STATUS_FILTERS.find((x) => x.value === filter);
  if (!f || f.value === 'all') {
    return true;
  }
  if (f.value === 'system') {
    return isSystemGenerated(campaign);
  }
  return !!entry && entry.bubble === f.bubble;
}

const updatedOf = (c) => Date.parse(String(c.updatedAt || c.updated_at || '')) || 0;

// The pick order's comparator over { entry, campaign } candidates: `first(entry)` true before
// false, then the latest updated_at, then the higher id. Negative = a before b.
function byPick(first) {
  return (a, b) => {
    const fa = first(a.entry) ? 1 : 0;
    const fb = first(b.entry) ? 1 : 0;
    if (fa !== fb) {
      return fb - fa;
    }
    const ua = updatedOf(a.campaign);
    const ub = updatedOf(b.campaign);
    if (ua !== ub) {
      return ub - ua;
    }
    return b.entry.id - a.entry.id;
  };
}

// D4 -- stepCell: one step's preview from its counted entries and the campaigns read (a Map or an
// object keyed by id), after the status filter. null -> the guide-text placeholder (no entry at
// all, or none after the status filter). Under a single language L (EN = EN+): the entries whose
// stored `covers` includes L; none -> { noneInLang: true } (never an English fallback); else the
// pick -- an exact `lang === L` before a language-less one, then the latest updated_at, then the
// higher id; `others` the rest. Under All: every entry; the pick -- `lang === 'en'` first, then
// latest, then id; `otherLangs` the distinct non-null langs among the other entries that differ
// from the shown entry's; `others` the remaining same-language (or language-less) extras.
// An id the campaigns read did not return is dropped.
export function stepCell(entries, campaignsById, { lang = 'all', status = 'all' } = {}) {
  const get = (id) => (campaignsById instanceof Map ? campaignsById.get(id) : (campaignsById || {})[id]);
  const cands = (Array.isArray(entries) ? entries : [])
    .map((entry) => ({ entry, campaign: get(entry.id) }))
    .filter((c) => isObj(c.campaign) && statusMatches(c.entry, c.campaign, status));
  if (!cands.length) {
    return null;
  }
  const L = typeof lang === 'string' && lang !== '' ? lang : 'all';
  if (L !== 'all') {
    const eligible = cands.filter((c) => Array.isArray(c.entry.covers) && c.entry.covers.includes(L));
    if (!eligible.length) {
      return {
        campaign: null, others: 0, otherLangs: 0, noneInLang: true,
      };
    }
    const sorted = [...eligible].sort(byPick((e) => e.lang === L));
    return {
      campaign: sorted[0].campaign, others: sorted.length - 1, otherLangs: 0, noneInLang: false,
    };
  }
  const sorted = [...cands].sort(byPick((e) => e.lang === 'en'));
  const shown = sorted[0];
  const rest = sorted.slice(1);
  const langs = new Set(rest.map((c) => c.entry.lang).filter((l) => l !== null && l !== shown.entry.lang));
  return {
    campaign: shown.campaign,
    others: rest.filter((c) => c.entry.lang === null || c.entry.lang === shown.entry.lang).length,
    otherLangs: langs.size,
    noneInLang: false,
  };
}

// journeyColumns (JOURNEY-PREVIEW I2): the guide's series (seriesGuide -- same order, titles, days
// and step texts), each step carrying the brand's preview cell (stepCell), null when the step shows
// the guide-text placeholder. `usage` is the selected brand's stored usage object.
export function journeyColumns(usage, campaignsById, opts = {}, t = (k) => k) {
  return seriesGuide(t).map((g) => ({
    ...g,
    steps: g.steps.map((s) => ({ ...s, preview: stepCell(stepEntries(usage, g.family, s.days), campaignsById, opts) })),
  }));
}

// D3 -- the brands the page offers: every brand-matrix row carrying a usage object, as slugs.
export function usageBrands(rows) {
  return (Array.isArray(rows) ? rows : [])
    .filter((r) => isObj(r) && typeof r.brand === 'string' && r.brand !== '' && isObj(r.usage))
    .map((r) => r.brand);
}

// pickBrand: the query's brand when it is offered, else the first, null when there is none.
export function pickBrand(brands, wanted) {
  const list = Array.isArray(brands) ? brands : [];
  if (wanted && list.includes(wanted)) {
    return wanted;
  }
  return list.length ? list[0] : null;
}

// frameScale: the CSS scale that fits the FRAME_WIDTH canvas into a column `width` px wide (never
// enlarged); 1 for an unmeasured width.
export function frameScale(width) {
  if (typeof width !== 'number' || !Number.isFinite(width) || width <= 0) {
    return 1;
  }
  return Math.min(1, width / FRAME_WIDTH);
}

// USAGE-PREVIEW-LANG D2 (replacing JOURNEY-PREVIEW D2's usageTarget): a Usage row's preview glyph
// route -- that row's brand (defined in brandUsage.mjs, beside previewGlyph, so the two modules
// import in one direction only).
export { previewTarget } from './brandUsage.mjs'; // eslint-disable-line import/extensions
