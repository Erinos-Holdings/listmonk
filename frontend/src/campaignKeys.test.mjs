// Fork (integrations ATTRIBS-CASE-SPEC I1-I3) -- the campaign camelCase predicate, through the
// real utils.camelKeys transform.
//
// utils.js does not load under a bare `node --test`: it imports the dayjs plugins extensionless
// (`dayjs/plugin/duration`), which Vite resolves and Node's ESM resolver does not. A one-line
// resolve hook (below) appends `.js` to those specifiers only, so the REAL utils.js is loaded --
// no copy of camelKeys. node --test runs each test file in its own process, so the hook reaches
// no other test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { register } from 'node:module';

import { campaignCamel } from './campaignKeys.mjs'; // eslint-disable-line import/extensions

const hook = `export async function resolve(s, c, next) {
  return next(/^dayjs\\/plugin\\/[^.]+$/.test(s) ? s + '.js' : s, c);
}`;
register(`data:text/javascript,${encodeURIComponent(hook)}`, import.meta.url);
const { default: Utils } = await import('./utils.js'); // eslint-disable-line import/extensions
const utils = new Utils(null);

// A campaign as the API returns it (snake_case throughout).
const campaign = () => ({
  id: 227,
  created_at: '2026-11-17T00:00:00Z',
  content_type: 'visual',
  headers: [{ 'X-SES-MESSAGE-TAGS': 'brand=ruze' }],
  attribs: {
    lang: 'en',
    alt_generated: { 'img-1': { alt_text: 'a', generated_at: 'x' } },
    journey_seed: 'abc',
    visual_direction: 'warm',
    smart_sending: false,
    nested_obj: { deep_key: [{ deeper_key: 1 }] },
  },
  archive_meta: { some_key: 1, other_obj: { inner_key: 2 } },
  smart_sending: true,
});

// I1 -- the predicate over the exact key-path shape camelKeys produces (leading '.', array
// indices as '*'), single campaign and the list's '.results.*' prefix.
test('I1: keeps headers and every key under attribs/archive_meta; camelCases the rest', () => {
  ['', '.results.*'].forEach((prefix) => {
    const p = campaignCamel(prefix);
    const keep = [
      '.headers', '.headers.*.X-SES-MESSAGE-TAGS',
      '.attribs.alt_generated', '.attribs.alt_generated.img-1.alt_text',
      '.attribs.nested_obj.deep_key.*.deeper_key',
      '.archive_meta.some_key', '.archive_meta.other_obj.inner_key',
    ];
    const camel = ['.attribs', '.archive_meta', '.created_at', '.smart_sending', '.content_type',
      '.attribs_x', '.archive_metadata'];
    keep.forEach((k) => assert.equal(p(`${prefix}${k}`), false, `keep ${prefix}${k}`));
    camel.forEach((k) => assert.equal(p(`${prefix}${k}`), true, `camel ${prefix}${k}`));
  });
  // The list prefix is not a single-campaign match, and vice versa.
  assert.equal(campaignCamel('.results.*')('.attribs.alt_generated'), true);
  assert.equal(campaignCamel('')('.results.*.attribs.alt_generated'), true);
});

test('I1/I3: single campaign round trip through camelKeys', () => {
  const out = utils.camelKeys(campaign(), campaignCamel(''));
  const src = campaign();
  assert.deepEqual(out.attribs, src.attribs);
  assert.deepEqual(out.archiveMeta, src.archive_meta);
  assert.deepEqual(out.headers, src.headers);
  assert.equal(out.archive_meta, undefined);
  assert.equal(out.createdAt, src.created_at);
  assert.equal(out.contentType, 'visual');
  assert.equal(out.smartSending, true);
  assert.equal(out.attribs.altGenerated, undefined);
  assert.equal(out.attribs.journey_seed, 'abc');
  assert.equal(out.archiveMeta.some_key, 1);
});

test('I1/I3: the Campaigns list (.results.*) round trip through camelKeys', () => {
  const resp = { results: [campaign(), campaign()], total: 2, per_page: 20 };
  const out = utils.camelKeys(resp, campaignCamel('.results.*'));
  assert.equal(out.perPage, 20);
  out.results.forEach((c) => {
    assert.deepEqual(c.attribs, campaign().attribs);
    assert.deepEqual(c.archiveMeta, campaign().archive_meta);
    assert.deepEqual(c.headers, campaign().headers);
    assert.equal(c.createdAt, campaign().created_at);
  });
});

// I2 -- agreement pin: every campaign read/write D2 names uses campaignCamel.
test('I2: every D2 campaign read/write in api/index.js uses campaignCamel', () => {
  const src = readFileSync(new URL('./api/index.js', import.meta.url), 'utf8');
  const block = (name) => {
    const start = src.indexOf(`export const ${name} = `);
    assert.ok(start >= 0, `${name} exported`);
    const end = src.indexOf('\nexport ', start + 1);
    return src.slice(start, end < 0 ? undefined : end);
  };
  const want = {
    getCampaigns: "'.results.*'",
    getCampaign: "''",
    getCampaignQuiet: "''",
    createCampaign: "''",
    updateCampaign: "''",
    changeCampaignStatus: "''",
  };
  Object.entries(want).forEach(([name, prefix]) => {
    const b = block(name);
    const uses = (b.match(/camelCase:\s*[^,}\n]+/g) || []).map((u) => u.trim());
    assert.deepEqual(uses, [`camelCase: campaignCamel(${prefix})`], name);
  });
});
