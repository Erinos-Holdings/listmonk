// integrations RENDER-CATALOG-SPEC §17.5 -- pins what the bundle from BEFORE the four Word fixes
// (F1-F4) compiled for word-fixes-docs.cjs's documents: every fixed document with the Outlook flag
// OFF, and every clean document (no fixed shape) with the flag ON. word-fixes.test.cjs requires
// the current bundle to compile all of them byte-identically.
//
//   node test/capture-word-fixes-before.cjs --umd <email-builder.umd.js built before the fixes>
//
// Writes test/fixtures/compile-snapshot-word-fixes-before.json (a compile-snapshot*.json name, so
// it is never read as a builder-document fixture). Never recapture with a bundle that has the
// fixes: the pin is the "before".
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const { loadUmd, compileInputs, FIXTURES } = require('./_umd.cjs');
const { fixedDocs, cleanDocs } = require('./word-fixes-docs.cjs');

const i = process.argv.indexOf('--umd');
if (i < 0 || !process.argv[i + 1]) {
  console.error('usage: node test/capture-word-fixes-before.cjs --umd <bundle built before the fixes>');
  process.exit(2);
}
const umd = path.resolve(process.argv[i + 1]);
const { dom, EB } = loadUmd(umd);
const { context, refs } = compileInputs();
const outputs = {};
for (const [name, d] of Object.entries(fixedDocs(false))) outputs[`flag-off:${name}`] = EB.compileDocument(d, context, refs);
for (const [name, d] of Object.entries(cleanDocs())) outputs[`clean:${name}`] = EB.compileDocument(d, context, refs);
dom.window.close();
const out = path.join(FIXTURES, 'compile-snapshot-word-fixes-before.json');
fs.writeFileSync(out, `${JSON.stringify({
  _comment: 'integrations RENDER-CATALOG-SPEC §17.5: compiled with the bundle BEFORE the Word fixes F1-F4 (fork eb2bb5b0). Never recapture with a bundle that has them.',
  bundleSha256: crypto.createHash('sha256').update(fs.readFileSync(umd)).digest('hex'),
  context,
  refs: refs.map((r) => ({ id: r.id, name: r.name })),
  outputs,
}, null, 2)}\n`);
console.log(`wrote ${path.relative(path.join(__dirname, '..'), out)}: ${Object.keys(outputs).length} outputs`);
