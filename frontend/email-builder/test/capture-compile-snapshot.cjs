// CONTAINER-NESTING-SPEC I8 -- capture the compiled-output snapshot that
// compile-unchanged.test.cjs compares against.
//
// THIS IS A STANDING TRIPWIRE. test/fixtures/compile-snapshot.json records what the builder
// compiled at `baseCommit`; compile-unchanged.test.cjs fails whenever the current bundle
// compiles any builder-document fixture differently. A change that is MEANT to change compiled
// output regenerates the snapshot deliberately with this same script, pointed at the commit
// whose output is the new truth:
//
//   node test/capture-compile-snapshot.cjs [<base commit>]      (default: 8b140ac4)
//   node test/capture-compile-snapshot.cjs <commit> --only <fixture.json,...> --out <file.json>
//
// --only/--out (BIBLE-OUTLOOK-FIXES-SPEC I9) capture a subset into another file: the flag-off
// fixture is pinned at b5ceb3ce, the commit before the Word fixes, in
// test/fixtures/compile-snapshot-flag-off.json, which a later recapture of the main snapshot
// never touches.
//
// The bundle is gitignored and run.cjs rebuilds it from the working tree, so a snapshot taken
// from the working tree would prove nothing. Instead this script checks the base commit out
// into a temporary `git worktree` OUTSIDE the repo, links the main clone's node_modules into it
// (the same installed, patch-package-patched dependencies the working-tree build uses), runs
// the same `yarn build`, compiles every builder-document fixture of the CURRENT test/fixtures
// with that bundle, writes the snapshot, and removes the worktree.
const { spawnSync } = require('child_process');
const crypto = require('crypto');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { SNAPSHOT, loadUmd, documentFixtures, compileInputs } = require('./_umd.cjs');

const builderDir = path.join(__dirname, '..');
const base = (process.argv[2] && !process.argv[2].startsWith('--')) ? process.argv[2] : '8b140ac4';
function opt(name) {
  const i = process.argv.indexOf(name);
  return i >= 0 ? process.argv[i + 1] : null;
}
const only = opt('--only') ? opt('--only').split(',').map((f) => f.trim()).filter(Boolean) : null;
const outFile = opt('--out') ? path.resolve(opt('--out')) : SNAPSHOT;

function git(args, opts = {}) {
  const r = spawnSync('git', args, { cwd: builderDir, encoding: 'utf8', ...opts });
  if (r.status !== 0) throw new Error(`git ${args.join(' ')} failed: ${r.stderr}`);
  return r.stdout.trim();
}

const baseCommit = git(['rev-parse', '--verify', `${base}^{commit}`]);
const repoRoot = git(['rev-parse', '--show-toplevel']);
const builderRel = path.relative(repoRoot, builderDir);
const tmpRoot = fs.mkdtempSync(path.join(process.env.SNAPSHOT_TMPDIR || os.tmpdir(), 'eb-compile-snapshot-'));
const worktree = path.join(tmpRoot, 'wt');

let exitCode = 1;
try {
  git(['worktree', 'add', '--detach', worktree, baseCommit]);
  const wtBuilder = path.join(worktree, builderRel);
  // patch-package patches live IN node_modules; linking the main clone's tree gives the base
  // build exactly the patched dependencies the working-tree build uses. Refuse if the patches
  // directory differs between the base commit and now (the link would then be wrong).
  const patchDiff = spawnSync('git', ['diff', '--quiet', baseCommit, '--', path.join(builderRel, 'patches'), path.join(builderRel, 'package.json'), path.join(builderRel, 'yarn.lock')], { cwd: repoRoot });
  if (patchDiff.status !== 0) {
    throw new Error(`patches/, package.json or yarn.lock changed since ${baseCommit}: the linked node_modules would not match the base; install in the worktree instead`);
  }
  fs.symlinkSync(path.join(builderDir, 'node_modules'), path.join(wtBuilder, 'node_modules'), 'dir');

  console.log(`building the bundle at ${baseCommit.slice(0, 10)} in ${worktree} ...`);
  const build = spawnSync('yarn', ['build'], {
    cwd: wtBuilder,
    stdio: 'inherit',
    env: { ...process.env, COREPACK_ENABLE_AUTO_PIN: '0' },
  });
  if (build.status !== 0) throw new Error('yarn build failed in the worktree');

  const umdPath = path.join(wtBuilder, 'dist', 'email-builder.umd.js');
  const umd = fs.readFileSync(umdPath);
  const bundleSha256 = crypto.createHash('sha256').update(umd).digest('hex');

  const { dom, EB } = loadUmd(umdPath);
  const { context, refs } = compileInputs();
  const outputs = {};
  const fixtures = documentFixtures().filter(({ file }) => !only || only.includes(file));
  if (only && fixtures.length !== only.length) throw new Error(`--only names a fixture that does not exist: ${only.join(',')}`);
  for (const { file, document } of fixtures) {
    outputs[file] = EB.compileDocument(document, context, refs);
  }
  dom.window.close();

  const snapshot = {
    _comment: only
      ? `BIBLE-OUTLOOK-FIXES-SPEC I9 pin: compiled HTML of ${only.join(', ')} at baseCommit. Never recapture from a later commit: it pins the flag-off output from before the Word fixes.`
      : 'CONTAINER-NESTING-SPEC I8 standing tripwire: compiled HTML of every builder-document fixture at baseCommit. Regenerate ONLY for an intentional compile change, with: node test/capture-compile-snapshot.cjs <commit>',
    baseCommit,
    bundleSha256,
    context,
    refs: refs.map((r) => ({ id: r.id, name: r.name })),
    outputs,
  };
  fs.writeFileSync(outFile, JSON.stringify(snapshot, null, 2) + '\n');
  console.log(`wrote ${path.relative(builderDir, outFile)}: ${Object.keys(outputs).length} fixtures, bundle sha256 ${bundleSha256}`);
  exitCode = 0;
} catch (e) {
  console.error(`capture failed: ${e.message}`);
} finally {
  // Unlink the node_modules link first so nothing below can ever reach the main clone's tree.
  try { fs.unlinkSync(path.join(worktree, builderRel, 'node_modules')); } catch (e) { /* absent */ }
  spawnSync('git', ['worktree', 'remove', '--force', worktree], { cwd: builderDir, stdio: 'inherit' });
  spawnSync('git', ['worktree', 'prune'], { cwd: builderDir });
  fs.rmSync(tmpRoot, { recursive: true, force: true });
}
process.exit(exitCode);
