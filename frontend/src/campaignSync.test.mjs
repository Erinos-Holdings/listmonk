// Fork (editor sync) -- integrations EDITOR-SYNC-SPEC I1 (the page never PUTs over a changed server
// copy without the user choosing to: `save` only when unchanged), I2 ("changed" is content only:
// the draft view plus From, messenger and attachments) and I3 (adopt never discards unsaved edits).
// The wiring in Campaign.vue (the decision taken before the PUT, after the read resolves) is not
// testable in this runner -- see the spec's gate G4 and Stage 4.
import test from 'node:test';
import assert from 'node:assert/strict';
import { extraView, sameDraft, syncDecision } from './campaignSync.mjs'; // eslint-disable-line import/extensions

const view = (over = {}) => ({
  name: 'Spring',
  subject: 'Hello',
  preheader: '',
  lang: 'en',
  smartSending: true,
  altbody: null,
  tags: [],
  headersStr: '[]',
  attribsStr: '{\n    "lang": "en"\n}',
  archive: false,
  archiveSlug: null,
  archiveMetaStr: '{}',
  archiveTemplateId: null,
  listIds: [3],
  sendAtDate: null,
  evergreen: false,
  sendDelayDays: 0,
  content: {
    body: '', bodySource: '{"root":{}}', contentType: 'visual', templateId: null,
  },
  ...over,
});

test('I1: syncDecision -- all four (changed, unsaved) pairs', () => {
  assert.equal(syncDecision({ changed: false, unsaved: false }), 'save');
  assert.equal(syncDecision({ changed: false, unsaved: true }), 'save');
  assert.equal(syncDecision({ changed: true, unsaved: false }), 'adopt');
  assert.equal(syncDecision({ changed: true, unsaved: true }), 'ask');
});

test('I1: syncDecision -- save only when not changed', () => {
  [true, false].forEach((changed) => [true, false].forEach((unsaved) => {
    const d = syncDecision({ changed, unsaved });
    assert.equal(d === 'save', !changed, `changed=${changed} unsaved=${unsaved} -> ${d}`);
  }));
});

test('I3: syncDecision never returns adopt when unsaved is true', () => {
  [true, false].forEach((changed) => {
    assert.notEqual(syncDecision({ changed, unsaved: true }), 'adopt');
  });
});

test('I2: sameDraft -- equal views are the same; a differing bodySource, subject or attribs string is not', () => {
  assert.equal(sameDraft(view(), view()), true);
  assert.equal(sameDraft(view(), view({
    content: {
      body: '', bodySource: '{"root":{"x":1}}', contentType: 'visual', templateId: null,
    },
  })), false);
  assert.equal(sameDraft(view(), view({ subject: 'Hello!' })), false);
  assert.equal(sameDraft(view(), view({ attribsStr: '{\n    "lang": "fr"\n}' })), false);
});

test('I2: extraView -- media order and a null vs empty From do not register; a differing From, messenger or media id does', () => {
  const base = { fromEmail: 'Ruze <hello@ruze.example>', messenger: 'email', media: [{ id: 2 }, { id: 9 }] };
  assert.deepEqual(extraView(base), extraView({ ...base, media: [{ id: 9 }, { id: 2 }] }));
  assert.deepEqual(extraView({ ...base, fromEmail: null }), extraView({ ...base, fromEmail: '' }));
  assert.deepEqual(extraView({ ...base, messenger: undefined, media: null }), extraView({ ...base, messenger: '', media: [] }));
  assert.notDeepEqual(extraView(base), extraView({ ...base, fromEmail: 'Natasha at Ruze <hello@ruze.example>' }));
  assert.notDeepEqual(extraView(base), extraView({ ...base, messenger: 'postback' }));
  assert.notDeepEqual(extraView(base), extraView({ ...base, media: [{ id: 2 }, { id: 10 }] }));
  // Ids are numeric: a string id from another reader compares as the same attachment.
  assert.deepEqual(extraView({ ...base, media: [{ id: '9' }, { id: 2 }] }), extraView(base));
  assert.deepEqual(extraView().media, []);
});

test('D1: the wiring -- updateCampaign pre-reads with getCampaignSync and decides through syncDecision; focus calls syncFromServer', async () => {
  const { readFileSync } = await import('node:fs');
  const src = readFileSync(new URL('./views/Campaign.vue', import.meta.url), 'utf8');
  const upd = /\n {4}updateCampaign\(typ, suppressWarnings = false, onAsk = null\) \{[\s\S]*?\n {4}\},\n/.exec(src)[0];
  assert.match(upd, /getCampaignSync\(/);
  // The decision is taken before the PUT (putCampaign holds the PUT itself).
  assert.ok(upd.indexOf('syncDecisionFor(fresh)') > -1);
  assert.ok(upd.indexOf('syncDecisionFor(fresh)') < upd.indexOf('this.putCampaign('));
  assert.doesNotMatch(upd, /\$api\.updateCampaign\(/);
  assert.match(src, /syncDecisionFor\(fresh\) \{[\s\S]*?return syncDecision\(\{ changed, unsaved \}\);/);
  assert.match(src, /onReviewFocus\(\) \{[\s\S]*?this\.syncFromServer\(\);/);
});
