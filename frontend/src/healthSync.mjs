// Fork (hourly health, integrations listmonk/HOURLY-HEALTH-SPEC D4/D5). The pure half of the
// navbar's brand-health sync indicator ("Last brand health sync: 2:00 pm / Next brand health
// sync: 3:00 pm") and of the Brand Usage pane's computed-at note: the newest stored
// `computedAt`, the next scheduled run, the re-read timer's delay and the date-prefix rule. No
// Vue, no DOM; tested by healthSync.test.mjs (yarn test). The fork computes nothing: the run is
// the integrations BrandHealth Lambda's, and its only signal here is the documents' `computedAt`.
import { formatMountainClock, formatMountainShortDate, mountainDay } from './mountainTime.mjs'; // eslint-disable-line import/extensions

// The BrandHealth Lambda's schedule: every hour at this minute (integrations cdk/stack.ts
// BrandHealthSchedule, cron(0 * * * ? *) -- a comment there names this constant; no test spans
// the repos). The one remaining cross-repo literal of the schedule.
export const HEALTH_SYNC_MINUTE = 0;

// The indicator re-reads the stored documents this long after each scheduled run, so the "Last"
// line follows the hourly run without a reload (a run takes ~22 s).
export const REREAD_DELAY_MS = 90 * 1000;

const HOUR_MS = 60 * 60 * 1000;
const MINUTE_MS = 60 * 1000;

export const MISSING = '—';

const ms = (v) => (typeof v === 'string' && v ? Date.parse(v) : NaN);

// newestComputedAt is the newest `computedAt` (an ISO string, returned as stored) among the
// documents (or shaped rows carrying `computedAt`), or null when none carries a parsable one.
export function newestComputedAt(docs) {
  let best = null;
  let bestMs = -Infinity;
  (Array.isArray(docs) ? docs : []).forEach((d) => {
    const v = d && d.computedAt;
    const t = ms(v);
    if (!Number.isNaN(t) && t > bestMs) {
      best = v;
      bestMs = t;
    }
  });
  return best;
}

// The scheduled run at or before `now` (epoch ms): the latest instant whose minute is
// HEALTH_SYNC_MINUTE. Every zone this UI cares about is a whole-hour offset from UTC, so the
// minute is the same in UTC and Mountain.
function runAtOrBefore(now) {
  const top = Math.floor(now / HOUR_MS) * HOUR_MS + HEALTH_SYNC_MINUTE * MINUTE_MS;
  return top <= now ? top : top - HOUR_MS;
}

// nextSyncAt is the next scheduled run strictly after `now` (epoch ms), as epoch ms.
export function nextSyncAt(now) {
  return runAtOrBefore(now) + HOUR_MS;
}

// rereadDelay is how long to wait from `now` (epoch ms) until the next re-read: REREAD_DELAY_MS
// after a scheduled run -- the current hour's when that moment is still ahead, else the next's.
export function rereadDelay(now) {
  const thisOne = runAtOrBefore(now) + REREAD_DELAY_MS;
  return (thisOne > now ? thisOne : thisOne + HOUR_MS) - now;
}

// syncText is one line's time: Mountain 'h:mm am/pm', prefixed with the Mountain short date
// ('Nov 5, 2:00 pm') only when the instant's Mountain day is not `now`'s; '—' when missing.
export function syncText(instant, now) {
  if (instant === null || instant === undefined || instant === '') {
    return MISSING;
  }
  const clock = formatMountainClock(instant);
  if (!clock) {
    return MISSING;
  }
  if (mountainDay(instant) === mountainDay(now)) {
    return clock;
  }
  return `${formatMountainShortDate(instant)}, ${clock}`;
}

// Fork (integrations SYNC-ALL-SPEC D4): the sync is BOTH display-feeding jobs -- BrandHealth
// (the documents' newest computedAt) and CampaignConversions (its stored doc's as_of, read from
// GET /api/campaigns/conversions). `conversionsMeta` is that read's body (snake_case or camelCase
// as_of), or null/{} when none is stored.

// conversionsAsOf is the meta's as_of (a parsable ISO string, returned as stored), or null.
export function conversionsAsOf(meta) {
  if (!meta || typeof meta !== 'object') {
    return null;
  }
  const v = meta.as_of !== undefined ? meta.as_of : meta.asOf;
  return Number.isNaN(ms(v)) ? null : v;
}

// syncStamps is each job's own stamp: { brandHealth, conversions } (ISO strings or null).
export function syncStamps(docs, conversionsMeta) {
  return { brandHealth: newestComputedAt(docs), conversions: conversionsAsOf(conversionsMeta) };
}

// olderStamp is the OLDER of the jobs' stamps (the weakest link), or null when either is missing.
// `jobs` names the jobs that count (default both); a job left out is ignored.
export function olderStamp(stamps, jobs = ['brandHealth', 'conversions']) {
  let worst = null;
  let worstMs = Infinity;
  for (let i = 0; i < jobs.length; i += 1) {
    const v = stamps ? stamps[jobs[i]] : null;
    const t = ms(v);
    if (Number.isNaN(t)) {
      return null;
    }
    if (t < worstMs) {
      worst = v;
      worstMs = t;
    }
  }
  return worst;
}

// syncLines is the indicator's two times: { last, next, jobs } -- last the OLDER of the jobs'
// stamps ('—' when either is missing), next the next scheduled run after `now`, jobs each job's
// own time for the hover ({ brandHealth, conversions }). With `jobs` = ['brandHealth'] (a user
// who cannot read the conversions meta) the conversions stamp is left out.
export function syncLines(docs, now, conversionsMeta = null, jobs = ['brandHealth', 'conversions']) {
  const stamps = syncStamps(docs, conversionsMeta);
  const each = {};
  jobs.forEach((j) => {
    each[j] = syncText(stamps[j], now);
  });
  return {
    last: syncText(olderStamp(stamps, jobs), now),
    next: syncText(nextSyncAt(now), now),
    jobs: each,
  };
}
