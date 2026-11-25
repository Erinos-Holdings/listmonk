// Fork (global brand, integrations GLOBAL-BRAND-SPEC D1-D3, D8, D10-D12). The global brand
// selector's pure half: the roster, the resolution of the stored selection, the effective list set
// and every value a page derives from them. No Vue, no store; tested with
// `node --test src/brandScope.test.mjs` (yarn test:brand-scope).
//
// The selector is a PRESENTATION filter, never a security boundary (S1): nothing here is read by
// the server. A scope reaches it only as the existing repeatable list_id (or tag=/nobrand=/brand= on the
// Lists listing), which the server intersects with the user's permission per endpoint.
//
// selection: { kind: 'all' } | { kind: 'brand', slug } | { kind: 'none' }
//   | { kind: 'group', group, slugs } (Fork (brand groups, integrations BRAND-GROUPS-SPEC D3) --
//   stored without slugs; resolveSelection adds them, never empty)
// lists: the lists store's rows ({ id, brand, ... }; GET /api/lists?minimal=true&status=active,
//   already permission-filtered by the server). brand is the list's slug, '' when brandless.
// brands: the GET /api/brands rows ({ slug, display_name, ... }), used for labels and order only.

export const ALL = Object.freeze({ kind: 'all' });
export const NONE = Object.freeze({ kind: 'none' });

// storeLists accepts the lists store as it is held ({ results: [...] } after a response, [] before
// one, or [] for an empty response) and returns the rows.
export function storeLists(lists) {
  if (Array.isArray(lists)) {
    return lists;
  }
  if (lists && Array.isArray(lists.results)) {
    return lists.results;
  }
  return [];
}

function brandOf(list) {
  return list && typeof list.brand === 'string' ? list.brand : '';
}

// brandLabelOf is a slug's display name from the brands rows, the slug when the row is missing.
export function brandLabelOf(slug, brands) {
  const row = (Array.isArray(brands) ? brands : []).find((b) => b && b.slug === slug);
  return row && row.display_name ? row.display_name : slug;
}

// D1: rosterFrom returns { brands: [{ slug, label }], none } -- the distinct brands among the
// permitted active lists, ordered by display name (slug when the brands row is missing; a failed
// brands read therefore leaves the roster slug-labelled, never empty), and none = true when any
// of those lists is brandless.
export function rosterFrom(lists, brands) {
  const rows = storeLists(lists);
  const slugs = [];
  let none = false;
  rows.forEach((l) => {
    const b = brandOf(l);
    if (b === '') {
      none = true;
    } else if (!slugs.includes(b)) {
      slugs.push(b);
    }
  });
  const out = slugs.map((slug) => ({ slug, label: brandLabelOf(slug, brands) }));
  out.sort((a, b) => {
    const c = a.label.localeCompare(b.label, 'en', { sensitivity: 'base' });
    return c !== 0 ? c : a.slug.localeCompare(b.slug);
  });
  return { brands: out, none };
}

// Fork (2026-11-02, user): the parent brand. Presentation only -- it orders the selector (All
// brands, the parent, a divider, the rest by display name, No brand) and gets its own band above
// the Brands › Usage header; it is never a scope or a permission (the lists store already holds
// only the user's permitted lists, so a user without the parent's lists sees neither).
export const PARENT_BRAND = 'curated';

// The selector's brand options from the offered roster's brands: { parent, rest } -- the parent
// brand's entry (null when not offered) and every other brand in roster order.
export function selectorBrands(brands) {
  const list = (Array.isArray(brands) ? brands : []).filter((b) => b && typeof b.slug === 'string');
  return {
    parent: list.find((b) => b.slug === PARENT_BRAND) || null,
    rest: list.filter((b) => b.slug !== PARENT_BRAND),
  };
}

// Fork (brand groups, integrations BRAND-GROUPS-SPEC D1). The two group options of the selector.
// ESTABLISHED_BRANDS is the ONE home of the established list (I11): a brand that becomes
// established is a fork release, and adding it here moves it out of Incubating in the same change.
// A group is never a permission and never read by the server (I12): it reaches the server only as
// list_id (scopeListIds) and the Lists listing's brand parameter (listsScopeParams).
export const ESTABLISHED_BRANDS = Object.freeze(['joybelle', 'shala', 'younique']);
export const BRAND_GROUPS = Object.freeze(['incubating', 'established']);

