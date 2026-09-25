// Test runner for the compile-time post-processor (src/postProcess.ts; `outlook.ts`
// until PARAGRAPH-SPACING-SPEC D7).
//
// The suites assert the STRUCTURE of generated output: the unconditional text-margin
// pass, mso/non-mso conditional pairing, dual-emit copies, VML button markup, column
// width math, and the Outlook-mobile style injection. They cannot assert real client
// rendering — any change to postProcess.ts still requires the four-client verification
// loop (Gmail desktop/mobile, Outlook desktop/mobile) documented in
// integrations/listmonk/LISTMONK-RUNBOOK.md before an image pickup.
//
// postProcess.ts is dependency-free, so it is compiled standalone with tsc into
// test/.build and the suites run against that under jsdom's DOMParser.
const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const srcDir = path.join(__dirname, '..', 'src');

// PARAGRAPH-SPACING-SPEC I9: the rename is complete — the old module is gone and nothing
// imports it (a stale import would otherwise fail only in the vite build, not here).
if (fs.existsSync(path.join(srcDir, 'outlook.ts'))) {
  console.error('src/outlook.ts exists — the post-processor is src/postProcess.ts (PARAGRAPH-SPACING-SPEC D7)');
  process.exit(1);
}
const staleImports = [];
(function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(full);
    } else if (/\.(ts|tsx)$/.test(entry.name) && /from\s+['"][./]+(?:[^'"]*\/)?outlook['"]/.test(fs.readFileSync(full, 'utf8'))) {
      staleImports.push(path.relative(srcDir, full));
    }
  }
})(srcDir);
if (staleImports.length) {
  console.error(`import of ./outlook in: ${staleImports.join(', ')}`);
  process.exit(1);
}

const buildDir = path.join(__dirname, '.build');
fs.rmSync(buildDir, { recursive: true, force: true });
fs.mkdirSync(buildDir, { recursive: true });

const tsc = path.join(__dirname, '..', 'node_modules', 'typescript', 'bin', 'tsc');

// Import-free modules compiled standalone: the post-processor, and the official-footer
// resolver, projection and insert transform (OFFICIAL-FOOTER-SPEC §3.1 -- they stay import-free
// exactly so they can be tested here without the bundle).
const STANDALONE = [
  ['postProcess.ts', 'postProcess'],
  [path.join('official', 'resolve.ts'), path.join('official', 'resolve')],
  [path.join('official', 'projection.ts'), path.join('official', 'projection')],
  [path.join('official', 'insert.ts'), path.join('official', 'insert')],
];
for (const [src, out] of STANDALONE) {
  const outDir = path.join(buildDir, path.dirname(out));
  const compile = spawnSync(process.execPath, [
    tsc,
    path.join(srcDir, src),
    '--module', 'commonjs',
    '--target', 'es2020',
    '--lib', 'es2020,dom',
    '--outDir', outDir,
  ], { stdio: 'inherit' });
  if (compile.status !== 0) {
    console.error(`tsc failed: ${src}`);
    process.exit(1);
  }
  // The package is "type":"module", so the CommonJS output must be .cjs.
  fs.renameSync(path.join(buildDir, `${out}.js`), path.join(buildDir, `${out}.cjs`));
}

// OFFICIAL-FOOTER-SPEC §3.1: official-compile.test.cjs loads the BUILT bundle -- the fork's
// first UMD-level suite. Rebuild it (vite, then the Makefile's copy into frontend/public) when
// it is absent or older than any builder source file. Yarn runs with corepack auto-pin OFF.
const umd = path.join(__dirname, '..', '..', 'public', 'static', 'email-builder', 'email-builder.umd.js');
function newestSourceMtime() {
  let newest = 0;
  const visit = (p) => {
    const st = fs.statSync(p);
    if (st.isDirectory()) {
      for (const e of fs.readdirSync(p)) visit(path.join(p, e));
    } else if (st.mtimeMs > newest) {
      newest = st.mtimeMs;
    }
  };
  visit(srcDir);
  for (const f of ['package.json', 'vite.config.ts', 'tsconfig.json']) {
    const p = path.join(__dirname, '..', f);
    if (fs.existsSync(p)) newest = Math.max(newest, fs.statSync(p).mtimeMs);
  }
  return newest;
}
if (!fs.existsSync(umd) || fs.statSync(umd).mtimeMs < newestSourceMtime()) {
  console.log('building the email-builder bundle (absent or stale)...');
  const build = spawnSync('yarn', ['build'], {
    cwd: path.join(__dirname, '..'),
    stdio: 'inherit',
    env: { ...process.env, COREPACK_ENABLE_AUTO_PIN: '0' },
  });
  if (build.status !== 0) {
    console.error('yarn build failed');
    process.exit(1);
  }
  fs.mkdirSync(path.dirname(umd), { recursive: true });
  fs.copyFileSync(path.join(__dirname, '..', 'dist', 'email-builder.umd.js'), umd);
}

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
