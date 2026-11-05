// Fork (review navigation) -- integrations REVIEW-NAVIGATION-SPEC I6 (a message from another
// origin, a frame, this window or without the type is ignored and unanswered; no reachable opener
// or no `received` within 1,500 ms opens the new tab), I9 (a block inside an official footer is
// never a link) and I10 (a thumbnail or `where.image` renders only from this host's /uploads/).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  ACK_TIMEOUT_MS, MSG_ACK, MSG_OPENER, MSG_RUN, MSG_SELECT, RESULT_TIMEOUT_MS, RUN_FALLBACK_MS,
  acceptRun, runBaseFromQuery, runFromQuery, runMessage, runView,
  acceptAck, acceptOpener, acceptSelect, ackMessage, afterAckWait, campaignLocation, clickDecision,
  fallbackUrl, fallbackWindowName, isFinalResult, isImageUrl, isValidBlockId, newToken, openerMessage, openerReachable,
  pushFailureResult, receiverRoute, referenceLine, resultToast, selectFromQuery, selectMessage,
  standardLinks, thumbnailUrl,
} from './reviewNavigate.mjs'; // eslint-disable-line import/extensions

const ORIGIN = 'https://email.curatedfor.you';
// A script URL, built so the linter's no-script-url does not read it as code.
const SCRIPT_URL = ['java', 'script:alert(1)'].join('');
const topWin = () => { const w = { closed: false, location: { origin: ORIGIN } }; w.top = w; return w; };

test('the timeouts are the spec\'s', () => {
  assert.equal(ACK_TIMEOUT_MS, 1500);
  assert.equal(RESULT_TIMEOUT_MS, 20000);
});

test('block ids: the builder forms only', () => {
  ['block-1790113969156', 'block-1790113969156-3', 'block-official-brand', 'block-5'].forEach((id) => assert.equal(isValidBlockId(id), true, id));
  ['', 'root', 'block-', 'block-"]', 'block-a b', 'block-x"], body', 'Block-1', 'block-1/../x', null, 17, {}].forEach((id) => assert.equal(isValidBlockId(id), false, String(id)));
});

test('message shapes', () => {
  assert.deepEqual(openerMessage(7), { type: MSG_OPENER, campaignId: 7 });
  assert.deepEqual(selectMessage(7, 'block-1', 'tk'), {
    type: MSG_SELECT, campaignId: 7, blockId: 'block-1', token: 'tk',
  });
  assert.deepEqual(ackMessage('tk', 'received'), { type: MSG_ACK, token: 'tk', result: 'received' });
  assert.notEqual(newToken(), newToken());
  assert.match(newToken(() => 0.5), /^[0-9a-z]+-[0-9a-z]+$/);
});

test('I6: the receiver accepts only our origin, a top-level other window, the type, valid ids', () => {
  const source = topWin();
  const self = topWin();
  const data = selectMessage(7, 'block-1', 'tk');
  const ok = {
    origin: ORIGIN, ourOrigin: ORIGIN, source, self, data, routeName: 'campaigns',
  };
  assert.deepEqual(acceptSelect(ok), { campaignId: 7, blockId: 'block-1', token: 'tk' });
  assert.equal(acceptSelect({ ...ok, origin: 'https://evil.example' }), null, 'another origin');
  assert.equal(acceptSelect({ ...ok, origin: 'null' }), null, 'an opaque origin');
  assert.equal(acceptSelect({ ...ok, data: { ...data, type: undefined } }), null, 'no type');
  assert.equal(acceptSelect({ ...ok, data: { ...data, type: 'lm-review:selectx' } }), null, 'another type');
  assert.equal(acceptSelect({ ...ok, data: 'lm-review:select' }), null, 'not an object');
  assert.equal(acceptSelect({ ...ok, source: self }), null, 'this window');
  assert.equal(acceptSelect({ ...ok, source: null }), null, 'no source');
  const frame = { top: source };
  assert.equal(acceptSelect({ ...ok, source: frame }), null, 'a frame (the builder iframe)');
  const crossTop = { get top() { throw new Error('cross-origin'); } };
  assert.equal(acceptSelect({ ...ok, source: crossTop }), null, 'an unreadable source');
  assert.equal(acceptSelect({ ...ok, routeName: 'campaignReview' }), null, 'the Inspect window itself');
  assert.equal(acceptSelect({ ...ok, data: { ...data, campaignId: '7' } }), null, 'a string id');
  assert.equal(acceptSelect({ ...ok, data: { ...data, campaignId: 0 } }), null, 'id 0');
  assert.equal(acceptSelect({ ...ok, data: { ...data, blockId: 'block-"]' } }), null, 'an invalid block id');
  assert.equal(acceptSelect({ ...ok, data: { ...data, token: '' } }), null, 'no token');
});