function slugsOf(roster) {
  const brands = roster && Array.isArray(roster.brands) ? roster.brands : [];
  return brands.filter((b) => b && typeof b.slug === 'string' && b.slug !== '').map((b) => b.slug);
}

// D7: the selector's options from the offered roster's brands and the offered groups
// ([{ group, slugs }], offeredGroups) -- { groups, parent, rest, divider }. groups keeps
// BRAND_GROUPS order and only groups with members; divider is true only when something besides All
// brands is above it (a group or the parent) and something is below it (rest, or No brand shown).
export function selectorOptions(offeredBrands, groups, showNone = false) {
  const { parent, rest } = selectorBrands(offeredBrands);
  const given = Array.isArray(groups) ? groups : [];
  const out = BRAND_GROUPS
    .map((g) => given.find((x) => x && x.group === g))
    .filter((x) => x && Array.isArray(x.slugs) && x.slugs.length > 0);
  return {
    groups: out,
    parent,
    rest,
    divider: (out.length > 0 || !!parent) && (rest.length > 0 || !!showNone),
  };
}

// A selection valid as a SCOPE. Fork (brand groups, BRAND-GROUPS-SPEC D4) -- a group only with a
// non-empty slugs array (the resolved shape); a stored group without slugs is no scope, so every
// scope function treats it as All brands (scopeListIds null, never []).
function isValidSelection(s) {
  if (!s || typeof s !== 'object') {
    return false;
  }
  if (s.kind === 'all' || s.kind === 'none') {
    return true;
  }
  if (s.kind === 'group') {
    return BRAND_GROUPS.includes(s.group) && Array.isArray(s.slugs) && s.slugs.length > 0
      && s.slugs.every((x) => typeof x === 'string' && x !== '');
  }
  return s.kind === 'brand' && typeof s.slug === 'string' && s.slug !== '';
}

// Fork (brand groups, BRAND-GROUPS-SPEC D3) -- a STORED selection is valid when it is one of the
// four kinds (a group only for a name in BRAND_GROUPS, slugs not required: resolution adds them).
function isStoredSelection(s) {
  if (s && typeof s === 'object' && s.kind === 'group') {
    return BRAND_GROUPS.includes(s.group);
  }
  return isValidSelection(s);
}

// Fork (brand groups, BRAND-GROUPS-SPEC D6) -- the family rule's row test: is a brand slug in a
// (scope-valid) brand or group selection? Used only after the all/none/invalid cases are handled.
function inSelection(slug, selection) {
  if (selection.kind === 'group') {
    return selection.slugs.includes(slug);
  }
  return slug === selection.slug;
}

// D2 + D3 (listsLoaded): resolveSelection returns
//   { pending: true }                                   before the lists have loaded (I13)
//   { pending: false, selection, locked, hidden }       after
// A stored brand not in the roster, or none when the roster has no brandless list, resolves to
// all (S7). A roster of exactly one brand and no brandless list locks every stored value to that
// brand (S5). An empty roster resolves to all with the control hidden.
// Fork (launch filter, integrations LAUNCH-FILTER-SPEC D6) -- `offered` is the roster the launch
// filter leaves offered (applyLaunch). hidden and locked are decided by the FULL roster; a stored
// brand is valid only when it is in the OFFERED roster, else all (S7, against the offered set).
// Omitted, offered is the roster: today's behaviour exactly.
// Fork (brand groups, BRAND-GROUPS-SPEC D3, I3) -- `groups` is the offered groups
// ([{ group, slugs }], offeredGroups; [] when groups are unavailable). A stored group resolves to
// { kind: 'group', group, slugs } -- slugs copied, never empty -- when that group is offered, else
// to all (the pref is not rewritten). A locked or hidden selector resolves exactly as before.
export function resolveSelection(stored, roster, loaded = true, offered = roster, groups = []) {
  if (!loaded) {
    return { pending: true };
  }
  const r = roster || { brands: [], none: false };
  const brands = Array.isArray(r.brands) ? r.brands : [];
  const o = offered || { brands: [], none: false };
  const offeredBrands = Array.isArray(o.brands) ? o.brands : [];

  if (brands.length === 0 && !r.none) {
    return {
      pending: false, selection: ALL, locked: false, hidden: true,
    };
  }
  if (brands.length === 1 && !r.none) {
    return {
      pending: false, selection: { kind: 'brand', slug: brands[0].slug }, locked: true, hidden: false,
    };
  }

  let selection = ALL;
  if (isStoredSelection(stored)) {
    const g = stored.kind === 'group'
      ? (Array.isArray(groups) ? groups : []).find((x) => x && x.group === stored.group)
      : null;
    if (stored.kind === 'brand' && offeredBrands.some((b) => b.slug === stored.slug)) {
      selection = { kind: 'brand', slug: stored.slug };
    } else if (stored.kind === 'none' && r.none) {
      selection = NONE;
    } else if (g && Array.isArray(g.slugs) && g.slugs.length > 0) {
      selection = { kind: 'group', group: stored.group, slugs: [...g.slugs] };
    }
  }
  return {
    pending: false, selection, locked: false, hidden: false,
  };
}

