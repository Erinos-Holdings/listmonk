// integrations REVIEW-NAVIGATION-SPEC I2 / §7: the builder's blockNumbers and the review Lambda's
// blockNames number every block identically. The shared fixture test/numbers/block-numbers.json
// is a byte-identical copy of integrations tests/lib/campaign-review/fixtures/block-numbers.json
// (the integrations test byte-compares the two); it lives OUTSIDE test/fixtures/, which
// _umd.cjs::documentFixtures() sweeps into the compile-snapshot test. A numbering change is one
// workstream touching both copies.
//
// Scope of the equality claim: the builder's renderWalk and the Lambda's value-rules.ts
// `reachable` agree on BUILDER-SHAPED documents only. The Lambda walks data.childrenIds,
// props.childrenIds and props.columns on every block type; the builder walks them by type
// (EmailLayout, Container, ColumnsContainer). An editor-saved document (zod-validated) cannot
// carry those props on any other type, so the two never differ on a real campaign.
const path = require('path');
const S = require(path.join(__dirname, '.build', 'documents', 'structure.cjs'));
const fixture = require('./numbers/block-numbers.json');

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail !== undefined ? '  [' + JSON.stringify(detail).slice(0, 400) + ']' : ''}`); }
const J = (o) => JSON.stringify(o);

const numbers = [...S.blockNumbers(fixture.document).entries()];
check('I2: blockNumbers yields exactly the fixture\'s expected ids and numbers, in order', J(numbers) === J(fixture.expected), numbers);
check('renderWalk starts at root and is blockNumbers plus root', J(S.renderWalk(fixture.document)) === J(['root', ...fixture.expected.map(([id]) => id)]));

const doc = fixture.document;
const cols = Object.entries(doc).find(([, b]) => b.type === 'ColumnsContainer');
const numbered = new Set(fixture.expected.map(([id]) => id));
check('a block in the hidden third column has no number', cols && cols[1].data.props.columns[2].childrenIds.every((id) => !numbered.has(id)));
check('an orphan has no number (reachableIds still differs: hidden columns are in the tree)',
  Object.keys(doc).some((id) => id !== 'root' && !S.reachableIds(doc).has(id) && !numbered.has(id))
  && cols[1].data.props.columns[2].childrenIds.every((id) => S.reachableIds(doc).has(id)));
check('an OfficialFooter is numbered', Object.entries(doc).some(([id, b]) => b.type === 'OfficialFooter' && numbered.has(id)));

// The walk itself, beyond the fixture.
const tiny = {
  root: { type: 'EmailLayout', data: { childrenIds: ['block-a', 'block-missing', 'block-cols3', 'block-a'] } },
  'block-a': { type: 'Text', data: { props: { text: 'a' } } },
  'block-cols3': { type: 'ColumnsContainer', data: { props: { columnsCount: 3, columns: [{ childrenIds: ['block-c1'] }, { childrenIds: [] }, { childrenIds: ['block-c3'] }] } } },
  'block-c1': { type: 'Text', data: { props: { text: '1' } } },
  'block-c3': { type: 'Text', data: { props: { text: '3' } } },
};
check('columnsCount 3 walks the third column; a missing id is skipped; a repeat is visited once',
  J([...S.blockNumbers(tiny).entries()]) === J([['block-a', 1], ['block-cols3', 2], ['block-c1', 3], ['block-c3', 4]]), [...S.blockNumbers(tiny).entries()]);
const noCount = JSON.parse(J(tiny));
delete noCount['block-cols3'].data.props.columnsCount;
check('columnsCount absent defaults to 2 (the third column is hidden)', !S.blockNumbers(noCount).has('block-c3') && S.blockNumbers(noCount).has('block-c1'));
check('blockNumbers is memoized on the document object', S.blockNumbers(tiny) === S.blockNumbers(tiny));

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
