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

const clockFmt = new Intl.DateTimeFormat('en-US', {
  timeZone: 'America/Denver',
  hour: 'numeric',
  minute: '2-digit',
});

const shortDateFmt = new Intl.DateTimeFormat('en-US', {
  timeZone: 'America/Denver',
  month: 'short',
  day: 'numeric',
});

// Mountain calendar day as YYYY-MM-DD (en-CA formats ISO-style).
const dayFmt = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'America/Denver',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
});

const toDate = (v) => {
  if (!v && v !== 0) {
    return null;
  }
  const d = v instanceof Date ? v : new Date(v);
  return Number.isNaN(d.getTime()) ? null : d;
};

// formatMountain('2026-10-26T16:36:30Z') -> 'Oct 26, 2026, 10:36 AM'. Empty string for a
// falsy or unparsable value (the caller's v-if hides the line).
export function formatMountain(iso) {
  const d = toDate(iso);
  return d ? fmt.format(d) : '';
}

// Fork (hourly health, integrations HOURLY-HEALTH-SPEC D4). formatMountainClock(instant) ->
// '2:00 pm' (Mountain wall clock, lower-case am/pm, no zone); '' when unparsable. Takes an ISO
// string, a Date or epoch ms.
export function formatMountainClock(v) {
  const d = toDate(v);
  if (!d) {
    return '';
  }
  return clockFmt.format(d).replace(/\s/gu, ' ').replace(/ ?(AM|PM)$/u, (m, p) => ` ${p.toLowerCase()}`);
}

// formatMountainShortDate(instant) -> 'Nov 5' (Mountain); '' when unparsable.
export function formatMountainShortDate(v) {
  const d = toDate(v);
  return d ? shortDateFmt.format(d) : '';
}

// mountainDay(instant) -> '2026-11-05', the Mountain calendar day; '' when unparsable.
export function mountainDay(v) {
  const d = toDate(v);
  return d ? dayFmt.format(d) : '';
}
