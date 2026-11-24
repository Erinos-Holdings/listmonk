// Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC I10). Run: npm test (node --test
// src/*.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  createBody, filterLinks, formFromLink, isLinkTarget, linkErrorMessage, shortUrl, smsTime, updateBody,
  DEFAULT_LINK_FILTER,
} from './smsAdmin.mjs'; // eslint-disable-line import/extensions

const en = JSON.parse(readFileSync(new URL('../../i18n/en.json', import.meta.url), 'utf8'));
const t = (k) => en[k] || k;

test('I10 — times are US Mountain, 12-hour, both sides of DST', () => {
  assert.equal(smsTime('2026-07-01T18:05:00Z'), 'Jul 1, 2026, 12:05 PM'); // MDT, UTC-6
  assert.equal(smsTime('2026-11-24T02:05:00Z'), 'Nov 23, 2026, 7:05 PM'); // MST, UTC-7
  assert.equal(smsTime('2026-01-01T06:30:00.123Z'), 'Dec 31, 2025, 11:30 PM');
  assert.equal(smsTime(null), '—');
  assert.equal(smsTime(''), '—');
  assert.equal(smsTime('not a time'), '—');
});

test('I10 — a target is a link only when it starts with https://', () => {
  assert.equal(isLinkTarget('https://curatedfor.you/'), true);
  const scriptUrl = ['java', 'script:alert(1)'].join('');
  ['http://curatedfor.you/', scriptUrl, ' https://x.test/', 'HTTPS://x.test/', 'https:/x', '', null, undefined, 7]
    .forEach((v) => assert.equal(isLinkTarget(v), false, String(v)));
});

test('I10 — the short URL is https://yq.link/<slug>', () => {
  assert.equal(shortUrl('tg-nov'), 'https://yq.link/tg-nov');
  assert.equal(shortUrl('abc234'), 'https://yq.link/abc234');
});

test('the filter defaults to Active and an absent active reads as active', () => {
  const links = [{ slug: 'a', active: true }, { slug: 'zz-smoke-test', active: false }, { slug: 'c' }];
  assert.equal(DEFAULT_LINK_FILTER, 'active');
  assert.deepEqual(filterLinks(links, 'active').map((l) => l.slug), ['a', 'c']);
  assert.deepEqual(filterLinks(links, 'inactive').map((l) => l.slug), ['zz-smoke-test']);
  assert.deepEqual(filterLinks(links, 'all').map((l) => l.slug), ['a', 'zz-smoke-test', 'c']);
  assert.deepEqual(filterLinks(null, 'all'), []);
});

test('the bodies: create carries the slug, update the whole record and active; no UTM defaults', () => {
  const stored = {
    slug: 'tg-nov',
    target: 'https://x.test/',
    utm_source: 'sms',
    active: false,
    clicks: 9,
  };
  const empty = {
    utm_medium: '',
    utm_campaign: '',
    utm_term: '',
    utm_content: '',
    brand: '',
    campaign: '',
    notes: '',
  };
  const form = formFromLink(stored);
  assert.deepEqual(form, {
    slug: 'tg-nov',
    target: 'https://x.test/',
    active: false,
    utm_source: 'sms',
    ...empty,
  });
  assert.deepEqual(updateBody(form), {
    target: 'https://x.test/',
    utm_source: 'sms',
    ...empty,
    active: false,
  });
  const created = createBody({ ...formFromLink(null), target: ' https://x.test/ ', slug: ' ' });
  assert.equal(created.slug, '');
  assert.equal(created.target, 'https://x.test/');
  assert.equal(created.utm_source, '');
  assert.equal('active' in created, false);
  assert.equal('clicks' in updateBody(form), false);
});

test('a 409 is the slug-taken message; anything else is the server text', () => {
  assert.equal(linkErrorMessage(409, 'slug exists', t), 'That slug is taken — pick another or edit the existing link');
  assert.equal(linkErrorMessage(400, 'target must be https', t), 'target must be https');
  assert.equal(linkErrorMessage(502, '', t), en['sms.adminUnavailable']);
});