// D3: scopeListIds is null for all (send no parameter -- every screen behaves exactly as today),
// else the ids of the permitted active lists whose brand equals the slug ('' for none). For a
// resolved brand/none selection the set is never empty (D2 resolves an empty one to all).
// Fork (brand groups, BRAND-GROUPS-SPEC D4, I5) -- a group: the lists whose brand is a member (a
// brandless list never is); a group without slugs is no scope (null, never []).
export function scopeListIds(lists, selection) {
  if (!selection || selection.kind === 'all' || !isValidSelection(selection)) {
    return null;
  }
  if (selection.kind === 'none') {
    return storeLists(lists).filter((l) => brandOf(l) === '').map((l) => l.id);
  }
  return storeLists(lists).filter((l) => brandOf(l) !== '' && inSelection(brandOf(l), selection)).map((l) => l.id);
}

// D6: the Lists listing's parameters for a selection -- tag=brand:<slug> for a brand, nobrand=true
// for none, nothing for all. Fork (brand groups, BRAND-GROUPS-SPEC D5, I8) -- a group is
// brand=<slug> per member (tag is AND-matched, so a group cannot use it).
export function listsScopeParams(selection) {
  if (!selection || selection.kind === 'all' || !isValidSelection(selection)) {
    return {};
  }
  if (selection.kind === 'none') {
    return { nobrand: true };
  }
  if (selection.kind === 'group') {
    return { brand: [...selection.slugs] };
  }
  return { tag: [`brand:${selection.slug}`] };
}

function idOf(x) {
  if (x && typeof x === 'object') {
    return x.id;
  }
  return x;
}

// D8/D10 (I6): scopeUnion is the option ids of a list picker -- the scope's ids united with every
// current-list array given (each array holds ids or { id } rows). null under all (no narrowing).
export function scopeUnion(scopeIds, currentArrays) {
  if (!Array.isArray(scopeIds)) {
    return null;
  }
  const out = [...scopeIds];
  (Array.isArray(currentArrays) ? currentArrays : []).forEach((arr) => {
    (Array.isArray(arr) ? arr : []).forEach((x) => {
      const id = idOf(x);
      if (typeof id === 'number' && id > 0 && !out.includes(id)) {
        out.push(id);
      }
    });
  });
  return out;
}

// filterByIds narrows store rows to ids; null ids returns the rows unchanged (all).
export function filterByIds(rows, ids) {
  const r = storeLists(rows);
  if (!Array.isArray(ids)) {
    return r;
  }
  return r.filter((l) => ids.includes(l.id));
}

// D8 (I12): the effective list ids of the Subscribers page -- the chosen list, else the scope,
// else null (no list filter).
export function effectiveListIds(listID, scopeIds) {
  if (listID) {
    return [listID];
  }
  if (Array.isArray(scopeIds)) {
    return [...scopeIds];
  }
  return null;
}

// The Subscribers page's segment/lang filter (LIST-GRID D7). Exported (integrations
// GLOBAL-LANG-SPEC D4, S2-11): the page's READ query is built through it too, so the read, the
// export and every by-query write carry the same filter -- the global language reaches all of
// them as the route's lang.
export function gridFilterOf(qp) {
  const out = {};
  if (qp.segment) {
    out.segment = qp.segment;
  }
  if (qp.lang) {
    out.lang = qp.lang;
  }
  return out;
}

// D8 (I12): subscriberQueryBody is the shared body of every Subscribers by-query action
// (blocklist, delete, list change) -- the page's search, query, subscription status and grid
// filter, and list_ids = the effective list ids. qp is the page's queryParams
// ({ search, queryExp, listID, subStatus, segment, lang }).
export function subscriberQueryBody(qp, scopeIds) {
  return {
    search: qp.search,
    query: qp.queryExp,
    list_ids: effectiveListIds(qp.listID, scopeIds),
    subscription_status: qp.subStatus,
    ...gridFilterOf(qp),
  };
}

