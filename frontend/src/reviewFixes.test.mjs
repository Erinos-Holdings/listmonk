// Fork (campaign review) -- integrations CAMPAIGN-INSPECT-SPEC I13: applyFixes applies only the four
// D10 kinds at the named path, leaves everything else byte-identical, and reports unknown kinds as
// skipped; the PUT payload key set equals campaignPayload's.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  applyFixes, reviewPayload, FIX_KINDS, dispositionsAfterFixes,
} from './reviewFixes.mjs'; // eslint-disable-line import/extensions
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
  // Must agree with integrations lib/campaign-review/report.ts FIX_KINDS (ALT-TEXT-SPEC §8).
  assert.deepEqual(FIX_KINDS, ['subject.trim', 'preheader.trim', 'link.collapseScheme', 'alt.stripPrefix', 'alt.set']);
});

// ALT-TEXT-SPEC I8: alt.set on an Image block and on the n-th <img>; identical fixes apply once.
const sliceDoc = {
  root: { type: 'EmailLayout', data: { childrenIds: ['s1', 's2', 's3', 'btn'] } },
  s1: { type: 'Image', data: { style: {}, props: { url: 'https://e/1.jpg', alt: 'Shop the fall collection now with free shipping on every single order' } } },
  s2: { type: 'Image', data: { style: {}, props: { url: 'https://e/2.jpg', alt: 'Ruze Mailing List' } } },
  s3: { type: 'Image', data: { style: {}, props: { url: 'https://e/3.jpg' } } },
  btn: { type: 'Button', data: { style: {}, props: { text: 'Shop', url: 'https://x.com/' } } },
};
const sliced = () => ({ ...visual(), body_source: JSON.stringify(sliceDoc) });

test('I8: alt.set on an Image block — the from guard, the undecided guard, a blank `to`', () => {
  const { campaign, applied, skipped, recompile } = applyFixes(sliced(), [
    { kind: 'alt.set', block: 's1', from: sliceDoc.s1.data.props.alt, to: 'Shop the fall collection' },
    { kind: 'alt.set', block: 's2', from: 'Ruze Mailing List', to: '' },
    { kind: 'alt.set', block: 's3', from: '', to: '', undecided: true },
  ]);
  assert.equal(applied.length, 3);
  assert.deepEqual(skipped, []);
  assert.equal(recompile, true);
  const d = JSON.parse(campaign.body_source);
  assert.equal(d.s1.data.props.alt, 'Shop the fall collection');
  assert.equal(d.s2.data.props.alt, '');
  assert.equal(d.s3.data.props.alt, '');
  assert.deepEqual(d.btn, sliceDoc.btn);
  assert.deepEqual(d.root, sliceDoc.root);
});

test('I8: alt.set is skipped on a stale from, on a decided alt under undecided, and on a non-Image block', () => {
  const { campaign, applied, skipped } = applyFixes(sliced(), [
    { kind: 'alt.set', block: 's2', from: 'something else', to: 'X' },
    { kind: 'alt.set', block: 's2', from: '', to: 'X', undecided: true },
    { kind: 'alt.set', block: 's3', from: '', to: 'X' }, // undecided alt, but the fix claims a decided ""
    { kind: 'alt.set', block: 'btn', from: '', to: 'X', undecided: true },
  ]);
  assert.equal(applied.length, 0);
  assert.equal(skipped.length, 4);
  assert.equal(campaign.body_source, JSON.stringify(sliceDoc));
});

test('I8: alt.set on the n-th <img> of a stored body — replace under the from guard, insert when undecided', () => {
  const body = '<img src="x.png" alt="photo"><img src="y.png"><img src="z.png" />';
  const before = { ...html(), body };
  const { campaign, applied, skipped } = applyFixes(before, [
    { kind: 'alt.set', index: 0, from: 'photo', to: 'The beach "at dusk" & sea' },
    { kind: 'alt.set', index: 1, from: '', to: 'Shop now', undecided: true },
    { kind: 'alt.set', index: 2, from: '', to: '', undecided: true },
  ]);
  assert.equal(applied.length, 3);
  assert.deepEqual(skipped, []);
  assert.equal(campaign.body, '<img src="x.png" alt="The beach &quot;at dusk&quot; &amp; sea"><img src="y.png" alt="Shop now"><img src="z.png" alt="" />');
  // An undecided fix on a tag that HAS an alt, and a stale from, are skipped.
  const r = applyFixes(before, [
    { kind: 'alt.set', index: 0, from: '', to: 'X', undecided: true },
    { kind: 'alt.set', index: 0, from: 'stale', to: 'X' },
  ]);
  assert.equal(r.applied.length, 0);
  assert.equal(r.campaign.body, body);
});

