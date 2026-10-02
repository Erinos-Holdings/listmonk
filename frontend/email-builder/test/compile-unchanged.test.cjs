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
const { SNAPSHOT, FLAG_OFF_SNAPSHOT, loadUmd, documentFixtures, compileInputs } = require('./_umd.cjs');

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
// integrations BIBLE-OUTLOOK-FIXES-SPEC §12 IA13: the snapshot covers the S5 fallback Button too, so
// a recapture's diff shows exactly what the table-cell fallback changed (and nothing else).
check('IA13: a fixture holds Buttons that fall back (fallback-buttons.json)', fixtures.some((x) => x.file === 'fallback-buttons.json'));
check('every fixture has a snapshot entry (a new fixture needs a deliberate re-capture)', fixtures.every((f) => typeof snapshot.outputs[f.file] === 'string'),
  fixtures.filter((f) => typeof snapshot.outputs[f.file] !== 'string').map((f) => f.file).join(','));

for (const { file, document } of fixtures) {
  const expected = snapshot.outputs[file];
  if (typeof expected !== 'string') continue;
  const html = EB.compileDocument(document, context, refs);
  check(`I8: ${file} compiles byte-identical to ${snapshot.baseCommit.slice(0, 8)}`, html === expected, firstDiff(html, expected));
  check(`I8: ${file} output carries no structure chrome`, !CHROME.test(html), (html.match(CHROME) || [])[0]);
}

// BIBLE-OUTLOOK-FIXES-SPEC I9: a document with the Outlook flag OFF compiles exactly as before
// the Word fixes. The fixture is the render canary's bible-06-layout-variants document (a
// Heading, column rows and Buttons); its output is pinned in a separate file captured from
// b5ceb3ce, the commit before the fixes, so a later recapture of the main snapshot cannot
// re-pin it. Any §4 pass that ran with the flag off would change this output.
const FLAG_OFF = 'bible-06-layout-variants.json';
const pin = JSON.parse(fs.readFileSync(FLAG_OFF_SNAPSHOT, 'utf8'));
check('I9: the flag-off pin was captured from b5ceb3ce (before the Word fixes)', /^b5ceb3ce/.test(pin.baseCommit), pin.baseCommit);
check('I9: the pin was taken with the same context and references', JSON.stringify(pin.context) === JSON.stringify(context)
  && JSON.stringify(pin.refs) === JSON.stringify(refs.map((r) => ({ id: r.id, name: r.name }))));
const flagOff = fixtures.find((f) => f.file === FLAG_OFF);
check('I9: the flag-off fixture exists and has the Outlook flag off', !!flagOff && flagOff.document.root.data.outlook === false);
check('I9: the fixture is still the render canary document (copied, never edited)',
  !!flagOff && JSON.stringify(flagOff.document) === JSON.stringify(JSON.parse(fs.readFileSync(require('path').join(__dirname, 'canary', 'bible-06-layout-variants.json'), 'utf8'))));
check('I9: it holds a Heading, a ColumnsContainer and a Button', !!flagOff
  && ['Heading', 'ColumnsContainer', 'Button'].every((t) => Object.values(flagOff.document).some((b) => b && b.type === t)));
if (flagOff && typeof pin.outputs[FLAG_OFF] === 'string') {
  const html = EB.compileDocument(flagOff.document, context, refs);
  check(`I9: ${FLAG_OFF} (Outlook flag off) compiles byte-identical to b5ceb3ce`, html === pin.outputs[FLAG_OFF], firstDiff(html, pin.outputs[FLAG_OFF]));
  check('I9: and carries no Word idiom (no Safe payload, no lm-cw- class, no v:textpath)', !/\{\{ Safe|lm-cw-|v:textpath/.test(html));
} else {
  check('I9: the pin holds the flag-off fixture', false);
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