test('the receiver selects in place on the same campaign page, navigates otherwise', () => {
  assert.equal(receiverRoute({ routeName: 'campaign', routeId: '7', campaignId: 7 }), 'emit');
  assert.equal(receiverRoute({ routeName: 'campaign', routeId: '8', campaignId: 7 }), 'push');
  assert.equal(receiverRoute({ routeName: 'campaigns', routeId: undefined, campaignId: 7 }), 'push');
  assert.deepEqual(campaignLocation(7, 'block-1'), {
    name: 'campaign', params: { id: '7' }, query: { select: 'block-1' }, hash: '#content',
  });
  assert.equal(pushFailureResult({ type: 4 }, (e) => e.type === 4), 'declined');
  assert.equal(pushFailureResult({ type: 2 }, (e) => e.type === 4), 'unknown');
});

test('I6: no reachable opener or no `received` in time -> the new tab', () => {
  const opener = topWin();
  const base = {
    opener, ourOrigin: ORIGIN, campaignId: 7, blockId: 'block-1',
  };
  assert.equal(clickDecision(base), 'post');
  assert.equal(clickDecision({ ...base, opener: null }), 'newTab', 'no opener');
  assert.equal(clickDecision({ ...base, opener: { ...opener, closed: true } }), 'newTab', 'closed');
  assert.equal(clickDecision({ ...base, opener: { closed: false, location: { origin: 'https://other.example' } } }), 'newTab', 'elsewhere');
  const crossOrigin = { closed: false, get location() { throw new Error('SecurityError'); } };
  assert.equal(clickDecision({ ...base, opener: crossOrigin }), 'newTab', 'unreadable origin');
  assert.equal(clickDecision({ ...base, blockId: 'nope' }), 'ignore', 'invalid block id: nothing happens');
  assert.equal(clickDecision({ ...base, campaignId: NaN }), 'ignore');
  assert.equal(openerReachable(opener, ORIGIN), true);
  assert.equal(afterAckWait('received'), 'wait');
  assert.equal(afterAckWait(null), 'newTab', 'nothing within 1,500 ms');
  assert.equal(afterAckWait('selected'), 'newTab', 'a result without received is not a receipt');
});

test('I6: the window adopts an opener and reads an ack only from our origin, its opener and its token', () => {
  const opener = topWin();
  const self = topWin();
  assert.equal(acceptOpener({
    origin: ORIGIN, ourOrigin: ORIGIN, source: opener, self, data: openerMessage(7), campaignId: 7,
  }), true);
  assert.equal(acceptOpener({
    origin: ORIGIN, ourOrigin: ORIGIN, source: { top: opener }, self, data: openerMessage(7), campaignId: 7,
  }), false, 'a frame source');
  assert.equal(acceptOpener({
    origin: ORIGIN, ourOrigin: ORIGIN, source: self, self, data: openerMessage(7), campaignId: 7,
  }), false, 'this window');
  assert.equal(acceptOpener({
    origin: ORIGIN, ourOrigin: ORIGIN, source: { get top() { throw new Error('cross-origin'); } }, self, data: openerMessage(7), campaignId: 7,
  }), false, 'an unreadable source');
  assert.equal(acceptOpener({
    origin: 'https://evil.example', ourOrigin: ORIGIN, source: opener, data: openerMessage(7), campaignId: 7,
  }), false);
  assert.equal(acceptOpener({
    origin: ORIGIN, ourOrigin: ORIGIN, source: opener, data: openerMessage(8), campaignId: 7,
  }), false, 'another campaign');
  assert.equal(acceptOpener({
    origin: ORIGIN, ourOrigin: ORIGIN, source: opener, data: { campaignId: 7 }, campaignId: 7,
  }), false, 'no type');
  const ack = {
    origin: ORIGIN, ourOrigin: ORIGIN, source: opener, expectedSource: opener, data: ackMessage('tk', 'received'), token: 'tk',
  };
  assert.equal(acceptAck(ack), 'received');
  assert.equal(acceptAck({ ...ack, data: ackMessage('tk', 'selected') }), 'selected');
  assert.equal(acceptAck({ ...ack, origin: 'https://evil.example' }), null);
  assert.equal(acceptAck({ ...ack, source: topWin() }), null, 'another window');
  assert.equal(acceptAck({ ...ack, token: 'other' }), null, 'another click');
  assert.equal(acceptAck({ ...ack, data: { token: 'tk', result: 'selected' } }), null, 'no type');
  assert.equal(acceptAck({ ...ack, data: ackMessage('tk', 'pwned') }), null, 'an unknown result');
  assert.equal(isFinalResult('received'), false);
  assert.equal(isFinalResult('declined'), true);
});

