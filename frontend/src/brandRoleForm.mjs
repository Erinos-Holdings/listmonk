// Fork (brand roles, integrations listmonk/BRAND-ROLES-SPEC D5/D7). The Brand role form's pure half:
// the two multiselects' options, the All brands rule and the request body. No Vue; tested with
// `node --test src/brandRoleForm.test.mjs` (npm run test:brand-roles). The server validates the
// body again (cmd/roles.go::brandGrantsProblem) -- these only keep the form from offering what it
// would refuse.
//
// A role's grants: [{ brand: slug | null, level: 'manage' | 'analytics' }]; brand null is All
// brands, analytics only. The form holds two arrays of picks: manage (slugs) and read (slugs, or
// ALL_BRANDS for All brands).

export const ALL_BRANDS = '*';

function slugsOf(brands) {
  return (Array.isArray(brands) ? brands : [])
    .map((b) => (b && typeof b === 'object' ? b.slug : b))
    .filter((s) => typeof s === 'string' && s !== '' && s !== ALL_BRANDS);
}

function picks(arr) {
  return Array.isArray(arr) ? arr : [];
}

// The Read/write options: every brand not picked read-only (a brand is never at both levels).
export function manageOptions(brands, readPicks) {
  const read = picks(readPicks);
  return slugsOf(brands).filter((s) => !read.includes(s));
}

// The Read-only options: All brands first, then every brand not picked read/write.
export function readOptions(brands, managePicks) {
  const manage = picks(managePicks);
  return [ALL_BRANDS, ...slugsOf(brands).filter((s) => !manage.includes(s))];
}

// Choosing All brands makes the other read-only picks redundant: the form clears them (the manage
// picks are kept -- manage beats analytics per list). Duplicates dropped, order kept.
export function normalizeReadPicks(readPicks) {
  const r = [...new Set(picks(readPicks))];
  return r.includes(ALL_BRANDS) ? [ALL_BRANDS] : r;
}

// A pick moved to one level leaves the other (the last choice wins).
export function moveTo(level, slug, managePicks, readPicks) {
  const manage = picks(managePicks).filter((s) => s !== slug);
  const read = picks(readPicks).filter((s) => s !== slug);
  if (level === 'manage' && slug !== ALL_BRANDS) {
    manage.push(slug);
  } else if (level === 'analytics') {
    read.push(slug);
  }
  return { manage, read: normalizeReadPicks(read) };
}

// The request body for POST / PUT /api/roles/lists: { name, brands } -- never a lists field.
export function brandRolePayload(name, managePicks, readPicks) {
  const manage = [...new Set(picks(managePicks).filter((s) => typeof s === 'string' && s !== '' && s !== ALL_BRANDS))];
  const read = normalizeReadPicks(picks(readPicks).filter((s) => typeof s === 'string' && s !== '' && !manage.includes(s)));
  return {
    name,
    brands: [
      ...manage.map((brand) => ({ brand, level: 'manage' })),
      ...read.map((s) => ({ brand: s === ALL_BRANDS ? null : s, level: 'analytics' })),
    ],
  };
}

// A role's stored grants as the form's two pick arrays.
export function picksFromRole(role) {
  const grants = role && Array.isArray(role.brands) ? role.brands : [];
  const manage = [];
  const read = [];
  grants.forEach((g) => {
    if (!g) {
      return;
    }
    if (g.level === 'manage' && typeof g.brand === 'string') {
      manage.push(g.brand);
    } else if (g.level === 'analytics') {
      read.push(g.brand === null || g.brand === undefined ? ALL_BRANDS : g.brand);
    }
  });
  return { manage, read: normalizeReadPicks(read) };
}

// The roles listing's text per level: { manage: [labels], analytics: [labels] } (All brands as the
// given label).
export function grantLabels(role, labelOf, allLabel) {
  const p = picksFromRole(role);
  const label = (s) => (s === ALL_BRANDS ? allLabel : labelOf(s));
  return { manage: p.manage.map(label), analytics: p.read.map(label) };
}