// D8 (I12): subscriberExportParams is the export URL's parameters as [key, value] pairs -- one
// list_id per effective list id; ids (the checked rows) only when given.
export function subscriberExportParams(qp, scopeIds, ids) {
  const out = [];
  if (qp.search) {
    out.push(['search', qp.search]);
  } else if (qp.queryExp) {
    out.push(['query', qp.queryExp]);
  }
  (effectiveListIds(qp.listID, scopeIds) || []).forEach((id) => out.push(['list_id', String(id)]));
  if (qp.subStatus) {
    out.push(['subscription_status', qp.subStatus]);
  }
  Object.entries(gridFilterOf(qp)).forEach(([k, v]) => out.push([k, v]));
  (Array.isArray(ids) ? ids : []).forEach((id) => out.push(['id', String(id)]));
  return out;
}

// D11: a campaign's brand is the brand of its lists looked up in the active-lists store (all the
// same by S10); undefined when none of them is in the store (all deleted or all archived) --
// underivable, so no notice.
export function campaignBrand(campaignLists, lists) {
  const rows = storeLists(lists);
  const ids = (Array.isArray(campaignLists) ? campaignLists : []).map(idOf);
  const hit = rows.find((l) => ids.includes(l.id));
  return hit ? brandOf(hit) : undefined;
}

// D11 (I5): contextNotice -- the S4 notice for a record outside the selection, as the labels the
// notice text needs ({ record, selected }), or null: under all, when the record's brand equals the
// selection, when the record has no brand under none, or when the record's brand cannot be
// derived (recordBrand undefined). opts.noneLabel labels the brandless side.
// Fork (brand groups, BRAND-GROUPS-SPEC D6) -- under a group: null for a record whose brand is a
// member, else the notice with selected = the group's label (opts.groupLabels, English defaults).
export const GROUP_LABELS = Object.freeze({ incubating: 'Incubating', established: 'Established' });

export function contextNotice(recordBrand, selection, brands, opts = {}) {
  if (!selection || selection.kind === 'all' || !isValidSelection(selection)) {
    return null;
  }
  if (recordBrand === undefined || recordBrand === null) {
    return null;
  }
  const noneLabel = opts.noneLabel || 'No brand';
  const label = (slug) => (slug === '' ? noneLabel : brandLabelOf(slug, brands));
  if (selection.kind === 'group') {
    if (recordBrand !== '' && selection.slugs.includes(recordBrand)) {
      return null;
    }
    const labels = { ...GROUP_LABELS, ...(opts.groupLabels || {}) };
    return { record: label(recordBrand), selected: labels[selection.group] || selection.group };
  }
  const want = selection.kind === 'none' ? '' : selection.slug;
  if (recordBrand === want) {
    return null;
  }
  return { record: label(recordBrand), selected: label(want) };
}

// D11 + D12: the record brand the template form hands contextNotice. A brandless template is
// every brand's (D12), so under a named brand it is in scope -- undefined, no notice. Under none
// a branded template still gets its notice; under a brand a template of another brand does.
// Fork (brand groups, BRAND-GROUPS-SPEC D6) -- under a group a brandless template is in scope too.
export function templateNoticeBrand(templateBrand, selection) {
  const b = typeof templateBrand === 'string' ? templateBrand : '';
  if (b === '' && selection && (selection.kind === 'brand' || selection.kind === 'group')) {
    return undefined;
  }
  return b;
}

// D12: the Templates listing under a selection -- a brand's rows plus the brandless rows (the
// account default and the shared/official templates are every brand's); under none the brandless
// rows only; under all every row.
export function filterTemplates(templates, selection) {
  const rows = Array.isArray(templates) ? templates : [];
  if (!selection || selection.kind === 'all' || !isValidSelection(selection)) {
    return rows;
  }
  const tb = (t) => (t && typeof t.brand === 'string' ? t.brand : '');
  if (selection.kind === 'none') {
    return rows.filter((t) => tb(t) === '');
  }
  // Fork (brand groups, BRAND-GROUPS-SPEC D6) -- a group: the brandless rows plus the members'.
  return rows.filter((t) => tb(t) === '' || inSelection(tb(t), selection));
}

