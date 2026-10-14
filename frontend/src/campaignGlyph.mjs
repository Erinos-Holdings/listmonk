// Fork (campaign review, integrations LIST-START-GLYPH-SPEC D3) -- which start-side glyph a
// Campaigns list row shows. With the review gate on, a draft/paused non-optin row shows "Open to
// inspect" unless the server marked it review_passed === true AND the user may send; a passed row
// then shows its ordinary Start/Resume/Schedule glyph. The server gate stays the control (D4):
// a true here is a courtesy, the status PUT re-evaluates.
//
// row: { status, type, send_at, review_passed } -- snake_case; the view maps its camelCased row.
// Returns 'inspect' | 'start' | 'resume' | 'schedule' | 'none'.

function ungated(row, canSend) {
  if (!canSend) return 'none';
  if (row.status === 'draft') return row.send_at ? 'schedule' : 'start';
  if (row.status === 'paused') return 'resume';
  return 'none';
}

// eslint-disable-next-line import/prefer-default-export
export function listGlyph(row, reviewEnabled, canSend) {
  const gated = !!reviewEnabled && row.type !== 'optin'
    && (row.status === 'draft' || row.status === 'paused');
  if (!gated) return ungated(row, canSend);
  if (row.review_passed === true && canSend) return ungated(row, canSend);
  return 'inspect';
}
