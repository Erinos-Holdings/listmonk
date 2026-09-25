// Fork (campaign review) -- integrations CAMPAIGN-INSPECT-SPEC I13: applyFixes applies only the four
// D10 kinds at the named path, leaves everything else byte-identical, and reports unknown kinds as
// skipped; the PUT payload key set equals campaignPayload's.
import test from 'node:test';
import assert from 'node:assert/strict';
import { applyFixes, reviewPayload, FIX_KINDS } from './reviewFixes.mjs'; // eslint-disable-line import/extensions
import { campaignPayload, CAMPAIGN_UPDATE_KEYS } from './officialSweep.mjs'; // eslint-disable-line import/extensions

const doc = {
  root: { type: 'EmailLayout', data: { childrenIds: ['btn', 'img', 'txt', 'html'] } },
  btn: { type: 'Button', data: { style: {}, props: { text: 'Shop', url: 'https://https://ruzepouches.com/x' } } },
  img: { type: 'Image', data: { style: {}, props: { url: 'https://e/a.png', alt: 'image of the pouch', linkHref: 'https://ruzepouches.com/' } } },
  txt: { type: 'Text', data: { style: {}, props: { text: 'See <a href="http://https://ruzepouches.com/y">this</a> and [that](http://https://ruzepouches.com/y)' } } },
  html: { type: 'Html', data: { style: {}, props: { contents: '<a href="https://https://x.com/">x</a>' } } },
};

const visual = () => ({
  id: 7,
  name: 'C_EN',
  subject: '  Sale ',
  content_type: 'visual',
  body: '<p>compiled</p>',
  body_source: JSON.stringify(doc),
  attribs: { lang: 'en', preheader: '   ' },
  lists: [{ id: 15, name: 'Ruze' }],
  media: [],
  headers: [],
  archive_meta: {},
});

const html = () => ({
  id: 8,
  name: 'H',
  subject: 'Hi',
  content_type: 'html',
  body: '<a href="https://ok.com/">a</a><a href="https://https://ok.com/b">b</a><img src="x.png" alt="photo de la plage"><img src="y.png" alt="keep">',
  attribs: {},
  lists: [],
});

test('I13: subject.trim and preheader.trim apply exactly and leave the rest byte-identical', () => {
  const before = visual();
  const {
    campaign, applied, skipped, recompile,
  } = applyFixes(before, [
    { kind: 'subject.trim', from: '  Sale ', to: 'Sale' },
    { kind: 'preheader.trim', from: '   ', to: '' },
  ]);
  assert.equal(applied.length, 2);
  assert.deepEqual(skipped, []);
  assert.equal(recompile, false);
  assert.equal(campaign.subject, 'Sale');
  assert.equal(campaign.attribs.preheader, '');
  assert.equal(campaign.attribs.lang, 'en');
  assert.equal(campaign.body_source, before.body_source); // untouched, byte-identical
  assert.equal(campaign.body, before.body);
  assert.equal(before.subject, '  Sale '); // the input is never mutated
});

test('I13: link.collapseScheme on a Button, a Text block (href and markdown) and an Html block', () => {
  const { campaign, applied, recompile } = applyFixes(visual(), [
    {
      kind: 'link.collapseScheme', block: 'btn', from: 'https://https://ruzepouches.com/x', to: 'https://ruzepouches.com/x',
    },
    {
      kind: 'link.collapseScheme', block: 'txt', from: 'http://https://ruzepouches.com/y', to: 'https://ruzepouches.com/y',
    },
    {
      kind: 'link.collapseScheme', block: 'html', from: 'https://https://x.com/', to: 'https://x.com/',
    },
  ]);
  assert.equal(applied.length, 3);
  assert.equal(recompile, true);
  const d = JSON.parse(campaign.body_source);
  assert.equal(d.btn.data.props.url, 'https://ruzepouches.com/x');
  assert.equal(d.txt.data.props.text, 'See <a href="https://ruzepouches.com/y">this</a> and [that](https://ruzepouches.com/y)');
  assert.equal(d.html.data.props.contents, '<a href="https://x.com/">x</a>');
  // Everything else in the document is unchanged.
  assert.deepEqual(d.img, doc.img);
  assert.deepEqual(d.root, doc.root);
});

test('I13: alt.stripPrefix on an Image block; a stale `from` is skipped, never forced', () => {
  const { campaign, applied, skipped } = applyFixes(visual(), [
    {
      kind: 'alt.stripPrefix', block: 'img', from: 'image of the pouch', to: 'The pouch',
    },
    {
      kind: 'alt.stripPrefix', block: 'img', from: 'image of something else', to: 'X',
    },
    {
      kind: 'link.collapseScheme', block: 'nope', from: 'a', to: 'b',
    },
  ]);
  assert.equal(applied.length, 1);
  assert.equal(skipped.length, 2);
  assert.equal(JSON.parse(campaign.body_source).img.data.props.alt, 'The pouch');
});

test('I13: non-visual fixes target the n-th <a>/<img> of the stored body only', () => {
  const before = html();
  const { campaign, applied } = applyFixes(before, [
    {
      kind: 'link.collapseScheme', index: 1, from: 'https://https://ok.com/b', to: 'https://ok.com/b',
    },
    {
      kind: 'alt.stripPrefix', index: 0, from: 'photo de la plage', to: 'La plage',
    },
  ]);
  assert.equal(applied.length, 2);
  assert.equal(campaign.body, '<a href="https://ok.com/">a</a><a href="https://ok.com/b">b</a><img src="x.png" alt="La plage"><img src="y.png" alt="keep">');
  // A wrong index (value mismatch) is skipped.
  const r = applyFixes(before, [{
    kind: 'link.collapseScheme', index: 0, from: 'https://https://ok.com/b', to: 'x',
  }]);
  assert.equal(r.applied.length, 0);
  assert.equal(r.campaign.body, before.body);
});

test('I13: unknown kinds are skipped and nothing changes', () => {
  const before = visual();
  const { campaign, applied, skipped } = applyFixes(before, [{ kind: 'body.rewrite', from: 'a', to: 'b' }, null]);
  assert.equal(applied.length, 0);
  assert.equal(skipped.length, 2);
  assert.equal(skipped[0].reason, 'unknown kind');
  assert.deepEqual(campaign, before);
  assert.deepEqual(FIX_KINDS, ['subject.trim', 'preheader.trim', 'link.collapseScheme', 'alt.stripPrefix']);
});

test('I13: the PUT payload key set equals campaignPayload\'s (the Campaign.vue key set)', () => {
  const c = visual();
  const p = reviewPayload(c, '<p>new</p>');
  assert.deepEqual(Object.keys(p), Object.keys(campaignPayload(c, '<p>new</p>')));
  assert.deepEqual(Object.keys(p), [...CAMPAIGN_UPDATE_KEYS]);
  assert.equal(p.body, '<p>new</p>');
  assert.deepEqual(p.lists, [15]);
});
