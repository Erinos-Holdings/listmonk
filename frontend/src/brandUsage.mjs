// Fork (brand usage, integrations listmonk/BRAND-USAGE-SPEC D2/D7; BRAND-USAGE-V2-SPEC D1-D5). The
// pure half of the Dashboard's Brand Usage pane: row shaping (display name, launch filter -- the
// Brand Matrix's rows, off the SAME fetch), the column/section layout, the cell mapping (Score
// text + breakdown, the stoplight circles, ✓/✗ booleans, Last broadcast and Cadence), the circle
// links, the Mountain-time note, the sort (clickSort -- the matrix's pattern) and the per-session
// persistence wrapper. No Vue, no DOM; tested by brandUsage.test.mjs (yarn test).
//
// Each GET /api/dashboard/brand-matrix row may carry `usage` -- the brand's latest brand_health
// doc's stored object, projected verbatim. Every usage rule and threshold (the journey states, the
// broadcast cadence and its ok/due/stale verdict, the per-state circle counts and the Score with
// its weights -- integrations lib/brand-usage.ts SCORE_WEIGHTS) is computed by the integrations
// lib/brand-usage.ts; this module only maps stored values to text, circles and colors (v1 I1,
// V2 I1/I3). It renders `score.parts` verbatim and computes no weight.
//
// DASHBOARDS-POLISH2-SPEC D2: the Journeys and broadcast links filter to the BRAND's active lists --
// a list-id set built here from the lists store the Dashboard already holds (fork-owned list data
// rendered, not a health rule).
import { brandLabelOf, matchesLaunch, storeLists } from './brandScope.mjs'; // eslint-disable-line import/extensions
import { healthDocs } from './brandMatrix.mjs'; // eslint-disable-line import/extensions
import {
  POST_PURCHASE_STEP_DAYS, REENGAGEMENT_DAYS, SERIES_POST_PURCHASE, SERIES_REENGAGEMENT, SERIES_WELCOME, WELCOME_STEP_DAYS,
  seriesStepKey,
} from './listTrigger.mjs'; // eslint-disable-line import/extensions

// USAGE-REENGAGEMENT-SPEC D1/D6: the Re-engagement group's two leaves (the sunset queues' asks),
// 180d and 270d -- display labels of the planned policy (D10). Cross-repo literal: integrations
// lib/brand-usage.ts REENGAGEMENT_DAYS stores usage.reengagement.steps at exactly these days, and
// the fields below are named after them (`reengagement:180`, `reengagement:270`).
export { REENGAGEMENT_DAYS };

// LIFECYCLE-SERIES-SPEC S9 (replacing DASHBOARDS-POLISH3-SPEC D4's provisional No purchase
// 3d | 7d | 15d leaves and the plain Welcome / 1st purchase columns): two series groups, one leaf
// per standard day -- the No-purchase series Day 0 | 2d | 5d | 10d and the Post-purchase series
// Day 0 | 5d | 14d | 30d. The lib stores usage.series.<family>.steps (one {days, state, rows} per
// leaf) and .legacy (the off-standard step rows, a grey chip each after the group's last leaf); the
// fork maps them and computes nothing.
export const SERIES_DAYS = Object.freeze({
  [SERIES_WELCOME]: Object.freeze([0, ...WELCOME_STEP_DAYS]),
  [SERIES_POST_PURCHASE]: Object.freeze([0, ...POST_PURCHASE_STEP_DAYS]),
});

// 2026-10-30: the journey guide below the table -- each series' steps as a visual list, the same
// D5 texts the leaf hovers carry (one home each, en.json via seriesStepKey). Each series is titled
// with the chart's own name (No-purchase / Post-purchase series) and sublined with the industry
// name (Welcome / New Customer series, after Klaviyo; the CSS sets it in grey capitals) -- the
// guide sits directly under the matching columns.
export const GUIDE_SERIES = Object.freeze([
  Object.freeze({ family: SERIES_WELCOME, title: 'dashboard.usage.noPurchase', subtitle: 'dashboard.usage.guideWelcome' }),
  Object.freeze({ family: SERIES_POST_PURCHASE, title: 'dashboard.usage.postPurchase', subtitle: 'dashboard.usage.guideNewCustomer' }),
  // USAGE-REENGAGEMENT-SPEC D6: the third series -- titled "Re-engagement series" (NOT the table's
  // group label), sublined "Sunset flow"; its days are REENGAGEMENT_DAYS, never SERIES_DAYS.
  Object.freeze({ family: SERIES_REENGAGEMENT, title: 'dashboard.usage.guideReengagement', subtitle: 'dashboard.usage.guideSunset' }),
]);

// The standard days a guide series lists: SERIES_DAYS for the two lifecycle series,
// REENGAGEMENT_DAYS for the Re-engagement series.
const guideDays = (family) => (family === SERIES_REENGAGEMENT ? REENGAGEMENT_DAYS : SERIES_DAYS[family]);

// parseStepText splits a D5 text ("Day 5 Address purchase hesitation: \"A few reasons …\" Include
// …") into { title, quote, body }: the title between the leading "Day N " and the FIRST ": " (a
// title may itself end in "?", e.g. "How's it going?"), an optional leading double-quoted example
// subject line, and the rest as the guidance. A text without ": " is all body.
export function parseStepText(text) {
  const s = String(text == null ? '' : text).trim();
  const i = s.indexOf(': ');
  if (i < 0) {
    return { title: '', quote: '', body: s };
  }
  const title = s.slice(0, i).replace(/^Day \d+\s+/, '').trim();
  let rest = s.slice(i + 2).trim();
  let quote = '';
  const m = rest.match(/^"([^"]+)"\s*/);
  if (m) {
    [, quote] = m;
    rest = rest.slice(m[0].length).trim();
  }
  return { title, quote, body: rest };
}

// seriesGuide is the guide's content: per series (GUIDE_SERIES order) its title/subtitle keys and
// one { days, title, quote, body } per standard day, parsed from the translated D5 text.
export function seriesGuide(t = (k) => k) {
  return GUIDE_SERIES.map((g) => ({
    ...g,
    steps: guideDays(g.family).map((days) => ({ days, ...parseStepText(t(seriesStepKey(g.family, days))) })),
  }));
}

// The narrowest a guide column may be beside the divider; below it the guide stacks.
export const GUIDE_MIN_COLUMN = 300;

// guideSplitX decides the guide's layout from the measured divider x (the Post-purchase group
// header's left edge, px from the guide's left) and the guide's width: the rounded x when the left
// side keeps GUIDE_MIN_COLUMN and the right side keeps one GUIDE_MIN_COLUMN per series right of the
// divider (USAGE-REENGAGEMENT-SPEC D6: Post-purchase and Re-engagement, so 2 x min), else null
// (stacked) -- a narrow window, or a table scrolled so far that the line sits off to one side.
export function guideSplitX(x, width, min = GUIDE_MIN_COLUMN, rightColumns = GUIDE_SERIES.length - 1) {
  if (typeof x !== 'number' || !Number.isFinite(x) || typeof width !== 'number' || !Number.isFinite(width)) {
    return null;
  }
  return x >= min && width - x >= min * rightColumns ? Math.round(x) : null;
}

