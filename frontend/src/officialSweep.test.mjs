// Fork (official footer) -- OFFICIAL-FOOTER-SPEC I9 (Vue twin) and I10. Run:
// npm run test:official-sweep (node --test src/officialSweep.test.mjs). No dependency.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  CAMPAIGN_UPDATE_KEYS, TEMPLATE_UPDATE_KEYS, campaignPayload, deriveContext, runSweep,
  selectSweepItems, templatePayload,
} from './officialSweep.mjs'; // eslint-disable-line import/extensions

const LISTS = [
  { id: 5, name: 'Curated', tags: ['brand:curated', 'from:Curated <hello@curatedfor.you>'] },
  { id: 6, name: 'Thirsty Girl', tags: ['brand:ThirstyGirl', 'from:x'] },
  { id: 15, name: 'Ruze Pouches', tags: ['brand:ruze', 'from:y'] },
  { id: 16, name: 'Shala', tags: ['brand:shala', 'from:z'] },
  { id: 25, name: 'Untagged', tags: [] },
];

const doc = (...kinds) => JSON.stringify({
  root: { type: 'EmailLayout', data: { childrenIds: kinds.map((k, i) => `of${i}`) } },
  ...Object.fromEntries(kinds.map((k, i) => [`of${i}`, { type: 'OfficialFooter', data: { props: { kind: k } } }])),
});
const PLAIN = JSON.stringify({ root: { type: 'EmailLayout', data: { childrenIds: [] } } });

const camp = (id, over = {}) => ({
  id,
  name: `c${id}`,
  status: 'draft',
  content_type: 'visual',
  evergreen: false,
  lists: [{ id: 15, name: 'Ruze Pouches' }],
  attribs: { lang: 'en' },
  body_source: doc('brand', 'corporate'),
  ...over,
});
const tpl = (id, name, over = {}) => ({
  id, name, type: 'campaign_visual', brand: '', lang: 'en', body_source: PLAIN, ...over,
});

test('I9: context = attribs.lang (empty -> en) + exactly one brand tag (none -> curated, two -> error)', () => {
  assert.deepEqual(deriveContext(camp(1), LISTS), { lang: 'en', brand: 'ruze', error: null });
  assert.deepEqual(deriveContext(camp(1, { attribs: {} }), LISTS), { lang: 'en', brand: 'ruze', error: null });
  assert.equal(deriveContext(camp(1, { attribs: { lang: 'es' } }), LISTS).lang, 'es');
  assert.equal(deriveContext(camp(1, { lists: [{ id: 6 }] }), LISTS).brand, 'thirstygirl', 'lower-folded');
  assert.equal(deriveContext(camp(1, { lists: [{ id: 25 }] }), LISTS).brand, 'curated');
  assert.equal(deriveContext(camp(1, { lists: [] }), LISTS).brand, 'curated');
  assert.equal(deriveContext(camp(1, { lists: [{ id: 15 }, { id: 15 }] }), LISTS).brand, 'ruze', 'one brand across lists');
  const two = deriveContext(camp(1, { lists: [{ id: 15 }, { id: 16 }] }), LISTS);
  assert.equal(two.brand, null);
  assert.match(two.error, /2 brands \(ruze, shala\)/);
});

test('I10: selects only items whose OfficialFooter blocks resolve to the saved template', () => {
  const campaigns = [
    camp(1), // ruze en: brand + corporate
    camp(2, { lists: [{ id: 16 }] }), // shala en
    camp(3, { attribs: { lang: 'es' } }), // ruze es
    camp(4, { body_source: PLAIN }), // no official block
    camp(5, { status: 'finished' }),
    camp(6, { status: 'running', evergreen: true }), // running evergreen, ruze en
    camp(7, { status: 'running', evergreen: false }), // a broadcast in flight: never touched
    camp(8, { content_type: 'richtext' }),
    camp(9, { status: 'paused' }),
    camp(10, { status: 'scheduled', lists: [{ id: 25 }] }), // curated: corporate only
    camp(11, { lists: [{ id: 15 }, { id: 16 }] }), // two brands
  ];
  const templates = [
    tpl(14, 'Official_RUZE_Footer_EN', { brand: 'ruze' }),
    tpl(30, 'Official_Footer_EN'),
    tpl(29, 'Shala_EN', { brand: 'shala', body_source: doc('brand', 'corporate') }),
    tpl(40, 'Ruze draft', { brand: 'ruze', body_source: doc('brand', 'corporate') }),
    tpl(41, 'Curated draft', { body_source: doc('corporate') }),
    tpl(42, 'Old html', { type: 'campaign', body_source: doc('corporate') }),
  ];

  const ruze = selectSweepItems(campaigns, templates, { id: 14, name: 'Official_RUZE_Footer_EN' }, LISTS);
  assert.deepEqual(ruze.campaigns.map((c) => c.id), [1, 9]);
  assert.deepEqual(ruze.evergreens.map((c) => c.id), [6]);
  assert.deepEqual(ruze.templates.map((t) => t.id), [40], 'a RUZE edit never selects Shala or curated items');
  assert.deepEqual(ruze.unresolved.map((c) => c.id), [11]);

  const corp = selectSweepItems(campaigns, templates, { id: 30, name: 'Official_Footer_EN' }, LISTS);
  assert.deepEqual(corp.campaigns.map((c) => c.id), [1, 2, 9, 10], 'every en carrier, any brand');
  assert.deepEqual(corp.evergreens.map((c) => c.id), [6]);
  assert.deepEqual(corp.templates.map((t) => t.id), [29, 40, 41]);
  assert.deepEqual(corp.unresolved.map((c) => c.id), [11]);

  const es = selectSweepItems(campaigns, templates, { id: 31, name: 'Official_Footer_ES' }, LISTS);
  assert.deepEqual(es.campaigns.map((c) => c.id), [3]);

  const none = selectSweepItems(campaigns, templates, { id: 99, name: 'Not official' }, LISTS);
  assert.equal(none.campaigns.length + none.evergreens.length + none.templates.length, 0);
});

