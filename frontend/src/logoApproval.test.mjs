// Fork (approved brand logos) -- integrations LOGO-APPROVAL-SPEC I12: logoCardState -- approved beats
// card; card is the first candidate on neither list; else warn; a null row is empty lists; an
// approved D3.5 leaves the advisory entries and the checklist's advisory count (and is listed under
// Passed); a D3.5 key already accepted still shows the card in Advisory.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  LOGO_PERMISSION, brandLists, brandRowFor, isLogoCard, logoAction, logoAwareSplit, logoCardState, logoQuestion, logoReference, withApprovedLogo,
} from './logoApproval.mjs'; // eslint-disable-line import/extensions
import { openEntries, sectionCounts, splitAccepted } from './reviewChecklist.mjs'; // eslint-disable-line import/extensions

const A = 'https://email.curatedfor.you/uploads/a.png';
const B = 'https://email.curatedfor.you/uploads/b.png';
const C = 'https://email.curatedfor.you/uploads/c.png';

const d35 = (cands = [{ url: A, location: 'block:a', n: 1 }, { url: B, location: 'block:b', n: 2 }]) => ({
  id: 'D3.5',
  tier: 'D',
  title: 'The brand logo is present',
  verdict: 'warn',
  acceptable: true,
  brand: 'curated',
  findings: [{
    key: 'D3.5#3501c1849f38fc88', id: 'D3.5', evidence: 'https://x/logo.jpg', location: 'logo', todo: 'Add the logo.',
  }],
  ...(cands ? { logoCandidates: cands } : {}),
});
const row = (approved = [], rejected = []) => ({ slug: 'curated', logos_approved: approved, logos_rejected: rejected });
const other = {
  id: 'D3.4',
  tier: 'D',
  verdict: 'warn',
  acceptable: true,
  findings: [{
    key: 'D3.4#x', id: 'D3.4', evidence: '1.2 MB', location: 'images', todo: 'Reduce.',
  }],
};

test('approved beats card: any candidate on the approved list → approved, with that url', () => {
  assert.deepEqual(logoCardState(d35(), row([B])), { state: 'approved', url: B });
  // Even when the first candidate is on neither list.
  assert.deepEqual(logoCardState(d35(), row([B], [])).state, 'approved');
});

test('card = the first candidate on neither list', () => {
  assert.deepEqual(logoCardState(d35(), row()), { state: 'card', candidate: { url: A, location: 'block:a', n: 1 } });
  assert.deepEqual(logoCardState(d35(), row([], [A])), { state: 'card', candidate: { url: B, location: 'block:b', n: 2 } });
});

test('else warn: every candidate rejected, no candidates, a non-warn D3.5, another item', () => {
  assert.deepEqual(logoCardState(d35(), row([], [A, B])), { state: 'warn' });
  assert.deepEqual(logoCardState(d35(null), row()), { state: 'warn' });
  assert.deepEqual(logoCardState(d35([]), row()), { state: 'warn' });
  assert.deepEqual(logoCardState({ ...d35(), verdict: 'pass' }, row([A])), { state: 'warn' });
  assert.deepEqual(logoCardState(other, row([A])), { state: 'warn' });
  assert.deepEqual(logoCardState(null, row()), { state: 'warn' });
});

test('a null row (a failed read) or an older fork row reads as empty lists', () => {
  assert.deepEqual(brandLists(null), { approved: [], rejected: [] });
  assert.deepEqual(brandLists({ slug: 'curated', from_email: 'x' }), { approved: [], rejected: [] });
  assert.deepEqual(logoCardState(d35(), null), { state: 'card', candidate: { url: A, location: 'block:a', n: 1 } });
  assert.deepEqual(logoCardState(d35(), undefined).state, 'card');
});

test('brandRowFor: the item brand from the rows array; none → null', () => {
  const r = row([A]);
  assert.equal(brandRowFor(d35(), [r]), r);
  assert.equal(brandRowFor(d35(), [{ ...r, slug: 'ruze' }]), null);
  assert.equal(brandRowFor({ ...d35(), brand: undefined }, [r]), null);
  assert.equal(brandRowFor(d35(), null), null);
});

test('an approved D3.5 leaves the advisory entries and the checklist count, and is listed under Passed', () => {
  const items = [d35(), other];
  assert.equal(sectionCounts(items).advisory, 2);
  const shown = withApprovedLogo(items, [row([A])]);
  assert.equal(sectionCounts(shown).advisory, 1);
  assert.deepEqual(openEntries(shown).map((e) => e.item.id), ['D3.4']);
  const passed = shown.filter((i) => i.verdict === 'pass' || i.verdict === 'n/a');
  assert.deepEqual(passed.map((i) => [i.id, i.approvedLogo, i.findings.length]), [['D3.5', A, 0]]);
  // Not approved: the items are returned as they are.
  const same = withApprovedLogo(items, [row()]);
  assert.equal(same[0], items[0]);
  assert.equal(same[1], items[1]);
  assert.equal(sectionCounts(same).advisory, 2);
});

