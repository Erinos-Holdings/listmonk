// Fork (integrations DELIVERABILITY-CONSISTENCY-SPEC I1/D1/D2). Run: node --test src/blocklistedLabel.test.mjs
// A count of status='blocklisted' subscribers reads "Blocklisted", never "Blocked" ("Block" is the
// 5.7.x bounce class -- the Matrix "Blocks" column). Pins the en.json values so a rebase that
// restores upstream's or an earlier label fails here.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));

test('I1: the blocklisted-count labels read "Blocklisted" (the Matrix abbreviates it "Blockl")', () => {
  // The Matrix header is abbreviated by user request (2026-10-29) -- still never "Blocked"/"Block".
  assert.equal(en['dashboard.matrix.blocked'], 'Blockl');
  assert.equal(en['lists.grid.blocked'], 'Blocklisted');
  // "Blocks" stays the bounce-class column.
  assert.equal(en['dashboard.matrix.blocks'], 'Blocks');
});

test('D2: the scope hovers exist', () => {
  assert.ok(en['dashboard.matrix.blockedHelp']);
  assert.ok(en['dashboard.blocklistedHelp']);
});