// DASHBOARDS-POLISH3-SPEC D5: the Languages section's Skipped leaves (the fork's I18N languages,
// English excluded by design -- the section is international transparency).
export const SKIPPED_LANGS = Object.freeze(['fr', 'es', 'de', 'it']);

const SK_PREFIX = 'skipped:';
export const WELCOME_FIELDS = Object.freeze(SERIES_DAYS[SERIES_WELCOME].map((n) => `${SERIES_WELCOME}:${n}`));
export const POST_PURCHASE_FIELDS = Object.freeze(SERIES_DAYS[SERIES_POST_PURCHASE].map((n) => `${SERIES_POST_PURCHASE}:${n}`));
export const SKIPPED_FIELDS = Object.freeze(SKIPPED_LANGS.map((l) => `${SK_PREFIX}${l}`));
const RE_PREFIX = `${SERIES_REENGAGEMENT}:`;
export const REENGAGEMENT_FIELDS = Object.freeze(REENGAGEMENT_DAYS.map((n) => `${RE_PREFIX}${n}`));

// USAGE-REENGAGEMENT-SPEC D6 (Stage 2 F4): the Re-engagement leaf of a field ('reengagement:180' ->
// { days: 180 }); null for any other field or day. NOT a series leaf: seriesLeafOf never matches
// these fields (SERIES_DAYS has no reengagement family), and cell / sortValue / circleLink check
// this first.
export function reengagementLeafOf(field) {
  if (typeof field !== 'string' || !field.startsWith(RE_PREFIX)) {
    return null;
  }
  const days = Number(field.slice(RE_PREFIX.length));
  return REENGAGEMENT_DAYS.includes(days) ? { days } : null;
}

// The series leaf of a field ('welcome:5' -> { family: 'welcome', days: 5 }; 'postPurchase:0' ->
// { family: 'postPurchase', days: 0 }); null for any other field or an off-standard day.
export function seriesLeafOf(field) {
  if (typeof field !== 'string') {
    return null;
  }
  const i = field.indexOf(':');
  if (i < 0) {
    return null;
  }
  const family = field.slice(0, i);
  const days = Number(field.slice(i + 1));
  const std = SERIES_DAYS[family];
  return std && std.includes(days) ? { family, days } : null;
}

export function skippedLangOf(field) {
  if (typeof field !== 'string' || !field.startsWith(SK_PREFIX)) {
    return null;
  }
  const l = field.slice(SK_PREFIX.length);
  return SKIPPED_LANGS.includes(l) ? l : null;
}

// V2 D2: the columns, left to right -- Score and Brand ungrouped, then the sections. POLISH3 D4/D5:
// a section's `items` are its leaf fields in order, a SUB-GROUP ({ group, label, help, fields })
// standing for its leaves under one label (since LIFECYCLE-SERIES-SPEC S9 the No-purchase series over
// Day 0 | 2d | 5d | 10d and the Post-purchase series over Day 0 | 5d | 14d | 30d; the Languages
// section's Skipped over FR ES DE IT, between Journeys and Configurations); `fields` is the flat
// leaf list.
// `small` (G5): the label takes the small header font (Skipped subscribers is long); otherwise it
// matches the single-row headers' size (No purchase).
const group = (key, label, help, fields, small = false) => Object.freeze({
  group: key, label, help, fields, small,
});
const section = (key, label, items) => Object.freeze({
  key, label, items: Object.freeze(items), fields: Object.freeze(items.flatMap((i) => (typeof i === 'string' ? [i] : i.fields))),
});
export const SECTIONS = Object.freeze([
  section('broadcasts', 'dashboard.usage.sectionBroadcasts', ['cadence', 'lastBroadcast']),
  // The Journeys count column retired 2026-10-30: the Score sorts by usage and the series leaves
  // show what it summed.
  section('journeys', 'dashboard.usage.sectionJourneys', [
    group(SERIES_WELCOME, 'dashboard.usage.noPurchase', 'dashboard.usage.noPurchaseHelp', WELCOME_FIELDS),
    group(SERIES_POST_PURCHASE, 'dashboard.usage.postPurchase', 'dashboard.usage.postPurchaseHelp', POST_PURCHASE_FIELDS),
    // USAGE-REENGAGEMENT-SPEC D6: the sunset queues' two asks (the lib's stored usage.reengagement).
    group(SERIES_REENGAGEMENT, 'dashboard.usage.reengagement', 'dashboard.usage.reengagementHelp', REENGAGEMENT_FIELDS),
    // 2026-10-30: the brand's SKU-triggered journeys (the lib's stored `usage.sku`), not scored.
    'sku',
  ]),
  section('languages', 'dashboard.usage.sectionLanguages', [
    // G5 round 2: Skipped subscribers shares the header font size with No purchase (small: false).
    group('skipped', 'dashboard.usage.skipped', 'dashboard.usage.skippedHelp', SKIPPED_FIELDS),
  ]),
  section('configurations', 'dashboard.usage.sectionConfigurations', ['attribution', 'sunset']),
]);

// Every sortable column, in display order.
export const SORT_FIELDS = Object.freeze(['score', 'brand', ...SECTIONS.flatMap((s) => s.fields)]);

// The stoplight columns (V2 D3) -- one circle per state. LIFECYCLE-SERIES-SPEC S9: the series
// leaves, then (USAGE-REENGAGEMENT-SPEC D6) the Re-engagement leaves, then the SKU column.
export const CIRCLE_FIELDS = Object.freeze([...WELCOME_FIELDS, ...POST_PURCHASE_FIELDS, ...REENGAGEMENT_FIELDS, 'sku']);

// POLISH3 D4/D5: the header layout over SECTIONS (the band alternates grey/white by section). Row
// 2 holds every plain leaf (rowspan 2) and every sub-group label (colspan = its leaves); row 3
// the sub-group leaves. `first` marks the cell opening its section (the band's left rule).
// `divide` (2026-10-30) marks an item boundary INSIDE a section that touches a group -- a group
// that is not the section's first item, or a plain leaf right after a group (the SKU column after
// the Post-purchase series) -- drawn as a rule through the header AND every body row.
export function headerLayout(sections = SECTIONS) {
  const row2 = [];
  const row3 = [];
  sections.forEach((s, si) => {
    const band = si % 2 === 0 ? 'grey' : 'white';
    s.items.forEach((it, i) => {
      const divide = i > 0 && (typeof it !== 'string' || typeof s.items[i - 1] !== 'string');
      if (typeof it === 'string') {
        row2.push({
          kind: 'leaf', field: it, band, first: i === 0, divide,
        });
        return;
      }
      row2.push({
        kind: 'group', key: it.group, label: it.label, help: it.help, span: it.fields.length, band, first: i === 0, small: it.small, divide,
      });
      it.fields.forEach((f, fi) => row3.push({
        kind: 'leaf', field: f, band, first: i === 0 && fi === 0, groupStart: fi === 0, divide: divide && fi === 0,
      }));
    });
  });
  return { row2, row3 };
}

