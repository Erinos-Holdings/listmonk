// Fork (campaign review, polish pass 2 -- integrations 2026-10-06): the Context section's pure half --
// the page per label and each line's advice note. Integrations REVIEW-TRIM-SPEC I11: a context
// line's `note` renders; the campaign-context rule's buttons and `contextKeys` are gone.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import * as advice from './contextAdvice.mjs'; // eslint-disable-line import/extensions

const {
  contextHref, contextPage, contextPath, contextRows,
} = advice;

const SES_NOTE = "The sending account's 30-day bounce or complaint rate is elevated. Send to engaged lists only — no imports and "
  + "no re-engagement queues until it reads ok — and watch the next day's health. Robbie is alerted separately.";
const BRAND_NOTE = "This brand's health is not ok (Brands page). Send to engaged lists only; Robbie is alerted separately.";
const REPORT = {
  context: [
    { label: 'Audience (send language)', value: '1362' },
    { label: 'List RUZE', value: "1204 (listmonk's cached count)" },
    {
      label: 'Brand health', value: 'issues', asOf: '2026-10-06', note: BRAND_NOTE,
    },
    { label: 'SES account (30 days)', value: 'warn', note: SES_NOTE },
    { label: 'Broadcasts running on these lists', value: '88 Fall drop' },
  ],
  items: [],
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

test('rows: the concern rule, the note on its line, the href; counts with a note are not concerns', () => {
  const rows = contextRows(REPORT, '/admin');
  assert.deepEqual(rows.map((r) => r.concern), [false, false, true, true, true]);
  assert.equal(rows[2].note, BRAND_NOTE);
  assert.equal(rows[3].note, SES_NOTE);
  assert.equal(rows[2].href, '/admin/brands');
  assert.equal(rows[2].asOf, '2026-10-06');
  assert.equal(rows[4].note, null, 'the running-broadcast line carries no note');
  assert.equal(rows[4].href, '/admin/campaigns');
  assert.equal(rows[0].note, null);
  assert.ok(rows.every((r) => !('finding' in r)), 'no finding on any line');
});

test('I11: an older report (no note, a D7.1 item) renders the lines only; contextKeys is gone', () => {
  const old = contextRows({ context: REPORT.context.map(({ note, ...c }) => c), items: [{ id: 'D7.1', verdict: 'warn', findings: [{ key: 'D7.1#1', location: 'Brand health' }] }] }, '/admin');
  assert.ok(old.every((r) => r.note === null));
  assert.equal(advice.contextKeys, undefined);
  assert.equal(advice.contextItem, undefined);
  assert.deepEqual(contextRows(null, '/admin'), []);
  assert.equal(contextRows({ context: [null] }, '/admin')[0].note, null);
});

test('I11: the window renders the note under the line in plain text, with no buttons in the Context section', () => {
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  const context = /data-cy="section-context"([\s\S]*?)<\/b-collapse>/.exec(tpl)[1];
  assert.match(context, /<p v-if="c\.note" class="context-note" data-cy="context-note">\{\{ c\.note \}\}<\/p>/);
  assert.doesNotMatch(context, /<b-button/);
  assert.doesNotMatch(context, /stage\(/);
  assert.doesNotMatch(context, /D7\.1/);
  assert.match(context, /data-cy="context-open"/);
  assert.match(context, /target="_blank" rel="noopener"/);
  assert.doesNotMatch(src, /contextKeys/);
});
