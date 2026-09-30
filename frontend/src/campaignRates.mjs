// Fork (campaign list rates, integrations CAMPAIGN-RATES-SPEC D5/D7/D8). Pure shaping for the
// campaigns list's Views / Clicks / Bounces cells: the rate over Sent is the value and the count
// rides in grey; with nothing sent the count stands alone. Tested by campaignRates.test.mjs
// (npm run test:rates).

const defaultFormat = new Intl.NumberFormat('en-US').format;

// rateCell returns { pct, count }: pct is count / sent as a percentage with `digits` decimals
// (not capped at 100%), or null when sent is not a finite number above 0; count is the formatted
// count. The Vue passes $utils.formatNumber as `fmt`; the en-US default keeps the output
// independent of the runtime's locale.
export function rateCell(count, sent, digits, fmt = defaultFormat) {
  const n = Number.isFinite(count) ? count : 0;
  const pct = Number.isFinite(sent) && sent > 0
    ? `${((n / sent) * 100).toFixed(digits)}%`
    : null;
  return { pct, count: fmt(n) };
}

// rateTipKey returns the i18n key for a rate cell's hover (the denominator and the counting mode),
// or null when there is no rate to explain (sent is 0 or not a finite number).
export function rateTipKey(unique, sent) {
  if (!(Number.isFinite(sent) && sent > 0)) {
    return null;
  }
  return unique ? 'campaigns.rateHelpUnique' : 'campaigns.rateHelpTotal';
}
