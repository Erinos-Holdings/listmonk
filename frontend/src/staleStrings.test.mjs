// Fork -- integrations DOC-ALIGNMENT-SPEC D5 / I15: no stale phrase in the fork's user-facing
// strings. The approved Usage text (lifecycle items 8, 9, 19, 22) says day 365 is an inactivity
// suppression (never an unsubscribe), a broadcast to a re-engagement queue changes no schedule,
// the Post-purchase steps are timed from delivery and the day-0 entry needs a newsletter
// subscription; these phrases contradicted it. Scans every en.json value and the non-test frontend
// modules (src/*.mjs). A deliberate use goes in ALLOW with its key and reason.
// Run: yarn test (node --test src/*.test.mjs).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';

const STALE = Object.freeze([
  /unsubscribe[- ]confirmation/i,
  /counts as the sunset ask/i,
  /days after the first order/i,
  /every customer's first order/i,
  /oldest archives in/i,
  // Integrations DOC-ALIGNMENT-SPEC D10 (B6): an off-standard Post-purchase day is not sent.
  /Post-purchase series \(5, 14, 30 days\)\. It keeps sending/i,
]);

const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
const srcDir = new URL('./', import.meta.url);
const modules = readdirSync(srcDir).filter((f) => f.endsWith('.mjs') && !f.endsWith('.test.mjs'));

test('I15: no en.json value carries a stale phrase', () => {
  Object.entries(en).forEach(([k, v]) => {
    STALE.forEach((re) => assert.equal(re.test(String(v)), false, `${k}: ${re}`));
  });
});

test('I15: no frontend module carries a stale phrase', () => {
  assert.ok(modules.length > 10, 'the scan found the modules');
  modules.forEach((f) => {
    const text = readFileSync(new URL(f, srcDir), 'utf8');
    STALE.forEach((re) => assert.equal(re.test(text), false, `${f}: ${re}`));
  });
});

test('I15: the scan is live -- each pattern matches its old wording', () => {
  [
    'a last-chance or unsubscribe-confirmation list',
    'Either way of sending counts as the sunset ask',
    'Days after the first order',
    'every customer\'s first order of this brand',
    'oldest archives in 12 d',
    'Outside the standard Post-purchase series (5, 14, 30 days). It keeps sending',
  ].forEach((s, i) => assert.equal(STALE[i].test(s), true, s));
});