// The body columns in display order -- { field, band, first, divide }, the same flags as the
// header's leaves (headerLayout), so a body rule always continues its header rule.
export function bodyColumns(sections = SECTIONS) {
  const { row2, row3 } = headerLayout(sections);
  const leaf = new Map([...row2, ...row3].filter((c) => c.kind === 'leaf').map((c) => [c.field, c]));
  return sections.flatMap((s) => s.fields).map((f) => {
    const c = leaf.get(f);
    return {
      field: f, band: c.band, first: c.first, divide: !!c.divide,
    };
  });
}

// The leaf columns' header label and hover, in display order of every field: label is an i18n key
// with params (a series leaf "Day 0" or "5d"; the Skipped leaves the uppercase language code, a
// language-neutral literal). LIFECYCLE-SERIES-SPEC S9: a series leaf's hover is its D5 description
// (listTrigger.mjs::seriesStepKey -- one home per text).
const COLUMN_META = Object.freeze({
  cadence: { label: 'dashboard.usage.cadence', help: 'dashboard.usage.cadenceHelp' },
  lastBroadcast: { label: 'dashboard.usage.lastBroadcast', help: 'dashboard.usage.lastBroadcastHelp' },
  sku: { label: 'dashboard.usage.sku', help: 'dashboard.usage.skuHelp' },
  attribution: { label: 'dashboard.usage.attribution', help: 'dashboard.usage.attributionHelp' },
  sunset: { label: 'dashboard.usage.sunset', help: 'dashboard.usage.sunsetHelp' },
});

export function columnMeta(field) {
  const re = reengagementLeafOf(field);
  if (re !== null) {
    return { label: 'dashboard.usage.npDays', params: { n: re.days }, help: seriesStepKey(SERIES_REENGAGEMENT, re.days) };
  }
  const leaf = seriesLeafOf(field);
  if (leaf !== null) {
    const help = seriesStepKey(leaf.family, leaf.days);
    return leaf.days === 0 ? { label: 'dashboard.usage.day0', help } : { label: 'dashboard.usage.npDays', params: { n: leaf.days }, help };
  }
  const l = skippedLangOf(field);
  if (l !== null) {
    return { text: l.toUpperCase(), help: 'dashboard.usage.skippedHelp' };
  }
  return COLUMN_META[field] ? { ...COLUMN_META[field] } : { label: field, help: '' };
}

// V2 D2: a fresh session sorts by Score, descending (best first); a stored session sort wins.
export const DEFAULT_STATE = Object.freeze({ sort: Object.freeze({ field: 'score', order: 'desc' }) });

// D7: the sessionStorage key of { sort: {field, order} } (unchanged in V2).
export const STORAGE_KEY = 'dashboard.usage';

export const MISSING = '—';

const OK = 'has-text-success';
const AMBER = 'has-text-warning-dark';
const RED = 'has-text-danger';

// V2 D1/F5: the Score breakdown's display order -- a FIXED list, because jsonb returns the stored
// `parts` keys lexicographically. Rendering order only; every number comes from `parts`.
// LIFECYCLE-SERIES-SPEC S9: the parts are broadcasts, welcomeSeries, postPurchaseSeries,
// attribution and sunset (integrations lib/brand-usage.ts SCORE_WEIGHTS). A part this list does not
// name -- a doc written before the release still carries welcome / purchase / noPurchase -- is
// skipped by the unknown-part rule (scoreTip), never mapped: there is no dual vocabulary.
export const SCORE_PARTS = Object.freeze([
  Object.freeze({ key: 'broadcasts', label: 'dashboard.usage.partBroadcasts' }),
  Object.freeze({ key: 'welcomeSeries', label: 'dashboard.usage.partWelcomeSeries' }),
  Object.freeze({ key: 'postPurchaseSeries', label: 'dashboard.usage.partPostPurchaseSeries' }),
  // USAGE-REENGAGEMENT-SPEC D5/D6.
  Object.freeze({ key: 'reengagement', label: 'dashboard.usage.partReengagement' }),
  Object.freeze({ key: 'attribution', label: 'dashboard.usage.partAttribution' }),
  Object.freeze({ key: 'sunset', label: 'dashboard.usage.partSunset' }),
]);

const BLANK_CELL = Object.freeze({ text: '', cls: '', tooltip: '' });

// boolCell: ✓ green / ✗ red (the header tooltip says "not in use", not "error"); non-boolean blank.
export function boolCell(v) {
  if (v === true) {
    return { glyph: '✓', cls: OK };
  }
  if (v === false) {
    return { glyph: '✗', cls: RED };
  }
  return { glyph: '', cls: '' };
}

// D7: the Last broadcast cell's TEXT color by the stored broadcasts.state (V2 D3: colored
// backgrounds belong to the circles alone).
const BROADCAST_CLASS = Object.freeze({
  ok: OK, due: AMBER, stale: RED, none: RED,
});

const isObj = (v) => !!v && typeof v === 'object' && !Array.isArray(v);
const numOrNull = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : null);
const countOrNull = (v) => (typeof v === 'number' && Number.isInteger(v) && v >= 0 ? v : null);

// DASHBOARDS-POLISH2-SPEC D2: the brand's ACTIVE list ids, ascending -- the lists store rows (array
// or { results }; GET /api/lists?status=active, already permission-filtered) whose brand is the
// row's; an archived row, were one ever present, is left out. [] when none.
export function brandListIds(lists, brand) {
  if (typeof brand !== 'string' || brand === '') {
    return [];
  }
  return storeLists(lists)
    .filter((l) => l && l.brand === brand && l.status !== 'archived' && Number.isInteger(l.id) && l.id > 0)
    .map((l) => l.id)
    .sort((a, b) => a - b);
}

// shapeRow flattens one API row: { brand, label, usage (the stored object, or null), zma, listIds
// (the brand's active list ids, ascending -- the Journeys/broadcast link set), skipped (POLISH3
// D5: the matrix endpoint's live per-language skipped object, or null) }.
export function shapeRow(r, brands, lists = []) {
  const usage = isObj(r && r.usage) ? r.usage : null;
  return {
    brand: r.brand,
    label: brandLabelOf(r.brand, brands),
    usage,
    zma: !!usage && usage.channel === 'zma',
    listIds: brandListIds(lists, r.brand),
    skipped: isObj(r && r.skipped) ? r.skipped : null,
  };
}

