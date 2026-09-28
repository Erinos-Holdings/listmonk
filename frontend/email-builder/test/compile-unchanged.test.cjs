// CONTAINER-NESTING-SPEC I8 (D10): compiled output is unchanged. The BUILT bundle's
// compileDocument over every builder-document fixture in test/fixtures (every .json but the
// snapshot itself), with the context and references official-compile.test.cjs uses, must be
// byte-identical to test/fixtures/compile-snapshot.json -- captured from the base commit by
// test/capture-compile-snapshot.cjs -- and must carry none of the structure chrome markers.
//
// A STANDING TRIPWIRE: a later change that is MEANT to alter compiled output regenerates the
// snapshot deliberately with `node test/capture-compile-snapshot.cjs <commit>`; never edit it by
// hand and never regenerate it to silence an unexplained diff.
const fs = require('fs');
const { SNAPSHOT, loadUmd, documentFixtures, compileInputs } = require('./_umd.cjs');

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail ? '  [' + String(detail).slice(0, 400) + ']' : ''}`); }

// The chrome markers the structure tools add to the canvas only.
const CHROME = /data-lm-structure|lm-structure-tab|data-lm-breadcrumb|data-lm-wrapper-alert|data-lm-confirm|data-lm-unwrap/;

const snapshot = JSON.parse(fs.readFileSync(SNAPSHOT, 'utf8'));
check('snapshot records its base commit and bundle sha256', /^[0-9a-f]{40}$/.test(snapshot.baseCommit) && /^[0-9a-f]{64}$/.test(snapshot.bundleSha256));

const { dom, EB } = loadUmd();
const { context, refs } = compileInputs();
check('snapshot was taken with the same context and references', JSON.stringify(snapshot.context) === JSON.stringify(context)
  && JSON.stringify(snapshot.refs) === JSON.stringify(refs.map((r) => ({ id: r.id, name: r.name }))));

const fixtures = documentFixtures();
check('fixtures include the three builder documents and the I6 c110-shaped fixture',
  ['campaign108-source.json', 'official-template-14.json', 'official-template-30.json', 'campaign110-shaped.json'].every((f) => fixtures.some((x) => x.file === f)));
check('every fixture has a snapshot entry (a new fixture needs a deliberate re-capture)', fixtures.every((f) => typeof snapshot.outputs[f.file] === 'string'),
  fixtures.filter((f) => typeof snapshot.outputs[f.file] !== 'string').map((f) => f.file).join(','));

for (const { file, document } of fixtures) {
  const expected = snapshot.outputs[file];
  if (typeof expected !== 'string') continue;
  const html = EB.compileDocument(document, context, refs);
  check(`I8: ${file} compiles byte-identical to ${snapshot.baseCommit.slice(0, 8)}`, html === expected, firstDiff(html, expected));
  check(`I8: ${file} output carries no structure chrome`, !CHROME.test(html), (html.match(CHROME) || [])[0]);
}

dom.window.close();

function firstDiff(a, b) {
  if (a === b) return '';
  let i = 0;
  while (i < a.length && a[i] === b[i]) i++;
  return `at ${i}: ${JSON.stringify(a.slice(Math.max(0, i - 80), i + 80))} vs ${JSON.stringify(b.slice(Math.max(0, i - 80), i + 80))}`;
}

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
