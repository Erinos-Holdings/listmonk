// Fork (usage decisions, integrations USAGE-DECISIONS-SPEC I10 / D5). Run: yarn test (node --test
// src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  approverText, canSubmitReject, choicesOf, columnsOf, decisionBody, denverDay, historyOf, isStale, itemNumber,
  publishedText, replaceItem, selectedChoice, showControls, stateLine, textLines, tableOf, SECTION_SUNSET_POLICY,
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

// 2026-10-31 polish: the toggle -- the server's choices with their hovers, nothing selected until
// pressed or stored, the POST passing the choice through, and the new state lines.
test('toggle: choices, selection, body and state lines for Current | Recommended | Reject', () => {
  const rec = (over = {}) => item({ key: 'both-per-brand', choices: ['current', 'recommended', 'reject'], ...over });
  assert.deepEqual(choicesOf(rec(), t).map((c) => [c.value, c.label]), [['current', 'Current'], ['recommended', 'Recommended'], ['reject', 'Reject']]);
  assert.equal(choicesOf(rec(), t)[2].help, en['decisions.rejectRecommendedHelp']);
  assert.ok(choicesOf(rec(), t).every((c) => c.help && !c.help.startsWith('decisions.')));
  assert.deepEqual(choicesOf(item({ choices: ['accept', 'reject'] }), t).map((c) => c.value), ['accept', 'reject']);
  assert.equal(choicesOf(item({ choices: ['accept', 'reject'] }), t)[1].help, en['decisions.rejectHelp']);
  // An older server's GET (no choices) and unknown values fall back to Accept | Reject.
  assert.deepEqual(choicesOf(item(), t).map((c) => c.value), ['accept', 'reject']);
  assert.deepEqual(choicesOf(item({ choices: ['defer'] }), t).map((c) => c.value), ['accept', 'reject']);

  assert.equal(selectedChoice(rec(), null, t), null); // awaiting: nothing blue
  assert.equal(selectedChoice(rec(), 'reject', t), 'reject'); // a Reject being written
  assert.equal(selectedChoice(rec({ current: row('recommended', '2026-10-31T18:00:00Z') }), null, t), 'recommended');
  assert.equal(selectedChoice(rec({ current: row('accept', '2026-10-31T18:00:00Z') }), null, t), null); // not one of its choices

  assert.deepEqual(decisionBody(rec(), 'current', 'x'), { hash: 'h1', decision: 'current', note: '' });
  assert.deepEqual(decisionBody(rec(), 'recommended'), { hash: 'h1', decision: 'recommended', note: '' });

  assert.equal(stateLine(rec({ current: row('current', '2026-10-31T18:00:00Z') }), t).text, 'Current kept by Kaylinn Brophy · 2026-10-31');
  assert.equal(stateLine(rec({ current: row('recommended', '2026-10-31T18:00:00Z') }), t).text, 'Recommended chosen by Kaylinn Brophy · 2026-10-31');
  assert.equal(historyOf(rec({ history: [row('current', '2026-10-31T18:00:00Z')] }), t).rows[0].text, 'Current kept by Kaylinn Brophy · 2026-10-31');
});

test('D5: header from the GET, columns by section, numbers, item replace', () => {
  const view = {
    approver: 'Kaylinn Brophy',
    published: '2026-10-30',
    can_decide: false,
    items: [item(), item({ key: 'pp-entry', section: 'post-purchase', n: 8 }), item({ key: 'both-language', section: 'both', n: null })],
  };
  assert.equal(publishedText(view, t), 'Spec published 2026-10-30');
  assert.equal(approverText(view, t), 'Approver: Kaylinn Brophy');
  const cols = columnsOf(view);
  assert.deepEqual([cols.noPurchase, cols.postPurchase, cols.both].map((c) => c.map((i) => i.key)), [['np-entry'], ['pp-entry'], ['both-language']]);
  assert.equal(itemNumber(view.items[1]), '8');
  assert.equal(itemNumber(view.items[2]), '');
  const next = replaceItem(view, item({ current: row('accept', '2026-10-30T18:00:00Z') }));
  assert.equal(next.items[0].current.decision, 'accept');
  assert.equal(view.items[0].current, null);
  assert.deepEqual(columnsOf(null), {
    noPurchase: [], postPurchase: [], both: [], sunsetPolicy: [],
  });
});