// POLISH3 D5: one language's stored skipped entry -- { count, lists } with a non-negative integer
// count and the positive-integer list ids; null when the row does not carry a well-formed one.
export function skippedOf(row, lang) {
  const s = row && isObj(row.skipped) ? row.skipped[lang] : null;
  if (!isObj(s) || !(typeof s.count === 'number' && Number.isInteger(s.count) && s.count >= 0)) {
    return null;
  }
  const ids = Array.isArray(s.lists) ? s.lists.filter((id) => typeof id === 'number' && Number.isInteger(id) && id > 0) : [];
  return { count: s.count, lists: ids };
}

// LIFECYCLE-SERIES-SPEC S9: a series family's stored object -- usage.series.<family> with its
// `steps` array; null when the doc does not carry it (a doc written before the release: every leaf
// of the group renders '—', never a false red 0, until the next daily run).
function seriesOf(u, family) {
  const s = isObj(u) && isObj(u.series) && isObj(u.series[family]) ? u.series[family] : null;
  return s && Array.isArray(s.steps) ? s : null;
}

// The stored leaf of (family, days): its { days, state, rows } entry, or null when absent.
function stepAt(u, family, days) {
  const s = seriesOf(u, family);
  if (!s) {
    return null;
  }
  const st = s.steps.find((x) => isObj(x) && x.days === days);
  return st || null;
}

const stepRows = (st) => (st && Array.isArray(st.rows) ? st.rows.filter(isObj) : []);

// A step row is green when the lib's stored facts say clean -- no error, a list, its automation
// running (the stored `evergreen` state, mapped, never re-derived); amber otherwise.
// The SKU column's stored rows (the lib's `usage.sku.rows`); [] when the doc does not carry them.
const skuRows = (u) => (isObj(u && u.sku) && Array.isArray(u.sku.rows) ? u.sku.rows.filter(isObj) : []);

const rowClean = (r) => !(typeof r.error === 'string' && r.error) && r.listId !== null && r.listId !== undefined && r.evergreen === 'running';

// D2: the Brand Matrix's rows exactly -- the API rows narrowed by the launch filter over the rows'
// own health documents, then flattened. The server has already applied the brand scope. `lists`
// is the lists store (POLISH2 D2's link set).
export function shapeRows(apiRows, { brands = [], launch = 'all', lists = [] } = {}) {
  const rows = (Array.isArray(apiRows) ? apiRows : []).filter((r) => r && typeof r.brand === 'string' && r.brand !== '');
  const docs = healthDocs(rows);
  return rows.filter((r) => matchesLaunch(r.brand, launch, docs)).map((r) => shapeRow(r, brands, lists));
}

// The columns a ZMA row does not score (its sends live in ZMA -- a red would be false, v1 I6),
// plus the Score (V2 D1: a ZMA row's score is null).
// USAGE-REENGAGEMENT-SPEC D6 (Stage 2 F6): the Re-engagement leaves too -- a ZMA brand has no
// listmonk sunset queue, like welcome Day 0.
const ZMA_DIMMED = Object.freeze(['score', WELCOME_FIELDS[0], ...REENGAGEMENT_FIELDS, 'lastBroadcast', 'cadence']);

// USAGE-REENGAGEMENT-SPEC D1: the stored step of a Re-engagement leaf -- usage.reengagement.steps'
// entry at `days`; `undefined` when the doc carries no reengagement object (a doc written before the
// release: '—'), null when it does but holds no entry at that day.
function reStepAt(u, days) {
  const re = isObj(u) && isObj(u.reengagement) && Array.isArray(u.reengagement.steps) ? u.reengagement : null;
  if (!re) {
    return undefined;
  }
  return re.steps.find((s) => isObj(s) && s.days === days) || null;
}

const RE_COLORS = Object.freeze(['green', 'amber', 'red']);

// The stored circle of a step ({ color, n }, rendered verbatim -- D3, never through circlesOf), or
// null when malformed.
function reCircle(st) {
  const c = st && isObj(st.circle) ? st.circle : null;
  return c && RE_COLORS.includes(c.color) && countOrNull(c.n) !== null ? { color: c.color, n: c.n } : null;
}

// D3: a Re-engagement circle's accessible name -- green "{n} running", amber "needs attention", red
// "no re-engagement list" (its own words; the series circles' circleName means something else).
export function reCircleName(color, n, t = (k) => k) {
  if (color === 'green') {
    return t('dashboard.usage.reCircleRunning', { n });
  }
  if (color === 'amber') {
    return t('dashboard.usage.reCircleAttention');
  }
  return t('dashboard.usage.reCircleNone');
}

// The welcome tooltip: the master list's evergreen campaigns ("#id status"), or none.
export function welcomeTip(w, t = (k) => k) {
  const camps = Array.isArray(w && w.campaigns) ? w.campaigns : [];
  if (!camps.length) {
    return t('dashboard.usage.noEvergreen');
  }
  return camps.map((c) => `#${c.id} ${c.status}`).join(', ');
}

// D7: the trigger-family tooltip -- each stored row's slug with its parser error, or its list and
// that list's evergreen state. Rows in the stored order, one per line.
export function triggerTip(p, t = (k) => k) {
  const rows = Array.isArray(p && p.rows) ? p.rows : [];
  return rows.map((r) => {
    if (typeof r.error === 'string' && r.error) {
      return `${r.slug}: ${r.error}`;
    }
    if (r.listId === null || r.listId === undefined) {
      return `${r.slug}: ${t('dashboard.usage.noList')}`;
    }
    return `${r.slug}: ${t('dashboard.usage.listEvergreen', { list: r.listId, state: r.evergreen })}`;
  }).join('\n');
}