// Phase 2 (integrations GLOBAL-BRAND-PHASE2-SPEC D1, I2): the Media page's seed under a
// selection -- a brand opens on its slug as one context chip (tag=<slug>); none opens on Untagged
// (untagged=true; the brandless context is the untagged media); all opens on All, today's.
// Fork (brand groups, BRAND-GROUPS-SPEC D6) -- a group opens on All: media tags are AND-matched,
// so several brands cannot be one context.
export function mediaScopeContext(selection) {
  if (!selection || selection.kind === 'all' || selection.kind === 'group' || !isValidSelection(selection)) {
    return { context: [], untagged: false };
  }
  if (selection.kind === 'none') {
    return { context: [], untagged: true };
  }
  return { context: [selection.slug], untagged: false };
}

// Phase 2 (D3, I11): the brand the preset-import preview's notice is given. The preview carries
// list.brand only for a list to be created and for an existing untagged list the confirm will tag;
// an existing list of the same brand is reused with no brand, so its brand is the lists store
// row's -- undefined when the store has no such row (not visible, or archived): underivable, no
// notice. A list to be created with no brand is '' (a brandless import list).
export function presetNoticeBrand(previewList, lists) {
  if (!previewList || typeof previewList !== 'object') {
    return undefined;
  }
  if (typeof previewList.brand === 'string' && previewList.brand !== '') {
    return previewList.brand;
  }
  if (previewList.exists) {
    const row = storeLists(lists).find((l) => l && l.id === previewList.id);
    return row ? brandOf(row) : undefined;
  }
  return '';
}

// Phase 2 (D6, I6): the Brands health table under a selection -- a brand shows its own row only;
// none and all show every row (a brand row is never brandless, so none has nothing to narrow to).
// Fork (brand groups, BRAND-GROUPS-SPEC D6) -- a group shows its members' rows.
export function filterBrandRows(rows, selection) {
  const r = Array.isArray(rows) ? rows : [];
  if (!selection || (selection.kind !== 'brand' && selection.kind !== 'group') || !isValidSelection(selection)) {
    return r;
  }
  return r.filter((x) => x && typeof x.brand === 'string' && inSelection(x.brand, selection));
}

// Fork (launch filter, integrations LAUNCH-FILTER-SPEC D1/D2; the values and the rule since
// integrations listmonk/LAUNCH-STAGE-SPEC D1-D10). The navbar's All | Launched | Waitlist |
// Pre-send filter: a PRESENTATION filter on the selector's options and the two health tables;
// the filter's VALUE is never a scope (D7 -- no scope function here takes a launch value). Fork
// (brand groups, integrations BRAND-GROUPS-SPEC D1, amending LAUNCH-FILTER-SPEC D7/I6 for groups
// only): launch STATE is -- the Incubating group's members are read from the health documents
// through launchOf, and reach the server as list_id and the Lists brand parameter. The filter's
// value only decides whether a group is offered, never what it contains. docs are the
// GET /api/brands/health rows ({ brand, stage, ... }). The stage is the integrations BrandHealth
// Lambda's (lib/brand-stage.ts::stageOf, LAUNCH-STAGE-SPEC D2) -- stored and rendered here, never
// computed.
export const LAUNCH_STAGES = Object.freeze(['launched', 'waitlist', 'presend']);
export const LAUNCH_VALUES = Object.freeze(['all', ...LAUNCH_STAGES]);

// LAUNCH-STAGE-SPEC D5: an unknown value -- a stored legacy 'unlaunched' included -- resolves to
// 'all' (the pref is not rewritten).
export function normalizeLaunch(launch) {
  return LAUNCH_VALUES.includes(launch) ? launch : 'all';
}

// D3 (amended 2026-10-28): whether the selection may resolve. The lists must have loaded and,
// while the launch filter narrows, the health documents too -- before them every brand reads as
// launched, so a stored brand outside that set would resolve to All brands for one fetch.
// Fork (brand groups, BRAND-GROUPS-SPEC D3, I4) -- groupWait (groupWaits) also holds the selection
// for the health documents: a stored group's members are read from them.
export function selectionLoaded(listsLoaded, launch, healthLoaded, groupWait = false) {
  return !!listsLoaded && ((normalizeLaunch(launch) === 'all' && !groupWait) || !!healthLoaded);
}