test('the toasts: one per final result', () => {
  assert.equal(resultToast('selected'), 'campaigns.review.nav.selected');
  assert.equal(resultToast('declined'), 'campaigns.review.nav.declined');
  assert.equal(resultToast('unknown'), 'campaigns.review.nav.unknown');
  assert.equal(resultToast('received'), null);
  assert.equal(resultToast(undefined), null);
});

test('the new-tab URL: the content page selecting on load; nothing for an invalid id', () => {
  assert.equal(fallbackUrl('/admin', 7, 'block-1790113969156'), '/admin/campaigns/7?select=block-1790113969156#content');
  assert.equal(fallbackUrl('/admin/', 7, 'block-1'), '/admin/campaigns/7?select=block-1#content');
  assert.equal(fallbackUrl('/admin', 7, SCRIPT_URL), null);
  assert.equal(fallbackUrl('/admin', -1, 'block-1'), null);
  assert.equal(fallbackWindowName(7), 'lm-review-editor-7');
  assert.equal(selectFromQuery({ select: 'block-1' }), 'block-1');
  assert.equal(selectFromQuery({ select: 'block-"]' }), null);
  assert.equal(selectFromQuery({ select: ['block-1', 'block-2'] }), null);
  assert.equal(selectFromQuery({}), null);
});

test('the reference: badge -- a linkable block with no image', () => {
  const where = {
    block: 'block-17', n: 7, label: 'button "MEET RUZE"', type: 'Button',
  };
  assert.deepEqual(referenceLine({ where, locationPlain: 'x', location: 'block block-17' }, ORIGIN), {
    kind: 'badge', text: 'the button "MEET RUZE"', n: 7, blockId: 'block-17',
  });
  assert.equal(referenceLine({ where, evidence: 'MEET RUZE' }, ORIGIN).kind, 'badge', 'text evidence is no image');
});

test('the reference: image -- where.image first, else an image evidence; this host only', () => {
  const up = `${ORIGIN}/uploads/hero.jpg`;
  const ev = `${ORIGIN}/uploads/pocket.png`;
  const where = {
    block: 'block-1', n: 3, label: 'image "RUZE Header"', type: 'Image', image: up,
  };
  assert.deepEqual(referenceLine({ where, evidence: ev }, ORIGIN), {
    kind: 'image', text: 'the image "RUZE Header"', n: 3, blockId: 'block-1', image: up,
  });
  const noImage = { ...where, image: undefined };
  assert.equal(referenceLine({ where: noImage, evidence: ev }, ORIGIN).image, ev, 'the evidence when where.image is absent');
  const offHost = { ...where, image: 'https://cdn.example/hero.jpg' };
  assert.equal(referenceLine({ where: offHost, evidence: ev }, ORIGIN).image, ev, 'an off-host where.image falls back to the evidence');
  assert.equal(referenceLine({ where: offHost, evidence: 'https://cdn.example/x.png' }, ORIGIN).kind, 'badge', 'no same-origin image -> badge (I10)');
  assert.equal(referenceLine({ where, evidence: ev }, '').kind, 'badge', 'no origin -> no image');
});

test('the reference: plain and none', () => {
  assert.deepEqual(referenceLine({ locationPlain: 'The subject', location: 'subject' }, ORIGIN), { kind: 'plain', text: 'The subject' });
  assert.deepEqual(referenceLine({ location: 'subject' }, ORIGIN), { kind: 'plain', text: 'subject' }, 'an older report');
  assert.deepEqual(referenceLine({}, ORIGIN), { kind: 'none', text: '' });
  assert.deepEqual(referenceLine(null, ORIGIN), { kind: 'none', text: '' });
  const bad = referenceLine({
    where: {
      block: 'block-"]', n: 2, label: 'text "x"', type: 'Text', image: `${ORIGIN}/uploads/a.png`,
    },
  }, ORIGIN);
  assert.equal(bad.kind, 'plain', 'an invalid id is never a link');
  assert.equal(bad.blockId, null);
});

