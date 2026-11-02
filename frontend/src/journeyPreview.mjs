// Fork (journey preview, integrations listmonk/JOURNEY-PREVIEW-SPEC D1/D3). The pure half of the
// journey preview page (views/JourneyPreview.vue): which campaign previews which journey step, per
// brand. No Vue, no DOM; tested by journeyPreview.test.mjs (yarn test).
//
// D1 -- a campaign carrying the tag `journey:<family>:<day>` is the preview of that step, where
// family is one of the journey guide's series ids (GUIDE_SERIES -- listTrigger.mjs SERIES_WELCOME,
// SERIES_POST_PURCHASE, SERIES_REENGAGEMENT) and day one of that series' standard days
// (brandUsage.mjs::guideDays -- the guide's own tables, one source). Its brand is the ONE `brand:`
// tag across its lists (the From derivation's rule); two brands or none: ignored. One campaign per
// (brand, family, day): the latest updated_at wins (then the higher id), the rest are counted in
// `others`. A malformed journey: tag is ignored, never an error. The plain tag `journey` rides
// along so one ?tag=journey query fetches every step preview.
import { GUIDE_SERIES, guideDays, seriesGuide } from './brandUsage.mjs'; // eslint-disable-line import/extensions

export const JOURNEY_TAG = 'journey';
const TAG_PREFIX = `${JOURNEY_TAG}:`;
const BRAND_TAG_PREFIX = 'brand:';

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

// listBrandOf: a list's `brand:<slug>` tag's slug, '' when it carries none.
export function listBrandOf(list) {
  const tags = list && Array.isArray(list.tags) ? list.tags : [];
  const t = tags.find((x) => typeof x === 'string' && x.startsWith(BRAND_TAG_PREFIX) && x.length > BRAND_TAG_PREFIX.length);
  return t ? t.slice(BRAND_TAG_PREFIX.length) : '';
}

// campaignBrand: the one brand across a campaign's lists (lists unknown to `listsById` -- not
// permitted to this user -- contribute nothing), null for none or several.
export function campaignBrand(campaign, listsById) {
  const brands = new Set();
  (campaign && Array.isArray(campaign.lists) ? campaign.lists : []).forEach((l) => {
    const b = listBrandOf(listsById.get(l && l.id));
    if (b) {
      brands.add(b);
    }
  });
  return brands.size === 1 ? [...brands][0] : null;
}

const updatedOf = (c) => String(c.updatedAt || c.updated_at || '');

// Later wins: updated_at (ISO strings compare chronologically), then the higher id.
function isLater(a, b) {
  const ua = Date.parse(updatedOf(a)) || 0;
  const ub = Date.parse(updatedOf(b)) || 0;
  return ua !== ub ? ua > ub : (Number(a.id) || 0) > (Number(b.id) || 0);
}

// stepPreviews (I1): campaigns (GET /api/campaigns?tag=journey rows) and lists (GET /api/lists
// rows) -> { brands: [slug, ...] sorted, byBrand: { slug: { family: { day: { campaign, others } } } } }.
export function stepPreviews(campaigns, lists) {
  const listsById = new Map((Array.isArray(lists) ? lists : []).filter((l) => l && l.id != null).map((l) => [l.id, l]));
  const byBrand = {};
  (Array.isArray(campaigns) ? campaigns : []).forEach((c) => {
    if (!c || !Array.isArray(c.tags)) {
      return;
    }
    const steps = c.tags.map(parseJourneyTag).filter(Boolean);
    if (!steps.length) {
      return;
    }
    const brand = campaignBrand(c, listsById);
    if (!brand) {
      return;
    }
    steps.forEach(({ family, day }) => {
      byBrand[brand] = byBrand[brand] || {};
      byBrand[brand][family] = byBrand[brand][family] || {};
      const cur = byBrand[brand][family][day];
      if (!cur) {
        byBrand[brand][family][day] = { campaign: c, others: 0 };
      } else {
        byBrand[brand][family][day] = { campaign: isLater(c, cur.campaign) ? c : cur.campaign, others: cur.others + 1 };
      }
    });
  });
  return { brands: Object.keys(byBrand).sort(), byBrand };
}

// journeyColumns (I2): the guide's series (seriesGuide -- same order, titles, days and step texts),
// each step carrying the brand's preview cell, null when the step has no campaign.
export function journeyColumns(previews, brand, t = (k) => k) {
  const cells = (previews && previews.byBrand && previews.byBrand[brand]) || {};
  return seriesGuide(t).map((g) => ({
    ...g,
    steps: g.steps.map((s) => ({ ...s, preview: (cells[g.family] && cells[g.family][s.days]) || null })),
  }));
}

// pickBrand: the query's brand when it has a preview, else the first, null when there is none.
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

// usageTarget (D2): the Usage pane's Preview button route -- the global selector's brand when it is
// one brand, otherwise no brand.
export function usageTarget(selection) {
  const brand = selection && selection.kind === 'brand' && selection.slug ? selection.slug : null;
  return brand ? { name: 'journeyPreview', query: { brand } } : { name: 'journeyPreview' };
}
