// CONTAINER-NESTING-SPEC I1-I7 against the standalone build of src/documents/structure.ts
// (import-free, compiled by run.cjs -- I9), plus I5's round trip through
// .build/official/insert.cjs.
//
//   I1  wrapperState follows D2/D3.
//   I2  parentOf / ancestorsOf / containerDepth in all three slot kinds (hidden column
//       included), null/partial for detached ids, termination on a cyclic document.
//   I3  unwrap: children at the container's index in order, exactly the container key removed,
//       every other block deep-equal, input never mutated; refusals.
//   I4  deleteSubtree: removes exactly {id} ∪ descendants minus externally shared ids; no slot
//       references a missing key; reachable set = key set on an orphan-free input; the
//       descendant count; input never mutated. unwrap refuses a shared id.
//   I5  deleteSubtree of a Container holding the footer pair, then insertOfficialFooter inserts
//       the pair again (the D12 orphan case is in official-insert.test.cjs).
//   I6  the c110-shaped fixture: group/9, and unwrap yields the 9 ids then the two footers.
//   I7  insertAt in each slot kind, refusing an existing id; freshId never returns a key;
//       Columns props preserved everywhere; duplicateSubtree fresh ids, footers skipped.
const path = require('path');
const assert = require('assert');
const S = require(path.join(__dirname, '.build', 'documents', 'structure.cjs'));
const { insertOfficialFooter } = require(path.join(__dirname, '.build', 'official', 'insert.cjs'));
const c110 = require('./fixtures/campaign110-shaped.json').body_source;

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${ok || detail === undefined ? '' : '  [' + String(typeof detail === 'string' ? detail : JSON.stringify(detail)).slice(0, 400) + ']'}`); }
const clone = (o) => JSON.parse(JSON.stringify(o));
const J = (o) => JSON.stringify(o);
const deepEq = (a, b) => { try { assert.deepStrictEqual(a, b); return true; } catch (e) { return false; } };

// Every id referenced from any slot.
function referenced(doc) {
  const out = [];
  for (const b of Object.values(doc)) out.push(...S.childIdsOf(b));
  return out;
}
const danglingRefs = (doc) => referenced(doc).filter((id) => !(id in doc));
const reachableEqualsKeys = (doc) => J([...S.reachableIds(doc)].sort()) === J(Object.keys(doc).sort());

// A tree with every slot kind: root -> [t0, box, cols, t9]; box (Container, styled) -> [t1,
// inner]; inner (Container, unstyled) -> [t2]; cols (columnsCount 2) -> col0 [t3, sub],
// col1 [t4], col2 (hidden) [t5]; sub (Container) -> [t6].
const tree = () => ({
  root: { type: 'EmailLayout', data: { backdropColor: '#F5F5F5', childrenIds: ['t0', 'box', 'cols', 't9'] } },
  t0: { type: 'Text', data: { props: { text: '0' } } },
  box: { type: 'Container', data: { style: { backgroundColor: '#0b3c49', padding: { top: 16, right: 24, bottom: 16, left: 24 } }, props: { childrenIds: ['t1', 'inner'] } } },
  t1: { type: 'Text', data: { props: { text: '1' } } },
  inner: { type: 'Container', data: { style: null, props: { childrenIds: ['t2'] } } },
  t2: { type: 'Text', data: { props: { text: '2' } } },
  cols: {
    type: 'ColumnsContainer',
    data: {
      style: { padding: { top: 8, right: 8, bottom: 8, left: 8 } },
      props: { columnsCount: 2, columnsGap: 16, fixedWidths: [200, null, null], contentAlignment: 'middle', columns: [{ childrenIds: ['t3', 'sub'], extra: 'a' }, { childrenIds: ['t4'] }, { childrenIds: ['t5'], extra: 'c' }] },
    },
  },
  t3: { type: 'Text', data: { props: { text: '3' } } },
  sub: { type: 'Container', data: { style: { padding: { top: 0, right: 0, bottom: 0, left: 0 } }, props: { childrenIds: ['t6'] } } },
  t4: { type: 'Text', data: { props: { text: '4' } } },
  t5: { type: 'Text', data: { props: { text: '5' } } },
  t6: { type: 'Text', data: { props: { text: '6' } } },
  t9: { type: 'Text', data: { props: { text: '9' } } },
});
const colsPropsSansColumns = (doc) => { const { columns, ...rest } = doc.cols.data.props; return { style: doc.cols.data.style, rest, extras: columns.map(({ childrenIds, ...e }) => e) }; };

// ---------------------------------------------------------------- I1
{
  const C = (style, childrenIds = ['x', 'y']) => ({ type: 'Container', data: { style, props: { childrenIds } } });
  const Z = { top: 0, right: 0, bottom: 0, left: 0 };
  const unstyled = [
    ['style null', C(null)],
    ['style absent', { type: 'Container', data: { props: { childrenIds: ['x', 'y'] } } }],
    ['style {}', C({})],
    ['explicit zero padding', C({ padding: Z })],
    ['padding null', C({ padding: null })],
    ['falsy colours', C({ backgroundColor: null, borderColor: '', padding: Z })],
    ['radius 0', C({ borderRadius: 0, padding: Z })],
    ['radius null', C({ borderRadius: null })],
    ['unknown falsy key', C({ somethingElse: null, padding: Z })],
  ];
  for (const [name, b] of unstyled) check(`I1: unstyled -- ${name}`, deepEq(S.wrapperState(b), { kind: 'group', count: 2 }), S.wrapperState(b));
  const styled = [
    ['born-with default padding 16/24/16/24', C({ padding: { top: 16, bottom: 16, left: 24, right: 24 } })],
    ['background colour', C({ backgroundColor: '#ffffff' })],
    ['border colour', C({ borderColor: '#cccccc', padding: Z })],
    ['radius > 0', C({ borderRadius: 8 })],
    ['one nonzero side', C({ padding: { top: 0, right: 0, bottom: 1, left: 0 } })],
    ['padding missing a side', C({ padding: { top: 0, right: 0, bottom: 0 } })],
    ['unknown truthy key', C({ padding: Z, textAlign: 'center' })],
  ];
  for (const [name, b] of styled) check(`I1: styled -- ${name}`, S.wrapperState(b) === null, S.wrapperState(b));
  check('I1: ColumnsContainer is never classified', S.wrapperState({ type: 'ColumnsContainer', data: { style: null, props: { columns: [{ childrenIds: [] }, { childrenIds: [] }, { childrenIds: [] }] } } }) === null);
  check('I1: a non-container is never classified', S.wrapperState({ type: 'Text', data: { style: null } }) === null && S.wrapperState(undefined) === null);
  check('I1: kind by direct child count -- 0 empty', deepEq(S.wrapperState(C(null, [])), { kind: 'empty', count: 0 }));
  check('I1: kind by direct child count -- 1 single', deepEq(S.wrapperState(C(null, ['x'])), { kind: 'single', count: 1 }));
  check('I1: kind by direct child count -- 3 group', deepEq(S.wrapperState(C(null, ['x', 'y', 'z'])), { kind: 'group', count: 3 }));
  check('I1: no childrenIds is empty', deepEq(S.wrapperState({ type: 'Container', data: { style: null } }), { kind: 'empty', count: 0 }));
  check('D3 wording', S.wrapperMessage({ kind: 'empty', count: 0 }) === 'Empty container'
    && S.wrapperMessage({ kind: 'single', count: 1 }) === 'Redundant wrapper — it adds nothing around its one block'
    && S.wrapperMessage({ kind: 'group', count: 9 }) === 'Groups 9 blocks with no styling of its own');
  check('styleSummary lists the styling a D4 confirm shows',
    J(S.styleSummary(C({ backgroundColor: '#0b3c49', borderColor: '#cccccc', borderRadius: 8, padding: { top: 16, right: 24, bottom: 16, left: 24 } })))
      === J(['background #0b3c49', 'border #cccccc', 'radius 8', 'padding 16/24/16/24']), S.styleSummary(C({ backgroundColor: '#0b3c49', borderColor: '#cccccc', borderRadius: 8, padding: { top: 16, right: 24, bottom: 16, left: 24 } })));
  check('styleSummary of an unstyled container is empty', S.styleSummary(C({ padding: Z, backgroundColor: null })).length === 0);
}

// ---------------------------------------------------------------- I2
{
  const d = tree();
  check('I2: parentOf root is null', S.parentOf(d, 'root') === null);
  check('I2: parentOf in the root slot', deepEq(S.parentOf(d, 'cols'), { parentId: 'root', column: null, index: 2 }));
  check('I2: parentOf in a Container slot', deepEq(S.parentOf(d, 'inner'), { parentId: 'box', column: null, index: 1 }));
  check('I2: parentOf in a Columns column', deepEq(S.parentOf(d, 'sub'), { parentId: 'cols', column: 0, index: 1 }));
  check('I2: parentOf in the hidden column', deepEq(S.parentOf(d, 't5'), { parentId: 'cols', column: 2, index: 0 }));
  check('I2: parentOf a detached id is null', S.parentOf({ ...d, lone: { type: 'Text', data: {} } }, 'lone') === null && S.parentOf(d, 'nope') === null);
  check('I2: ancestorsOf root-first, parent last', J(S.ancestorsOf(d, 't6')) === J(['root', 'cols', 'sub']) && J(S.ancestorsOf(d, 't2')) === J(['root', 'box', 'inner']));
  check('I2: ancestorsOf a root child is [root]; of root is []', J(S.ancestorsOf(d, 't0')) === J(['root']) && J(S.ancestorsOf(d, 'root')) === J([]));
  check('I2: ancestorsOf a detached id is []', J(S.ancestorsOf(d, 'nope')) === J([]));
  const orphaned = { ...clone(d), orphan: { type: 'Container', data: { style: null, props: { childrenIds: ['o1'] } } }, o1: { type: 'Text', data: {} } };
  check('I2: ancestorsOf under an orphan is partial (no root)', J(S.ancestorsOf(orphaned, 'o1')) === J(['orphan']));
  check('I2: containerDepth', S.containerDepth(d, 't0') === 0 && S.containerDepth(d, 'box') === 0 && S.containerDepth(d, 'inner') === 1 && S.containerDepth(d, 't2') === 2 && S.containerDepth(d, 't6') === 2 && S.containerDepth(d, 't5') === 1);
  // Cyclic: a -> b -> a, hung off root via a.
  const cyc = {
    root: { type: 'EmailLayout', data: { childrenIds: ['a'] } },
    a: { type: 'Container', data: { style: null, props: { childrenIds: ['b'] } } },
    b: { type: 'Container', data: { style: null, props: { childrenIds: ['a'] } } },
  };
  const anc = S.ancestorsOf(cyc, 'b');
  check('I2: ancestorsOf terminates on a cycle', Array.isArray(anc) && anc.length <= 3, anc);
  const cyc2 = { x: { type: 'Container', data: { props: { childrenIds: ['y'] } } }, y: { type: 'Container', data: { props: { childrenIds: ['x'] } } } };
  check('I2: a rootless cycle terminates too', Array.isArray(S.ancestorsOf(cyc2, 'x')) && typeof S.containerDepth(cyc2, 'y') === 'number');
  check('I2: descendantsOf terminates on a cycle, each id once', J(S.descendantsOf(cyc, 'a')) === J(['b']));
  check('I2: descendantsOf is depth-first over every slot kind', J(S.descendantsOf(d, 'root')) === J(['t0', 'box', 't1', 'inner', 't2', 'cols', 't3', 'sub', 't6', 't4', 't5', 't9']), S.descendantsOf(d, 'root'));
  check('breadcrumb labels', J(S.breadcrumb(d, 't6').map((s) => s.label)) === J(['Email', 'Columns (col 1)', 'Container', 'Text'])
    && J(S.breadcrumb(d, 't5').map((s) => s.label)) === J(['Email', 'Columns (col 3, hidden)', 'Text'])
    && J(S.breadcrumb(d, 'inner').map((s) => s.label)) === J(['Email', 'Container', 'Container']), S.breadcrumb(d, 't5'));
}

// ---------------------------------------------------------------- I3
{
  const d = tree();
  const frozen = J(d);
  const res = S.unwrap(d, 'sub');
  check('I3: input never mutated', J(d) === frozen);
  check('I3: children replace the container at its index in the same slot', J(res.doc.cols.data.props.columns[0].childrenIds) === J(['t3', 't6']));
  check('I3: firstChildId', res.firstChildId === 't6');
  check('I3: exactly the container key removed', J(Object.keys(d).filter((k) => !(k in res.doc))) === J(['sub']) && Object.keys(res.doc).length === Object.keys(d).length - 1);
  check('I3: every other block deep-equal', Object.keys(res.doc).filter((k) => k !== 'cols').every((k) => deepEq(res.doc[k], d[k])));
  check('I3/I7: Columns and column props preserved', deepEq(colsPropsSansColumns(res.doc), colsPropsSansColumns(d)));
  const r2 = S.unwrap(tree(), 'box');
  check('I3: unwrap in the root slot keeps order', J(r2.doc.root.data.childrenIds) === J(['t0', 't1', 'inner', 'cols', 't9']) && r2.doc.root.data.backdropColor === '#F5F5F5');
  const empty = tree();
  empty.inner.data.props.childrenIds = [];
  const r3 = S.unwrap(empty, 'inner');
  check('I3: an empty container unwraps to nothing, firstChildId null', r3.firstChildId === null && J(r3.doc.box.data.props.childrenIds) === J(['t1']));
  check('I3: refuses root', S.unwrap(tree(), 'root').refused === 'root');
  check('I3: refuses a non-Container (ColumnsContainer, Text, missing)', S.unwrap(tree(), 'cols').refused === 'not-container' && S.unwrap(tree(), 't0').refused === 'not-container' && S.unwrap(tree(), 'nope').refused === 'not-container');
  check('I3: refuses a detached id', S.unwrap({ ...tree(), lone: { type: 'Container', data: { props: { childrenIds: [] } } } }, 'lone').refused === 'detached');
  const fic = tree();
  fic.sub.data.props.childrenIds = ['t6', 'ofb'];
  fic.ofb = { type: 'OfficialFooter', data: { props: { kind: 'brand' } } };
  check('I3 (D9): refuses a column container that directly holds an OfficialFooter', S.unwrap(fic, 'sub').refused === 'footer-in-column');
  const fir = tree();
  fir.inner.data.props.childrenIds = ['t2', 'ofb'];
  fir.ofb = { type: 'OfficialFooter', data: { props: { kind: 'brand' } } };
  check('D9 is column-only: a Container in a Container may unwrap its footer', !('refused' in S.unwrap(fir, 'inner')));
}

// ---------------------------------------------------------------- I4
{
  const d = tree();
  const frozen = J(d);
  const res = S.deleteSubtree(d, 'cols');
  check('I4: input never mutated', J(d) === frozen);
  check('I4: removes exactly {id} ∪ descendants (nested, hidden column included)', J(Object.keys(d).filter((k) => !(k in res.doc)).sort()) === J(['cols', 'sub', 't3', 't4', 't5', 't6']));
  check('I4: reports the descendant count', res.removed === 5, res.removed);
  check('I4: no slot references a missing key', danglingRefs(res.doc).length === 0, danglingRefs(res.doc));
  check('I4: orphan-free input -> reachable set equals the key set', reachableEqualsKeys(d) && reachableEqualsKeys(res.doc));
  const r2 = S.deleteSubtree(tree(), 'box');
  check('I4: Container subtree removed, nested', r2.removed === 3 && !('inner' in r2.doc) && !('t2' in r2.doc) && reachableEqualsKeys(r2.doc) && danglingRefs(r2.doc).length === 0);
  const r3 = S.deleteSubtree(tree(), 't5');
  check('I4: a leaf in the hidden column: count 0, column stripped, props kept', r3.removed === 0 && J(r3.doc.cols.data.props.columns[2].childrenIds) === J([]) && deepEq(colsPropsSansColumns(r3.doc), colsPropsSansColumns(tree())));
  const rr = S.deleteSubtree(tree(), 'root');
  check('I4: root refused (input returned, removed 0)', rr.removed === 0 && deepEq(rr.doc, tree()));
  // Shared: t2 also referenced from the root slot; inner's subtree is deleted, t2 is spared.
  const sh = tree();
  sh.root.data.childrenIds = [...sh.root.data.childrenIds, 't2'];
  const rs = S.deleteSubtree(sh, 'box');
  check('I4: an externally shared descendant is spared', 't2' in rs.doc && !('inner' in rs.doc) && !('box' in rs.doc) && !('t1' in rs.doc) && rs.removed === 2, Object.keys(rs.doc));
  check('I4: after sparing, no slot references a missing key', danglingRefs(rs.doc).length === 0);
  // Shared subtree: sub referenced from root too -> sub and t6 both spared.
  const sh2 = tree();
  sh2.root.data.childrenIds = [...sh2.root.data.childrenIds, 'sub'];
  const rs2 = S.deleteSubtree(sh2, 'cols');
  check('I4: a shared descendant keeps its own subtree', 'sub' in rs2.doc && 't6' in rs2.doc && danglingRefs(rs2.doc).length === 0 && rs2.removed === 3, Object.keys(rs2.doc));
  const shU = tree();
  shU.root.data.childrenIds = [...shU.root.data.childrenIds, 'inner'];
  check('I4: unwrap refuses a shared id', S.unwrap(shU, 'inner').refused === 'shared');
}

// ---------------------------------------------------------------- I5
{
  const d = tree();
  d.box.data.props.childrenIds = ['t1', 'inner', 'ofb', 'ofc'];
  d.ofb = { type: 'OfficialFooter', data: { props: { kind: 'brand' } } };
  d.ofc = { type: 'OfficialFooter', data: { props: { kind: 'corporate' } } };
  check('I5 precondition: the pair is present, so Insert is a no-op', insertOfficialFooter(d, 'root', 0, { brand: 'ruze' }) === d);
  const del = S.deleteSubtree(d, 'box').doc;
  check('I5: the footers left with their container', !Object.values(del).some((b) => b.type === 'OfficialFooter'));
  const again = insertOfficialFooter(del, 'root', 4, { lang: 'en', brand: 'ruze' });
  const kinds = again.root.data.childrenIds.filter((id) => again[id].type === 'OfficialFooter').map((id) => again[id].data.props.kind);
  check('I5: insertOfficialFooter inserts the pair again after a subtree delete', J(kinds) === J(['brand', 'corporate']), kinds);
}

// ---------------------------------------------------------------- I6
{
  const d = clone(c110);
  const wrapperId = d.root.data.childrenIds[0];
  const nine = d[wrapperId].data.props.childrenIds;
  const footers = d.root.data.childrenIds.slice(1);
  check('I6 fixture: root -> [wrapper, OfficialFooter, OfficialFooter]', footers.length === 2 && footers.every((id) => d[id].type === 'OfficialFooter'));
  check('I6: the wrapper classifies as group/9', deepEq(S.wrapperState(d[wrapperId]), { kind: 'group', count: 9 }));
  check('I6: zero orphans in the fixture', reachableEqualsKeys(d));
  const res = S.unwrap(d, wrapperId);
  check('I6: unwrap yields root = the 9 ids then the two footers', J(res.doc.root.data.childrenIds) === J([...nine, ...footers]));
  check('I6: no orphan after unwrap', reachableEqualsKeys(res.doc) && danglingRefs(res.doc).length === 0);
}

// ---------------------------------------------------------------- I7
{
  const d = tree();
  const frozen = J(d);
  const nb = { type: 'Text', data: { props: { text: 'new' } } };
  const a = S.insertAt(d, { parentId: 'root', column: null }, 1, 'n1', nb);
  check('I7: insertAt in the root slot', J(a.root.data.childrenIds) === J(['t0', 'n1', 'box', 'cols', 't9']) && a.n1 === nb && a.root.data.backdropColor === '#F5F5F5');
  check('I7: other slots untouched', ['box', 'inner', 'cols', 'sub'].every((k) => a[k] === d[k]));
  const b = S.insertAt(d, { parentId: 'box', column: null }, 0, 'n1', nb);
  check('I7: insertAt in a Container slot', J(b.box.data.props.childrenIds) === J(['n1', 't1', 'inner']) && deepEq(b.box.data.style, d.box.data.style));
  const c = S.insertAt(d, { parentId: 'cols', column: 2 }, 5, 'n1', nb);
  check('I7: insertAt in the hidden column (index clamped)', J(c.cols.data.props.columns[2].childrenIds) === J(['t5', 'n1']) && J(c.cols.data.props.columns[0].childrenIds) === J(['t3', 'sub']));
  check('I7: insertAt preserves Columns and column props', deepEq(colsPropsSansColumns(c), colsPropsSansColumns(d)));
  check('I7: insertAt refuses an existing id', S.insertAt(d, { parentId: 'root', column: null }, 0, 't0', nb) === d);
  check('I7: insertAt refuses a missing slot', S.insertAt(d, { parentId: 'cols', column: null }, 0, 'n1', nb) === d && S.insertAt(d, { parentId: 'box', column: 1 }, 0, 'n1', nb) === d && S.insertAt(d, { parentId: 'nope', column: null }, 0, 'n1', nb) === d);
  check('I7: input never mutated', J(d) === frozen);

  // freshId: never a key, even when the natural candidates are taken.
  const now = Date.now;
  Date.now = () => 1234;
  const busy = { ...tree(), 'block-1234-0': { type: 'Text', data: {} }, 'block-1234-1': { type: 'Text', data: {} } };
  const f = S.freshId(busy);
  const taken = new Set();
  const g1 = S.freshId(busy, taken);
  const g2 = S.freshId(busy, taken);
  Date.now = now;
  check('I7: freshId never returns a key', !(f in busy) && f === 'block-1234-2', f);
  check('I7: freshId with a taken set never repeats', g1 !== g2 && !(g1 in busy) && !(g2 in busy));

  // Every helper preserves Columns props (deleteSubtree/unwrap covered above; duplicate here).
  const dup = S.duplicateSubtree(d, 'sub');
  check('I7: duplicateSubtree places the clone after the source in its slot', J(dup.doc.cols.data.props.columns[0].childrenIds) === J(['t3', 'sub', dup.newId]));
  check('I7: duplicateSubtree preserves Columns and column props', deepEq(colsPropsSansColumns(dup.doc), colsPropsSansColumns(d)));
  const newKeys = Object.keys(dup.doc).filter((k) => !(k in d));
  check('I7: every clone has a fresh id', newKeys.length === 2 && newKeys.includes(dup.newId) && new Set(newKeys).size === 2);
  check('I7: the clone references its own cloned child', dup.doc[dup.newId].data.props.childrenIds.length === 1 && newKeys.includes(dup.doc[dup.newId].data.props.childrenIds[0]));
  check('I7: duplicateSubtree never mutates its input', J(d) === frozen);
  const dupCols = S.duplicateSubtree(d, 'cols');
  const cc = dupCols.doc[dupCols.newId];
  check('I7: duplicating a Columns row clones every column (hidden included) and keeps its props', cc.data.props.columns.every((col, i) => col.childrenIds.length === d.cols.data.props.columns[i].childrenIds.length && col.childrenIds.every((id) => !(id in d)))
    && cc.data.props.columnsGap === 16 && cc.data.props.columns[0].extra === 'a' && danglingRefs(dupCols.doc).length === 0);

  // D13: footers skipped.
  const fd = tree();
  fd.box.data.props.childrenIds = ['t1', 'ofb', 'inner', 'ofc'];
  fd.ofb = { type: 'OfficialFooter', data: { props: { kind: 'brand' } } };
  fd.ofc = { type: 'OfficialFooter', data: { props: { kind: 'corporate' } } };
  const fdup = S.duplicateSubtree(fd, 'box');
  const footersAfter = Object.values(fdup.doc).filter((b) => b.type === 'OfficialFooter').length;
  check('I7 (D13): duplicating a Container holding the footer pair copies no OfficialFooter', footersAfter === 2, footersAfter);
  check('I7 (D13): the clone keeps its other children in order', fdup.doc[fdup.newId].data.props.childrenIds.map((id) => fdup.doc[id].type).join(',') === 'Text,Container');
  check('D13: an OfficialFooter itself is never duplicated', S.duplicateSubtree(fd, 'ofb').newId === null);
  check('duplicateSubtree refuses root and detached ids', S.duplicateSubtree(fd, 'root').newId === null && S.duplicateSubtree({ ...fd, lone: { type: 'Text', data: {} } }, 'lone').newId === null);
  check('duplicate in the root slot', J(S.duplicateSubtree(tree(), 't0').doc.root.data.childrenIds.slice(0, 1)) === J(['t0']));
}

// ---------------------------------------------------------------- review fixes
// (1) the memoized position index equals the uncached scan, on every fixture and edge doc.
{
  const fx = ['campaign108-source.json', 'official-template-14.json', 'official-template-30.json', 'campaign110-shaped.json']
    .map((f) => require('./fixtures/' + f).body_source);
  const shared = tree();
  shared.root.data.childrenIds = [...shared.root.data.childrenIds, 't2', 'sub'];
  const orphaned = { ...tree(), orphan: { type: 'Container', data: { props: { childrenIds: ['o1', 't0'] } } }, o1: { type: 'Text', data: {} } };
  const cyc = { root: { type: 'EmailLayout', data: { childrenIds: ['a'] } }, a: { type: 'Container', data: { props: { childrenIds: ['b', 'b'] } } }, b: { type: 'Container', data: { props: { childrenIds: ['a'] } } } };
  const docs = [tree(), shared, orphaned, cyc, ...fx.map(clone)];
  let mismatches = [];
  for (const d of docs) {
    for (const id of [...Object.keys(d), 'nope']) {
      if (!deepEq(S.parentOf(d, id), S.parentOfUncached(d, id))) mismatches.push(id);
    }
  }
  check('fix 1: the memoized parentOf equals the uncached scan on every fixture (shared, orphaned, cyclic included)', mismatches.length === 0, mismatches);
  const d = tree();
  const p1 = S.parentOf(d, 'sub');
  p1.index = 99;
  check('fix 1: a returned position is a copy (mutating it cannot poison the cache)', S.parentOf(d, 'sub').index === 1);
  check('fix 1: a new document object gets a fresh index', S.parentOf(S.unwrap(d, 'sub').doc, 't6').index === 1);

  // Timing sanity: a 1000-block document, containers nested in chains, one full depth pass on a
  // cold document object (the pre-fix code took ~142 ms; the bound is generous against flakiness).
  const big = { root: { type: 'EmailLayout', data: { childrenIds: [] } } };
  let n = 0;
  for (let chain = 0; chain < 50 && n < 1000; chain++) {
    let parent = null;
    for (let depth = 0; depth < 4; depth++) {
      const cid = `c${chain}-${depth}`;
      const kids = [];
      for (let k = 0; k < 4; k++) { const tid = `t${chain}-${depth}-${k}`; big[tid] = { type: 'Text', data: {} }; kids.push(tid); n++; }
      big[cid] = { type: 'Container', data: { style: null, props: { childrenIds: kids } } };
      n++;
      if (parent) big[parent].data.props.childrenIds.push(cid); else big.root.data.childrenIds.push(cid);
      parent = cid;
    }
  }
  const keys = Object.keys(big);
  const t0 = process.hrtime.bigint();
  let total = 0;
  for (const id of keys) total += S.containerDepth(big, id);
  const ms = Number(process.hrtime.bigint() - t0) / 1e6;
  check(`fix 1: full containerDepth pass over ${keys.length} blocks is fast (${ms.toFixed(1)} ms < 150 ms)`, keys.length >= 1000 && ms < 150 && total > 0, ms);
}

// (4) unwrap's shared refusal: only reachable references count; a child already in the target
// slot is refused.
{
  const orphanRef = { ...tree(), orphan: { type: 'Container', data: { props: { childrenIds: ['inner'] } } } };
  const r = S.unwrap(orphanRef, 'inner');
  check('fix 4a (I4): a reference from an ORPHANED block does not make the id shared', !('refused' in r) && J(r.doc.box.data.props.childrenIds) === J(['t1', 't2']), r);
  const dupChild = tree();
  dupChild.box.data.props.childrenIds = ['t1', 'inner', 't2'];
  check('fix 4b (I3): refuses when a child already sits in the target slot (no duplicate id in one slot)', S.unwrap(dupChild, 'inner').refused === 'shared');
  const dupRoot = tree();
  dupRoot.root.data.childrenIds = ['t0', 'box', 'cols', 't9', 't1'];
  check('fix 4b (I3): … in the root slot too', S.unwrap(dupRoot, 'box').refused === 'shared');
}

// (5) moveWithinSlot: props preserved, ends no-op, input not mutated, every slot kind.
{
  const d = tree();
  const frozen = J(d);
  const up = S.moveWithinSlot(d, 'cols', 'up');
  check('fix 5 (I7): move up in the root slot', J(up.root.data.childrenIds) === J(['t0', 'cols', 'box', 't9']) && up.root.data.backdropColor === '#F5F5F5');
  const dn = S.moveWithinSlot(d, 't1', 'down');
  check('fix 5 (I7): move down in a Container slot keeps its style', J(dn.box.data.props.childrenIds) === J(['inner', 't1']) && deepEq(dn.box.data.style, d.box.data.style));
  const c0 = S.moveWithinSlot(d, 'sub', 'up');
  check('fix 5 (I7): move in a Columns column preserves Columns and column-entry props', J(c0.cols.data.props.columns[0].childrenIds) === J(['sub', 't3']) && deepEq(colsPropsSansColumns(c0), colsPropsSansColumns(d)), colsPropsSansColumns(c0));
  const hidden = tree();
  hidden.cols.data.props.columns[2].childrenIds = ['t5', 'x'];
  hidden.x = { type: 'Text', data: {} };
  const h = S.moveWithinSlot(hidden, 'x', 'up');
  check('fix 5 (I7): move in the hidden column, props preserved', J(h.cols.data.props.columns[2].childrenIds) === J(['x', 't5']) && h.cols.data.props.columns[2].extra === 'c' && deepEq(colsPropsSansColumns(h), colsPropsSansColumns(hidden)));
  check('fix 5 (I7): no-op at the ends (same object)', S.moveWithinSlot(d, 't0', 'up') === d && S.moveWithinSlot(d, 't9', 'down') === d && S.moveWithinSlot(d, 't6', 'up') === d && S.moveWithinSlot(d, 'nope', 'up') === d);
  check('fix 5 (I7): untouched blocks keep their identity', up.box === d.box && up.cols === d.cols && c0.root === d.root);
  check('fix 5 (I7): input never mutated', J(d) === frozen);
}

// Re-review finding 1: a block listing itself as a child is never rewritten by its own move.
{
  const d = {
    root: { type: 'EmailLayout', data: { childrenIds: ['a', 'box'] } },
    a: { type: 'Text', data: { props: { text: 'a' } } },
    box: { type: 'Container', data: { style: {}, props: { childrenIds: ['x', 'box'] } } },
    x: { type: 'Text', data: { props: { text: 'x' } } },
  };
  const next = S.moveWithinSlot(d, 'box', 'up');
  check('move parity: the moved block\'s own slot untouched', J(next.box) === J(d.box) && J(next.root.data.childrenIds) === J(['box', 'a']));
}

// integrations BIBLE-OUTLOOK-FIXES-SPEC §12 IA1: isUnstyledContainer against the SAME case table
// the integrations review pins its port against (tests/lib/campaign-review/fingerprint-v2.test.ts,
// BARE_CASES — `isBareContainer` there). The two tables agree by copy; each file names the other.
{
  const BARE_CASES = [
    ['null style', { type: 'Container', data: { style: null, props: { childrenIds: [] } } }, true],
    ['absent style', { type: 'Container', data: { props: { childrenIds: [] } } }, true],
    ['empty style', { type: 'Container', data: { style: {}, props: { childrenIds: [] } } }, true],
    ['a non-object style', { type: 'Container', data: { style: 'padding:0', props: { childrenIds: [] } } }, false],
    ['zero padding', { type: 'Container', data: { style: { padding: { top: 0, right: 0, bottom: 0, left: 0 } }, props: {} } }, true],
    ['null padding', { type: 'Container', data: { style: { padding: null }, props: {} } }, true],
    ['one non-zero padding side', { type: 'Container', data: { style: { padding: { top: 0, right: 0, bottom: 1, left: 0 } }, props: {} } }, false],
    ['a padding object missing a side', { type: 'Container', data: { style: { padding: { top: 0, right: 0, bottom: 0 } }, props: {} } }, false],
    ['a non-object padding', { type: 'Container', data: { style: { padding: 0 }, props: {} } }, false],
    ['background', { type: 'Container', data: { style: { backgroundColor: '#ffffff' }, props: {} } }, false],
    ['empty-string background', { type: 'Container', data: { style: { backgroundColor: '' }, props: {} } }, true],
    ['border colour', { type: 'Container', data: { style: { borderColor: '#000000' }, props: {} } }, false],
    ['empty-string border colour', { type: 'Container', data: { style: { borderColor: '' }, props: {} } }, true],
    ['radius 0', { type: 'Container', data: { style: { borderRadius: 0 }, props: {} } }, true],
    ['radius null', { type: 'Container', data: { style: { borderRadius: null }, props: {} } }, true],
    ['radius 4', { type: 'Container', data: { style: { borderRadius: 4 }, props: {} } }, false],
    ['radius "0"', { type: 'Container', data: { style: { borderRadius: '0' }, props: {} } }, false],
    ['an unknown truthy key', { type: 'Container', data: { style: { textAlign: 'center' }, props: {} } }, false],
    ['an unknown falsy key', { type: 'Container', data: { style: { textAlign: null }, props: {} } }, true],
    ['a non-Container block', { type: 'Text', data: { style: null, props: { text: 'x' } } }, false],
  ];
  for (const [name, block, bare] of BARE_CASES) check(`IA1: ${name} -> ${bare ? 'unstyled' : 'styled'}`, S.isUnstyledContainer(block) === bare, String(S.isUnstyledContainer(block)));
}

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
