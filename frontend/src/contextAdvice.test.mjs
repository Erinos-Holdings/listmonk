// Fork (campaign review, polish pass 2 -- integrations 2026-10-06): the Context section's pure half --
// the page per label, the D7.1 finding per concern line, and the keys Done waits for.
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  contextHref, contextKeys, contextPage, contextPath, contextRows,
} from './contextAdvice.mjs'; // eslint-disable-line import/extensions

const f = (label, value) => ({
  key: `D7.1#${label.length}`, id: 'D7.1', evidence: `${label}: ${value}`, location: label, todo: `todo for ${label}`,
});
const REPORT = {
  context: [
    { label: 'Audience (send language)', value: '1362' },
    { label: 'List RUZE', value: "1204 (listmonk's cached count)" },
    { label: 'Brand health', value: 'issues', asOf: '2026-10-06' },
    { label: 'SES account (30 days)', value: 'ok' },
    { label: 'Broadcasts running on these lists', value: '88 Fall drop' },
  ],
  items: [{
    id: 'D7.1', tier: 'D', verdict: 'warn', findings: [f('Brand health', 'issues'), f('Broadcasts running on these lists', '88 Fall drop')],
  }],
};

test('the page per label: Brand health → /brands, SES account → the dashboard, Broadcasts running → /campaigns, else none', () => {
  assert.equal(contextPath('Brand health'), '/brands');
  assert.equal(contextPath('SES account (30 days)'), '/');
  assert.equal(contextPath('Broadcasts running on these lists'), '/campaigns');
  assert.equal(contextPath('Audience (send language)'), null);
  assert.equal(contextPage('Brand health').page, 'brands');
  assert.equal(contextHref('/admin', 'Brand health'), '/admin/brands');
  assert.equal(contextHref('/admin/', 'SES account (30 days)'), '/admin/');
  assert.equal(contextHref('/admin', 'List RUZE'), null);
});

test('rows: the concern rule, the D7.1 finding on its line, the href; counts with a note are not concerns', () => {
  const rows = contextRows(REPORT, '/admin');
  assert.deepEqual(rows.map((r) => r.concern), [false, false, true, false, true]);
  assert.equal(rows[2].finding.todo, 'todo for Brand health');
  assert.equal(rows[2].href, '/admin/brands');
  assert.equal(rows[2].asOf, '2026-10-06');
  assert.equal(rows[4].finding.location, 'Broadcasts running on these lists');
  assert.equal(rows[4].href, '/admin/campaigns');
  assert.equal(rows[0].finding, null);
  assert.equal(rows[3].finding, null);
});

test('Done waits for every D7.1 finding; a passing or absent D7.1 adds nothing', () => {
  assert.deepEqual(contextKeys(REPORT), ['D7.1#12', 'D7.1#33']);
  assert.deepEqual(contextKeys({ ...REPORT, items: [{ id: 'D7.1', verdict: 'pass', findings: [] }] }), []);
  assert.deepEqual(contextKeys({ context: [], items: [] }), []);
  assert.deepEqual(contextKeys(null), []);
  const old = contextRows({ context: REPORT.context, items: [] }, '/admin');
  assert.ok(old.every((r) => r.finding === null), 'an older report without D7.1 renders the lines only');
});
