// Fork (client stats, integrations CLIENT-STATS-SPEC I7). Run: npm run test:clients. The roster
// mapping is advisory display data (D8): this pins only that it maps tokens the classifier can
// emit, never the live Mailgun roster.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { CLIENT_TOKENS } from './clientRows.mjs'; // eslint-disable-line import/extensions
import { CLIENT_ROSTER, rosterFor } from './clientRoster.mjs'; // eslint-disable-line import/extensions

test('I7 every roster mapping key is a classifier vocabulary token', () => {
  const vocab = new Set(CLIENT_TOKENS);
  Object.keys(CLIENT_ROSTER).forEach((k) => assert.ok(vocab.has(k), `mapping key ${k} is not a vocabulary token`));
});

test('I7 the mapping carries the spec D8 examples, no id twice, and rosterFor copies', () => {
  assert.ok(rosterFor('outlook-windows').includes('outlook2024_win_lm_dt'));
  assert.ok(rosterFor('outlook-windows').includes('outlook2024_win_dm_dt'));
  assert.deepEqual(rosterFor('apple-mail').slice(0, 2), ['applemail16', 'applemail16_dm']);
  assert.ok(rosterFor('gmail-proxy').includes('gmailcom-lm_chrcurrent_win10'));
  assert.deepEqual(rosterFor('thunderbird'), []);
  assert.deepEqual(rosterFor(''), []);
  assert.deepEqual(rosterFor('constructor'), []);
  const all = Object.values(CLIENT_ROSTER).flat();
  assert.equal(new Set(all).size, all.length);
  const copy = rosterFor('outlook-mac');
  copy.push('x');
  assert.equal(rosterFor('outlook-mac').length, 2);
});
