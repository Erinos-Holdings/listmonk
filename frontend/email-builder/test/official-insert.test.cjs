// OFFICIAL-FOOTER-SPEC I6. insertOfficialFooter adds brand (unless curated) then corporate at the
// given position and never duplicates a kind; the input document is never mutated.
const path = require('path');
const { insertOfficialFooter } = require(path.join(__dirname, '.build', 'official', 'insert.cjs'));

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

const base = () => ({
  root: { type: 'EmailLayout', data: { backdropColor: '#F5F5F5', childrenIds: ['a', 'box', 'b'] } },
  a: { type: 'Text', data: { props: { text: 'a' } } },
  b: { type: 'Text', data: { props: { text: 'b' } } },
  box: { type: 'Container', data: { style: {}, props: { childrenIds: ['c'] } } },
  c: { type: 'Text', data: { props: { text: 'c' } } },
  cols: { type: 'ColumnsContainer', data: { props: { columns: [{ childrenIds: [] }, { childrenIds: [] }, { childrenIds: [] }] } } },
});
const kinds = (doc, ids) => ids.map((id) => (doc[id].type === 'OfficialFooter' ? `OF:${doc[id].data.props.kind}` : id));

let doc = base();
const frozen = JSON.stringify(doc);
let next = insertOfficialFooter(doc, 'root', 3, { lang: 'en', brand: 'ruze' });
check('input not mutated', JSON.stringify(doc) === frozen);
check('appends brand then corporate at the end', JSON.stringify(kinds(next, next.root.data.childrenIds)) === JSON.stringify(['a', 'box', 'b', 'OF:brand', 'OF:corporate']), JSON.stringify(kinds(next, next.root.data.childrenIds)));
check('root props kept', next.root.data.backdropColor === '#F5F5F5');

next = insertOfficialFooter(base(), 'root', 1, { lang: 'en', brand: 'ruze' });
check('inserts at the menu position', JSON.stringify(kinds(next, next.root.data.childrenIds)) === JSON.stringify(['a', 'OF:brand', 'OF:corporate', 'box', 'b']));

next = insertOfficialFooter(base(), 'root', 3, { lang: 'en', brand: 'curated' });
check('curated: corporate only', JSON.stringify(kinds(next, next.root.data.childrenIds)) === JSON.stringify(['a', 'box', 'b', 'OF:corporate']));
next = insertOfficialFooter(base(), 'root', 3, { lang: 'en', brand: 'Curated' });
check('curated is case-insensitive', JSON.stringify(kinds(next, next.root.data.childrenIds)) === JSON.stringify(['a', 'box', 'b', 'OF:corporate']));
next = insertOfficialFooter(base(), 'root', 3, { lang: 'en', brand: null });
check('unknown brand (no list yet): the pair (the brand block resolves later)', JSON.stringify(kinds(next, next.root.data.childrenIds)) === JSON.stringify(['a', 'box', 'b', 'OF:brand', 'OF:corporate']));

// Idempotent: a kind present ANYWHERE is not inserted again.
const once = insertOfficialFooter(base(), 'box', 1, { lang: 'en', brand: 'ruze' });
check('inserts into a Container', JSON.stringify(kinds(once, once.box.data.props.childrenIds)) === JSON.stringify(['c', 'OF:brand', 'OF:corporate']));
const twice = insertOfficialFooter(once, 'root', 3, { lang: 'en', brand: 'ruze' });
check('second insert is a no-op (same object)', twice === once);
check('exactly one block of each kind', Object.values(twice).filter((b) => b.type === 'OfficialFooter').length === 2);

// Only corporate present -> only brand added.
const corpOnly = insertOfficialFooter(base(), 'root', 3, { lang: 'en', brand: 'curated' });
const thenBrand = insertOfficialFooter(corpOnly, 'root', 3, { lang: 'en', brand: 'ruze' });
check('missing kind only is added', JSON.stringify(kinds(thenBrand, thenBrand.root.data.childrenIds)) === JSON.stringify(['a', 'box', 'b', 'OF:brand', 'OF:corporate']), JSON.stringify(kinds(thenBrand, thenBrand.root.data.childrenIds)));

// Not a footer position.
const cols = base();
check('ColumnsContainer parent: unchanged', insertOfficialFooter(cols, 'cols', 0, { brand: 'ruze' }) === cols);
check('unknown parent: unchanged', insertOfficialFooter(cols, 'nope', 0, { brand: 'ruze' }) === cols);
const ids = Object.keys(insertOfficialFooter(base(), 'root', 0, { brand: 'ruze' })).filter((k) => !(k in base()));
check('fresh unique ids', ids.length === 2 && ids[0] !== ids[1], ids.join(','));

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
