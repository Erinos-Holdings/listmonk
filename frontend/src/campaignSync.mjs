// Fork (editor sync, integrations EDITOR-SYNC-SPEC D1/D2) -- the campaign page's one rule for a
// server copy that changed under it (the Inspect window's Fix for me, a second tab, an API writer),
// pure half. Campaign.vue::updateCampaign reads the campaign before every save of an existing
// campaign, and onReviewFocus reads it when the user comes back to the page; both decide here.
//
// The compared view is the page's own whitelisted draft view (Campaign.vue draftFromCampaign /
// draftFromForm -- the shape isUnsaved() compares) PLUS the three fields the PUT sends that the draft
// view omits (S3): the From, the messenger and the attachments (`extraView`). Content only: counts,
// status and updated_at are in neither, so a send count or a status change elsewhere never reads as
// "changed" (I2).

// Two draft views are the same content (I2). Both arguments are draft views, never raw campaigns.
export function sameDraft(a, b) {
  return JSON.stringify(a) === JSON.stringify(b);
}

// The three PUT fields the draft view omits, normalized: a null and an empty From are equal, and the
// attachments compare as their numeric ids, sorted (order is not content).
export function extraView({ fromEmail, messenger, media } = {}) {
  return {
    fromEmail: fromEmail || '',
    messenger: messenger || '',
    media: (Array.isArray(media) ? media : [])
      .map((m) => Number(m && typeof m === 'object' ? m.id : m))
      .sort((x, y) => x - y),
  };
}

// The decision (I1/I3): `save` whenever the server copy is unchanged (whatever `unsaved` is);
// `adopt` (load the server's copy, no PUT) when it changed and the page holds no edits; `ask` when it
// changed AND the page holds edits. Never `adopt` over unsaved edits.
export function syncDecision({ changed, unsaved }) {
  if (!changed) return 'save';
  return unsaved ? 'ask' : 'adopt';
}