// A page's brandKey watcher decision: { seen, changed } from the key it last fetched under
// (`seen`, null before its first fetch) and the new key. Pending never fetches; the first
// resolution only records (afterListsLoaded runs that fetch); after that a fetch is due exactly
// when the resolved key differs -- so a change that passes through pending (a launch-filter switch
// while the health documents load) is not lost, and a pending round trip back to the same key
// does not refetch.
export function brandKeyStep(seen, now) {
  if (now === 'pending') {
    return { seen, changed: false };
  }
  if (seen === null || seen === undefined) {
    return { seen: now, changed: false };
  }
  return { seen: now, changed: now !== seen };
}

// D4: the filter applies only while the selector is shown and unlocked -- decided by the FULL
// roster, as resolveSelection decides hidden/locked. Where it does not apply the store resolves
// the launch value to 'all', so a value stored under a wider roster (or by another user of the
// same browser) can never narrow a dropdown or table whose control is not on screen.
export function launchApplies(roster) {
  const r = resolveSelection(null, roster, true);
  return !r.hidden && !r.locked;
}

// LAUNCH-STAGE-SPEC D4: a brand's stage -- the document's own `stage` when it holds one of the
// three stages or null (null = no stage, a domain that is not a brand); a document without a
// `stage` key (written before the stage existed), no document, or any other value is launched.
export function launchOf(slug, docs) {
  const doc = (Array.isArray(docs) ? docs : []).find((d) => d && d.brand === slug);
  if (!doc || typeof doc !== 'object' || !Object.prototype.hasOwnProperty.call(doc, 'stage')) {
    return 'launched';
  }
  if (doc.stage === null || LAUNCH_STAGES.includes(doc.stage)) {
    return doc.stage;
  }
  return 'launched';
}

// LAUNCH-STAGE-SPEC D6: a stage button of the launch control is disabled when its count is 0 or
// while the selection is pending (All is never disabled by count).
export function launchOptionDisabled(count, pending) {
  return !!pending || count === 0;
}

// Fork (brand groups, integrations BRAND-GROUPS-SPEC D1, I1; LAUNCH-STAGE-SPEC D8) -- a brand's
// group: the parent is in none; an established brand is Established whatever its stage; any other
// brand is Incubating when its stage is launched or waitlist (the brands we send for), else in
// none (selectable one by one).
export function groupOf(slug, docs) {
  if (slug === PARENT_BRAND) {
    return null;
  }
  if (ESTABLISHED_BRANDS.includes(slug)) {
    return 'established';
  }
  const stage = launchOf(slug, docs);
  return stage === 'launched' || stage === 'waitlist' ? 'incubating' : null;
}

// D1 (I2): a group's members -- the FULL roster's brands (so never a brand with no permitted
// active list) whose group it is, sorted by slug (so equal sets give one brandKey, I6).
export function groupMembers(group, roster, docs) {
  if (!BRAND_GROUPS.includes(group)) {
    return [];
  }
  return slugsOf(roster).filter((s) => groupOf(s, docs) === group).sort();
}

// D2: groups are available only for a brands:get user (canBrands), with the selector shown and
// unlocked (the full roster), once the health documents have loaded (a failed read releases it).
export function groupsAvailable(canBrands, roster, healthLoaded) {
  return !!canBrands && launchApplies(roster) && !!healthLoaded;
}

// D1/D2: the offered groups, [{ group, slugs }] in BRAND_GROUPS order. A group is whole or absent:
// offered only when available, its members non-empty and every member in the offered roster (the
// launch filter's applyLaunch). The filter's value never changes a group's members.
export function offeredGroups(roster, offered, docs, available) {
  if (!available) {
    return [];
  }
  const offeredSlugs = slugsOf(offered);
  return BRAND_GROUPS
    .map((group) => ({ group, slugs: groupMembers(group, roster, docs) }))
    .filter((g) => g.slugs.length > 0 && g.slugs.every((s) => offeredSlugs.includes(s)));
}

// D3 (I4): a stored group waits for the health documents -- for a brands:get user with the selector
// shown and unlocked only; a user without brands:get never waits (the group resolves to all).
export function groupWaits(stored, canBrands, roster) {
  return isStoredSelection(stored) && stored.kind === 'group' && !!canBrands && launchApplies(roster);
}

