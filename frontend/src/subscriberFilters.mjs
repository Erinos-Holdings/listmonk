// Fork (Subscribers filters, integrations listmonk/SUBSCRIBERS-FILTERS-SPEC). The pure half of the
// Subscribers page's controls row (Lists ▾ · Status ▾ · search) and of the router guard's one
// decision for every navigation to a Subscribers route. No Vue, no store -- main.js and
// Subscribers.vue only call it. Tested with `node --test src/subscriberFilters.test.mjs`.
//
// The route stays the page's single source of the filter: list = route `subscribers_list` param
// `listID`, `segment` and `lang` = query keys. The controls WRITE THE ROUTE AND NOTHING ELSE; every
// correction of a route happens in the guard (subscribersRouteFix) before the page mounts, so the
// page never issues a request the server refuses.
import { LANG_VALUES, SUBSCRIBER_ROUTES, normalizeLang } from './langScope.mjs'; // eslint-disable-line import/extensions
import { scopeListIds, storeLists } from './brandScope.mjs'; // eslint-disable-line import/extensions
import { groupLists } from './listGroups.mjs'; // eslint-disable-line import/extensions

// D5 -- the Status dropdown's segments, in display order (the server's subscription_segment()
// values; labels are the lists.grid.<segment> keys).
export const SEGMENTS = Object.freeze(['active', 'pending', 'held', 'unsubscribed', 'blocked']);

const has = (q, k) => Object.prototype.hasOwnProperty.call(q, k);

// A route language the toggle can hold ('all' is never a route value -- All is the absent key).
// SUBSCRIBERS-FILTERS-2-SPEC D1 (I2): en_only and none are no longer toggle values, so a route
// carrying one is dropped (rule 2) and, under a non-All selection, replaced by it (rule 4).
function isRouteLang(v) {
  return typeof v === 'string' && v !== 'all' && LANG_VALUES.includes(v);
}

// A route listID: a positive integer (string or number), else null.
export function routeListIdOf(params) {
  const raw = params && params.listID;
  const n = typeof raw === 'number' ? raw : parseInt(raw, 10);
  return Number.isInteger(n) && n > 0 && String(n) === String(raw).trim() ? n : null;
}

// D3 -- the guard's decision for a navigation to `to`, under the raw selection (the store's
// langSubscribers) and whether the toggle is visible. Returns {} (no change) or
// { query?: <the corrected query>, commitLang?: <value> }:
//   1. toggle hidden: drop any route lang;
//   2. route lang not a toggle value: drop it;
//   3. route lang a toggle value different from the selection: commitLang = it (query unchanged);
//   4. no route lang (after 1-2) and the selection not All: add lang=<selection>;
//   5. a segment without a route listID, or not one of SEGMENTS: drop it;
//   6. otherwise no change.
export function subscribersRouteFix(to, selection, toggleVisible) {
  if (!to || !SUBSCRIBER_ROUTES.includes(to.name)) {
    return {};
  }
  const q = { ...(to.query || {}) };
  const sel = toggleVisible === true ? normalizeLang(selection) : 'all';
  let changed = false;
  const out = {};

  if (has(q, 'lang')) {
    if (toggleVisible !== true || !isRouteLang(q.lang)) {
      delete q.lang;
      changed = true;
    } else if (q.lang !== sel) {
      out.commitLang = q.lang;
    }
  }
  if (!has(q, 'lang') && sel !== 'all') {
    q.lang = sel;
    changed = true;
  }

  if (has(q, 'segment')) {
    const listed = to.name === 'subscribers_list' && routeListIdOf(to.params) !== null;
    if (!listed || typeof q.segment !== 'string' || !SEGMENTS.includes(q.segment)) {
      delete q.segment;
      changed = true;
    }
  }

  if (changed) {
    out.query = q;
  }
  return out;
}

const nameOf = (l) => (l && typeof l.name === 'string' ? l.name : '');

// D4 -- the Lists dropdown: { enabled, ordinary, system, extra }. ordinary/system are { id, name }
// sorted by name (system = the server's journey_locked); extra is the route's list when it is not
// among them (out of the selected brand, or not in the lists store -- labelled `#<id>`), else
// null. Enabled under a brand or No brand; under All brands only when the route names a list, and
// then the only option besides All lists is that list (so the arrival can be cleared).
export function listOptions(lists, selection, routeListId) {
  const all = storeLists(lists);
  const rid = Number.isInteger(routeListId) && routeListId > 0 ? routeListId : null;
  // Fork (brand groups, integrations BRAND-GROUPS-SPEC D6) -- a group is scoped too: its lists.
  const scoped = selection && (selection.kind === 'brand' || selection.kind === 'none' || selection.kind === 'group');
  const ids = scoped ? scopeListIds(all, selection) : null;
  // SUBSCRIBERS-FILTERS-2-SPEC D5 -- the one grouping, shared with the Campaigns page.
  const { ordinary, system } = groupLists(ids ? all.filter((l) => l && ids.includes(l.id)) : []);
  let extra = null;
  if (rid !== null && !(ids && ids.includes(rid))) {
    const known = all.find((l) => l && l.id === rid);
    extra = { id: rid, name: known && nameOf(known) ? nameOf(known) : `#${rid}` };
  }
  return {
    enabled: !!ids || rid !== null, ordinary, system, extra,
  };
}

// D4 -- the ONE route builder of a list pick (and of the brand-switch navigation): the list route
// for an id, the plain subscribers route for null. Drops `segment` and the legacy
// `subscription_status` (a status is a status OF a list); keeps lang, search and the rest.
export function listRoute(query, listId) {
  const { segment: _s, subscription_status: _ss, ...rest } = query || {};
  if (Number.isInteger(listId) && listId > 0) {
    return { name: 'subscribers_list', params: { listID: String(listId) }, query: rest };
  }
  return { name: 'subscribers', query: rest };
}

// SUBSCRIBERS-FILTERS-2-SPEC D3 (I9) -- the Lang column shows while the page's language (the
// ROUTE's lang, the caption's source) is absent (All), en (EN+: stored en and no language) or other
// (the rows differ); under es / fr / de / it every row reads the same, so it is hidden.
export function langColumnShown(routeLang) {
  return routeLang === undefined || routeLang === null || routeLang === '' || routeLang === 'en' || routeLang === 'other';
}

// D3 (I9) -- a subscriber's Lang cell: the stored attribs.lang -- a string or a finite number,
// String()-ed, trimmed and upper-cased (EN, PT-BR) -- else an em dash (empty, whitespace-only,
// absent, or any other JSON type). Display only: the bucket rule is the server's subscriber_lang().
export const LANG_CELL_EMPTY = '—';
export function langCell(attribs) {
  const v = attribs && typeof attribs === 'object' && !Array.isArray(attribs) ? attribs.lang : undefined;
  if (typeof v !== 'string' && !(typeof v === 'number' && Number.isFinite(v))) {
    return LANG_CELL_EMPTY;
  }
  const s = String(v).trim().toUpperCase();
  return s === '' ? LANG_CELL_EMPTY : s;
}

// D5 -- the Status dropdown is enabled only when the route names a list.
export function statusEnabled(routeListId) {
  return Number.isInteger(routeListId) && routeListId > 0;
}

// D5 -- the query of a status pick: segment set ('' / All statuses removes it), the legacy
// subscription_status dropped.
export function statusQuery(query, segment) {
  const { segment: _s, subscription_status: _ss, ...rest } = query || {};
  return SEGMENTS.includes(segment) ? { ...rest, segment } : rest;
}