test('a D3.5 key already accepted still shows the card in Advisory; a warn D3.5 accepted moves to Accepted', () => {
  const entries = openEntries([d35(), other]);
  const latest = { 'D3.5#3501c1849f38fc88': { item_key: 'D3.5#3501c1849f38fc88', action: 'accept' }, 'D3.4#x': { item_key: 'D3.4#x', action: 'accept' } };
  const rows = [row()];
  assert.equal(isLogoCard(entries[0], rows), true);
  const { open, accepted } = logoAwareSplit(entries, latest, splitAccepted, rows);
  assert.deepEqual(open.map((e) => e.item.id), ['D3.5']);
  assert.equal(open[0].section, 'advisory');
  assert.deepEqual(accepted.map((e) => e.item.id), ['D3.4']);
  // Today's warn (every candidate rejected): the ordinary Accepted rule applies.
  const warnRows = [row([], [A, B])];
  const w = logoAwareSplit(entries, latest, splitAccepted, warnRows);
  assert.deepEqual(w.open.map((e) => e.item.id), []);
  assert.deepEqual(w.accepted.map((e) => e.item.id), ['D3.5', 'D3.4']);
  // Undecided: the order is preserved.
  assert.deepEqual(logoAwareSplit(entries, {}, splitAccepted, rows).open.map((e) => e.item.id), ['D3.5', 'D3.4']);
});

test('the question: with n "section N", without n the NoBlock form; Yes approves, No rejects', () => {
  const t = (k, p) => `${k}|${JSON.stringify(p)}`;
  // integrations REVIEW-LOCATIONS-SPEC D5: the section-N sentence drops {url} (the thumbnail shows it).
  assert.equal(logoQuestion({ url: A, n: 2 }, t, true), `campaigns.review.logoQuestion|${JSON.stringify({ n: 2 })}`);
  // Stage 4 F1: without the standard thumbnail the sentence carries the URL.
  assert.equal(logoQuestion({ url: A, n: 2 }, t), `campaigns.review.logoQuestionNoBlock|${JSON.stringify({ url: A })}`);
  assert.equal(logoQuestion({ url: C, location: 'img[0]' }, t), `campaigns.review.logoQuestionNoBlock|${JSON.stringify({ url: C })}`);
  const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
  assert.ok(!en['campaigns.review.logoQuestion'].includes('{url}'));
  assert.match(en['campaigns.review.logoQuestion'], /section \{n\}/);
  assert.equal(logoAction(true), 'approve');
  assert.equal(logoAction(false), 'reject');
  assert.equal(LOGO_PERMISSION, 'brands:logos');
});

// integrations REVIEW-LOCATIONS-SPEC I7 (D5): a candidate whose `where` holds a valid block renders as
// the standard image reference with the square (its OWN url, this host's uploads only; else the
// square alone), Image or Html block alike, linked to the block; no `where` -> null (today's bare
// picture). The card shows no "Location:" line under its own thumbnail.
test('REVIEW-LOCATIONS-SPEC I7: the logo card reference -- thumbnail + square from where, no Location line', () => {
  const ORIGIN = 'https://email.curatedfor.you';
  const imageWhere = {
    block: 'block-17', n: 2, label: 'image "Logo"', type: 'Image', image: `${ORIGIN}/uploads/other.png`,
  };
  assert.deepEqual(logoReference({
    url: A, location: 'block:block-17', n: 2, where: imageWhere,
  }, ORIGIN), {
    kind: 'image', text: 'the image "Logo"', n: 2, blockId: 'block-17', image: A,
  }, 'the candidate\'s own url, not where.image');
  const htmlWhere = {
    block: 'block-9', n: 4, label: 'HTML block', type: 'Html',
  };
  assert.equal(logoReference({
    url: B, location: 'block:block-9', n: 4, where: htmlWhere,
  }, ORIGIN).kind, 'image', 'an Html block alike');
  // Stage 4 F1: an off-host file gets the square (badge), and the card still shows the file and its URL.
  const offHost = {
    url: 'https://cdn.example/x.png', location: 'block:block-9', n: 4, where: htmlWhere,
  };
  assert.equal(logoReference(offHost, ORIGIN).kind, 'badge', 'an off-host file: the square');
  const tq = (k, p) => `${k}|${JSON.stringify(p)}`;
  assert.equal(logoQuestion(offHost, tq, false), `campaigns.review.logoQuestionNoBlock|${JSON.stringify({ url: offHost.url })}`, 'the badge card\'s sentence names the URL');
  assert.equal(logoReference({ url: C, location: 'img[0]' }, ORIGIN), null, 'no where: today\'s bare picture');
  assert.equal(logoReference({ url: C, where: { ...htmlWhere, block: 'nope' } }, ORIGIN), null, 'an invalid block id');
  const src = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  const tpl = /<template>([\s\S]*)<\/template>\s*<script>/.exec(src)[1];
  const card = /<div v-if="logoCardOf\(e\)" class="logo-card"[\s\S]*?<\/div>\s*<\/div>/.exec(tpl)[0];
  assert.match(card, /<p v-if="logoRefOf\(e\)"[\s\S]*data-cy="btn-goto-block"[\s\S]*@click\.prevent="goto\(logoRefOf\(e\)\.blockId\)"/);
  assert.match(card, /class="lm-structure-number">\{\{ logoRefOf\(e\)\.n \}\}/);
  // The bare picture shows whenever the reference is not the standard thumbnail (none, or a badge).
  assert.match(card, /<img v-if="!logoThumbOf\(e\)" :src="logoCardOf\(e\)\.url" alt="" class="evidence-thumb logo-card-thumb"/);
  assert.match(card, /logoQuestionOf\(logoCardOf\(e\), logoThumbOf\(e\)\)/);
  assert.match(src, /logoThumbOf\(e\) \{\s*const r = this\.logoRefOf\(e\);\s*return !!r && r\.kind === 'image';/);
  // No Location line under a card that shows its own thumbnail.
  assert.match(tpl, /<template v-if="!logoRefOf\(e\)">\s*<p v-if="reference\(e\.finding\)\.kind === 'image'"/);
});
