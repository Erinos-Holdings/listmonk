// Fork (campaign mode, integrations listmonk/CAMPAIGN-MODE-SPEC D1-D13). The pure half of Campaign
// mode: the navbar toggle's value and applicability, the name key's JS half, the Campaign Matrix's
// bucketing by the server's keys and its group rows, the Campaigns page's flattening of the grouped
// response, and the Analytics picker's group labels and fill. No Vue, no DOM; tested by
// campaignGroups.test.mjs (yarn test:groups).
//
// The grouping rule is the server's (internal/core/campaign_groups.go): this module never derives a
// group from names or brands -- it buckets by the set_key / series_key the Matrix returns, or renders
// the groups GET /api/campaigns?grouped=true and GET /api/analytics/campaigns?grouped=true return
// (D2). nameKey exists only because the duplicate-name warning (naming.mjs::duplicateOf) runs in the
// browser; it is pinned with core.NameKeySQL by testdata/campaign-groups.json (I1).
import {
  STATUS_ORDER, filterByName, sortRows, statusRank, sumRows, totalsRow,
} from './campaignMatrix.mjs'; // eslint-disable-line import/extensions
import { LANG_VALUES } from './naming.mjs'; // eslint-disable-line import/extensions
import { langApplicability } from './langScope.mjs'; // eslint-disable-line import/extensions
import { pickerLabel as promoPickerLabel } from './promoSeries.mjs'; // eslint-disable-line import/extensions
import { nameKey } from './nameKey.mjs'; // eslint-disable-line import/extensions

// D3 -- the name key (defined in the leaf module nameKey.mjs so naming.mjs can import it without an
// import cycle; this module's nameKey is that function).
export { nameKey };

// D1 -- the toggle's values and its pref. An unknown stored value reads 'off' (I14). The pref's ONE
// writer is the store's selectCampaignMode mutation, called only by CampaignModeToggle.vue.
export const MODES = Object.freeze(['off', 'groups']);
export const MODE_PREF = 'campaigns.mode';
export function normalizeMode(v) {
  return MODES.includes(v) ? v : 'off';
}

// D13 -- the mode's applicability per named route (exhaustive over router/index.js, as
// langScope.mjs::LANG_APPLICABILITY): Campaigns and both Analytics panes group; every other route none.
const NONE = Object.freeze({ state: 'none' });
const ALL = Object.freeze({ state: 'all' });
export const MODE_APPLICABILITY = Object.freeze({
  '404_page': NONE,
  dashboard: NONE,
  lists: NONE,
  list: NONE,
  forms: NONE,
  brands: NONE,
  journeyPreview: NONE,
  brand: NONE,
  subscribers: NONE,
  subscribers_list: NONE,
  subscriber: NONE,
  import: NONE,
  bounces: NONE,
  campaigns: ALL,
  media: NONE,
  templates: NONE,
  campaignAnalytics: Object.freeze({
    view: 'analytics',
    defaultPane: 'matrix',
    panes: Object.freeze({ matrix: ALL, campaign: ALL }),
  }),
  campaignReview: NONE,
  campaign: NONE,
  userProfile: NONE,
  settings: NONE,
  logs: NONE,
  users: NONE,
  userRoles: NONE,
  listRoles: NONE,
  maintenance: NONE,
  // Fork (SMS Admin, integrations SMS-LINKS-ADMIN-SPEC D1) -- short links and SMS codes, no campaigns.
  smsAdmin: NONE,
});

// The applicability state ('all' | 'none') of a route (and its pane value, '<view>:<pane>').
export function modeApplicability(routeName, pane) {
  const def = Object.prototype.hasOwnProperty.call(MODE_APPLICABILITY, routeName) ? MODE_APPLICABILITY[routeName] : NONE;
  if (def.panes) {
    const s = typeof pane === 'string' ? pane : '';
    const prefix = `${def.view}:`;
    const p = s.startsWith(prefix) ? s.slice(prefix.length) : '';
    return (Object.prototype.hasOwnProperty.call(def.panes, p) ? def.panes[p] : def.panes[def.defaultPane]).state;
  }
  return def.state;
}

