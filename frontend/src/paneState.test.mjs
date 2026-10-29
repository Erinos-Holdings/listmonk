// Fork (campaign matrix, integrations CAMPAIGN-MATRIX-SPEC I7) -- the pane prefs: the Brands page's
// List | Matrix | Usage (brands.pane, default list), Campaigns > Analytics' Campaign | Matrix
// (analytics.pane, default campaign; ?id= forces campaign), and the Dashboard's Overview | SES
// (dashboard.pane: a stored matrix or usage reads as overview). Run: node --test src/paneState.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  ANALYTICS_PANES, BRANDS_PANES, initialAnalyticsPane, initialBrandsPane,
} from './paneState.mjs'; // eslint-disable-line import/extensions
import { PANES, initialPane } from './brandMatrix.mjs'; // eslint-disable-line import/extensions

test('I7: brands.pane -- list by default, matrix and usage kept, anything else list', () => {
  assert.deepEqual(BRANDS_PANES, ['list', 'matrix', 'usage']);
  assert.equal(initialBrandsPane(undefined), 'list');
  assert.equal(initialBrandsPane(null), 'list');
  assert.equal(initialBrandsPane('matrix'), 'matrix');
  assert.equal(initialBrandsPane('usage'), 'usage');
  assert.equal(initialBrandsPane('ses'), 'list');
});

test('I7: analytics.pane -- campaign by default; ?id= forces campaign over a stored matrix', () => {
  assert.deepEqual(ANALYTICS_PANES, ['campaign', 'matrix']);
  assert.equal(initialAnalyticsPane(undefined, false), 'campaign');
  assert.equal(initialAnalyticsPane('matrix', false), 'matrix');
  assert.equal(initialAnalyticsPane('matrix', true), 'campaign');
  assert.equal(initialAnalyticsPane('bogus', false), 'campaign');
});

test('I7: dashboard.pane -- Overview | SES; a stored matrix or usage reads as overview', () => {
  assert.deepEqual(PANES, ['overview', 'ses']);
  assert.equal(initialPane(true, 'matrix', false), 'overview');
  assert.equal(initialPane(true, 'usage', true), 'overview');
  assert.equal(initialPane(true, 'ses', false), 'ses');
  assert.equal(initialPane(true, undefined, true), 'ses');
});