// S9: the group's chips, after its LAST leaf -- one grey "Nd" chip (USAGE-DECISIONS D7) per stored legacy row
// with a whole `days` (its hover the legacy description, then the row), and one amber ⚠ per stored
// row with NO parseable `days` (the lib stores null; its parser error in the hover -- the
// POLISH3 F5 badge). [] on every other field, a pre-release doc, or a usage-less row. `kind` is
// 'legacy' or 'warn' (the Vue styles them).
export function cellBadges(row, field, t = (k) => k) {
  const family = [SERIES_WELCOME, SERIES_POST_PURCHASE].find((f) => {
    const fields = f === SERIES_WELCOME ? WELCOME_FIELDS : POST_PURCHASE_FIELDS;
    return field === fields[fields.length - 1];
  });
  const s = family ? seriesOf(row && row.usage, family) : null;
  if (!s || !Array.isArray(s.legacy)) {
    return [];
  }
  return s.legacy.filter(isObj).map((r) => {
    if (typeof r.days === 'number' && Number.isInteger(r.days) && r.days > 0) {
      const badge = {
        kind: 'legacy',
        glyph: t('dashboard.usage.legacyChip', { n: r.days }),
        tooltip: `${t(seriesStepKey(family, r.days))}\n${triggerTip({ rows: [r] }, t)}`,
      };
      // Fork (usage decisions, integrations USAGE-DECISIONS-SPEC D7) -- the chip opens the row's
      // stored evergreens (lib/brand-usage.ts `TriggerRowUsage.campaigns`), the Day 0 circle's
      // link shape; no `link` when the row stores none or predates the field. Never a hardcoded id.
      const ids = Array.isArray(r.campaigns)
        ? r.campaigns.filter(isObj).map((c) => c.id).filter((v) => typeof v === 'number' && Number.isInteger(v) && v > 0)
        : [];
      if (ids.length) {
        badge.link = { id: [...new Set(ids)].sort((a, b) => a - b).map(String), evergreen: 'all' };
      }
      return badge;
    }
    return {
      kind: 'warn',
      glyph: '⚠',
      tooltip: t('dashboard.usage.npInvalid', { slug: String(r.slug), error: typeof r.error === 'string' && r.error ? r.error : t('dashboard.usage.npNoDays') }),
    };
  });
}

// D7: the Last broadcast tooltip -- cadence, ratio and windowCount.
export function broadcastTip(b, t = (k) => k) {
  const cadence = numOrNull(b && b.cadenceDays);
  const ratio = numOrNull(b && b.ratio);
  return t('dashboard.usage.broadcastHelp', {
    cadence: cadence === null ? MISSING : cadence.toFixed(1),
    ratio: ratio === null ? MISSING : ratio.toFixed(2),
    count: numOrNull(b && b.windowCount) ?? 0,
  });
}

// V2 D1: the Score breakdown -- "Broadcasts 40/40 · Welcome 5/15 · …", every number the stored
// part's points/max verbatim, in SCORE_PARTS' fixed order. A part the doc lacks is skipped.
export function scoreTip(score, t = (k) => k) {
  const parts = isObj(score && score.parts) ? score.parts : {};
  return SCORE_PARTS.filter((p) => isObj(parts[p.key]))
    .map((p) => `${t(p.label)} ${parts[p.key].points}/${parts[p.key].max}`)
    .join(' · ');
}

// V2 D3: the stored per-state numbers of a stoplight -- { green, amber } from the welcome family's
// stored `counts` (the No-purchase series Day 0 leaf, `welcome`) or the SKU column's (`sku`); null when
// the doc does not carry them (a doc written before them). The fork classifies nothing.
export function storedCounts(usage, field) {
  if (!isObj(usage)) {
    return null;
  }
  let green = null;
  let amber = null;
  if (field === 'welcome' || field === 'sku') {
    const c = isObj(usage[field]) && isObj(usage[field].counts) ? usage[field].counts : {};
    green = countOrNull(c.green);
    amber = countOrNull(c.amber);
  }
  return green === null || amber === null ? null : { green, amber };
}

// V2 D3/I3: the circles of one stoplight cell -- ONE circle per state, the stored count inside:
// green (running/active) and amber (configured but idle), each omitted at 0; a single red circle
// showing 0 when both are 0. Returns [{ color, n }].
export function circlesOf(counts) {
  if (!counts) {
    return [];
  }
  const out = [];
  if (counts.green > 0) {
    out.push({ color: 'green', n: counts.green });
  }
  if (counts.amber > 0) {
    out.push({ color: 'amber', n: counts.amber });
  }
  return out.length ? out : [{ color: 'red', n: 0 }];
}

// circleName (2026-10-30) is ONE circle's accessible name -- since each circle is its own link, two
// links in a cell must not share a name: green "N running", amber "N not running", red "none in use".
export function circleName(color, n, t = (k) => k) {
  if (color === 'green') {
    return t('dashboard.usage.circleRunning', { n });
  }
  if (color === 'amber') {
    return t('dashboard.usage.circleIdle', { n });
  }
  return t('dashboard.usage.circleNone');
}

// V2 D3/I3: the words every circle of the cell carries -- color is never the only carrier:
// "1 running, 2 not running", or "none in use".
export function circleWords(counts, t = (k) => k) {
  if (!counts) {
    return '';
  }
  const parts = [];
  if (counts.green > 0) {
    parts.push(t('dashboard.usage.circleRunning', { n: counts.green }));
  }
  if (counts.amber > 0) {
    parts.push(t('dashboard.usage.circleIdle', { n: counts.amber }));
  }
  return parts.length ? parts.join(', ') : t('dashboard.usage.circleNone');
}

const posInt = (v) => typeof v === 'number' && Number.isInteger(v) && v > 0;

// MATRIX-POLISH-SPEC D10.3: the usage doc's broadcasts.lastIds (the newest send event's campaign
// ids, written by the BrandHealth Lambda) when it is a non-empty array of positive integers, else
// null (absent on a doc written before the field, or malformed).
function lastIdsOf(u) {
  const b = isObj(u) && isObj(u.broadcasts) ? u.broadcasts : null;
  const ids = b && Array.isArray(b.lastIds) ? b.lastIds : null;
  return ids && ids.length > 0 && ids.every(posInt) ? ids : null;
}

// circleLink (2026-10-30) is ONE circle's link: the click filters to what THAT circle counts, never
// the whole cell. Welcome Day 0 -- its stored campaigns (`welcome.campaigns`), green the running
// ones, amber the rest (the lib's welcome counts split exactly so) -> { id, evergreen: 'all' }.
// Every other series leaf and SKU -- the stored rows' lists, green the clean rows (rowClean, the
// lib's isClean), amber the others -> { list_id, evergreen: 'true' }; the rows store no campaign
// ids, so a list is the finest filter, and an invalid row with no list is counted but cannot be
// linked (an amber circle of only such rows does not link). Red (none) never links, nor a
// ZMA-dimmed or `—` cell.
export function circleLink(row, field, color) {
  const u = row && row.usage;
  if (!u || (color !== 'green' && color !== 'amber') || (row.zma && ZMA_DIMMED.includes(field))) {
    return null;
  }
  // USAGE-REENGAGEMENT-SPEC D6: a Re-engagement leaf's stored circle (green or amber) opens the
  // queue list's automations -- { list_id: [listId], evergreen: 'true' }; a circle of another color
  // than the stored one, a red, or a step with no list never links.
  const re = reengagementLeafOf(field);
  if (re !== null) {
    const st = reStepAt(u, re.days);
    const c = reCircle(st);
    return c && c.color === color && posInt(st.listId) ? { list_id: [String(st.listId)], evergreen: 'true' } : null;
  }
  const green = color === 'green';
  const leaf = seriesLeafOf(field);
  if (leaf !== null && leaf.family === SERIES_WELCOME && leaf.days === 0) {
    const camps = isObj(u.welcome) && Array.isArray(u.welcome.campaigns) ? u.welcome.campaigns.filter(isObj) : [];
    const ids = camps.filter((c) => (c.status === 'running') === green).map((c) => c.id).filter(posInt);
    // evergreen 'all': an id scope forces All on the Campaigns page anyway (routeEvergreen), the
    // Last broadcast link's shape.
    return ids.length ? { id: [...new Set(ids)].sort((a, b) => a - b).map(String), evergreen: 'all' } : null;
  }
  let rows = null;
  if (leaf !== null) {
    rows = seriesOf(u, leaf.family) ? stepRows(stepAt(u, leaf.family, leaf.days)) : null;
  } else if (field === 'sku') {
    rows = storedCounts(u, 'sku') ? skuRows(u) : null;
  }
  if (!rows) {
    return null;
  }
  const ids = [...new Set(rows.filter((r) => rowClean(r) === green).map((r) => r.listId).filter(posInt))].sort((a, b) => a - b);
  return ids.length ? { list_id: ids.map(String), evergreen: 'true' } : null;
}

