// Fork (campaign matrix, integrations listmonk/CAMPAIGN-MATRIX-SPEC D1/D3). The sticky pane prefs of
// the Brands page (List | Matrix | Usage, pref brands.pane) and of Campaigns > Analytics
// (Campaign | Matrix, pref analytics.pane). The Dashboard's own pane rule is
// brandMatrix.mjs::initialPane (D2: Overview | SES). No Vue; tested by paneState.test.mjs.

// D1: the Brands list view's panes; `list` (the brands table) is the default.
export const BRANDS_PANES = Object.freeze(['list', 'matrix', 'usage']);
export const BRANDS_PANE_PREF = 'brands.pane';

export function initialBrandsPane(stored) {
  return BRANDS_PANES.includes(stored) ? stored : 'list';
}

// D3: the Analytics panes; `campaign` (the single-campaign view) is the default, and a URL carrying
// ?id= always opens it (a matrix row's link, a shared link) whatever the stored pref says -- the pref
// itself is not rewritten by that.
export const ANALYTICS_PANES = Object.freeze(['campaign', 'matrix']);
export const ANALYTICS_PANE_PREF = 'analytics.pane';

export function initialAnalyticsPane(stored, hasIds) {
  if (hasIds) {
    return 'campaign';
  }
  return ANALYTICS_PANES.includes(stored) ? stored : 'campaign';
}