test('I8: identical alt.set fixes apply once and every copy counts as applied (each disposition `fixed`)', () => {
  const mk = () => ({ kind: 'alt.set', block: 's1', from: sliceDoc.s1.data.props.alt, to: 'Shop the fall collection' });
  const [a, b, c] = [mk(), mk(), mk()];
  const other = { kind: 'alt.set', block: 's2', from: 'Ruze Mailing List', to: 'Other' };
  const { campaign, applied, skipped } = applyFixes(sliced(), [a, b, c, other]);
  assert.equal(applied.length, 4);
  assert.deepEqual(skipped, []);
  assert.equal(JSON.parse(campaign.body_source).s1.data.props.alt, 'Shop the fall collection');
  const staged = [a, b, c].map((fix, i) => ({
    key: `A4.1b#${i}`, rubric_id: 'A4.1b', action: 'fixed', fix,
  }));
  assert.deepEqual(dispositionsAfterFixes(staged, applied).map((d) => d.action), ['fixed', 'fixed', 'fixed']);
  // A different `to` on the same image is not a copy: its from no longer matches, so it is skipped.
  const r = applyFixes(sliced(), [mk(), { ...mk(), to: 'Something else' }]);
  assert.equal(r.applied.length, 1);
  assert.equal(r.skipped.length, 1);
});

test('I8: unknown kinds are still skipped beside alt.set, and the PUT key set is unchanged', () => {
  const { applied, skipped, campaign } = applyFixes(sliced(), [
    { kind: 'alt.rewrite', block: 's1', from: 'a', to: 'b' },
    { kind: 'alt.set', block: 's2', from: 'Ruze Mailing List', to: '' },
  ]);
  assert.equal(applied.length, 1);
  assert.equal(skipped[0].reason, 'unknown kind');
  assert.deepEqual(Object.keys(reviewPayload(campaign, '<p>x</p>')), [...CAMPAIGN_UPDATE_KEYS]);
});

test('I13: the PUT payload key set equals campaignPayload\'s (the Campaign.vue key set)', () => {
  const c = visual();
  const p = reviewPayload(c, '<p>new</p>');
  assert.deepEqual(Object.keys(p), Object.keys(campaignPayload(c, '<p>new</p>')));
  assert.deepEqual(Object.keys(p), [...CAMPAIGN_UPDATE_KEYS]);
  assert.equal(p.body, '<p>new</p>');
  assert.deepEqual(p.lists, [15]);
});

test('Stage 4 finding 11: a non-visual fix edits the n-th opening tag over the RAW body (the Lambda\'s numbering, shared fixture)', () => {
  const fx = JSON.parse(readFileSync(new URL('./testdata/review-fix-indices.json', import.meta.url), 'utf8'));
  fx.cases.filter((c) => c.fix).forEach((c) => {
    const { campaign, applied } = applyFixes({ content_type: 'html', body: fx.html }, [c.fix]);
    assert.equal(applied.length, 1, c.tag);
    // Exactly that tag changed; everything else byte-identical.
    const i = fx.html.indexOf(c.tag);
    assert.ok(i >= 0);
    assert.equal(campaign.body.slice(0, i), fx.html.slice(0, i));
    assert.ok(campaign.body.slice(i).startsWith(c.tag.replace(c.fix.from, c.fix.to)));
  });
});

test('Stage 4 finding 6: only applied fixes are recorded as fixed; skipped ones become fixme; others pass through', () => {
  const f1 = { kind: 'subject.trim', from: '  Sale ', to: 'Sale' };
  const f2 = {
    kind: 'alt.stripPrefix', block: 'img', from: 'stale', to: 'X',
  };
  const staged = [
    {
      key: 'D1.4#a', rubric_id: 'D1.4', action: 'fixed', fix: f1,
    },
    {
      key: 'D3.3b#b', rubric_id: 'D3.3b', action: 'fixed', fix: f2,
    },
    { key: 'D2.3#c', rubric_id: 'D2.3', action: 'accept' },
    { key: 'D1.1#d', rubric_id: 'D1.1', action: 'fixme' },
  ];
  const { applied } = applyFixes(visual(), [f1, f2]);
  assert.deepEqual(dispositionsAfterFixes(staged, applied).map((d) => [d.key, d.action]), [
    ['D1.4#a', 'fixed'], ['D3.3b#b', 'fixme'], ['D2.3#c', 'accept'], ['D1.1#d', 'fixme'],
  ]);
});