// D13 -- the toggle's grey line (an i18n key), or null under 'off'. On an `all` route: the
// series-only line exactly when the language toggle applies to that route/pane (its applicability
// is `all`) and the effective language is not All; otherwise the groups line. On a `none` route:
// the none line. langEffective is the language the views apply (the store's langEffective -- never
// the raw selection, which only Subscribers and the language toggle read).
export function modeNote(mode, routeName, { pane, langEffective } = {}) {
  if (normalizeMode(mode) !== 'groups') {
    return null;
  }
  if (modeApplicability(routeName, pane) !== 'all') {
    return 'naming.modeApp.none';
  }
  const lang = langApplicability(routeName, { pane, selection: langEffective });
  if (lang.state === 'all' && (langEffective || 'all') !== 'all') {
    return 'naming.modeApp.seriesOnly';
  }
  return 'naming.modeApp.groups';
}

// D8 -- the status summary: [{ status, n }] in STATUS_ORDER (an unknown status last, by name).
export function statusSummary(rows) {
  const counts = new Map();
  (Array.isArray(rows) ? rows : []).forEach((r) => {
    const s = r && typeof r.status === 'string' ? r.status : '';
    if (s) counts.set(s, (counts.get(s) || 0) + 1);
  });
  return [...counts.keys()]
    .sort((a, b) => (statusRank(a) - statusRank(b)) || a.localeCompare(b))
    .map((status) => ({ status, n: counts.get(status) }));
}

// D8 -- the summary's text, e.g. "4 finished, 1 draft"; label(status) is the translated status.
export function statusSummaryText(summary, label = (s) => s) {
  return (Array.isArray(summary) ? summary : []).map((e) => `${e.n} ${String(label(e.status)).toLowerCase()}`).join(', ');
}

// D8 -- the members' languages, distinct, in LANG_VALUES order (an unknown code after them, sorted).
export function langCodes(langs) {
  const set = [...new Set((Array.isArray(langs) ? langs : []).map((l) => (typeof l === 'string' ? l.trim().toLowerCase() : '')).filter(Boolean))];
  const rank = (l) => {
    const i = LANG_VALUES.indexOf(l);
    return i === -1 ? LANG_VALUES.length : i;
  };
  return set.sort((a, b) => (rank(a) - rank(b)) || a.localeCompare(b));
}

const maxBy = (rows, f) => rows.reduce((best, r) => (best === null || f(r) > f(best) ? r : best), null);

