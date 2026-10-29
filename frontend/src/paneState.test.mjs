// Fork (campaign matrix, integrations CAMPAIGN-MATRIX-SPEC I7) -- the pane choices; the Dashboard's
// Overview | SES (dashboard.pane: a stored matrix or usage reads as overview).
// Fork (matrix polish, integrations MATRIX-POLISH-SPEC I9) -- Brands: Matrix | Usage | Health,
// default matrix; Analytics: Matrix | Campaign, default matrix, ?id= -> campaign without writing;
// both kept in sessionStorage. Run: node --test src/paneState.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  ANALYTICS_PANES, ANALYTICS_PANE_PREF, BRANDS_PANES, BRANDS_PANE_PREF,
  initialAnalyticsPane, initialBrandsPane, readPane, writePane,
} from './paneState.mjs'; // eslint-disable-line import/extensions
import { PANES, initialPane } from './brandMatrix.mjs'; // eslint-disable-line import/extensions

// A sessionStorage stand-in: a new instance is a new browser session.
function fakeSession() {
  const m = new Map();
  return {
    getItem: (k) => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => { m.set(k, String(v)); },
    map: m,
  };
}

test('I9: brands.pane -- Matrix | Usage | Health, matrix by default, anything else matrix', () => {
  assert.deepEqual(BRANDS_PANES, ['matrix', 'usage', 'health']);
  assert.equal(initialBrandsPane(undefined), 'matrix');
  assert.equal(initialBrandsPane(null), 'matrix');
  assert.equal(initialBrandsPane('usage'), 'usage');
  assert.equal(initialBrandsPane('health'), 'health');
  assert.equal(initialBrandsPane('list'), 'matrix', 'the former List key is no pane');
  assert.equal(initialBrandsPane('ses'), 'matrix');
});

test('I9: analytics.pane -- Matrix | Campaign, matrix by default; ?id= forces campaign', () => {
  assert.deepEqual(ANALYTICS_PANES, ['matrix', 'campaign']);
  assert.equal(initialAnalyticsPane(undefined, false), 'matrix');
  assert.equal(initialAnalyticsPane('campaign', false), 'campaign');
  assert.equal(initialAnalyticsPane('matrix', true), 'campaign');
  assert.equal(initialAnalyticsPane(null, true), 'campaign');
  assert.equal(initialAnalyticsPane('bogus', false), 'matrix');
});

test('I9: the choice is kept for the session; a new session starts on matrix', () => {
  const s1 = fakeSession();
  assert.equal(initialBrandsPane(readPane(s1, BRANDS_PANE_PREF)), 'matrix');
  writePane(s1, BRANDS_PANE_PREF, 'health');
  assert.equal(initialBrandsPane(readPane(s1, BRANDS_PANE_PREF)), 'health');
  writePane(s1, ANALYTICS_PANE_PREF, 'campaign');
  assert.equal(initialAnalyticsPane(readPane(s1, ANALYTICS_PANE_PREF), false), 'campaign');

  const s2 = fakeSession(); // a new browser session
  assert.equal(initialBrandsPane(readPane(s2, BRANDS_PANE_PREF)), 'matrix');
  assert.equal(initialAnalyticsPane(readPane(s2, ANALYTICS_PANE_PREF), false), 'matrix');
});

test('I9: ?id= opens campaign and does not write the stored choice', () => {
  const s = fakeSession();
  writePane(s, ANALYTICS_PANE_PREF, 'matrix');
  assert.equal(initialAnalyticsPane(readPane(s, ANALYTICS_PANE_PREF), true), 'campaign');
  assert.equal(s.getItem(ANALYTICS_PANE_PREF), 'matrix');
  assert.equal(initialAnalyticsPane(readPane(s, ANALYTICS_PANE_PREF), false), 'matrix');
});

test('I9: no store, or a throwing one, reads null and writes nothing (matrix)', () => {
  assert.equal(readPane(null, BRANDS_PANE_PREF), null);
  writePane(null, BRANDS_PANE_PREF, 'usage');
  const bad = { getItem() { throw new Error('denied'); }, setItem() { throw new Error('denied'); } };
  assert.equal(readPane(bad, BRANDS_PANE_PREF), null);
  writePane(bad, BRANDS_PANE_PREF, 'usage');
  assert.equal(initialBrandsPane(readPane(bad, BRANDS_PANE_PREF)), 'matrix');
});

test('I7: dashboard.pane -- Overview | SES; a stored matrix or usage reads as overview', () => {
  assert.deepEqual(PANES, ['overview', 'ses']);
  assert.equal(initialPane(true, 'matrix', false), 'overview');
  assert.equal(initialPane(true, 'usage', true), 'overview');
  assert.equal(initialPane(true, 'ses', false), 'ses');
  assert.equal(initialPane(true, undefined, true), 'ses');
});