// cellLink is a TEXT cell's link query (the circle cells link per circle, circleLink). DASHBOARDS-
// POLISH2-SPEC D2/I3 + POLISH3 D2: Cadence / Last broadcast open the BRAND's active lists in the
// Broadcasts scope with status=finished -- { evergreen: 'false', status, list_id: [ids] }, the row's
// listIds as strings, ascending; an empty set, a ZMA-dimmed or '—' / blank cell never links.
// MATRIX-POLISH D10.3: Last broadcast opens the newest send event's campaigns when stored. POLISH3
// D5: a Skipped leaf with a count > 0 links { list_id: that language's skipping lists, evergreen:
// 'all' }, from the row's live `skipped` (independent of usage), never on a ZMA-dimmed row or at 0.
// null = no link.
export function cellLink(row, field) {
  const lang = skippedLangOf(field);
  if (lang !== null) {
    const s = row && !row.zma ? skippedOf(row, lang) : null;
    return s && s.count > 0 && s.lists.length ? { list_id: s.lists.map(String), evergreen: 'all' } : null;
  }
  const u = row && row.usage;
  if (!u || (row.zma && ZMA_DIMMED.includes(field))) {
    return null;
  }
  // The circle cells (series leaves, SKU) link per circle -- circleLink (2026-10-30) -- never here.
  // Fork (matrix polish, integrations MATRIX-POLISH-SPEC D10.3) -- Last broadcast opens the newest
  // send event's campaigns (the stored broadcasts.lastIds) under the All kind; a doc written before
  // lastIds existed (or with a malformed one) keeps the brand-lists link below.
  if (field === 'lastBroadcast') {
    const last = lastIdsOf(u);
    if (last) {
      return { id: last.map(String), evergreen: 'all' };
    }
  }
  if (field === 'cadence' || field === 'lastBroadcast') {
    const ids = (Array.isArray(row.listIds) ? row.listIds : []).filter(posInt);
    if (!ids.length) {
      return null;
    }
    const b = isObj(u.broadcasts) ? u.broadcasts : {};
    const shown = field === 'cadence' ? numOrNull(b.cadenceDays) !== null : (b.state === 'none' || numOrNull(b.daysSince) !== null);
    return shown ? { evergreen: 'false', status: 'finished', list_id: ids.map(String) } : null;
  }
  return null;
}

// DASHBOARDS-POLISH-SPEC D2; POLISH2-SPEC D2 (stage 2 F3): the note a linked broadcast cell's
// tooltip carries -- the link opens the brand's FINISHED broadcasts (POLISH3 D2; every one of the
// brand's active lists). POLISH3 D5: a linked Skipped cell notes its landing (those lists'
// campaigns of all ages, both kinds). '' for every other cell, and for an unlinked one.
export function linkTip(row, field, t = (k) => k) {
  // MATRIX-POLISH-SPEC D10.3: the Last broadcast link names the last send's campaigns.
  if (field === 'lastBroadcast' && row && row.usage && lastIdsOf(row.usage) && cellLink(row, field)) {
    return t('dashboard.usage.lastBroadcastLinkHelp');
  }
  if ((field === 'cadence' || field === 'lastBroadcast') && cellLink(row, field)) {
    return t('dashboard.usage.broadcastLinkHelp');
  }
  if (skippedLangOf(field) !== null && cellLink(row, field)) {
    return t('dashboard.usage.skippedLinkHelp');
  }
  return '';
}