// LAUNCH-STAGE-SPEC D7: 'all' (and any unknown value) matches every brand; a stage value matches
// exactly that stage; a null-stage brand matches only 'all'.
export function matchesLaunch(slug, launch, docs) {
  const l = normalizeLaunch(launch);
  return l === 'all' || launchOf(slug, docs) === l;
}

// D5: the roster narrowed to the matching brands, roster order kept; none unchanged (a brandless
// list is no brand -- No brand stays offered under every value).
export function applyLaunch(roster, launch, docs) {
  const r = roster || { brands: [], none: false };
  const brands = Array.isArray(r.brands) ? r.brands : [];
  if (normalizeLaunch(launch) === 'all') {
    return r;
  }
  return { ...r, brands: brands.filter((b) => matchesLaunch(b.slug, launch, docs)) };
}

// LAUNCH-STAGE-SPEC D7: the control's counts over the roster's brands, one per stage; a null-stage
// brand counts in none, so the three sum to the roster minus the null-stage brands.
export function launchCounts(roster, docs) {
  const brands = roster && Array.isArray(roster.brands) ? roster.brands : [];
  const out = { launched: 0, waitlist: 0, presend: 0 };
  brands.forEach((b) => {
    const stage = launchOf(b.slug, docs);
    if (stage !== null) {
      out[stage] += 1;
    }
  });
  return out;
}

// D8/D9: the table rows whose brand matches, row order kept. slugOf gives a row's brand; docs is
// the documents the launch state is read from (the Brands rows themselves, or the store's for the
// SES pane, where a row with no document -- unattributed -- is launched).
export function filterRowsByLaunch(rows, launch, slugOf, docs) {
  const r = Array.isArray(rows) ? rows : [];
  if (normalizeLaunch(launch) === 'all') {
    return r;
  }
  return r.filter((x) => matchesLaunch(slugOf(x), launch, docs));
}

// Fork (integrations EDITOR-DEFAULTS-SPEC D1) -- New needs one brand: the Campaigns page's New
// button and the navbar's New-campaign item are enabled only under a single-brand selection (never
// all, none or a selection still loading -- the store's brandSelection is all while pending). The
// ONE predicate both read. A UI ask, not a security boundary: /campaigns/new has no route guard.
export function canCreateCampaign(selection) {
  return isValidSelection(selection) && selection.kind === 'brand';
}

// D10: the brand a form presets and disables under a selection -- { value, locked }. Under all
// nothing is preset (value null, locked false). Under a brand the slug, locked, on both forms.
// Under none: the template form presets '' and locks; the list form stays enabled and required
// as today (form 'list' -- a brandless list is never created by the form, buildListRequest throws).
// Fork (brand groups, BRAND-GROUPS-SPEC D6) -- a group is several brands: as all.
export function formBrandDefault(selection, form = 'template') {
  if (!selection || selection.kind === 'all' || selection.kind === 'group' || !isValidSelection(selection)) {
    return { value: null, locked: false };
  }
  if (selection.kind === 'none') {
    return form === 'list' ? { value: null, locked: false } : { value: '', locked: true };
  }
  return { value: selection.slug, locked: true };
}

// Fork (brand roles, integrations listmonk/BRAND-ROLES-SPEC D4/D8). A brand role's analytics level
// reaches the selector through the profile's list:analytics entries (accessPolicy.mjs::
// analyticsListEntries, a separate getter) -- NEVER through the lists store, which every writing
// picker reads. Those entries make an analytics brand selectable on the analytics pages only.

// The analytics pages (route names): Campaign Analytics (its Matrix pane included), the Dashboard and
// the Brands page. Every other page is a non-analytics page.
export const ANALYTICS_PAGES = Object.freeze(['dashboard', 'campaignAnalytics', 'brands', 'brand']);

export function isAnalyticsPage(routeName) {
  return ANALYTICS_PAGES.includes(routeName);
}

// rosterLists is the selector's roster rows: the lists store's rows united with the analytics entries
// ({ id, brand }) whose id the store does not hold. Used for the roster and the scope's ids only --
// never handed to a list picker.
export function rosterLists(lists, analyticsEntries) {
  const rows = storeLists(lists);
  const ids = new Set(rows.map((l) => l && l.id));
  const extra = (Array.isArray(analyticsEntries) ? analyticsEntries : [])
    .filter((e) => e && typeof e.id === 'number' && typeof e.brand === 'string' && e.brand !== '' && !ids.has(e.id))
    .map((e) => ({ id: e.id, brand: e.brand }));
  return [...rows, ...extra];
}