// D7/D8 -- a Campaign Matrix group row from its shaped member rows (campaignMatrix.mjs::shapeRow):
// the numbers are sumRows (the totals row's rule -- counts sum, rates over each column's own Sent,
// conversions sum, the gap pooled); Status the summary, sorting as the most active member; Started
// the earliest; Lang the codes; Code for a series, Step for a set when every member agrees; GP shown
// only when every broadcast member carrying a GP value shares one send day (else, with broadcasts on
// several send days, gpMixed -- review S3); views and
// clicks are counted per campaign (perCampaign). The name is the newest member's (max id -- the
// Matrix rows carry no created_at).
export function groupRow(members, { kind = 'set', key = '' } = {}) {
  const m = Array.isArray(members) ? members : [];
  const newest = maxBy(m, (r) => r.id) || {};
  const started = m.filter((r) => r.startedAt).sort((a, b) => (a.startedAt < b.startedAt ? -1 : 1))[0] || null;
  const gpRows = m.filter((r) => !r.evergreen && typeof r.gp_rate === 'number');
  const gpDays = [...new Set(gpRows.map((r) => r.date))];
  const gpOne = gpRows.length > 0 && gpDays.length === 1;
  const sendDays = [...new Set(m.filter((r) => !r.evergreen).map((r) => r.date))];
  const steps = [...new Set(m.map((r) => r.promo_step))];
  const lastSent = m.map((r) => r.lastSent).filter(Boolean).sort().pop() || null;
  return {
    id: newest.id,
    key,
    kind,
    group: true,
    members: m,
    name: typeof newest.name === 'string' ? newest.name : '',
    former_name: null,
    status: m.map((r) => r.status).sort((a, b) => statusRank(a) - statusRank(b))[0] || '',
    statusSummary: statusSummary(m),
    evergreen: m.length > 0 && m.every((r) => r.evergreen),
    brand: newest.brand || '',
    label: newest.label || '',
    lang: langCodes(m.map((r) => r.lang)).join(', '),
    promo_code: kind === 'series' ? (newest.promo_code || '') : '',
    promo_step: kind === 'set' && steps.length === 1 && typeof steps[0] === 'number' ? steps[0] : null,
    date: sendDays.length === 1 ? (sendDays[0] || null) : null,
    lastSent,
    startedAt: started ? started.startedAt : null,
    started: started ? started.started : null,
    gp_rate: gpOne ? gpRows[0].gp_rate : null,
    gpThresholds: gpOne ? gpRows[0].gpThresholds : null,
    // Review S3 -- any group with no shown value whose broadcasts span several send days.
    gpMixed: !gpOne && sendDays.length > 1,
    perCampaign: true,
    totals: false,
    ...sumRows(m),
  };
}

// D8 -- a group row's cell hover as { key } (Views and Clicks: counted per campaign; GP on mixed
// send days), or null.
export function groupCellTip(row, field) {
  if (!row || !row.group) {
    return null;
  }
  if (field === 'views' || field === 'clicks') {
    return { key: 'analytics.matrix.perCampaign' };
  }
  if (field === 'gp_rate' && row.gpMixed) {
    return { key: 'analytics.matrix.gpMixedDays' };
  }
  return null;
}

// The member (campaign) rows of a node ({ row, children }), every depth.
export function nodeMembers(node) {
  if (!node) return [];
  if (!node.children || node.children.length === 0) return [node.row];
  return node.children.flatMap(nodeMembers);
}

// D7 / D10 (I11) -- the Matrix's bucketing: rows are the shaped rows AFTER the language and launch
// filters; they bucket by series_key, then set_key; then the name filter keeps a top-level node when
// any member passes campaignMatrix.mjs::filterByName, and shows it whole. A series is always a group
// row; a set of one is the campaign's own row. Returns nodes { row, children } in first-seen order.
export function bucketMatrix(rows, nameQuery = '') {
  const top = [];
  const series = new Map();
  const sets = new Map();
  (Array.isArray(rows) ? rows : []).forEach((r) => {
    if (r.series_key) {
      let s = series.get(r.series_key);
      if (!s) {
        s = { kind: 'series', key: `s:${r.series_key}`, sets: new Map() };
        series.set(r.series_key, s);
        top.push(s);
      }
      const sk = r.set_key || `c:${r.id}`;
      if (!s.sets.has(sk)) s.sets.set(sk, []);
      s.sets.get(sk).push(r);
    } else if (r.set_key) {
      let s = sets.get(r.set_key);
      if (!s) {
        s = { kind: 'set', key: `e:${r.set_key}`, members: [] };
        sets.set(r.set_key, s);
        top.push(s);
      }
      s.members.push(r);
    } else {
      top.push({ kind: 'campaign', key: `c:${r.id}`, members: [r] });
    }
  });

  const leaf = (r) => ({ row: r, children: [] });
  const setNode = (key, members) => (members.length === 1 ? leaf(members[0])
    : { row: groupRow(members, { kind: 'set', key }), children: members.map(leaf) });
  const nodes = top.map((e) => {
    if (e.kind === 'series') {
      const children = [...e.sets.entries()].map(([k, members]) => setNode(`e:${k}`, members));
      const members = [...e.sets.values()].flat();
      return { row: groupRow(members, { kind: 'series', key: e.key }), children };
    }
    if (e.kind === 'set') return setNode(e.key, e.members);
    return leaf(e.members[0]);
  });

  return nodes.filter((n) => filterByName(nodeMembers(n), nameQuery).length > 0);
}