test('I10: payload key sets equal CAMPAIGN_UPDATE_KEYS / TEMPLATE_UPDATE_KEYS (brand, lang included)', () => {
  assert.deepEqual([...CAMPAIGN_UPDATE_KEYS], [
    'archive_slug', 'name', 'subject', 'lists', 'from_email', 'messenger', 'type', 'tags', 'send_at',
    'headers', 'attribs', 'template_id', 'content_type', 'body', 'body_source', 'altbody', 'archive',
    'archive_template_id', 'archive_meta', 'media', 'evergreen', 'send_delay_secs',
  ]);
  assert.deepEqual([...TEMPLATE_UPDATE_KEYS], ['id', 'name', 'type', 'subject', 'body', 'body_source', 'brand', 'lang']);
  const c = campaignPayload({
    ...camp(1), lists: null, media: [{ id: 39 }], archive_meta: { a_b: 1 },
  }, '<b>');
  assert.deepEqual(Object.keys(c), [...CAMPAIGN_UPDATE_KEYS]);
  assert.deepEqual(c.lists, [], 'null lists -> [] (never undefined, which would clear)');
  assert.deepEqual(c.media, [39]);
  assert.deepEqual(c.archive_meta, { a_b: 1 }, 'nested keys untouched (raw rows)');
  assert.equal(c.body, '<b>');
  const t = templatePayload(tpl(40, 'x', { brand: 'ruze', lang: 'fr', is_default: false }), '<i>');
  assert.deepEqual(Object.keys(t), [...TEMPLATE_UPDATE_KEYS]);
  assert.equal(t.brand, 'ruze');
  assert.equal(t.lang, 'fr');
  assert.equal(templatePayload(tpl(41, 'y', { brand: undefined, lang: '' }), '').brand, '');
});

function fakeApi(over = {}) {
  const calls = [];
  const rows = {
    1: camp(1), 6: camp(6, { status: 'running', evergreen: true }), 9: camp(9, { status: 'paused' }),
  };
  return {
    calls,
    getCampaign: async (id) => { calls.push(['get', id]); return rows[id]; },
    getTemplate: async (id) => { calls.push(['getT', id]); return tpl(id, `t${id}`, { brand: 'ruze', body_source: doc('brand', 'corporate') }); },
    updateCampaign: async (id) => { calls.push(['put', id]); if (over.failPut === id) throw new Error('footer guard'); },
    updateTemplate: async (p) => { calls.push(['putT', p.id]); },
    changeStatus: async (id, s) => { calls.push([s, id]); if (over.failResume === id && s === 'running') throw new Error('guard'); },
  };
}
const PLAN = {
  campaigns: [{ id: 1 }, { id: 9 }], evergreens: [{ id: 6 }], templates: [{ id: 40 }], unresolved: [],
};
const compile = (d, ctx) => `<html>${ctx.lang}:${ctx.brand}</html>`;

test('I10: evergreens are paused before and resumed after their write when the user can campaigns:send', async () => {
  const api = fakeApi();
  const s = await runSweep(PLAN, {
    api, compile, canSend: true, lists: LISTS,
  });
  const ev = api.calls.filter((c) => c[1] === 6).map((c) => c[0]);
  assert.deepEqual(ev, ['paused', 'get', 'put', 'running']);
  assert.deepEqual(s.saved, ['c1', 'c9', 'c6', 't40']);
  assert.deepEqual(s.failed, []);
  assert.deepEqual(s.leftPaused, []);
});

test('I10: the resume runs even when the write fails', async () => {
  const api = fakeApi({ failPut: 6 });
  const s = await runSweep(PLAN, {
    api, compile, canSend: true, lists: LISTS,
  });
  assert.deepEqual(api.calls.filter((c) => c[1] === 6).map((c) => c[0]), ['paused', 'get', 'put', 'running']);
  assert.deepEqual(s.failed.map((f) => f.ref), ['c6']);
  assert.deepEqual(s.leftPaused, []);
  assert.deepEqual(s.saved, ['c1', 'c9', 't40'], 'one failure never stops the rest');
});

test('I10: a failed resume is reported as left paused', async () => {
  const api = fakeApi({ failResume: 6 });
  const s = await runSweep(PLAN, {
    api, compile, canSend: true, lists: LISTS,
  });
  assert.deepEqual(s.leftPaused.map((l) => l.ref), ['c6']);
});

test('I10: without campaigns:send running evergreens are listed, never touched', async () => {
  const api = fakeApi();
  const s = await runSweep(PLAN, {
    api, compile, canSend: false, lists: LISTS,
  });
  assert.equal(api.calls.filter((c) => c[1] === 6).length, 0);
  assert.deepEqual(s.notResaved, [6]);
  assert.deepEqual(s.saved, ['c1', 'c9', 't40']);
});

test('the compile receives each item\'s own context', async () => {
  const seen = [];
  const api = fakeApi();
  await runSweep(PLAN, {
    api, compile: (d, ctx) => { seen.push(ctx); return '<x>'; }, canSend: true, lists: LISTS,
  });
  assert.deepEqual(seen[0], { lang: 'en', brand: 'ruze' });
  assert.deepEqual(seen[3], { lang: 'en', brand: 'ruze' }, 'template: its own lang/brand columns');
});
