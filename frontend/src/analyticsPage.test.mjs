// Fork (page state) -- integrations listmonk/PAGE-STATE-SPEC I3/I4/I5 for Campaign Analytics' Campaign
// pane (D8). Run: node --test src/analyticsPage.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { ANALYTICS_LINK_KEYS, analyticsRestoreQuery, saveAnalyticsPage } from './analyticsPage.mjs'; // eslint-disable-line import/extensions

test('PAGE-STATE I3: the searched id/from/to round-trip; nothing else', () => {
  const saved = saveAnalyticsPage({
    id: ['12', '9', '12'], from: '1763000000', to: '1763600000', pane: 'campaign', lang: 'fr',
  });
  assert.deepEqual(saved, { id: ['12', '9'], from: '1763000000', to: '1763600000' });
  assert.deepEqual(analyticsRestoreQuery(JSON.parse(JSON.stringify(saved)), {}, 'campaign'), saved);
  assert.deepEqual(saveAnalyticsPage({ id: '7' }), { id: ['7'] });
});

test('PAGE-STATE I4: never replace when the route has a query, or off the Campaign pane', () => {
  const stored = saveAnalyticsPage({ id: '7', from: '1', to: '2' });
  assert.deepEqual([...ANALYTICS_LINK_KEYS], ['id', 'from', 'to']);
  ANALYTICS_LINK_KEYS.forEach((k) => {
    assert.equal(analyticsRestoreQuery(stored, { [k]: '1' }, 'campaign'), null, k);
  });
  assert.equal(analyticsRestoreQuery(stored, {}, 'matrix'), null);
  assert.equal(analyticsRestoreQuery(null, {}, 'campaign'), null);
});

test('PAGE-STATE I5: bad ids and times are dropped; no valid id is nothing to restore', () => {
  assert.equal(saveAnalyticsPage({ id: ['x', '0', '-3', '1.5'] }), null);
  assert.equal(analyticsRestoreQuery({ id: ['x'] }, {}, 'campaign'), null);
  assert.deepEqual(analyticsRestoreQuery({ id: ['4', 'x'], from: 'soon', to: -1 }, {}, 'campaign'), { id: ['4'] });
  assert.deepEqual(saveAnalyticsPage({ id: [5, 6], from: 10 }), { id: ['5', '6'], from: '10' });
});
