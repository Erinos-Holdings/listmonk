// OFFICIAL-FOOTER-SPEC I1. An OfficialFooter block stores only `kind`; the reference is resolved
// from the context on every render, BY NAME: `en` for an empty lang, `curated` -> no brand
// block, absent name -> `missing` (no cross-language fallback), no context -> `no-context`,
// two rows of one name -> lowest id + `duplicate`. The template lang/brand columns drive nothing.
const path = require('path');
const { resolveOfficial, officialSlug, parseOfficialName } = require(path.join(__dirname, '.build', 'official', 'resolve.cjs'));
const { insertOfficialFooter } = require(path.join(__dirname, '.build', 'official', 'insert.cjs'));

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

const ref = (id, name, extra = {}) => ({ id, name, body_source: '{"root":{"type":"EmailLayout","data":{"childrenIds":[]}}}', ...extra });
const REFS = [
  ref(30, 'Official_Footer_EN', { lang: 'fr', brand: 'ruze' }), // columns deliberately wrong: they drive nothing
  ref(31, 'Official_Footer_ES'),
  ref(14, 'Official_RUZE_Footer_EN'),
  ref(19, 'Official_RUZE_Footer_ES'),
  ref(24, 'Official_ThirstyGirl_Footer_EN'),
];

let r = resolveOfficial('corporate', { lang: 'en', brand: 'ruze' }, REFS);
check('corporate en -> Official_Footer_EN', r.status === 'ok' && r.ref.id === 30 && r.name === 'Official_Footer_EN', JSON.stringify(r));
r = resolveOfficial('corporate', { lang: '', brand: 'ruze' }, REFS);
check('empty lang resolves as en', r.status === 'ok' && r.ref.id === 30);
r = resolveOfficial('corporate', { lang: null, brand: null }, REFS);
check('corporate needs lang only (brand unknown still resolves)', r.status === 'ok' && r.ref.id === 30);
r = resolveOfficial('corporate', { lang: 'es' }, REFS);
check('corporate es -> 31', r.status === 'ok' && r.ref.id === 31);
r = resolveOfficial('corporate', { lang: 'de' }, REFS);
check('absent corporate de -> missing, no fallback to EN', r.status === 'missing' && r.name === 'Official_Footer_DE' && !r.ref, JSON.stringify(r));

r = resolveOfficial('brand', { lang: 'en', brand: 'ruze' }, REFS);
check('brand ruze en -> 14 (RUZE slug)', r.status === 'ok' && r.ref.id === 14);
r = resolveOfficial('brand', { lang: 'es', brand: 'RUZE' }, REFS);
check('brand context is slug-compared (RUZE)', r.status === 'ok' && r.ref.id === 19);
r = resolveOfficial('brand', { lang: 'en', brand: 'thirstygirl' }, REFS);
check('ThirstyGirl -> thirstygirl', r.status === 'ok' && r.ref.id === 24);
r = resolveOfficial('brand', { lang: 'fr', brand: 'ruze' }, REFS);
check('brand fr absent -> missing (no cross-language fallback)', r.status === 'missing' && r.name === 'Official_ruze_Footer_FR');
r = resolveOfficial('brand', { lang: 'en', brand: 'liyora' }, REFS);
check('brand with no template -> missing', r.status === 'missing');
r = resolveOfficial('brand', { lang: 'en', brand: 'curated' }, REFS);
check('brand curated -> none', r.status === 'none');
r = resolveOfficial('brand', { lang: 'en', brand: null }, REFS);
check('brand null -> no-context', r.status === 'no-context');
r = resolveOfficial('brand', null, REFS);
check('no context -> no-context (brand)', r.status === 'no-context');
r = resolveOfficial('corporate', undefined, REFS);
check('no context -> no-context (corporate)', r.status === 'no-context');

const dup = [...REFS, ref(40, 'Official_Footer_EN'), ref(12, 'Official_Footer_EN')];
r = resolveOfficial('corporate', { lang: 'en' }, dup);
check('duplicate name -> lowest id + duplicate', r.status === 'duplicate' && r.ref.id === 12, JSON.stringify(r && r.ref));

// A non-official look-alike name never resolves.
r = resolveOfficial('corporate', { lang: 'en' }, [ref(5, 'Copy of Official_Footer_EN')]);
check('only the exact name resolves', r.status === 'missing');

check('slug strips non-alphanumerics', officialSlug('Thirsty-Girl_2') === 'thirstygirl2');
check('parse corporate', JSON.stringify(parseOfficialName('Official_Footer_IT')) === JSON.stringify({ kind: 'corporate', brand: '', lang: 'IT' }));
check('parse brand', JSON.stringify(parseOfficialName('Official_ThirstyGirl_Footer_DE')) === JSON.stringify({ kind: 'brand', brand: 'thirstygirl', lang: 'DE' }));

// The block stores ONLY kind: what the Insert action writes.
const doc = { root: { type: 'EmailLayout', data: { childrenIds: [] } } };
const next = insertOfficialFooter(doc, 'root', 0, { brand: 'ruze', lang: 'en' });
const blocks = Object.entries(next).filter(([, b]) => b.type === 'OfficialFooter').map(([, b]) => b);
check('inserted blocks carry only data.props.kind',
  blocks.length === 2 && blocks.every((b) => JSON.stringify(Object.keys(b.data)) === '["props"]' && JSON.stringify(Object.keys(b.data.props)) === '["kind"]'),
  JSON.stringify(blocks));

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
