const path = require('path');
const fs = require('fs');

// markdownToVisualBlock (frontend/src/components/editor.js — the Vue SPA, not
// this package) is the OTHER new-document constructor: every format conversion
// to visual builds its document here, and stored visual documents never pass
// through it. This is the exact path where the EMPTY_EMAIL_MESSAGE default
// silently failed to apply (erinos.24/25), so its default is pinned by test.
// The file is pure and dependency-free; transpile-and-evaluate standalone,
// same approach as new-document-default.test.cjs.
const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));

const file = path.join(builderRoot, '..', 'src', 'components', 'editor.js');
const js = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, esModuleInterop: true },
}).outputText;

const mod = { exports: {} };
new Function('module', 'exports', 'require', js)(
  mod, mod.exports,
  (name) => { throw new Error(`unexpected import "${name}" in editor.js — update this test`); },
);
const markdownToVisualBlock = mod.exports.default;

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

const doc = markdownToVisualBlock('# Hello\n\nSome body text');
check('converted root is an EmailLayout', doc.root && doc.root.type === 'EmailLayout');
check('converted documents default Outlook compatibility ON', doc.root.data.outlook === true, `outlook=${JSON.stringify(doc.root.data.outlook)}`);
check('conversion produced blocks (right function evaluated)',
  Array.isArray(doc.root.data.childrenIds) && doc.root.data.childrenIds.length === 2
    && doc.root.data.childrenIds.every((id) => doc[id] && doc[id].type),
  `children=${JSON.stringify(doc.root.data.childrenIds)}`);
check('heading and text blocks intact',
  doc[doc.root.data.childrenIds[0]].type === 'Heading' && doc[doc.root.data.childrenIds[1]].type === 'Text');

const empty = markdownToVisualBlock('');
check('empty-body conversion still defaults ON', empty.root.data.outlook === true);

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
