// Fork (global brand, integrations GLOBAL-BRAND-SPEC D1-D3, D8, D10-D12). The global brand
// selector's pure half: the roster, the resolution of the stored selection, the effective list set
// and every value a page derives from them. No Vue, no store; tested with
// `node --test src/brandScope.test.mjs` (yarn test:brand-scope).
//
// The selector is a PRESENTATION filter, never a security boundary (S1): nothing here is read by
// the server. A scope reaches it only as the existing repeatable list_id (or tag=/nobrand= on the
// Lists listing), which the server intersects with the user's permission per endpoint.
//
// selection: { kind: 'all' } | { kind: 'brand', slug } | { kind: 'none' }
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

function isValidSelection(s) {
  if (!s || typeof s !== 'object') {
    return false;
  }
  if (s.kind === 'all' || s.kind === 'none') {
    return true;
  }
  return s.kind === 'brand' && typeof s.slug === 'string' && s.slug !== '';
}

// D2 + D3 (listsLoaded): resolveSelection returns
//   { pending: true }                                   before the lists have loaded (I13)
//   { pending: false, selection, locked, hidden }       after
// A stored brand not in the roster, or none when the roster has no brandless list, resolves to
// all (S7). A roster of exactly one brand and no brandless list locks every stored value to that
// brand (S5). An empty roster resolves to all with the control hidden.
export function resolveSelection(stored, roster, loaded = true) {
  if (!loaded) {
    return { pending: true };
  }
  const r = roster || { brands: [], none: false };
  const brands = Array.isArray(r.brands) ? r.brands : [];

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
  if (isValidSelection(stored)) {
    if (stored.kind === 'brand' && brands.some((b) => b.slug === stored.slug)) {
      selection = { kind: 'brand', slug: stored.slug };
    } else if (stored.kind === 'none' && r.none) {
      selection = NONE;
    }
  }
  return {
    pending: false, selection, locked: false, hidden: false,
  };
}

// D3: scopeListIds is null for all (send no parameter -- every screen behaves exactly as today),
// else the ids of the permitted active lists whose brand equals the slug ('' for none). For a
// resolved brand/none selection the set is never empty (D2 resolves an empty one to all).
export function scopeListIds(lists, selection) {
  if (!selection || selection.kind === 'all' || !isValidSelection(selection)) {
    return null;
  }
  const want = selection.kind === 'none' ? '' : selection.slug;
  return storeLists(lists).filter((l) => brandOf(l) === want).map((l) => l.id);
}

// D6: the Lists listing's parameters for a selection -- tag=brand:<slug> for a brand, nobrand=true
// for none, nothing for all.
export function listsScopeParams(selection) {
  if (!selection || selection.kind === 'all' || !isValidSelection(selection)) {
    return {};
  }
  if (selection.kind === 'none') {
    return { nobrand: true };
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

function gridFilterOf(qp) {
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
export function contextNotice(recordBrand, selection, brands, opts = {}) {
  if (!selection || selection.kind === 'all' || !isValidSelection(selection)) {
    return null;
  }
  if (recordBrand === undefined || recordBrand === null) {
    return null;
  }
  const want = selection.kind === 'none' ? '' : selection.slug;
  if (recordBrand === want) {
    return null;
  }
  const noneLabel = opts.noneLabel || 'No brand';
  const label = (slug) => (slug === '' ? noneLabel : brandLabelOf(slug, brands));
  return { record: label(recordBrand), selected: label(want) };
}

// D11 + D12: the record brand the template form hands contextNotice. A brandless template is
// every brand's (D12), so under a named brand it is in scope -- undefined, no notice. Under none
// a branded template still gets its notice; under a brand a template of another brand does.
export function templateNoticeBrand(templateBrand, selection) {
  const b = typeof templateBrand === 'string' ? templateBrand : '';
  if (b === '' && selection && selection.kind === 'brand') {
    return undefined;
  }
  return b;
}

// D12: the Templates listing under a selection -- a brand's rows plus the brandless rows (the
// account default and the shared/Official_ templates are every brand's); under none the brandless
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
  return rows.filter((t) => tb(t) === '' || tb(t) === selection.slug);
}

// D10: the brand a form presets and disables under a selection -- { value, locked }. Under all
// nothing is preset (value null, locked false). Under a brand the slug, locked, on both forms.
// Under none: the template form presets '' and locks; the list form stays enabled and required
// as today (form 'list' -- a brandless list is never created by the form, buildListRequest throws).
export function formBrandDefault(selection, form = 'template') {
  if (!selection || selection.kind === 'all' || !isValidSelection(selection)) {
    return { value: null, locked: false };
  }
  if (selection.kind === 'none') {
    return form === 'list' ? { value: null, locked: false } : { value: '', locked: true };
  }
  return { value: selection.slug, locked: true };
}