// cell is one column's presentation for a row: { text, cls, tooltip, circles } -- tooltip a plain
// string (t formats it -- the Vue passes (k, p) => this.$t(k, p)); circles (stoplight columns
// only) [{ color, n }], each to carry `tooltip`. A row without usage is blank (v1 D7); a ZMA row
// shows '—' in Score/Welcome/Journeys/Last broadcast/Cadence.
//
// POLISH3 D5: a Skipped leaf renders from the row's live `skipped` whether or not the row has
// usage -- a red count (linked) when > 0, a plain grey 0 at 0 (§10 A11, replacing G5's ✓), '—'
// on a ZMA-dimmed row (or a malformed entry). D4: a No purchase leaf -- the circle(s) of the step(s)
// at exactly that day (green running, amber idle/invalid), a red 0 when none; '—' for the whole
// sub-group on a pre-`days` doc.
export function cell(row, field, t = (k) => k) {
  const lang = skippedLangOf(field);
  if (lang !== null) {
    if (row && row.zma) {
      return {
        text: MISSING, cls: '', tooltip: t('dashboard.usage.zmaHelp'), circles: [],
      };
    }
    const s = skippedOf(row, lang);
    if (!s) {
      return {
        text: MISSING, cls: '', tooltip: '', circles: [],
      };
    }
    if (s.count > 0) {
      return {
        text: String(s.count), cls: RED, tooltip: t('dashboard.usage.skippedCell', { n: s.count, lists: s.lists.join(', ') }), circles: [],
      };
    }
    // 0 renders a plain grey "0" (2026-10-30, replacing G5's green ✓): the column is a COUNT of
    // skipped people, and 0 has three causes -- a campaign in that language ran, nobody of that
    // language is on the brand's lists, or no campaign activity in the window -- so a ✓ implied a
    // coverage it could not know. The tooltip names the causes; the sort key stays the count 0.
    return {
      text: '0', cls: 'has-text-grey', tooltip: t('dashboard.usage.skippedZero'), circles: [],
    };
  }
  const u = row && row.usage;
  if (!u) {
    return { ...BLANK_CELL, circles: [] };
  }
  if (row.zma && ZMA_DIMMED.includes(field)) {
    return {
      text: MISSING, cls: '', tooltip: t('dashboard.usage.zmaHelp'), circles: [],
    };
  }
  // USAGE-REENGAGEMENT-SPEC D6: a Re-engagement leaf -- the stored circle verbatim (each circle
  // carries its own accessible `name`, D3); the tooltip is the step text, the circle's name, "{n}
  // waiting" when `waiting` is a number, and "Not covered: FR, ES" when `uncovered` is non-empty. A
  // doc without `reengagement` renders '—'.
  const re = reengagementLeafOf(field);
  if (re !== null) {
    const desc = t(seriesStepKey(SERIES_REENGAGEMENT, re.days));
    const st = reStepAt(u, re.days);
    const c = reCircle(st);
    if (!c) {
      return {
        text: MISSING, cls: '', tooltip: `${desc}\n${t('dashboard.usage.seriesMissing')}`, circles: [],
      };
    }
    const name = reCircleName(c.color, c.n, t);
    const waiting = countOrNull(st.waiting);
    const uncovered = Array.isArray(st.uncovered) ? st.uncovered.filter((l) => typeof l === 'string' && l !== '') : [];
    const lines = [desc, name];
    if (waiting !== null) {
      lines.push(t('dashboard.usage.reWaiting', { n: waiting }));
    }
    if (uncovered.length) {
      lines.push(t('dashboard.usage.reUncovered', { langs: uncovered.map((l) => l.toUpperCase()).join(', ') }));
    }
    return {
      text: '', cls: '', tooltip: lines.join('\n'), circles: [{ color: c.color, n: c.n, name }],
    };
  }
  // LIFECYCLE-SERIES-SPEC S9: a series leaf -- its D5 description first, then the circle words and
  // the rows. Welcome Day 0 is the welcome family's stored counts and campaigns (today's welcome
  // state); every other leaf the circle(s) of its stored rows (green clean, amber idle/invalid), a
  // red 0 when none. A doc without `series` renders the leaf '—'.
  const leaf = seriesLeafOf(field);
  if (leaf !== null) {
    const desc = t(seriesStepKey(leaf.family, leaf.days));
    if (!seriesOf(u, leaf.family)) {
      return {
        text: MISSING, cls: '', tooltip: `${desc}\n${t('dashboard.usage.seriesMissing')}`, circles: [],
      };
    }
    let counts;
    let detail;
    if (leaf.family === SERIES_WELCOME && leaf.days === 0) {
      counts = storedCounts(u, 'welcome');
      if (!counts) {
        return {
          text: MISSING, cls: '', tooltip: `${desc}\n${t('dashboard.usage.countsMissing')}`, circles: [],
        };
      }
      detail = welcomeTip(isObj(u.welcome) ? u.welcome : {}, t);
    } else {
      const at = stepRows(stepAt(u, leaf.family, leaf.days));
      counts = { green: at.filter(rowClean).length, amber: at.filter((r) => !rowClean(r)).length };
      detail = triggerTip({ rows: at }, t);
    }
    const words = circleWords(counts, t);
    return {
      text: '', cls: '', tooltip: [desc, words, detail].filter(Boolean).join('\n'), circles: circlesOf(counts),
    };
  }
  switch (field) {
    case 'score': {
      const total = numOrNull(isObj(u.score) ? u.score.total : null);
      if (total === null) {
        return {
          text: MISSING, cls: '', tooltip: t('dashboard.usage.scoreMissing'), circles: [],
        };
      }
      return {
        text: String(total), cls: '', tooltip: scoreTip(u.score, t), circles: [],
      };
    }
    case 'sku': {
      // The stored counts (green running, amber idle/invalid), then each row. A brand with NO SKU
      // journey shows '—', never a red 0 (user, 2026-10-30): the journey is optional -- it may or
      // may not apply to a brand -- so its absence is not a campaign gap like the series leaves'.
      const counts = storedCounts(u, field);
      if (!counts) {
        return {
          text: MISSING, cls: '', tooltip: t('dashboard.usage.countsMissing'), circles: [],
        };
      }
      if (counts.green + counts.amber === 0) {
        return {
          text: MISSING, cls: '', tooltip: t('dashboard.usage.skuNone'), circles: [],
        };
      }
      return {
        text: '', cls: '', tooltip: [circleWords(counts, t), triggerTip({ rows: skuRows(u) }, t)].filter(Boolean).join('\n'), circles: circlesOf(counts),
      };
    }
    case 'sunset':
    case 'attribution': {
      const c = boolCell(u[field]);
      return {
        text: c.glyph, cls: c.cls, tooltip: '', circles: [],
      };
    }
    case 'lastBroadcast': {
      const b = isObj(u.broadcasts) ? u.broadcasts : {};
      const cls = Object.prototype.hasOwnProperty.call(BROADCAST_CLASS, b.state) ? BROADCAST_CLASS[b.state] : '';
      if (b.state === 'none') {
        return {
          text: t('dashboard.usage.never'), cls, tooltip: broadcastTip(b, t), circles: [],
        };
      }
      const since = numOrNull(b.daysSince);
      if (since === null) {
        return { ...BLANK_CELL, circles: [] };
      }
      return {
        text: t('dashboard.usage.daysAgo', { n: Math.floor(since) }), cls, tooltip: broadcastTip(b, t), circles: [],
      };
    }
    case 'cadence': {
      const b = isObj(u.broadcasts) ? u.broadcasts : {};
      const c = numOrNull(b.cadenceDays);
      return {
        text: c === null ? MISSING : t('dashboard.usage.cadenceDays', { n: Math.round(c) }), cls: '', tooltip: '', circles: [],
      };
    }
    default:
      return { ...BLANK_CELL, circles: [] };
  }
}

// D1: the pane note's date -- the newest stored `asOf` (YYYY-MM-DD) among the rendered rows'
// usage objects, or null when no row carries one (the Vue then shows the dateless note).
export function latestAsOf(rows) {
  let best = null;
  (Array.isArray(rows) ? rows : []).forEach((r) => {
    const d = r && r.usage && r.usage.asOf;
    if (typeof d === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(d) && (best === null || d > best)) {
      best = d;
    }
  });
  return best;
}

// V2 D5/I4: the run's time as Mountain time, DST-correct. RUN_TIME_UTC mirrors the BrandHealth
// Lambda's daily schedule (integrations cdk/ BrandHealth rule, 15:30 UTC -- a comment there names
// this constant; no test spans the repos). Formatted through Intl with the America/Denver zone,
// never a hardcoded offset (8:30 AM MST in winter, 9:30 AM MDT in summer). null when asOf is not
// a YYYY-MM-DD day or Intl refuses.
export const RUN_TIME_UTC = '15:30:00Z';
export const MOUNTAIN_ZONE = 'America/Denver';

