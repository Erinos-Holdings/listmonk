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

// Fork (global language, integrations GLOBAL-LANG-SPEC D5/D7, I11). A grid's subscriber counts
// under a language selection L ('all' | en | es | fr | de | it, already the EFFECTIVE selection --
// All while app.lang_enable is off). lineOf is L's row -- the send-language line, so EN includes
// the no-language subscribers (the grid's en line, which its EN link matches) -- and a language
// with no key in the grid reads ZERO, never the all line. Under All it is the all row.
export const ZERO_ROW = Object.freeze({
  active: 0, held: 0, unsubscribed: 0, pending: 0, blocked: 0, total: 0,
});

export function lineOf(subscriberGrid, selection) {
  const grid = subscriberGrid || {};
  const sel = typeof selection === 'string' && selection ? selection : 'all';
  if (sel === 'all') {
    return grid.all || {};
  }
  return grid[sel] || ZERO_ROW;
}

// The Lists page's lines under L (D7): under All, gridLines unchanged; under L, L's line alone
// (with the en line's no-language subset for its tooltip), whether or not the row is expanded.
export function langGridLines(subscriberGrid, langEnabled, selection) {
  const sel = typeof selection === 'string' && selection ? selection : 'all';
  if (!langEnabled || sel === 'all') {
    return gridLines(subscriberGrid, langEnabled);
  }
  const grid = subscriberGrid || {};
  return [{ key: sel, row: lineOf(grid, sel), none: sel === 'en' ? (grid.none || null) : null }];
}
