const path = require('path');
const fs = require('fs');

// The color picker's hex text input accepts 3-digit hex (react-colorful validates 3 or 6),
// while every color prop schema is 6-digit only — so a typed `#888` painted the swatch and
// was then dropped by the sidebar panel's safeParse, never reaching the document (seen live
// on the Link color field, 2026-09-10). normalizeHex expands the short form at the picker.
const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));

function transpile(relSourcePath) {
  const src = fs.readFileSync(path.join(builderRoot, 'src', relSourcePath), 'utf8');
  return ts.transpileModule(src, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
  }).outputText;
}

function evaluate(js) {
  const mod = { exports: {} };
  new Function('module', 'exports', 'require', js)(mod, mod.exports, () => {
    throw new Error('normalizeHex must stay dependency-free');
  });
  return mod.exports;
}

const { normalizeHex } = evaluate(
  transpile('App/InspectorDrawer/ConfigurationPanel/input-panels/helpers/inputs/ColorInput/normalizeHex.ts')
);

const COLOR_SCHEMA = /^#[0-9a-fA-F]{6}$/;

let failed = 0;
function check(name, actual, expected) {
  const ok = actual === expected;
  if (!ok) failed++;
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}  [got ${JSON.stringify(actual)}]`);
}

check('3-digit expands to 6', normalizeHex('#888'), '#888888');
check('3-digit mixed case expands digit-wise', normalizeHex('#aB1'), '#aaBB11');
check('6-digit passes through untouched', normalizeHex('#888888'), '#888888');
check('surrounding whitespace is tolerated', normalizeHex(' #888 '), '#888888');
check('non-hex passes through untouched', normalizeHex('#88'), '#88');
check('empty passes through untouched', normalizeHex(''), '');
check('expanded form satisfies COLOR_SCHEMA', COLOR_SCHEMA.test(normalizeHex('#888')), true);
check('the raw short form did NOT satisfy COLOR_SCHEMA', COLOR_SCHEMA.test('#888'), false);

if (failed) {
  console.log(`\n${failed} FAILED`);
  process.exit(1);
}
console.log('\nALL PASS');
