// Test runner for the Outlook post-processor (src/outlook.ts).
//
// The suites assert the STRUCTURE of generated output: mso/non-mso conditional
// pairing, dual-emit copies, VML button markup, column width math, and the
// Outlook-mobile style injection. They cannot assert real client rendering —
// any change to outlook.ts still requires the four-client verification loop
// (Gmail desktop/mobile, Outlook desktop/mobile) documented in
// integrations/listmonk/LISTMONK-RUNBOOK.md before an image pickup.
//
// outlook.ts is dependency-free, so it is compiled standalone with tsc into
// test/.build and the suites run against that under jsdom's DOMParser.
const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const buildDir = path.join(__dirname, '.build');
fs.rmSync(buildDir, { recursive: true, force: true });
fs.mkdirSync(buildDir, { recursive: true });

const tsc = path.join(__dirname, '..', 'node_modules', 'typescript', 'bin', 'tsc');
const compile = spawnSync(process.execPath, [
  tsc,
  path.join(__dirname, '..', 'src', 'outlook.ts'),
  '--module', 'commonjs',
  '--target', 'es2020',
  '--lib', 'es2020,dom',
  '--outDir', buildDir,
], { stdio: 'inherit' });
if (compile.status !== 0) {
  console.error('tsc failed');
  process.exit(1);
}
// The package is "type":"module", so the CommonJS output must be .cjs.
fs.renameSync(path.join(buildDir, 'outlook.js'), path.join(buildDir, 'outlook.cjs'));

const suites = fs.readdirSync(__dirname).filter((f) => f.endsWith('.test.cjs')).sort();
let failed = 0;
for (const suite of suites) {
  const result = spawnSync(process.execPath, [path.join(__dirname, suite)], { stdio: 'inherit' });
  const ok = result.status === 0;
  if (!ok) failed++;
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${suite}\n`);
}

console.log(failed === 0 ? `ALL ${suites.length} SUITES PASS` : `${failed}/${suites.length} SUITES FAILED`);
process.exit(failed === 0 ? 0 : 1);