// D7 -- the sort: top-level nodes by their row's value (a group's aggregate), children within their
// parent by the same sort (campaignMatrix.mjs::sortRows, the one rule).
export function sortTree(nodes, sort, mode, locale = 'en') {
  const list = Array.isArray(nodes) ? nodes : [];
  const byRow = new Map(list.map((n) => [n.row, n]));
  return sortRows(list.map((n) => n.row), sort, mode, locale).map((r) => {
    const n = byRow.get(r);
    return { row: n.row, children: sortTree(n.children, sort, mode, locale) };
  });
}

// D7 -- the rows shown: each node's row with { depth, rowKey, expandable, expanded }, its children
// following only when expanded (the in-page set of expanded keys). Member rows are copied, never
// mutated.
export function flattenTree(nodes, expanded = new Set(), depth = 0) {
  const out = [];
  (Array.isArray(nodes) ? nodes : []).forEach((n) => {
    const expandable = n.children.length > 0;
    const rowKey = n.row.group ? n.row.key : `c:${n.row.id}`;
    const open = expandable && expanded.has(rowKey);
    out.push({
      ...n.row, depth, rowKey, expandable, expanded: open,
    });
    if (open) out.push(...flattenTree(n.children, expanded, depth + 1));
  });
  return out;
}

// D7 -- the Matrix's table in Campaign mode: the bucketed, name-filtered, sorted, flattened rows,
// with the totals row over every member that passed the filters, expanded or not (pinned last).
export function groupedTableRows(rows, {
  nameQuery = '', sort, mode, locale = 'en', totalsLabel = 'Total', expanded = new Set(),
} = {}) {
  const nodes = sortTree(bucketMatrix(rows, nameQuery), sort, mode, locale);
  const flat = flattenTree(nodes, expanded);
  const t = totalsRow(nodes.flatMap(nodeMembers), totalsLabel);
  return t ? [...flat, t] : flat;
}

// --- The Campaigns page (D9) -------------------------------------------------------------------

// Whether results are the grouped response's top-level rows.
export function isGroupedResults(results) {
  return Array.isArray(results) && results.some((r) => r && typeof r.kind === 'string' && Array.isArray(r.campaigns));
}

// D9 (I12, review R5) -- every member of every top-level row, expanded or not (ungrouped: the rows
// themselves). The page's poll (stale / displayedRunning) and the clone-name list read this.
export function memberRows(results) {
  const r = Array.isArray(results) ? results : [];
  return isGroupedResults(r) ? r.flatMap((g) => (Array.isArray(g.campaigns) ? g.campaigns : [])) : r;
}

// A grouped top-level row's in-page key (its kind and newest member id).
export function groupKey(g) {
  const ids = (g && Array.isArray(g.campaigns) ? g.campaigns : []).map((c) => c.id);
  return `${g && g.kind}:${ids.length ? ids[0] : ''}`;
}

const sum = (rows, k) => rows.reduce((t, r) => t + (Number.isFinite(r[k]) ? r[k] : 0), 0);

