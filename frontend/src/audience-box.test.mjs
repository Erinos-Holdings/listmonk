// Fork (campaign-page audience box). Run: npm run test:audience
// (node --test src/audience-box.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  audienceBox, hasEnSplit, enSplit, toSendOf,
} from './audience-box.mjs'; // eslint-disable-line import/extensions

test('no box for a new campaign or an estimate the server could not make', () => {
  assert.equal(audienceBox(null), null);
  assert.equal(audienceBox({}), null);
  assert.equal(audienceBox({ id: 1, status: 'draft', evergreen: false }), null);
  assert.equal(audienceBox({
    id: 1, status: 'scheduled', evergreen: false, audience: null,
  }), null);
});

test('draft and scheduled broadcasts show the live audience: green with somebody, red with nobody', () => {
  assert.deepEqual(audienceBox({
    id: 1, status: 'draft', evergreen: false, audience: 1353, audienceNoLang: 1247, sent: 0,
  }), {
    kind: 'audience', tone: 'success', count: 1353, noLang: 1247, skipped: 0, total: 1353,
  });
  assert.deepEqual(audienceBox({
    id: 1, status: 'scheduled', evergreen: false, audience: 0, audienceNoLang: 0, sent: 0,
  }), {
    kind: 'audience', tone: 'danger', count: 0, noLang: 0, skipped: 0, total: 0,
  });
});

// Integrations SMART-SENDING-SPEC I7 -- the box and the list show audience - audience_skipped, and
// carry both numbers for the hover; all skipped is red; a skipped above the audience never goes
// negative.
test('Smart Sending: the count is the audience minus the would-be skips', () => {
  assert.deepEqual(audienceBox({
    id: 1, status: 'draft', evergreen: false, audience: 1353, audienceNoLang: 0, audienceSkipped: 53,
  }), {
    kind: 'audience', tone: 'success', count: 1300, noLang: 0, skipped: 53, total: 1353,
  });
  assert.equal(audienceBox({
    id: 1, status: 'scheduled', evergreen: false, audience: 4, audienceSkipped: 4,
  }).tone, 'danger');
  assert.deepEqual(toSendOf({ audience: 10, audienceSkipped: 3 }), { count: 7, skipped: 3, total: 10 });
  assert.deepEqual(toSendOf({ audience: 2, audienceSkipped: 9 }), { count: 0, skipped: 2, total: 2 });
  assert.deepEqual(toSendOf({ audience: 5, audienceSkipped: null }), { count: 5, skipped: 0, total: 5 });
  assert.equal(toSendOf({ audience: null, audienceSkipped: 3 }), null);
  assert.equal(toSendOf(null), null);
});

test('running and paused broadcasts show sent so far in blue', () => {
  ['running', 'paused'].forEach((status) => {
    assert.deepEqual(audienceBox({
      id: 1, status, evergreen: false, audience: null, sent: 42,
    }), {
      kind: 'sending', tone: 'info', count: 42, noLang: 0, en: null,
    });
  });
});

test('an evergreen is blue with its sent count until it is done, never an audience', () => {
  ['draft', 'scheduled', 'running', 'paused'].forEach((status) => {
    assert.deepEqual(audienceBox({
      id: 1, status, evergreen: true, audience: 99, sent: 7,
    }), {
      kind: 'sending', tone: 'info', count: 7, noLang: 0, en: 0,
    });
  });
  // The sent split rides on the evergreen's sent_en / sent_no_lang; a broadcast never splits.
  assert.deepEqual(audienceBox({
    id: 1, status: 'running', evergreen: true, sent: 175, sentEn: 52, sentNoLang: 120,
  }), {
    kind: 'sending', tone: 'info', count: 175, noLang: 120, en: 52,
  });
  assert.deepEqual(audienceBox({
    id: 1, status: 'running', evergreen: false, sent: 175, sentEn: 52, sentNoLang: 120,
  }), {
    kind: 'sending', tone: 'info', count: 175, noLang: 0, en: null,
  });
  assert.deepEqual(audienceBox({
    id: 1, status: 'finished', evergreen: true, sent: 7,
  }), {
    kind: 'sent', tone: 'grey', count: 7, noLang: 0,
  });
});

test('finished and cancelled show the sent count in grey', () => {
  ['finished', 'cancelled'].forEach((status) => {
    assert.deepEqual(audienceBox({
      id: 1, status, evergreen: false, sent: 1000, toSend: 1353,
    }), {
      kind: 'sent', tone: 'grey', count: 1000, noLang: 0,
    });
  });
});

test('the EN+ split applies to an English audience with no-language rows only', () => {
  assert.equal(hasEnSplit('en', 'audience', 1247), true);
  assert.equal(hasEnSplit('EN', 'audience', 1), true);
  assert.equal(hasEnSplit('en', 'audience', 0), false);
  assert.equal(hasEnSplit('es', 'audience', 5), false);
  assert.equal(hasEnSplit('', 'audience', 5), false);
  assert.equal(hasEnSplit('en', 'sending', 5), true);
  assert.equal(hasEnSplit('en', 'sending', 0), false);
  assert.equal(hasEnSplit('en', 'sent', 5), false);
});

test('the split arithmetic: an audience derives en, an evergreen carries it, never negative', () => {
  assert.deepEqual(enSplit(1353, 1247), { en: 106, none: 1247 });
  assert.deepEqual(enSplit(1353, 1247, undefined), { en: 106, none: 1247 });
  // Evergreen: sent counter lags the recorded rows (sent 0, one no-language row sent).
  assert.deepEqual(enSplit(0, 1, 0), { en: 0, none: 1 });
  assert.deepEqual(enSplit(175, 120, 52), { en: 52, none: 120 });
  // Defensive: a noLang above count (should not happen) still renders non-negative.
  assert.deepEqual(enSplit(3, 5), { en: 0, none: 5 });
  assert.deepEqual(enSplit(undefined, null), { en: 0, none: 0 });
});