export function mountainRunTime(asOf, locale = 'en-US') {
  if (typeof asOf !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(asOf)) {
    return null;
  }
  const d = new Date(`${asOf}T${RUN_TIME_UTC}`);
  if (Number.isNaN(d.getTime())) {
    return null;
  }
  try {
    return new Intl.DateTimeFormat(locale, {
      timeZone: MOUNTAIN_ZONE,
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      timeZoneName: 'short',
    }).format(d);
  } catch {
    return null;
  }
}

// The default direction of a column on its first click: Brand ascending; Score and the date
// column descending (best score / longest silence first); the stoplight and boolean columns
// descending too -- their sort keys are v1's BADNESS, so descending is worst first (stage 2 F8).
export function defaultOrder(field) {
  return field === 'brand' ? 'asc' : 'desc';
}

// V2 D2: clickSort is the sort after a header click (the pane owns its own header -- the Brand
// Matrix's brandMatrix.mjs::clickSort pattern; no Buefy direction in play). A click on a NEW
// column takes that column's default direction; a click on the current column toggles it; an
// unsortable field leaves the sort unchanged.
export function clickSort(current, field) {
  const cur = current || DEFAULT_STATE.sort;
  if (!SORT_FIELDS.includes(field)) {
    return { ...cur };
  }
  if (cur.field !== field) {
    return { field, order: defaultOrder(field) };
  }
  return { field, order: cur.order === 'asc' ? 'desc' : 'asc' };
}

// Sort keys: the Score by its stored total; states and booleans as BADNESS (higher = worse, so desc
// is worst first -- v1's comparators exactly); the SKU column by its stored count; cadence by days; Last
// broadcast by daysSince with a never-broadcast row above every number. null = missing (no usage,
// no stored score, a ZMA-dimmed column, an unrecognized value) -- ranks last in BOTH orders.
const BADNESS = Object.freeze({ active: 0, idle: 1, none: 2 });

//
// LIFECYCLE-SERIES-SPEC S9: a series leaf by badness (green 0, any amber 1, absent red 2; '—' null). D5: a
// Skipped leaf by its count (desc = most skipped first); ZMA '—' or a missing entry null -- it
// does not need usage.
export function sortValue(row, field) {
  const lang = skippedLangOf(field);
  if (lang !== null) {
    const s = row && !row.zma ? skippedOf(row, lang) : null;
    return s ? s.count : null;
  }
  const u = row && row.usage;
  if (!u || (row.zma && ZMA_DIMMED.includes(field))) {
    return null;
  }
  // USAGE-REENGAGEMENT-SPEC D6: a Re-engagement leaf by its stored state's rank (active 0, idle 1,
  // none 2, as a series leaf sorts); '—' (no reengagement object, or an unrecognized state) null.
  const re = reengagementLeafOf(field);
  if (re !== null) {
    const st = reStepAt(u, re.days);
    const s = st ? st.state : undefined;
    return Object.prototype.hasOwnProperty.call(BADNESS, s) ? BADNESS[s] : null;
  }
  // S9: a series leaf by badness -- Welcome Day 0 by the stored welcome state; every other leaf
  // green 0, any amber 1, absent red 2; '—' (no series) null.
  const leaf = seriesLeafOf(field);
  if (leaf !== null) {
    if (!seriesOf(u, leaf.family)) {
      return null;
    }
    if (leaf.family === SERIES_WELCOME && leaf.days === 0) {
      const s = isObj(u.welcome) ? u.welcome.state : undefined;
      return Object.prototype.hasOwnProperty.call(BADNESS, s) ? BADNESS[s] : null;
    }
    const at = stepRows(stepAt(u, leaf.family, leaf.days));
    if (!at.length) {
      return 2;
    }
    return at.every(rowClean) ? 0 : 1;
  }
  switch (field) {
    case 'score':
      return numOrNull(isObj(u.score) ? u.score.total : null);
    case 'sku': {
      // By how many SKU journeys the brand has (desc = most first); '—' (no stored counts) null.
      const c = storedCounts(u, field);
      return c ? c.green + c.amber : null;
    }
    case 'sunset':
    case 'attribution':
      if (typeof u[field] !== 'boolean') {
        return null;
      }
      return u[field] ? 0 : 1;
    case 'cadence':
      return numOrNull(isObj(u.broadcasts) ? u.broadcasts.cadenceDays : null);
    case 'lastBroadcast': {
      const b = isObj(u.broadcasts) ? u.broadcasts : {};
      if (b.state === 'none') {
        return Infinity;
      }
      return numOrNull(b.daysSince);
    }
    default:
      return null;
  }
}

// I7 / V2 I2: sortRows -- the requested column and direction; missing last in both orders; ties
// by name.
export function sortRows(rows, sort = DEFAULT_STATE.sort, locale = 'en') {
  const field = SORT_FIELDS.includes(sort && sort.field) ? sort.field : DEFAULT_STATE.sort.field;
  const dir = sort && sort.order === 'asc' ? 1 : -1;
  const byName = (a, b) => a.label.localeCompare(b.label, locale, { sensitivity: 'base' }) || a.brand.localeCompare(b.brand);
  return [...(Array.isArray(rows) ? rows : [])].sort((a, b) => {
    if (field === 'brand') {
      return byName(a, b) * dir;
    }
    const va = sortValue(a, field);
    const vb = sortValue(b, field);
    if (va === null || vb === null) {
      if (va === vb) {
        return byName(a, b);
      }
      return va === null ? 1 : -1;
    }
    if (va === vb) {
      return byName(a, b);
    }
    return (va < vb ? -1 : 1) * dir;
  });
}

// The persisted state, validated -- anything unreadable is the default.
export function normalizeState(s) {
  const sort = s && s.sort;
  const ok = sort && SORT_FIELDS.includes(sort.field) && (sort.order === 'asc' || sort.order === 'desc');
  return { sort: ok ? { field: sort.field, order: sort.order } : { ...DEFAULT_STATE.sort } };
}

// I7 (matrix I7's rule): loadState reads the session state from store (the Vue passes
// brandMatrix.mjs::sessionStore(window)); a missing store, a throwing accessor or bad JSON is the
// default, never an error.
export function loadState(store) {
  try {
    const raw = store ? store.getItem(STORAGE_KEY) : null;
    return normalizeState(raw ? JSON.parse(raw) : null);
  } catch {
    return normalizeState(null);
  }
}

// saveState writes the session state; a failing store is ignored. Returns whether it wrote.
export function saveState(store, state) {
  try {
    if (!store) {
      return false;
    }
    store.setItem(STORAGE_KEY, JSON.stringify(normalizeState(state)));
    return true;
  } catch {
    return false;
  }
}
