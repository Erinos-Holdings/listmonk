// Fork (brand analytics, integrations BRAND-ANALYTICS-SPEC §2.3). The UI's one reading of what a
// user may see, derived from existing grants -- never a role id beyond Super Admin (1). Pure: no
// Vue, no store; tested with `node --test src/accessPolicy.test.mjs`. The server remains the
// authority; these only shape the menu, links and route guards so a restricted user meets no 403
// toasts. Every predicate takes the /api/profile shape (camelCased) --
//   { userRole: { id, permissions: [] }, listRole: { lists: [{ id, permissions: [] }] } | null }
// -- tolerates listRole null (users without a list role) and a missing or partial profile, and
// never throws.

const SUPER_ADMIN_ROLE_ID = 1;

function userPerms(profile) {
  const perms = profile && profile.userRole && profile.userRole.permissions;
  return Array.isArray(perms) ? perms : [];
}

function isSuperAdmin(profile) {
  return !!(profile && profile.userRole && profile.userRole.id === SUPER_ADMIN_ROLE_ID);
}

function has(profile, perm) {
  return isSuperAdmin(profile) || userPerms(profile).includes(perm);
}

// D1: not Super Admin, holds campaigns:get_analytics, holds neither campaigns:get nor
// campaigns:get_all. Mirrors the Go isAnalyticsOnly (cmd/campaigns.go).
export function isAnalyticsOnly(profile) {
  if (!profile || isSuperAdmin(profile)) {
    return false;
  }
  const perms = userPerms(profile);
  return perms.includes('campaigns:get_analytics')
    && !perms.includes('campaigns:get')
    && !perms.includes('campaigns:get_all');
}

// D9: may open and save the list form. Super Admin, lists:manage_all, or per-list list:manage on
// id. lists:get_all alone is NOT enough (unlike $canList, which returns true for it).
export function canManageList(profile, id) {
  if (!profile) {
    return false;
  }
  if (isSuperAdmin(profile) || userPerms(profile).includes('lists:manage_all')) {
    return true;
  }
  const lists = profile.listRole && Array.isArray(profile.listRole.lists) ? profile.listRole.lists : [];
  return lists.some((l) => l && l.id === id && Array.isArray(l.permissions) && l.permissions.includes('list:manage'));
}

// D9: the brand pages (and the health chip's link / default-sender chip) need brands:get.
export function canViewBrand(profile) {
  return !!profile && has(profile, 'brands:get');
}

// D10: route guards for analytics-only users only. Returns the location to redirect to, or null
// (no redirect -- every other user, and a profile not yet loaded).
const TO_DASHBOARD = ['campaigns', 'campaign', 'campaignReview', 'media', 'templates'];
const TO_LISTS = ['forms', 'list'];

// eslint-disable-next-line no-unused-vars
export function routeRedirect(profile, routeName, params) {
  if (!profile || !isAnalyticsOnly(profile)) {
    return null;
  }
  if (TO_DASHBOARD.includes(routeName)) {
    return { name: 'dashboard' };
  }
  if ((routeName === 'brands' || routeName === 'brand') && !canViewBrand(profile)) {
    return { name: 'dashboard' };
  }
  if (TO_LISTS.includes(routeName)) {
    return { name: 'lists' };
  }
  return null;
}

function toDate(v) {
  if (v === null || v === undefined || v === '') {
    return null;
  }
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? null : d;
}

// D11: the default From for the selected campaigns -- the start (local midnight) of the day of the
// earliest send start, a campaign that never started counting by its created_at. Accepts the
// picker's camelCased rows (startedAt/createdAt) and raw rows (started_at/created_at). null for an
// empty selection (the caller then leaves From unchanged).
export function defaultFromDate(campaigns) {
  if (!Array.isArray(campaigns)) {
    return null;
  }
  let earliest = null;
  campaigns.forEach((c) => {
    if (!c) {
      return;
    }
    const d = toDate(c.startedAt !== undefined ? c.startedAt : c.started_at)
      || toDate(c.createdAt !== undefined ? c.createdAt : c.created_at);
    if (d && (earliest === null || d < earliest)) {
      earliest = d;
    }
  });
  if (earliest === null) {
    return null;
  }
  const out = new Date(earliest.getTime());
  out.setHours(0, 0, 0, 0);
  return out;
}