// analyticsOnlyBrands is the slugs reached ONLY through analytics entries: no list of the brand is
// in the lists store (the user's get|manage lists). Sorted.
export function analyticsOnlyBrands(lists, analyticsEntries) {
  const held = new Set(storeLists(lists).map(brandOf).filter((b) => b !== ''));
  const out = [];
  (Array.isArray(analyticsEntries) ? analyticsEntries : []).forEach((e) => {
    const b = e && typeof e.brand === 'string' ? e.brand : '';
    if (b !== '' && !held.has(b) && !out.includes(b)) {
      out.push(b);
    }
  });
  return out.sort();
}

// D4 (S5): THE predicate -- an analytics-only brand is disabled in the selector on a page that is
// not an analytics page; a stored selection of it resolves to All there (resolveSelection's
// `disabled`), the pref not rewritten.
export function brandDisabledOn(slug, analyticsOnly, routeName) {
  return !isAnalyticsPage(routeName) && (Array.isArray(analyticsOnly) ? analyticsOnly : []).includes(slug);
}

// The disabled slugs for a page (brandDisabledOn over analyticsOnly).
export function disabledBrands(analyticsOnly, routeName) {
  return (Array.isArray(analyticsOnly) ? analyticsOnly : []).filter((s) => brandDisabledOn(s, analyticsOnly, routeName));
}

// The roster without the given slugs (none unchanged): the roster a selection may resolve against.
export function withoutBrands(roster, slugs) {
  const r = roster || { brands: [], none: false };
  const drop = Array.isArray(slugs) ? slugs : [];
  if (drop.length === 0) {
    return r;
  }
  return { ...r, brands: (Array.isArray(r.brands) ? r.brands : []).filter((b) => !drop.includes(b.slug)) };
}

// resolveOnPage is resolveSelection for a page: the selection resolves against the offered roster
// minus the page's disabled brands, and a selector LOCKED to a disabled brand resolves to All (still
// locked, so the control stays as it was). hidden/locked stay the full roster's.
export function resolveOnPage(stored, roster, loaded, offered, groups, disabled) {
  const off = Array.isArray(disabled) ? disabled : [];
  const r = resolveSelection(stored, roster, loaded, withoutBrands(offered, off), groups);
  if (!r.pending && r.locked && r.selection.kind === 'brand' && off.includes(r.selection.slug)) {
    return { ...r, selection: ALL };
  }
  // A group (its members come from the FULL roster, analytics-only brands included) drops the
  // page's disabled slugs; none left resolves to All (the pref is not rewritten). Review R1.
  if (!r.pending && r.selection.kind === 'group' && off.length > 0) {
    const slugs = r.selection.slugs.filter((s) => !off.includes(s));
    return { ...r, selection: slugs.length > 0 ? { ...r.selection, slugs } : ALL };
  }
  return r;
}

// A group option is disabled on a page when every member is disabled there (Review R1): it would
// resolve to All.
export function groupDisabledOn(group, disabled) {
  const off = Array.isArray(disabled) ? disabled : [];
  const slugs = group && Array.isArray(group.slugs) ? group.slugs : [];
  return off.length > 0 && slugs.length > 0 && slugs.every((s) => off.includes(s));
}

// D4: a campaign link (to the campaign page, which checkCampaignPerm guards) is rendered only when
// the campaign's brand is not analytics-only for the user.
export function campaignLinkable(brand, analyticsOnly) {
  return !(typeof brand === 'string' && brand !== '' && (Array.isArray(analyticsOnly) ? analyticsOnly : []).includes(brand));
}

// LAUNCH-STAGE-SPEC D9: GET /api/brands/launch's rows ([{ brand, stage }]) as the documents
// launchOf reads -- { brand, stage }; a row without a stage key maps to a document without one
// (launched).
export function launchDocsFrom(rows) {
  return (Array.isArray(rows) ? rows : [])
    .filter((r) => r && typeof r.brand === 'string')
    .map((r) => (Object.prototype.hasOwnProperty.call(r, 'stage') ? { brand: r.brand, stage: r.stage } : { brand: r.brand }));
}

// D8: the launch filter is shown whenever the selector is shown and unlocked (no brands:get
// condition: without it the launch state comes from GET /api/brands/launch).
export function launchFilterShown(scope) {
  return !!scope && !scope.hidden && !scope.locked;
}
