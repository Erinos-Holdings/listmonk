// Fork (brand usage, integrations listmonk/BRAND-USAGE-SPEC D2/D7). The pure half of the
// Dashboard's Brand Usage pane: row shaping (display name, launch filter -- the Brand Matrix's
// rows, off the SAME fetch), glyph and cell mapping, the sort comparators and the per-session
// persistence wrapper. No Vue, no DOM; tested by brandUsage.test.mjs (yarn test).
//
// Each GET /api/dashboard/brand-matrix row may carry `usage` -- the brand's latest brand_health
// doc's stored object, projected verbatim. Every usage rule and threshold (the journey states, the
// broadcast cadence and its ok/due/stale verdict) is computed by the integrations
// lib/brand-usage.ts; this module only maps stored states to glyphs and colors (I1).
import { brandLabelOf, matchesLaunch } from './brandScope.mjs'; // eslint-disable-line import/extensions
import { healthDocs } from './brandMatrix.mjs'; // eslint-disable-line import/extensions

// Every sortable column, in display order.
export const SORT_FIELDS = Object.freeze([
  'brand', 'welcome', 'purchase', 'noPurchase', 'sunset', 'attribution', 'journeys', 'lastBroadcast', 'cadence',
]);

// D7: the default sort is Last broadcast, worst first (never broadcast, then the longest silence).
export const DEFAULT_STATE = Object.freeze({ sort: Object.freeze({ field: 'lastBroadcast', order: 'desc' }) });

// D7: the sessionStorage key of { sort: {field, order} }.
export const STORAGE_KEY = 'dashboard.usage';

export const MISSING = '—';

const OK = 'has-text-success';
const AMBER = 'has-text-warning-dark';
const RED = 'has-text-danger';

// D7: S (active/idle/none) and E (running/idle/none) glyphs -- ✓ green, ⚠ amber, ✗ red.
const STATE_CELL = Object.freeze({
  active: Object.freeze({ glyph: '✓', cls: OK }),
  running: Object.freeze({ glyph: '✓', cls: OK }),
  idle: Object.freeze({ glyph: '⚠', cls: AMBER }),
  none: Object.freeze({ glyph: '✗', cls: RED }),
});

const BLANK = Object.freeze({ glyph: '', cls: '' });
const DIMMED = Object.freeze({ glyph: MISSING, cls: '' });

// stateCell maps a stored S/E state to { glyph, cls }; anything else is blank.
export function stateCell(state) {
  return Object.prototype.hasOwnProperty.call(STATE_CELL, state) ? STATE_CELL[state] : BLANK;
}

// boolCell: ✓ green / ✗ red (the header tooltip says "not in use", not "error"); non-boolean blank.
export function boolCell(v) {
  if (v === true) {
    return STATE_CELL.active;
  }
  if (v === false) {
    return STATE_CELL.none;
  }
  return BLANK;
}

// D7: the Last broadcast cell's color by the stored broadcasts.state.
const BROADCAST_CLASS = Object.freeze({
  ok: OK, due: AMBER, stale: RED, none: RED,
});

const isObj = (v) => !!v && typeof v === 'object' && !Array.isArray(v);
const numOrNull = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : null);

// shapeRow flattens one API row: { brand, label, usage (the stored object, or null), zma }.
export function shapeRow(r, brands) {
  const usage = isObj(r && r.usage) ? r.usage : null;
  return {
    brand: r.brand,
    label: brandLabelOf(r.brand, brands),
    usage,
    zma: !!usage && usage.channel === 'zma',
  };
}

// D2: the Brand Matrix's rows exactly -- the API rows narrowed by the launch filter over the rows'
// own health documents, then flattened. The server has already applied the brand scope.
export function shapeRows(apiRows, { brands = [], launch = 'all' } = {}) {
  const rows = (Array.isArray(apiRows) ? apiRows : []).filter((r) => r && typeof r.brand === 'string' && r.brand !== '');
  const docs = healthDocs(rows);
  return rows.filter((r) => matchesLaunch(r.brand, launch, docs)).map((r) => shapeRow(r, brands));
}

// The columns a ZMA row does not score (its sends live in ZMA -- a red would be false, I6).
const ZMA_DIMMED = Object.freeze(['welcome', 'journeys', 'lastBroadcast', 'cadence']);

