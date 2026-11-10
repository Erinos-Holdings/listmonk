// Fork (sunset v2, integrations SUNSET-V2-SPEC D8). The Lists page's re-engagement queue
// countdown: one dot-separated item on a queue list's grey sub-line. The ListmonkSunset Lambda
// computes {oldestDays, archiveInDays, asOf} each run and the server stores it (lists.sunset,
// PUT /api/lists/:id/sunset); this module only formats it -- the fork stores and renders, never
// computes. Since integrations SUNSET-V4-SPEC D10 every one of the four queues carries a number
// ("oldest suppressed in N d" -- 185 days from the oldest hold.at on the queue; integrations
// DOC-ALIGNMENT-SPEC D5: the screen says the day-365 state's own word, "archive" stays the
// mechanism word in code) unless it is empty (archiveInDays null: "oldest waiting 0 d"). A list
// without a sunset (every brand list) renders nothing. Tested by listSunset.test.mjs.

const int = (v) => (Number.isInteger(v) ? v : null);

// listSunsetItem is the sub-line item for a list's sunset document, or '' when there is none.
export function listSunsetItem(sunset) {
  if (!sunset || typeof sunset !== 'object' || Array.isArray(sunset)) return '';
  const archive = int(sunset.archiveInDays);
  if (archive !== null) return `oldest suppressed in ${archive} d`;
  const oldest = int(sunset.oldestDays);
  if (oldest !== null) return `oldest waiting ${oldest} d`;
  return '';
}

// withSunsetItem appends the item to the sub-line text (`#id · brand`) with the page's ' · '
// separator, or returns the text unchanged when the list has no sunset.
export function withSunsetItem(subline, sunset) {
  const item = listSunsetItem(sunset);
  return item ? `${subline} · ${item}` : subline;
}
