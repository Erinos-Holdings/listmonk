// Mountain-time display formatting (integrations LISTMONK-RUNBOOK "UI time display rule",
// user directive 2026-10-26): no UTC anywhere in the admin UI. Raw ISO instants (the stored
// docs' computedAt etc.) are formatted in America/Denver with no zone annotation. Pure: no
// Vue, no DOM; tested by mountainTime.test.mjs.

const fmt = new Intl.DateTimeFormat('en-US', {
  timeZone: 'America/Denver',
  year: 'numeric',
  month: 'short',
  day: 'numeric',
  hour: 'numeric',
  minute: '2-digit',
});

// formatMountain('2026-10-26T16:36:30Z') -> 'Oct 26, 2026, 10:36 AM'. Empty string for a
// falsy or unparsable value (the caller's v-if hides the line).
// eslint-disable-next-line import/prefer-default-export
export function formatMountain(iso) {
  if (!iso) {
    return '';
  }
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return '';
  }
  return fmt.format(d);
}
