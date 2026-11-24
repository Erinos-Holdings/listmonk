// Fork (integrations LIST-AUDIENCE-SPEED-SPEC D3-D5, I3) -- the Campaigns page's deferred To send.
// The list is read with no_audience=true; every row the server marks audiencePending is then read
// through GET /api/campaigns/:id and only the five audience fields are copied onto it. A read is
// tagged with the fetch generation that issued it: one that lands after the page fetched a new set
// of rows changes nothing. Rows are mutated through `set` (Vue.set on the page) so the change is
// reactive and the row objects -- the table's checked-row identity -- are kept.

export const AUDIENCE_FIELDS = [
  'audience', 'audienceNoLang', 'audienceSkipped', 'audienceSilentSkipped', 'audiencePurchasedSkipped',
];

const assign = (o, k, v) => { o[k] = v; }; // eslint-disable-line no-param-reassign

// The ids to read: the rows the server marked pending.
export function pendingIds(rows) {
  return (rows || []).filter((r) => r && r.audiencePending === true).map((r) => r.id);
}

function pendingRow(rows, gen, currentGen, id) {
  if (gen !== currentGen) {
    return null;
  }
  return (rows || []).find((r) => r && r.id === id && r.audiencePending === true) || null;
}

// D3 -- merge one read into its row by id: the five fields, pending cleared. Returns whether it
// applied (false for an older generation or a row that is not pending).
export function mergeAudience(rows, gen, currentGen, id, campaign, set = assign) {
  const row = pendingRow(rows, gen, currentGen, id);
  if (!row || !campaign) {
    return false;
  }
  AUDIENCE_FIELDS.forEach((k) => set(row, k, campaign[k] === undefined ? null : campaign[k]));
  set(row, 'audiencePending', false);
  return true;
}

// D4 -- a failed read: the figure is unknown (shown "—"), never zero.
export function failAudience(rows, gen, currentGen, id, set = assign) {
  const row = pendingRow(rows, gen, currentGen, id);
  if (!row) {
    return false;
  }
  set(row, 'audiencePending', false);
  set(row, 'audienceUnknown', true);
  return true;
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