// D9 -- the group row the page renders for a top-level series / set: the Status summary, the name
// and the returned (group) brand, Lang codes, Code / Step, Lists (distinct), Started earliest, the
// summed Sent, Views, Clicks, H. Bounces, Complaints, and every member id (the analytics glyph's
// ?id=). Members are the camelCased server rows of the same response.
export function groupView(g) {
  const members = Array.isArray(g.campaigns) ? g.campaigns : [];
  const lists = [];
  const seen = new Set();
  members.forEach((c) => (Array.isArray(c.lists) ? c.lists : []).forEach((l) => {
    if (l && !seen.has(l.id)) {
      seen.add(l.id);
      lists.push(l);
    }
  }));
  const started = members.map((c) => c.startedAt).filter(Boolean).sort()[0] || null;
  return {
    group: true,
    key: groupKey(g),
    kind: g.kind,
    brand: typeof g.brand === 'string' ? g.brand : '',
    code: g.kind === 'series' && typeof g.code === 'string' ? g.code : '',
    step: g.kind === 'set' && Number.isInteger(g.step) ? g.step : null,
    name: typeof g.name === 'string' ? g.name : '',
    count: members.length,
    memberIds: members.map((c) => c.id),
    statusSummary: statusSummary(members),
    langs: langCodes(members.map((c) => (c.attribs && c.attribs.lang) || '')),
    lists,
    startedAt: started,
    sent: sum(members, 'sent'),
    views: sum(members, 'views'),
    clicks: sum(members, 'clicks'),
    hardBounces: sum(members, 'hardBounces'),
    complaints: sum(members, 'complaints'),
  };
}

// D9 (I12) -- the table's data: a singleton (kind campaign) is its own unmodified row; a series / set
// is its group row, followed by its members -- the unmodified server rows -- only when expanded.
export function flattenGroups(results, expanded = new Set()) {
  if (!isGroupedResults(results)) {
    return Array.isArray(results) ? results : [];
  }
  const out = [];
  results.forEach((g) => {
    const members = Array.isArray(g.campaigns) ? g.campaigns : [];
    if (g.kind === 'campaign') {
      out.push(...members);
      return;
    }
    const v = groupView(g);
    v.expanded = expanded.has(v.key);
    out.push(v);
    if (v.expanded) out.push(...members);
  });
  return out;
}

// G5 amendment (2026-11-27) -- a group row's send line, the sum of what each member row shows: an
// unstarted member its To send (toSendOf(c), the live audience; "…"/"—" while its read is pending
// or failed -- stateOf), a started member its Sent / To send (statsOf(c): the live poll's row when
// running), an evergreen member its Sent with no denominator. label 'toSend' while nothing has
// started (the draft row's line); total null when every member is evergreen.
export function groupSendLine(members, { statsOf = (c) => c, toSendOf = () => null, stateOf = () => null } = {}) {
  const rows = Array.isArray(members) ? members : [];
  let sent = 0;
  let total = 0;
  let started = 0;
  let pending = false;
  let unknown = false;
  rows.forEach((c) => {
    const st = stateOf(c);
    if (st === 'pending') {
      pending = true;
      return;
    }
    if (st === 'unknown') {
      unknown = true;
      return;
    }
    const ts = c.evergreen ? null : toSendOf(c);
    if (ts) {
      total += Number(ts.count) || 0;
      return;
    }
    const s = statsOf(c) || c;
    started += 1;
    sent += Number(s.sent) || 0;
    if (!c.evergreen) total += Number(s.toSend) || 0;
  });
  const allEvergreen = rows.length > 0 && rows.every((c) => c.evergreen);
  return {
    label: started === 0 && !allEvergreen ? 'toSend' : 'sent',
    sent,
    total: allEvergreen ? null : total,
    state: (pending && 'pending') || (unknown && 'unknown') || null,
  };
}

// The Campaigns page's Status cell attributes: upstream's utils.js tdID reads row.id, which a group
// row (groupView: a key, no id) lacks -- it threw inside Buefy and the table read "Nothing here".
export function statusTdAttrs(row) {
  if (!row || row.group) return {};
  return { 'data-id': String(row.id) };
}

