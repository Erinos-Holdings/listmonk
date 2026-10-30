// Fork (usage decisions, integrations USAGE-DECISIONS-SPEC I10 / D5). Run: yarn test (node --test
// src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  canSubmitReject, columnsOf, decisionBody, denverDay, headerText, historyOf, isStale, itemNumber,
  replaceItem, showControls, stateLine,
} from './usageDecisions.mjs'; // eslint-disable-line import/extensions

const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
const t = (k, p) => (en[k] || k).replace(/\{(\w+)\}/g, (m, name) => (p && name in p ? String(p[name]) : m));

const item = (over = {}) => ({
  key: 'np-entry', section: 'no-purchase', n: 1, since: '2026-10-30', text: 'Entry.', hash: 'h1', current: null, history: [], ...over,
});
const row = (decision, at, over = {}) => ({
  decision, note: null, user_name: 'Kaylinn Brophy', created_at: at, voided: false, ...over,
});

test('I10: dates are the America/Denver calendar day', () => {
  // 04:30Z on Oct 31 is still Oct 30 in Denver (MDT, UTC-6).
  assert.equal(denverDay('2026-10-31T04:30:00Z'), '2026-10-30');
  assert.equal(denverDay('2026-10-31T06:30:00Z'), '2026-10-31');
  // Winter (MST, UTC-7): 06:30Z on Dec 2 is Dec 1.
  assert.equal(denverDay('2026-12-02T06:30:00Z'), '2026-12-01');
  assert.equal(denverDay(''), '');
  assert.equal(denverDay('nope'), '');
});

test('I10: the state line -- Awaiting since, Accepted by, Rejected by with the note', () => {
  assert.deepEqual(stateLine(item(), t), { kind: 'awaiting', text: 'Awaiting since 2026-10-30', note: '' });
  assert.deepEqual(stateLine(item({ current: row('accept', '2026-10-31T04:30:00Z') }), t), {
    kind: 'accepted', text: 'Accepted by Kaylinn Brophy · 2026-10-30', note: '',
  });
  assert.deepEqual(stateLine(item({ current: row('reject', '2026-11-02T18:00:00Z', { note: 'Not yet.\nLater.' }) }), t), {
    kind: 'rejected', text: 'Rejected by Kaylinn Brophy · 2026-11-02', note: 'Not yet.\nLater.',
  });
});

test('I10: history newest first as stored, voided rows marked "text changed"', () => {
  const h = historyOf(item({
    history: [
      row('accept', '2026-11-03T18:00:00Z'),
      row('reject', '2026-11-01T18:00:00Z', { note: 'no', voided: true }),
    ],
  }), t);
  assert.equal(h.summary, 'History (2)');
  assert.deepEqual(h.rows, [
    {
      text: 'Accepted by Kaylinn Brophy · 2026-11-03', note: '', voided: false, mark: '',
    },
    {
      text: 'Rejected by Kaylinn Brophy · 2026-11-01', note: 'no', voided: true, mark: 'text changed',
    },
  ]);
  assert.equal(historyOf(item(), t).summary, 'History (0)');
});

test('I10: controls only on can_decide; Reject Submit disabled on a blank note', () => {
  assert.equal(showControls({ can_decide: true }), true);
  assert.equal(showControls({ can_decide: false }), false);
  assert.equal(showControls({}), false);
  assert.equal(showControls(null), false);
  assert.equal(showControls({ can_decide: 'true' }), false);
  assert.equal(canSubmitReject(''), false);
  assert.equal(canSubmitReject('  \n\t '), false);
  assert.equal(canSubmitReject(undefined), false);
  assert.equal(canSubmitReject(' why '), true);
  assert.deepEqual(decisionBody(item(), 'accept', 'ignored'), { hash: 'h1', decision: 'accept', note: '' });
  assert.deepEqual(decisionBody(item(), 'reject', ' why '), { hash: 'h1', decision: 'reject', note: ' why ' });
  assert.equal(isStale({ response: { status: 409 } }), true);
  assert.equal(isStale({ response: { status: 400 } }), false);
  assert.equal(isStale(null), false);
});

test('D5: header from the GET, columns by section, numbers, item replace', () => {
  const view = {
    approver: 'Kaylinn Brophy',
    published: '2026-10-30',
    can_decide: false,
    items: [item(), item({ key: 'pp-entry', section: 'post-purchase', n: 8 }), item({ key: 'both-language', section: 'both', n: null })],
  };
  assert.equal(headerText(view, t), 'Approver: Kaylinn Brophy · Spec published 2026-10-30');
  const cols = columnsOf(view);
  assert.deepEqual([cols.noPurchase, cols.postPurchase, cols.both].map((c) => c.map((i) => i.key)), [['np-entry'], ['pp-entry'], ['both-language']]);
  assert.equal(itemNumber(view.items[1]), '8');
  assert.equal(itemNumber(view.items[2]), '');
  const next = replaceItem(view, item({ current: row('accept', '2026-10-30T18:00:00Z') }));
  assert.equal(next.items[0].current.decision, 'accept');
  assert.equal(view.items[0].current, null);
  assert.deepEqual(columnsOf(null), { noPurchase: [], postPurchase: [], both: [] });
});

test('D5: the pane renders decisions through interpolation only (no v-html) and every key is in en.json', () => {
  const vue = readFileSync(new URL('./components/BrandUsage.vue', import.meta.url), 'utf8');
  assert.ok(!vue.includes('v-html'), 'BrandUsage.vue uses v-html');
  ['decisions.header', 'decisions.both', 'decisions.accept', 'decisions.reject', 'decisions.submit', 'decisions.cancel',
    'decisions.notePlaceholder', 'decisions.accepted', 'decisions.rejected', 'decisions.awaiting', 'decisions.history',
    'decisions.voided', 'decisions.stale'].forEach((k) => assert.equal(typeof en[k], 'string', k));
});
