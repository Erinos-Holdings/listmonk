// Fork (polish 2026-11-14): templatesPage.mjs -- the Templates page honours every global filter and
// its Footer | Other toggle. Run: node --test src/templatesPage.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { TEMPLATE_KINDS, normalizeTemplateKind, templateRows } from './templatesPage.mjs'; // eslint-disable-line import/extensions

const ALL = { kind: 'all' };
const T = [
  {
    id: 1, name: 'Newsletter', brand: 'ruze', lang: 'en',
  },
  {
    id: 2, name: 'Promo', brand: 'bnkd', lang: 'fr',
  },
  {
    id: 3, name: 'Plain', brand: '', lang: '',
  },
  {
    id: 4, name: 'Official Footer EN', brand: '', lang: 'en',
  },
  {
    id: 5, name: 'Official RUZE Footer FR', brand: 'ruze', lang: 'fr',
  },
];
// bnkd is unlaunched (status unknown); ruze launched.
const DOCS = [{ brand: 'bnkd', status: 'unknown' }, { brand: 'ruze', status: 'ok' }];
const ids = (opts) => templateRows(T, { selection: ALL, ...opts }).map((t) => t.id);

test('Footer | Other: official templates are Footer, everything else Other', () => {
  assert.deepEqual([...TEMPLATE_KINDS], ['other', 'footer']);
  assert.equal(normalizeTemplateKind('x'), 'other');
  assert.deepEqual(ids({}), [1, 2, 3]);
  assert.deepEqual(ids({ kind: 'footer' }), [4, 5]);
});

test('brand selector: a brand and the brandless; No brand the brandless', () => {
  assert.deepEqual(ids({ selection: { kind: 'brand', slug: 'ruze' } }), [1, 3]);
  assert.deepEqual(ids({ selection: { kind: 'none' } }), [3]);
});

test('launch filter: branded rows narrowed, brandless rows always kept', () => {
  assert.deepEqual(ids({ launch: 'launched', docs: DOCS }), [1, 3]);
  assert.deepEqual(ids({ launch: 'unlaunched', docs: DOCS }), [2, 3]);
  // The corporate footer is curated's (no health document = launched), so not unlaunched.
  assert.deepEqual(ids({ launch: 'unlaunched', docs: DOCS, kind: 'footer' }), []);
});

test('language toggle: that language plus the language-less', () => {
  assert.deepEqual(ids({ lang: 'fr' }), [2, 3]);
  assert.deepEqual(ids({ lang: 'en', kind: 'footer' }), [4]);
});

test('polish 2026-11-16: official footers file under their NAME\'s brand; the corporate ones under curated', () => {
  const F = [
    {
      id: 10, name: 'Official Footer EN', brand: '', lang: 'en',
    },
    {
      id: 11, name: 'Official Footer FR', brand: '', lang: 'fr',
    },
    {
      id: 12, name: 'Official Curated Footer', brand: '', lang: '',
    },
    {
      id: 13, name: 'Official BNKD Footer', brand: '', lang: '',
    },
    {
      id: 14, name: 'Official Thirsty Girl Footer ES', brand: 'thirstygirl', lang: 'es',
    },
    {
      id: 15, name: 'Plain', brand: '', lang: '',
    },
  ];
  const f = (selection) => templateRows(F, { selection, kind: 'footer' }).map((t) => t.id);
  assert.deepEqual(f({ kind: 'all' }), [10, 11, 12, 13, 14]);
  assert.deepEqual(f({ kind: 'brand', slug: 'curated' }), [10, 11, 12]);
  assert.deepEqual(f({ kind: 'brand', slug: 'bnkd' }), [13]);
  assert.deepEqual(f({ kind: 'brand', slug: 'thirstygirl' }), [14]);
  assert.deepEqual(f({ kind: 'none' }), []);
  // Other rows keep the brand column rule: the brandless show under every brand.
  assert.deepEqual(templateRows(F, { selection: { kind: 'brand', slug: 'bnkd' } }).map((t) => t.id), [15]);
  // Fork (integrations BRAND-GROUPS-SPEC I7) -- a group: the members' footers (curated's corporate
  // ones only when curated is a member -- it never is), the brandless Other rows kept.
  const grp = { kind: 'group', group: 'incubating', slugs: ['bnkd', 'thirstygirl'] };
  assert.deepEqual(f(grp), [13, 14]);
  assert.deepEqual(templateRows(F, { selection: grp }).map((t) => t.id), [15]);
});
