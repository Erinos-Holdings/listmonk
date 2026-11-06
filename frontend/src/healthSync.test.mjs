// Fork (hourly health, integrations listmonk/HOURLY-HEALTH-SPEC I3). Run:
// node --test src/healthSync.test.mjs.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  HEALTH_SYNC_MINUTE, MISSING, REREAD_DELAY_MS, conversionsAsOf, newestComputedAt, nextSyncAt, olderStamp, rereadDelay,
  syncLines, syncStamps, syncText,
} from './healthSync.mjs'; // eslint-disable-line import/extensions
import { formatMountainClock, formatMountainShortDate, mountainDay } from './mountainTime.mjs'; // eslint-disable-line import/extensions

const at = (iso) => Date.parse(iso);

test('I3: last sync = the newest computedAt among the documents; null when none carries one', () => {
  assert.equal(newestComputedAt([
    { brand: 'a', computedAt: '2026-11-06T19:00:21Z' },
    { brand: 'b', computedAt: '2026-11-06T21:00:19Z' },
    { brand: 'c', computedAt: '2026-11-06T20:00:20Z' },
    { brand: 'd' },
    { brand: 'e', computedAt: 'garbage' },
    null,
  ]), '2026-11-06T21:00:19Z');
  assert.equal(newestComputedAt([{ brand: 'd' }, { computedAt: '' }]), null);
  assert.equal(newestComputedAt([]), null);
  assert.equal(newestComputedAt(null), null);
});

test('I3: next sync = the next top of the hour strictly after now', () => {
  assert.equal(HEALTH_SYNC_MINUTE, 0);
  assert.equal(nextSyncAt(at('2026-11-06T21:14:05Z')), at('2026-11-06T22:00:00Z'));
  assert.equal(nextSyncAt(at('2026-11-06T21:59:59.999Z')), at('2026-11-06T22:00:00Z'));
  assert.equal(nextSyncAt(at('2026-11-06T22:00:00Z')), at('2026-11-06T23:00:00Z'));
  assert.equal(nextSyncAt(at('2026-11-06T23:30:00Z')), at('2026-11-07T00:00:00Z'));
});

test('I3: the re-read timer fires 90 s after each top of the hour', () => {
  assert.equal(REREAD_DELAY_MS, 90000);
  // 21:14:05 -> 22:01:30
  assert.equal(rereadDelay(at('2026-11-06T21:14:05Z')), at('2026-11-06T22:01:30Z') - at('2026-11-06T21:14:05Z'));
  // Exactly on the hour -> 90 s.
  assert.equal(rereadDelay(at('2026-11-06T22:00:00Z')), 90000);
  // Inside the 90 s -> the rest of it.
  assert.equal(rereadDelay(at('2026-11-06T22:00:30Z')), 60000);
  // Exactly at :01:30 -> the next hour's.
  assert.equal(rereadDelay(at('2026-11-06T22:01:30Z')), 60 * 60 * 1000);
});

test('I3: times are Mountain h:mm am/pm; the date only when the day is not today (Mountain)', () => {
  // 2026-11-06T21:00Z is 2:00 pm MST.
  const now = at('2026-11-06T21:14:05Z');
  assert.equal(syncText('2026-11-06T21:00:19Z', now), '2:00 pm');
  assert.equal(syncText(at('2026-11-06T22:00:00Z'), now), '3:00 pm');
  // 2026-11-07T03:00Z is still Nov 6 in Mountain (8:00 pm) -- no prefix.
  assert.equal(syncText('2026-11-07T03:00:00Z', now), '8:00 pm');
  // Yesterday (Mountain) carries the date.
  assert.equal(syncText('2026-11-05T16:30:00Z', now), 'Nov 5, 9:30 am');
  // Late on Nov 6 Mountain, the next run is tomorrow (Mountain) -> prefixed.
  const late = at('2026-11-07T06:30:00Z'); // 11:30 pm Nov 6 MST
  assert.equal(syncText(nextSyncAt(late), late), 'Nov 7, 12:00 am');
  assert.equal(syncText(null, now), MISSING);
  assert.equal(syncText('garbage', now), MISSING);
});

