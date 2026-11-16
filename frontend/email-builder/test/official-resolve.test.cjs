// OFFICIAL-FOOTER-SPEC I1, amended by integrations BRAND-FOOTER-SPEC I1/I2. An OfficialFooter block
// stores only `kind`; the reference is resolved from the context on every render, BY NAME, under
// the name grammar of BRAND-FOOTER-SPEC D1 and the resolution of D2 -- the shared case table
// test/fixtures/official-names.json (read by the Vue, Go and integrations suites too).
const fs = require('fs');
const path = require('path');
const { resolveOfficial, officialSlug, parseOfficialName, isOfficialName } = require(path.join(__dirname, '.build', 'official', 'resolve.cjs'));
const { insertOfficialFooter } = require(path.join(__dirname, '.build', 'official', 'insert.cjs'));

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

const TABLE = JSON.parse(fs.readFileSync(path.join(__dirname, 'fixtures', 'official-names.json'), 'utf8'));
const ref = (id, name, extra = {}) => ({ id, name, body_source: '{"root":{"type":"EmailLayout","data":{"childrenIds":[]}}}', ...extra });

check('the table has parse and resolve cases', TABLE.parse.length >= 20 && TABLE.resolve.length >= 15);
for (const c of TABLE.parse) {
  const got = parseOfficialName(c.name);
  check(`parse ${JSON.stringify(c.name)}`, JSON.stringify(got) === JSON.stringify(c.want), JSON.stringify(got));
}
for (const c of TABLE.resolve) {
  const r = resolveOfficial(c.kind, c.context, c.refs.map(([id, name]) => ref(id, name)));
  const got = { status: r.status, name: r.name, id: r.ref ? r.ref.id : null };
  check(`resolve: ${c.case}`, JSON.stringify(got) === JSON.stringify(c.want), JSON.stringify(got));
}

// The template lang/brand columns drive nothing.
let r = resolveOfficial('corporate', { lang: 'en', brand: 'ruze' }, [ref(30, 'Official Footer EN', { lang: 'fr', brand: 'ruze' })]);
check('columns drive nothing', r.status === 'ok' && r.ref.id === 30, JSON.stringify(r));
// CURATED-FOOTER-SPEC I1: no context brand yields status none.
const REFS = TABLE.resolve.flatMap((c) => c.refs.map(([id, name]) => ref(id, name)));
const anyNone = ['curated', 'Curated', 'ruze', 'liyora', '', null].some((b) =>
  ['en', 'es', 'fr'].some((l) => ['brand', 'corporate'].some((k) => resolveOfficial(k, { lang: l, brand: b }, REFS).status === 'none')));
check('I1: no context brand yields status none', !anyNone);

check('slug strips non-alphanumerics', officialSlug('Thirsty-Girl_2') === 'thirstygirl2');
check('isOfficialName: the new prefix only', isOfficialName('Official RUZE Footer') && !isOfficialName('Official_RUZE_Footer_EN') && !isOfficialName('official Footer EN'));

// The block stores ONLY kind: what the Insert action writes.
const doc = { root: { type: 'EmailLayout', data: { childrenIds: [] } } };
const next = insertOfficialFooter(doc, 'root', 0, { brand: 'ruze', lang: 'en' });
const blocks = Object.entries(next).filter(([, b]) => b.type === 'OfficialFooter').map(([, b]) => b);
check('inserted blocks carry only data.props.kind',
  blocks.length === 2 && blocks.every((b) => JSON.stringify(Object.keys(b.data)) === '["props"]' && JSON.stringify(Object.keys(b.data.props)) === '["kind"]'),
  JSON.stringify(blocks));

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
