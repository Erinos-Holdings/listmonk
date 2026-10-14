// Fork (list grid, integrations LIST-GRID-SPEC D8; dashboard counts, DASHBOARD-COUNTS-SPEC D7).
// The ONE lines rule of a subscriber grid, shared by the Lists page and the Dashboard's
// Subscribers container. Pure; tested by gridLines.test.mjs (npm run test:dashboard-subs).

// Send-language lines in display order. The API's en already includes none.
export const GRID_LANGS = ['en', 'fr', 'es', 'de', 'it', 'other'];

// The lines of one grid: "all", then one per send language with subscribers, in a fixed order.
// Language lines show only with app.lang_enable on AND when they say something the "all" line
// does not: more than one send language, a no-language residue (none, a subset of en), or an
// unrecognised value (other -- an alarm, so it is never hidden behind the one-language rule).
export function gridLines(subscriberGrid, langEnabled) {
  const grid = subscriberGrid || {};
  const lines = [{ key: 'all', row: grid.all || {} }];
  if (!langEnabled) {
    return lines;
  }

  const langs = GRID_LANGS.filter((k) => grid[k] && grid[k].total > 0);
  const hasNone = grid.none && grid.none.total > 0;
  if (langs.length > 1 || hasNone || langs.includes('other')) {
    langs.forEach((k) => lines.push({ key: k, row: grid[k], none: k === 'en' ? grid.none : null }));
  }
  return lines;
}
