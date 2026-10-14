// Fork (campaign review, integrations LIST-START-GLYPH-SPEC I3) -- the Campaigns list's start-side
// glyph rule. Run: node --test src/campaignGlyph.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { listGlyph } from './campaignGlyph.mjs'; // eslint-disable-line import/extensions

const row = (status, extra = {}) => ({
  status, type: 'regular', send_at: null, ...extra,
});
const ROWS = [
  ['draft', row('draft'), 'start'],
  ['scheduled draft', row('draft', { send_at: '2026-10-20T00:00:00Z' }), 'schedule'],
  ['paused', row('paused'), 'resume'],
];

test('gated row that has not passed shows inspect (absent or false), with or without send', () => {
  ROWS.forEach(([name, r]) => {
    [true, false].forEach((canSend) => {
      assert.equal(listGlyph(r, true, canSend), 'inspect', `${name} absent canSend=${canSend}`);
      assert.equal(listGlyph({ ...r, review_passed: false }, true, canSend), 'inspect', `${name} false canSend=${canSend}`);
    });
  });
});

test('passed gated row shows its start/resume/schedule glyph when the user can send', () => {
  ROWS.forEach(([name, r, want]) => {
    assert.equal(listGlyph({ ...r, review_passed: true }, true, true), want, name);
  });
});

test('passed gated row without campaigns:send keeps inspect', () => {
  ROWS.forEach(([name, r]) => {
    assert.equal(listGlyph({ ...r, review_passed: true }, true, false), 'inspect', name);
  });
});

test('only a strict true counts as passed', () => {
  assert.equal(listGlyph(row('paused', { review_passed: 'true' }), true, true), 'inspect');
  assert.equal(listGlyph(row('paused', { review_passed: 1 }), true, true), 'inspect');
});

test('gate off: the ungated glyphs, whatever review_passed says', () => {
  ROWS.forEach(([name, r, want]) => {
    assert.equal(listGlyph(r, false, true), want, name);
    assert.equal(listGlyph({ ...r, review_passed: false }, false, true), want, `${name} false`);
  });
});

test('optin is never inspect', () => {
  ROWS.forEach(([name, r, want]) => {
    const o = { ...r, type: 'optin' };
    assert.equal(listGlyph(o, true, true), want, name);
    assert.notEqual(listGlyph(o, true, false), 'inspect', `${name} no send`);
    assert.notEqual(listGlyph({ ...o, review_passed: false }, true, true), 'inspect', `${name} false`);
  });
});

test('other statuses show no start-side glyph', () => {
  ['running', 'scheduled', 'finished', 'cancelled'].forEach((st) => {
    [true, false].forEach((enabled) => {
      assert.equal(listGlyph(row(st, { review_passed: true }), enabled, true), 'none', `${st} ${enabled}`);
    });
  });
});

test('no campaigns:send: no start-side glyph on an ungated row', () => {
  assert.equal(listGlyph(row('draft'), false, false), 'none');
  assert.equal(listGlyph(row('paused', { type: 'optin' }), true, false), 'none');
});