// USAGE-REENGAGEMENT-SPEC F6 -- the Curated Sunset Policy block: columnsOf returns its items;
// tableOf returns the stored table verbatim, or null when absent or malformed.
test('F6: columnsOf returns sunsetPolicy; tableOf returns the stored table or null', () => {
  const table = {
    columns: ['No engagement for', 'What happens'],
    rows: [['0–180 days', 'Normal sending'], ['180–360 days*', 'Regular sends stop'], ['360+ days', 'Suppressed from marketing']],
    note: '*and at least 6 broadcasts.',
  };
  const view = {
    items: [item(), item({ key: 'sp-scope', section: 'sunset-policy', n: 15 }), item({ key: 'sp-safety', section: 'sunset-policy', n: 22 })],
    tables: { 'sunset-policy': table },
  };
  assert.deepEqual(columnsOf(view).sunsetPolicy.map((i) => i.key), ['sp-scope', 'sp-safety']);
  assert.deepEqual(columnsOf(view).noPurchase.map((i) => i.key), ['np-entry']);
  assert.equal(itemNumber(view.items[1]), '15');
  assert.equal(SECTION_SUNSET_POLICY, 'sunset-policy');
  assert.deepEqual(tableOf(view, 'sunset-policy'), table);
  assert.equal(tableOf(view, 'both'), null);
  assert.equal(tableOf({ items: [] }, 'sunset-policy'), null); // an older fork's GET
  assert.equal(tableOf(null, 'sunset-policy'), null);
  assert.equal(tableOf({ tables: { 'sunset-policy': { ...table, rows: [['only one']] } } }, 'sunset-policy'), null); // ragged
  assert.equal(tableOf({ tables: { 'sunset-policy': { ...table, columns: [] } } }, 'sunset-policy'), null);
  assert.deepEqual(tableOf({ tables: { 'sunset-policy': { columns: ['a'], rows: [] } } }, 'sunset-policy'), { columns: ['a'], rows: [], note: '' });
  // The block's strings exist, and the Vue renders the table through interpolation (no v-html).
  assert.equal(en['decisions.sunsetPolicy'], 'Curated Sunset Policy');
  // 2026-10-31 polish 2: the planned-policy line (USAGE-REENGAGEMENT-SPEC D10) is gone with its key.
  assert.equal(en['decisions.sunsetPolicyPlanned'], undefined);
  // The real lifecycle.json's table is well-formed for tableOf.
  const doc = JSON.parse(readFileSync(new URL('../../internal/decisions/lifecycle.json', import.meta.url), 'utf8'));
  const real = tableOf(doc, 'sunset-policy');
  assert.ok(real && real.rows.length === 3 && real.columns.length === 2 && real.note);
  assert.deepEqual(columnsOf(doc).sunsetPolicy.map((i) => i.n), [15, 16, 17, 18, 19, 20, 21, 22]);
});

test('textLines: the first sentence and the Recommended:/NOTE: labels are the bold leads; the recommendation is flagged to its end; text is unchanged', () => {
  const s = 'Exit on purchase (your W01–W05 request). Any paid order stops it.\nNOTE: Before.\nRecommended: Skip it.\nNOTE: A skip.\n1. A rule.';
  const lines = textLines(s);
  assert.deepEqual(lines, [
    { lead: 'Exit on purchase (your W01–W05 request).', rest: ' Any paid order stops it.', recommended: false },
    { lead: 'NOTE:', rest: ' Before.', recommended: false },
    { lead: 'Recommended:', rest: ' Skip it.', recommended: true },
    { lead: 'NOTE:', rest: ' A skip.', recommended: true },
    { lead: '', rest: '1. A rule.', recommended: true },
  ]);
  assert.equal(lines.map((l) => l.lead + l.rest).join('\n'), s);
  assert.deepEqual(textLines('Language.'), [{ lead: 'Language.', rest: '', recommended: false }]);
  // A first-line "Recommended:" is the lead, not a recommendation (internal/decisions::HasRecommended agrees).
  assert.deepEqual(textLines('Recommended:'), [{ lead: 'Recommended:', rest: '', recommended: false }]);
  assert.deepEqual(textLines(null), [{ lead: '', rest: '', recommended: false }]);
});

test('D5: the pane renders decisions through interpolation only (no v-html) and every key is in en.json', () => {
  const vue = readFileSync(new URL('./components/BrandUsage.vue', import.meta.url), 'utf8');
  assert.ok(!vue.includes('v-html'), 'BrandUsage.vue uses v-html');
  ['decisions.title', 'decisions.published', 'decisions.approver', 'decisions.both', 'decisions.accept', 'decisions.reject', 'decisions.submit', 'decisions.cancel',
    'decisions.notePlaceholder', 'decisions.accepted', 'decisions.rejected', 'decisions.awaiting', 'decisions.history',
    'decisions.voided', 'decisions.stale'].forEach((k) => assert.equal(typeof en[k], 'string', k));
});
