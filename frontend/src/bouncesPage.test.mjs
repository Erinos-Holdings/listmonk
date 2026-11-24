// Fork (page state) -- integrations listmonk/PAGE-STATE-SPEC I3/I4/I5 for the Bounces page.
// Run: node --test src/bouncesPage.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { BOUNCES_LINK_KEYS, restoreBouncesPage, saveBouncesPage } from './bouncesPage.mjs'; // eslint-disable-line import/extensions

test('PAGE-STATE I3: sort and page round-trip; never the campaign_id scope, the source or a selection', () => {
  const saved = saveBouncesPage({
    orderBy: 'email', order: 'asc', page: 3, campaignID: 9, bulk: { all: true },
  });
  assert.deepEqual(saved, { orderBy: 'email', order: 'asc', page: 3 });
  assert.deepEqual(restoreBouncesPage({ ...saved, campaign_id: 9, source: 'ses' }, {}), {
    orderBy: 'email', order: 'asc', page: 3,
  });
});

test('Review R1(c): a fetch under a campaign_id or source link saves page 1', () => {
  assert.deepEqual(saveBouncesPage({ orderBy: 'email', page: 4, campaign_id: 7 }), { orderBy: 'email', order: 'desc', page: 1 });
  assert.equal(saveBouncesPage({ page: 4, source: 'ses' }).page, 1);
  assert.equal(saveBouncesPage({ page: 4, campaign_id: 0, source: '' }).page, 4);
});

test('PAGE-STATE I4: a campaign_id or source link wins', () => {
  assert.deepEqual([...BOUNCES_LINK_KEYS], ['campaign_id', 'source']);
  const stored = saveBouncesPage({ orderBy: 'email', page: 2 });
  assert.equal(restoreBouncesPage(stored, { campaign_id: '9' }), null);
  assert.equal(restoreBouncesPage(stored, { source: 'ses' }), null);
  assert.equal(restoreBouncesPage(null, {}), null);
  assert.equal(restoreBouncesPage(stored, {}).page, 2);
});

test('PAGE-STATE I5: unknown sort fields and bad pages default', () => {
  const r = restoreBouncesPage({ orderBy: 'meta', order: 'up' }, {});
  assert.deepEqual([r.orderBy, r.order], ['created_at', 'desc']);
  [0, -2, 'x', 1.5].forEach((p) => assert.equal(restoreBouncesPage({ page: p }, {}).page, 1, String(p)));
});