// D9 (I12) -- a group row's actions: only Analytics works; the rest render disabled with their
// hover -- Phase 2 for the status actions, Inspect and Delete, "not available" for Clone and Preview.
export const GROUP_ACTIONS = Object.freeze([
  {
    action: 'inspect', icon: 'creation', label: 'campaigns.review.openToInspect', tip: 'campaigns.group.phase2',
  },
  {
    action: 'start', icon: 'rocket-launch-outline', label: 'campaigns.start', tip: 'campaigns.group.phase2',
  },
  {
    action: 'pause', icon: 'pause-circle-outline', label: 'campaigns.pause', tip: 'campaigns.group.phase2',
  },
  {
    action: 'resume', icon: 'rocket-launch-outline', label: 'campaigns.send', tip: 'campaigns.group.phase2',
  },
  {
    action: 'schedule', icon: 'clock-start', label: 'campaigns.schedule', tip: 'campaigns.group.phase2',
  },
  {
    action: 'cancel', icon: 'cancel', label: 'globals.buttons.cancel', tip: 'campaigns.group.phase2',
  },
  {
    action: 'preview', icon: 'file-find-outline', label: 'campaigns.preview', tip: 'campaigns.group.notForGroups',
  },
  {
    action: 'clone', icon: 'file-multiple-outline', label: 'globals.buttons.clone', tip: 'campaigns.group.notForGroups',
  },
  {
    action: 'analytics', icon: 'chart-bar', label: 'globals.terms.analytics', tip: 'globals.terms.analytics', enabled: true,
  },
  {
    action: 'delete', icon: 'trash-can-outline', label: 'globals.buttons.delete', tip: 'campaigns.group.phase2',
  },
].map((a) => Object.freeze({ enabled: false, ...a })));

// D9 -- the analytics glyph's route: every member id of the row as shown.
export function groupAnalyticsRoute(view) {
  return { name: 'campaignAnalytics', query: { id: (view && view.memberIds) || [] } };
}

// --- The Analytics picker (D6) -----------------------------------------------------------------

// A picker member's chip label: the existing `#id: name` (and the promo Code / Step).
export function memberLabel(c) {
  return promoPickerLabel(`#${c.id}: ${c.name}`, c.promoCode, c.promoStep);
}

// D6 (I16) -- a grouped picker row's label: `<slug> · <Code> · N steps` for a series,
// `<slug> · <name> · N campaigns` for a set, a single campaign its existing label. nSteps / nCampaigns are the caller's
// translations. Never "N languages" (review R2).
export function pickerLabel(row, { nSteps = (n) => `${n} steps`, nCampaigns = (n) => `${n} campaigns` } = {}) {
  const members = row && Array.isArray(row.campaigns) ? row.campaigns : [];
  // Review S4 -- a group row leads with its brand slug, so the same Code or name in two brands reads
  // as two options.
  const slug = row && typeof row.brand === 'string' && row.brand ? `${row.brand} · ` : '';
  if (row && row.kind === 'series') {
    const steps = Array.isArray(row.sets) ? row.sets.length : 0;
    return `${slug}${typeof row.code === 'string' ? row.code : ''} · ${nSteps(steps)}`;
  }
  if (row && row.kind === 'set') {
    return `${slug}${row.name} · ${nCampaigns(members.length)}`;
  }
  return members.length ? memberLabel(members[0]) : String((row && row.name) || '');
}

// D6 (I16) -- choosing a group row puts EVERY member id into the campaigns input, de-duplicated
// against the chips already there (existing chips keep their place). Returns the new selection.
export function pickerFill(selected, row) {
  const out = Array.isArray(selected) ? [...selected] : [];
  const have = new Set(out.map((c) => c.id));
  (row && Array.isArray(row.campaigns) ? row.campaigns : []).forEach((c) => {
    if (!have.has(c.id)) {
      have.add(c.id);
      out.push({ ...c, name: memberLabel(c) });
    }
  });
  return out;
}

export { STATUS_ORDER };
