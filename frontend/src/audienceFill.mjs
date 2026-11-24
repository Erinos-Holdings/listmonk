// Fork (integrations LIST-AUDIENCE-SPEED-SPEC D3-D5, I3) -- the Campaigns page's deferred To send.
// The list is read with no_audience=true; the rows the server marks audiencePending are then read
// in ONE follow-up list request for their ids (without no_audience), and only the five audience
// fields of each returned row are copied onto the displayed row with the same id. Matched by id
// alone: the figure is the same live count whichever fetch asked for it, and a response for a
// campaign no longer on screen finds no row. Rows are mutated through `set` (Vue.set on the page)
// so the change is reactive and the row objects -- the table's checked-row identity -- are kept.

export const AUDIENCE_FIELDS = [
  'audience', 'audienceNoLang', 'audienceSkipped', 'audienceSilentSkipped', 'audiencePurchasedSkipped',
];

const assign = (o, k, v) => { o[k] = v; }; // eslint-disable-line no-param-reassign

// The ids to read: the rows the server marked pending.
export function pendingIds(rows) {
  return (rows || []).filter((r) => r && r.audiencePending === true).map((r) => r.id);
}

// A displayed row awaiting its figure: still pending, or marked unknown by an earlier failure
// (a later success overwrites "—").
function awaitingRow(rows, id) {
  return (rows || []).find((r) => r && r.id === id
    && (r.audiencePending === true || r.audienceUnknown === true)) || null;
}

// D3 -- merge one returned campaign into its row by id: the five fields, pending and unknown
// cleared. Returns whether it applied.
export function mergeAudience(rows, campaign, set = assign) {
  const row = campaign ? awaitingRow(rows, campaign.id) : null;
  if (!row) {
    return false;
  }
  AUDIENCE_FIELDS.forEach((k) => set(row, k, campaign[k] === undefined ? null : campaign[k]));
  set(row, 'audiencePending', false);
  if (row.audienceUnknown === true) {
    set(row, 'audienceUnknown', false);
  }
  return true;
}

// D4 -- a failed read: a still-pending row's figure is unknown (shown "—"), never zero.
export function failAudience(rows, id, set = assign) {
  const row = (rows || []).find((r) => r && r.id === id && r.audiencePending === true);
  if (!row) {
    return false;
  }
  set(row, 'audiencePending', false);
  set(row, 'audienceUnknown', true);
  return true;
}

// The follow-up response: merge every returned row; a requested id the response does not carry
// (deleted meanwhile, or out of scope) is unknown rather than left on "…".
export function fillFromList(rows, ids, results, set = assign) {
  const got = new Set();
  (results || []).forEach((c) => {
    if (c) {
      got.add(c.id);
      mergeAudience(rows, c, set);
    }
  });
  (ids || []).filter((id) => !got.has(id)).forEach((id) => failAudience(rows, id, set));
}

// The To send cell's state for a row: 'pending' ("…"), 'unknown' ("—"), or null (the figure, or
// no To send cell when the row carries no audience).
export function audienceState(row) {
  if (row && row.audiencePending === true) {
    return 'pending';
  }
  if (row && row.audienceUnknown === true) {
    return 'unknown';
  }
  return null;
}