test('I9: a block inside an official footer is never a link', () => {
  const r = referenceLine({
    where: {
      block: 'block-official-brand', n: 13, label: 'official footer', type: 'OfficialFooter', image: `${ORIGIN}/uploads/logo.png`,
    },
    evidence: `${ORIGIN}/uploads/logo.png`,
  }, ORIGIN);
  assert.equal(r.kind, 'plain');
  assert.equal(r.blockId, null);
  assert.equal(r.image, undefined);
  assert.equal(r.text, 'the official footer (block 13)');
});

test('I10: thumbnails only from this host\'s /uploads/, a raster extension, never repeated', () => {
  const up = `${ORIGIN}/uploads/hero.jpg`;
  assert.equal(isImageUrl(ORIGIN, up), true);
  assert.equal(isImageUrl(ORIGIN, ` ${up}?v=2 `), true);
  ['https://evil.example/uploads/hero.jpg', `${ORIGIN}/other/hero.jpg`, `${ORIGIN}/uploads/x.svg`, `${ORIGIN}.evil.example/uploads/x.png`,
    `${ORIGIN}/uploads/a b.png`, SCRIPT_URL, '', null].forEach((bad) => {
    assert.equal(isImageUrl(ORIGIN, bad), false, String(bad));
  });
  assert.equal(isImageUrl('', up), false);
  const where = (image) => ({
    block: 'block-1', n: 1, label: 'image', type: 'Image', image,
  });
  assert.equal(thumbnailUrl(ORIGIN, { where: where(up), evidence: 'POCKET SIZE' }), up);
  assert.equal(thumbnailUrl(ORIGIN, { where: where(up), evidence: up }), null, 'the evidence thumbnail shows it already');
  assert.equal(thumbnailUrl(ORIGIN, { where: where('https://cdn.example/hero.jpg'), evidence: 'x' }), null);
  assert.equal(thumbnailUrl(ORIGIN, { where: { ...where(undefined) }, evidence: 'x' }), null);
  assert.equal(thumbnailUrl(ORIGIN, { evidence: up }), null, 'no where');
});

test('§4.3: standards links, https only', () => {
  const item = {
    references: [
      { label: 'WCAG 1.4.3', url: 'https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html' },
      { label: 'bad', url: SCRIPT_URL },
      { label: 'plain http', url: 'http://example.com/' },
      { label: '', url: 'https://x.example/' },
      null,
    ],
  };
  assert.deepEqual(standardLinks(item), [{ label: 'WCAG 1.4.3', url: 'https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html' }]);
  assert.deepEqual(standardLinks({}), []);
  assert.deepEqual(standardLinks(null), []);
});

// ------------------------------------------------------------------ integrations REVIEW-TRIM-2-SPEC I12
// D4: a window URL carrying `run` renders the starting state and no report whose job id equals
// `baseJob`; a `failed` message or the 90 s bound ends it on the latest report with the stated line;
// a URL without `run` renders the latest report.
test('REVIEW-TRIM-2-SPEC I12: the run param, the run message and its acceptance', () => {
  assert.equal(RUN_FALLBACK_MS, 90000);
  const run = newToken();
  assert.equal(runFromQuery({ run }), run);
  [undefined, '', 'a b', '<x>', 'x'.repeat(65), 7].forEach((bad) => assert.equal(runFromQuery({ run: bad }), null, String(bad)));
  assert.equal(runFromQuery(null), null);
  // Post-release fix (2026-11-05): the base job id rides in the URL -- a job id, `none` (null), or
  // undefined when the page sent nothing (the window then captures it at first load).
  assert.equal(runBaseFromQuery({ base: 'fd2b4f4c-f9ed-4170-b3eb-4395adcc99dc' }), 'fd2b4f4c-f9ed-4170-b3eb-4395adcc99dc');
  assert.equal(runBaseFromQuery({ base: 'none' }), null);
  [undefined, '', 'a b', '<x>', 7].forEach((bad) => assert.equal(runBaseFromQuery({ base: bad }), undefined, String(bad)));
  assert.equal(runBaseFromQuery(null), undefined);
  // The race the fix closes: a window that learns the base from the URL renders the new job at once.
  assert.equal(runView({ run, baseJob: 'old-job', row: { job_id: 'new-job', status: 'complete' } }).mode, 'latest');
  assert.equal(runView({ run, baseJob: null, row: { job_id: 'new-job', status: 'complete' } }).mode, 'latest');
  assert.deepEqual(runMessage(7, run), {
    type: MSG_RUN, campaignId: 7, run, phase: 'failed',
  });
  const opener = topWin();
  const ok = {
    origin: ORIGIN, ourOrigin: ORIGIN, source: opener, opener, data: runMessage(7, run), campaignId: 7, run,
  };
  assert.equal(acceptRun(ok), true);
  assert.equal(acceptRun({ ...ok, origin: 'https://evil.example' }), false);
  assert.equal(acceptRun({ ...ok, source: topWin() }), false);
  assert.equal(acceptRun({ ...ok, data: runMessage(8, run) }), false);
  assert.equal(acceptRun({ ...ok, data: runMessage(7, 'another-run') }), false);
  assert.equal(acceptRun({ ...ok, run: null }), false);
  assert.equal(acceptRun({ ...ok, data: { ...runMessage(7, run), phase: 'done' } }), false);
});

