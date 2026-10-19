// Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D8). The fit rule for the end-of-bar
// labels Chart.vue's barLabels plugin draws on a bar chart whose dataset carries `texts`: a label
// goes INSIDE the bar (white, right-aligned, `pad` px from the bar end) when it fits with `pad` on
// both sides, otherwise OUTSIDE (dark, just past the bar end). A zero (or negative) bar is always
// outside. Pure: no canvas, tested with Node's built-in runner (`yarn test`); the drawing itself
// lives in Chart.vue.

export const INSIDE = 'inside';
export const OUTSIDE = 'outside';

// barLabelPlacement returns INSIDE or OUTSIDE for a bar `barPx` long and a label `textPx` wide.
export function barLabelPlacement(barPx, textPx, pad) {
  const bar = Number(barPx);
  const text = Number(textPx);
  const p = Number(pad) || 0;
  if (!Number.isFinite(bar) || !Number.isFinite(text) || bar <= 0) {
    return OUTSIDE;
  }
  return text + 2 * p <= bar ? INSIDE : OUTSIDE;
}
