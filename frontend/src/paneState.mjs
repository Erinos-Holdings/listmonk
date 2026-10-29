// Fork (campaign matrix, integrations listmonk/CAMPAIGN-MATRIX-SPEC D1/D3). The pane choice of the
// Brands page (Matrix | Usage | Health, key brands.pane) and of Campaigns > Analytics
// (Matrix | Campaign, key analytics.pane). The Dashboard's own pane rule is
// brandMatrix.mjs::initialPane (D2: Overview | SES). No Vue; tested by paneState.test.mjs.
// Fork (matrix polish, integrations MATRIX-POLISH-SPEC D1/D2) -- Matrix first and the default on both
// pages; the choice is kept in sessionStorage, so a new browser session starts on Matrix and the last
// choice within the session is kept. A localStorage brands.pane / analytics.pane pref from before is
// ignored (no migration).

// D1: the Brands list view's panes, in pill order; `health` (the brand health table, the former
// "List") is last, `matrix` the default.
export const BRANDS_PANES = Object.freeze(['matrix', 'usage', 'health']);
export const BRANDS_PANE_PREF = 'brands.pane';

export function initialBrandsPane(stored) {
  return BRANDS_PANES.includes(stored) ? stored : 'matrix';
}

// D2: the Analytics panes, in pill order; `matrix` is the default, and a URL carrying ?id= always
// opens `campaign` (the Campaigns list's analytics glyph, a matrix row's link, a shared link)
// whatever the stored choice says -- the stored choice itself is not rewritten by that.
export const ANALYTICS_PANES = Object.freeze(['matrix', 'campaign']);
export const ANALYTICS_PANE_PREF = 'analytics.pane';

export function initialAnalyticsPane(stored, hasIds) {
  if (hasIds) {
    return 'campaign';
  }
  return ANALYTICS_PANES.includes(stored) ? stored : 'matrix';
}

// readPane / writePane: the stored choice in `store` (the Vue passes window.sessionStorage through
// brandMatrix.mjs::sessionStore). A missing store or a throwing accessor reads null and writes
// nothing -- a refused store simply does not remember.
export function readPane(store, key) {
  try {
    return store ? store.getItem(key) : null;
  } catch (e) {
    return null;
  }
}

export function writePane(store, key, value) {
  try {
    if (store) {
      store.setItem(key, value);
    }
  } catch (e) {
    // Ignore -- the choice simply isn't kept.
  }
}
