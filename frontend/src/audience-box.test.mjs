// Fork (campaign-page audience box). Run: npm run test:audience
// (node --test src/audience-box.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { audienceBox, hasEnSplit } from './audience-box.mjs'; // eslint-disable-line import/extensions

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
    kind: 'audience', tone: 'success', count: 1353, noLang: 1247,
  });
  assert.deepEqual(audienceBox({
    id: 1, status: 'scheduled', evergreen: false, audience: 0, audienceNoLang: 0, sent: 0,
  }), {
    kind: 'audience', tone: 'danger', count: 0, noLang: 0,
  });
});

test('running and paused broadcasts show sent so far in blue', () => {
  ['running', 'paused'].forEach((status) => {
    assert.deepEqual(audienceBox({
      id: 1, status, evergreen: false, audience: null, sent: 42,
    }), {
      kind: 'sending', tone: 'info', count: 42, noLang: 0,
    });
  });
});

test('an evergreen is blue with its sent count until it is done, never an audience', () => {
  ['draft', 'scheduled', 'running', 'paused'].forEach((status) => {
    assert.deepEqual(audienceBox({
      id: 1, status, evergreen: true, audience: 99, sent: 7,
    }), {
      kind: 'sending', tone: 'info', count: 7, noLang: 0,
    });
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
  assert.equal(hasEnSplit('en', 'sending', 5), false);
  assert.equal(hasEnSplit('en', 'sent', 5), false);
});