test('REVIEW-TRIM-2-SPEC I12: runView -- starting until the run\'s job, never the baseJob report; failed and the 90 s bound end on the latest', () => {
  const run = 'r-1';
  const old = { job_id: 'job-old', status: 'complete' };
  // No run: the latest, whatever the rows.
  assert.deepEqual(runView({ run: null, baseJob: undefined, row: old }), { mode: 'latest' });
  // With a run: starting before the first load, and while the latest row is the baseJob's.
  assert.equal(runView({ run, baseJob: undefined, row: null }).mode, 'starting');
  assert.equal(runView({ run, baseJob: undefined, row: old }).mode, 'starting');
  assert.equal(runView({ run, baseJob: 'job-old', row: old }).mode, 'starting');
  assert.equal(runView({ run, baseJob: 'job-old', row: { job_id: 'job-old', status: 'failed' } }).mode, 'starting');
  // The run's job appears (a new id, or running): the latest renders.
  assert.equal(runView({ run, baseJob: 'job-old', row: { job_id: 'job-new', status: 'running' } }).mode, 'latest');
  assert.equal(runView({ run, baseJob: 'job-old', row: { job_id: 'job-new', status: 'complete' } }).mode, 'latest');
  assert.equal(runView({ run, baseJob: null, row: { job_id: 'job-1', status: 'running' } }).mode, 'latest');
  assert.equal(runView({ run, baseJob: null, row: null }).mode, 'starting');
  // No job follows: the failed message, else the 90 s bound -- each with its line, on the latest report.
  assert.deepEqual(runView({
    run, baseJob: 'job-old', row: old, failed: true,
  }), { mode: 'failed', line: 'campaigns.review.run.failed' });
  assert.deepEqual(runView({
    run, baseJob: 'job-old', row: old, elapsedMs: RUN_FALLBACK_MS - 1,
  }), { mode: 'starting' });
  assert.deepEqual(runView({
    run, baseJob: 'job-old', row: old, elapsedMs: RUN_FALLBACK_MS,
  }), { mode: 'timeout', line: 'campaigns.review.run.timeout' });
  // A job that follows even after the failure line still replaces it.
  assert.equal(runView({
    run, baseJob: 'job-old', row: { job_id: 'job-new', status: 'running' }, failed: true, elapsedMs: RUN_FALLBACK_MS,
  }).mode, 'latest');
});

test('REVIEW-TRIM-2-SPEC I12: Campaign.vue opens the window with ?run= and posts the failed message on a start failure', () => {
  const src = readFileSync(new URL('./views/Campaign.vue', import.meta.url), 'utf8');
  // Post-release fix (2026-11-05): the base job id travels with the run token.
  assert.match(src, /query: \{ run, base \}/);
  assert.match(src, /reviewState\.review\.job_id\) \|\| 'none'/);
  assert.match(src, /runMessage\(id, run, 'failed'\)/);
  const win = readFileSync(new URL('./views/CampaignReview.vue', import.meta.url), 'utf8');
  assert.match(win, /runFromQuery\(/);
  assert.match(win, /runView\(/);
  assert.match(win, /acceptRun\(/);
});
