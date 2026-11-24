// Integrations LIST-AUDIENCE-SPEED-SPEC I3 (and I1's merge half): which ids are read, the merge
// copies exactly the five audience fields by id, a failed read marks the row unknown, a later
// success overwrites an unknown, and a response for a campaign not on screen changes nothing.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  AUDIENCE_FIELDS, pendingIds, mergeAudience, failAudience, fillFromList, audienceState,
} from './audienceFill.mjs'; // eslint-disable-line import/extensions

const rows = () => [
  {
    id: 1, name: 'Draft', audiencePending: true, audience: null, sent: 0,
  },
  { id: 2, name: 'Finished', sent: 50 },
  {
    id: 3, name: 'Scheduled', audiencePending: true, audience: null,
  },
];

const read = {
  id: 1,
  name: 'Renamed on the server',
  sent: 99,
  audience: 120,
  audienceNoLang: 20,
  audienceSkipped: 7,
  audienceSilentSkipped: 3,
  audiencePurchasedSkipped: 1,
  smartSending: false,
};

test('pendingIds: the rows marked audiencePending, nothing else', () => {
  assert.deepEqual(pendingIds(rows()), [1, 3]);
  assert.deepEqual(pendingIds([]), []);
  assert.deepEqual(pendingIds(null), []);
  assert.deepEqual(pendingIds([{ id: 9, audiencePending: false }]), []);
});

test('the five named fields', () => {
  assert.deepEqual(AUDIENCE_FIELDS, [
    'audience', 'audienceNoLang', 'audienceSkipped', 'audienceSilentSkipped', 'audiencePurchasedSkipped',
  ]);
});

test('mergeAudience copies exactly the five fields onto the row by id and clears pending', () => {
  const rs = rows();
  assert.equal(mergeAudience(rs, read), true);
  assert.deepEqual(rs[0], {
    id: 1,
    name: 'Draft',
    sent: 0,
    audiencePending: false,
    audience: 120,
    audienceNoLang: 20,
    audienceSkipped: 7,
    audienceSilentSkipped: 3,
    audiencePurchasedSkipped: 1,
  });
  assert.equal(audienceState(rs[0]), null);
  // The other rows are untouched.
  assert.deepEqual(rs.slice(1), rows().slice(1));
  // A second response for the same row (no longer awaiting) changes nothing.
  assert.equal(mergeAudience(rs, { ...read, audience: 1 }), false);
  assert.equal(rs[0].audience, 120);
});

test('mergeAudience writes through the given setter (Vue.set on the page)', () => {
  const rs = rows();
  const seen = [];
  mergeAudience(rs, { ...read, id: 3 }, (o, k, v) => { seen.push(k); o[k] = v; }); // eslint-disable-line no-param-reassign
  assert.deepEqual(seen, [...AUDIENCE_FIELDS, 'audiencePending']);
  assert.equal(rs[2].audience, 120);
});

test('a null audience on the read is copied as null (the server count failed)', () => {
  const rs = rows();
  mergeAudience(rs, { id: 1, audience: null });
  assert.equal(rs[0].audience, null);
  assert.equal(rs[0].audienceNoLang, null);
  assert.equal(rs[0].audiencePending, false);
});

test('failAudience marks a pending row unknown, never zero', () => {
  const rs = rows();
  assert.equal(audienceState(rs[0]), 'pending');
  assert.equal(failAudience(rs, 1), true);
  assert.equal(rs[0].audienceUnknown, true);
  assert.equal(rs[0].audiencePending, false);
  assert.equal(rs[0].audience, null);
  assert.equal(audienceState(rs[0]), 'unknown');
  // A row that already has its figure is never turned unknown.
  mergeAudience(rs, { ...read, id: 3 });
  assert.equal(failAudience(rs, 3), false);
  assert.equal(audienceState(rs[2]), null);
});

test('a later success overwrites an earlier unknown', () => {
  const rs = rows();
  failAudience(rs, 1);
  assert.equal(mergeAudience(rs, read), true);
  assert.equal(rs[0].audience, 120);
  assert.equal(rs[0].audienceUnknown, false);
  assert.equal(audienceState(rs[0]), null);
});

test('a response for a campaign not on screen changes nothing', () => {
  const rs = rows();
  const before = JSON.parse(JSON.stringify(rs));
  assert.equal(mergeAudience(rs, { ...read, id: 42 }), false);
  assert.equal(failAudience(rs, 42), false);
  assert.equal(failAudience(rs, 2), false);
  assert.deepEqual(rs, before);
});

test('fillFromList merges every returned row and marks a requested id it lacks unknown', () => {
  const rs = rows();
  fillFromList(rs, [1, 3], [read, { id: 42, audience: 5 }]);
  assert.equal(rs[0].audience, 120);
  assert.equal(audienceState(rs[0]), null);
  assert.equal(audienceState(rs[2]), 'unknown');
  assert.deepEqual(rs[1], rows()[1]);
});
