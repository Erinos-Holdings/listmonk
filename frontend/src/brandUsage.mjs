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

// V2 D2: the columns, left to right -- Score and Brand ungrouped, then the three sections.
export const SECTIONS = Object.freeze([
  Object.freeze({ key: 'broadcasts', label: 'dashboard.usage.sectionBroadcasts', fields: Object.freeze(['cadence', 'lastBroadcast']) }),
  Object.freeze({ key: 'journeys', label: 'dashboard.usage.sectionJourneys', fields: Object.freeze(['journeys', 'welcome', 'purchase', 'noPurchase']) }),
  Object.freeze({ key: 'configurations', label: 'dashboard.usage.sectionConfigurations', fields: Object.freeze(['attribution', 'sunset']) }),
]);

// Every sortable column, in display order.
export const SORT_FIELDS = Object.freeze(['score', 'brand', ...SECTIONS.flatMap((s) => s.fields)]);

// The stoplight columns (V2 D3) -- one circle per state.
export const CIRCLE_FIELDS = Object.freeze(['journeys', 'welcome', 'purchase', 'noPurchase']);

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
export const SCORE_PARTS = Object.freeze([
  Object.freeze({ key: 'broadcasts', label: 'dashboard.usage.partBroadcasts' }),
  Object.freeze({ key: 'welcome', label: 'dashboard.usage.partWelcome' }),
  Object.freeze({ key: 'purchase', label: 'dashboard.usage.partPurchase' }),
  Object.freeze({ key: 'noPurchase', label: 'dashboard.usage.partNoPurchase' }),
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
// (the brand's active list ids, ascending -- the Journeys/broadcast link set) }.
export function shapeRow(r, brands, lists = []) {
  const usage = isObj(r && r.usage) ? r.usage : null;
  return {
    brand: r.brand,
    label: brandLabelOf(r.brand, brands),
    usage,
    zma: !!usage && usage.channel === 'zma',
    listIds: brandListIds(lists, r.brand),
  };
}

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
const ZMA_DIMMED = Object.freeze(['score', 'welcome', 'journeys', 'lastBroadcast', 'cadence']);

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

// V2 D3: the stored per-state numbers of a stoplight column -- { green, amber } from the family's
// stored `counts` (Welcome, 1st purchase, No purchase) or Journeys' stored { running, idle }; null
// when the doc does not carry them (a doc written before V2). The fork classifies nothing.
export function storedCounts(usage, field) {
  if (!isObj(usage)) {
    return null;
  }
  let green = null;
  let amber = null;
  if (field === 'journeys') {
    const j = isObj(usage.journeys) ? usage.journeys : {};
    green = countOrNull(j.running);
    amber = countOrNull(j.idle);
  } else if (field === 'welcome' || field === 'purchase' || field === 'noPurchase') {
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

// V2 D4: a stoplight cell's link query -- the Campaigns page with the list preselected AND the
// Automations scope (welcome/trigger campaigns are evergreens). Welcome links its master list; a
// 1st purchase / No purchase cell links only when the family has exactly ONE row (with a list).
// DASHBOARDS-POLISH2-SPEC D2/I3 (superseding POLISH D2's master-list / no-list landings): the
// Journeys circles link the Automations scope and the Cadence / Last broadcast cells the
// Broadcasts scope, both filtered to the BRAND's active lists -- { evergreen, list_id: [ids] }, the
// row's listIds as strings, ascending (a repeated list_id: the Campaigns page's route list scope,
// or its single filter when the brand has one list). An empty set does not link. A ZMA-dimmed
// cell, a '—' / blank cell (no stored value) and the booleans never link. null = no link.
export function cellLink(row, field) {
  const u = row && row.usage;
  if (!u || (row.zma && ZMA_DIMMED.includes(field))) {
    return null;
  }
  if (field === 'journeys' || field === 'cadence' || field === 'lastBroadcast') {
    const ids = (Array.isArray(row.listIds) ? row.listIds : []).filter(posInt);
    if (!ids.length) {
      return null;
    }
    if (field === 'journeys') {
      return storedCounts(u, field) ? { evergreen: 'true', list_id: ids.map(String) } : null;
    }
    const b = isObj(u.broadcasts) ? u.broadcasts : {};
    const shown = field === 'cadence' ? numOrNull(b.cadenceDays) !== null : (b.state === 'none' || numOrNull(b.daysSince) !== null);
    return shown ? { evergreen: 'false', list_id: ids.map(String) } : null;
  }
  let listId = null;
  if (field === 'welcome') {
    listId = isObj(u.welcome) ? u.welcome.listId : null;
  } else if (field === 'purchase' || field === 'noPurchase') {
    const rows = isObj(u[field]) && Array.isArray(u[field].rows) ? u[field].rows : [];
    listId = rows.length === 1 ? rows[0].listId : null;
  }
  if (!posInt(listId)) {
    return null;
  }
  return { list_id: String(listId), evergreen: 'true' };
}

// DASHBOARDS-POLISH-SPEC D2; POLISH2-SPEC D2 (stage 2 F3): the note a linked broadcast cell's
// tooltip carries -- the link opens the brand's broadcasts (every one of the brand's active lists).
// '' for every other cell, and for an unlinked one.
export function linkTip(row, field, t = (k) => k) {
  if ((field === 'cadence' || field === 'lastBroadcast') && cellLink(row, field)) {
    return t('dashboard.usage.broadcastLinkHelp');
  }
  return '';
}

// cell is one column's presentation for a row: { text, cls, tooltip, circles } -- tooltip a plain
// string (t formats it -- the Vue passes (k, p) => this.$t(k, p)); circles (stoplight columns
// only) [{ color, n }], each to carry `tooltip`. A row without usage is blank (v1 D7); a ZMA row
// shows '—' in Score/Welcome/Journeys/Last broadcast/Cadence.
export function cell(row, field, t = (k) => k) {
  const u = row && row.usage;
  if (!u) {
    return { ...BLANK_CELL, circles: [] };
  }
  if (row.zma && ZMA_DIMMED.includes(field)) {
    return {
      text: MISSING, cls: '', tooltip: t('dashboard.usage.zmaHelp'), circles: [],
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
    case 'journeys':
    case 'welcome':
    case 'purchase':
    case 'noPurchase': {
      const counts = storedCounts(u, field);
      if (!counts) {
        return {
          text: MISSING, cls: '', tooltip: t('dashboard.usage.countsMissing'), circles: [],
        };
      }
      let detail = '';
      if (field === 'welcome') {
        detail = welcomeTip(isObj(u.welcome) ? u.welcome : {}, t);
      } else if (field !== 'journeys') {
        detail = triggerTip(isObj(u[field]) ? u[field] : {}, t);
      }
      const words = circleWords(counts, t);
      return {
        text: '', cls: '', tooltip: detail ? `${words}\n${detail}` : words, circles: circlesOf(counts),
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
// is worst first -- v1's comparators exactly); journeys by running count; cadence by days; Last
// broadcast by daysSince with a never-broadcast row above every number. null = missing (no usage,
// no stored score, a ZMA-dimmed column, an unrecognized value) -- ranks last in BOTH orders.
const BADNESS = Object.freeze({ active: 0, idle: 1, none: 2 });

export function sortValue(row, field) {
  const u = row && row.usage;
  if (!u || (row.zma && ZMA_DIMMED.includes(field))) {
    return null;
  }
  switch (field) {
    case 'score':
      return numOrNull(isObj(u.score) ? u.score.total : null);
    case 'welcome':
    case 'purchase':
    case 'noPurchase': {
      const s = isObj(u[field]) ? u[field].state : undefined;
      return Object.prototype.hasOwnProperty.call(BADNESS, s) ? BADNESS[s] : null;
    }
    case 'sunset':
    case 'attribution':
      if (typeof u[field] !== 'boolean') {
        return null;
      }
      return u[field] ? 0 : 1;
    case 'journeys':
      return numOrNull(isObj(u.journeys) ? u.journeys.running : null);
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
