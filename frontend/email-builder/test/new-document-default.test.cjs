const path = require('path');
const fs = require('fs');

// New-document defaults live in EMPTY_EMAIL_MESSAGE: it is the store's initial
// document (VisualEditor.vue renders with `data: {}`, a no-op merge), while
// stored campaigns arrive via resetDocument() and replace it wholesale. The
// file is a pure literal with a type-only import, so transpile it standalone
// and evaluate — same isolation approach as run.cjs takes for outlook.ts.
const ts = require(path.join(__dirname, '..', 'node_modules', 'typescript'));

const file = path.join(__dirname, '..', 'src', 'getConfiguration', 'sample', 'empty-email-message.ts');
const js = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;

const mod = { exports: {} };
new Function('module', 'exports', 'require', js)(mod, mod.exports, () => ({}));
const EMPTY = mod.exports.default;

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

check('EMPTY_EMAIL_MESSAGE root is an EmailLayout', EMPTY && EMPTY.root && EMPTY.root.type === 'EmailLayout');
check('new documents default Outlook compatibility ON', EMPTY.root.data.outlook === true, `outlook=${JSON.stringify(EMPTY.root.data.outlook)}`);
check('empty document has no children (right object evaluated)', Array.isArray(EMPTY.root.data.childrenIds) && EMPTY.root.data.childrenIds.length === 0);

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