test('I3: syncLines combines the two (brand health alone, the conversions job left out)', () => {
  const now = at('2026-11-06T21:14:05Z');
  const BH = ['brandHealth'];
  assert.deepEqual(syncLines([{ computedAt: '2026-11-06T21:00:19Z' }], now, null, BH), { last: '2:00 pm', next: '3:00 pm', jobs: { brandHealth: '2:00 pm' } });
  assert.deepEqual(syncLines([], now, null, BH), { last: MISSING, next: '3:00 pm', jobs: { brandHealth: MISSING } });
});

test('SYNC-ALL I4: conversionsAsOf reads as_of in either key style; null when none or unparsable', () => {
  assert.equal(conversionsAsOf({ as_of: '2026-11-06T21:01:05Z', tracked_stale: false }), '2026-11-06T21:01:05Z');
  assert.equal(conversionsAsOf({ asOf: '2026-11-06T21:01:05Z' }), '2026-11-06T21:01:05Z');
  assert.equal(conversionsAsOf({}), null);
  assert.equal(conversionsAsOf(null), null);
  assert.equal(conversionsAsOf({ as_of: null }), null);
  assert.equal(conversionsAsOf({ as_of: 'garbage' }), null);
});

test('SYNC-ALL I4: Last = the OLDER of the two stamps; "—" when either is missing; the hover lists each', () => {
  const now = at('2026-11-06T21:14:05Z');
  const docs = [{ computedAt: '2026-11-06T21:00:19Z' }, { computedAt: '2026-11-06T20:00:20Z' }];
  // Both within the hour: the older (conversions 2:01 pm vs brand health 2:00 pm -> 2:00 pm).
  assert.deepEqual(syncLines(docs, now, { as_of: '2026-11-06T21:01:05Z' }), {
    last: '2:00 pm', next: '3:00 pm', jobs: { brandHealth: '2:00 pm', conversions: '2:01 pm' },
  });
  // A failed conversions job: Last shows the stale one (yesterday, date-prefixed).
  assert.deepEqual(syncLines(docs, now, { as_of: '2026-11-05T21:45:00Z' }), {
    last: 'Nov 5, 2:45 pm', next: '3:00 pm', jobs: { brandHealth: '2:00 pm', conversions: 'Nov 5, 2:45 pm' },
  });
  // Either missing -> '—', the other still listed in the hover.
  assert.deepEqual(syncLines(docs, now, {}), { last: MISSING, next: '3:00 pm', jobs: { brandHealth: '2:00 pm', conversions: MISSING } });
  assert.deepEqual(syncLines([], now, { as_of: '2026-11-06T21:01:05Z' }), { last: MISSING, next: '3:00 pm', jobs: { brandHealth: MISSING, conversions: '2:01 pm' } });
  assert.deepEqual(syncStamps(docs, { as_of: '2026-11-06T21:01:05Z' }), { brandHealth: '2026-11-06T21:00:19Z', conversions: '2026-11-06T21:01:05Z' });
  assert.equal(olderStamp({ brandHealth: '2026-11-06T21:00:19Z', conversions: '2026-11-06T21:01:05Z' }), '2026-11-06T21:00:19Z');
  assert.equal(olderStamp({ brandHealth: '2026-11-06T21:00:19Z', conversions: null }), null);
  assert.equal(olderStamp({ brandHealth: '2026-11-06T21:00:19Z', conversions: null }, ['brandHealth']), '2026-11-06T21:00:19Z');
});

test('mountainTime helpers: clock, short date and day in America/Denver, DST-correct', () => {
  assert.equal(formatMountainClock('2026-07-15T15:30:00Z'), '9:30 am');
  assert.equal(formatMountainClock('2026-01-15T15:30:00Z'), '8:30 am');
  assert.equal(formatMountainClock('nope'), '');
  assert.equal(formatMountainShortDate('2026-11-06T03:00:00Z'), 'Nov 5');
  assert.equal(mountainDay('2026-11-06T03:00:00Z'), '2026-11-05');
  assert.equal(mountainDay(at('2026-11-06T21:00:00Z')), '2026-11-06');
});