// The welcome tooltip: the master list's evergreen campaigns ("#id status"), or none.
export function welcomeTip(w, t = (k) => k) {
  const camps = Array.isArray(w && w.campaigns) ? w.campaigns : [];
  if (!camps.length) {
    return t('dashboard.usage.noEvergreen');
  }
  return camps.map((c) => `#${c.id} ${c.status}`).join(', ');
}

// D7: the ⚠ tooltip -- each trigger row's slug with its parser error, or its list and that list's
// evergreen state when not running. Rows in the stored order, one per line.
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

// cell is one column's presentation for a row: { text, cls, tooltip } where tooltip is a plain
// string (t formats it -- the Vue passes (k, p) => this.$t(k, p)). A row without usage is blank
// (D7); a ZMA row shows '—' in Welcome/Journeys/Last broadcast/Cadence.
export function cell(row, field, t = (k) => k) {
  const u = row && row.usage;
  if (!u) {
    return { text: '', cls: '', tooltip: '' };
  }
  if (row.zma && ZMA_DIMMED.includes(field)) {
    return { text: DIMMED.glyph, cls: '', tooltip: t('dashboard.usage.zmaHelp') };
  }
  switch (field) {
    case 'welcome': {
      const w = isObj(u.welcome) ? u.welcome : {};
      const c = stateCell(w.state);
      return { text: c.glyph, cls: c.cls, tooltip: welcomeTip(w, t) };
    }
    case 'purchase':
    case 'noPurchase': {
      const p = isObj(u[field]) ? u[field] : {};
      const c = stateCell(p.state);
      return { text: c.glyph, cls: c.cls, tooltip: triggerTip(p, t) };
    }
    case 'sunset':
    case 'attribution': {
      const c = boolCell(u[field]);
      return { text: c.glyph, cls: c.cls, tooltip: '' };
    }
    case 'journeys': {
      const j = isObj(u.journeys) ? u.journeys : {};
      const running = numOrNull(j.running);
      if (running === null) {
        return { text: '', cls: '', tooltip: '' };
      }
      return {
        text: t('dashboard.usage.running', { n: running }),
        cls: '',
        tooltip: t('dashboard.usage.idleCount', { n: numOrNull(j.idle) ?? 0 }),
      };
    }
    case 'lastBroadcast': {
      const b = isObj(u.broadcasts) ? u.broadcasts : {};
      const cls = Object.prototype.hasOwnProperty.call(BROADCAST_CLASS, b.state) ? BROADCAST_CLASS[b.state] : '';
      if (b.state === 'none') {
        return { text: t('dashboard.usage.never'), cls, tooltip: broadcastTip(b, t) };
      }
      const since = numOrNull(b.daysSince);
      if (since === null) {
        return { text: '', cls: '', tooltip: '' };
      }
      return { text: t('dashboard.usage.daysAgo', { n: Math.floor(since) }), cls, tooltip: broadcastTip(b, t) };
    }
    case 'cadence': {
      const b = isObj(u.broadcasts) ? u.broadcasts : {};
      const c = numOrNull(b.cadenceDays);
      return { text: c === null ? MISSING : t('dashboard.usage.cadenceDays', { n: Math.round(c) }), cls: '', tooltip: '' };
    }
    default:
      return { text: '', cls: '', tooltip: '' };
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

// The default direction of a column on its first click: brand ascending, every other descending.
export function defaultOrder(field) {
  return field === 'brand' ? 'asc' : 'desc';
}

// Sort keys: states and booleans as BADNESS (higher = worse, so desc is worst first, like the
// default); journeys by running count; cadence by days; Last broadcast by daysSince with a
// never-broadcast row above every number. null = missing (no usage, a ZMA-dimmed column, an
// unrecognized value) -- ranks last in BOTH orders.
const BADNESS = Object.freeze({ active: 0, idle: 1, none: 2 });

export function sortValue(row, field) {
  const u = row && row.usage;
  if (!u || (row.zma && ZMA_DIMMED.includes(field))) {
    return null;
  }
  switch (field) {
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

// I7: sortRows -- the requested column and direction; missing last in both orders; ties by name.
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
