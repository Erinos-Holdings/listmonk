const path = require('path');
const fs = require('fs');

// IMAGE-WIDTH-SPEC Part A, the pure module behind the sidebar's Width auto-fill
// (src/documents/blocks/Img/imageWidth.ts). run.cjs compiles only outlook.ts, so this
// suite transpiles and evaluates the module directly (the alt-text-nudge pattern).
//   I4  measureImageWidth resolves naturalWidth on load, null on error / zero natural
//       width, and a superseded measurer call resolves null (its result is ignored);
//   I5  decideWidth: empty → min(measured, cap); authored → kept; equal to the previous
//       auto-fill → re-filled; the cap is CANVAS_WIDTH − padding.left − padding.right.
const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));

function transpile(relSourcePath) {
  const src = fs.readFileSync(path.join(builderRoot, 'src', relSourcePath), 'utf8');
  return ts.transpileModule(src, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, esModuleInterop: true },
  }).outputText;
}
function evaluate(js, requireMap = {}) {
  const mod = { exports: {} };
  const req = (name) => {
    if (name in requireMap) return requireMap[name];
    throw new Error(`unexpected import "${name}" — add it to the test's requireMap`);
  };
  new Function('module', 'exports', 'require', js)(mod, mod.exports, req);
  return mod.exports;
}

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

const canvas = evaluate(transpile(path.join('documents', 'canvasWidth.ts')));
check('CANVAS_WIDTH is exported and is 600', canvas.CANVAS_WIDTH === 600, String(canvas.CANVAS_WIDTH));

const { measureImageWidth, createImageWidthMeasurer, decideWidth, autoFillCap } = evaluate(
  transpile(path.join('documents', 'blocks', 'Img', 'imageWidth.ts')),
  { '../../canvasWidth': canvas },
);

// Stub Image: `fixtures[url]` = natural width (0 = loads with no intrinsic size),
// 'error' = fires onerror, and an optional delay so one load can outlive a later one.
const fixtures = {};
global.Image = class {
  set src(url) {
    const f = fixtures[url] || { width: 'error', delay: 0 };
    setTimeout(() => {
      if (f.width === 'error') { this.onerror && this.onerror(new Error('404')); return; }
      this.naturalWidth = f.width;
      this.onload && this.onload();
    }, f.delay);
  }
};
fixtures['x://1200.png'] = { width: 1200, delay: 0 };
fixtures['x://300.png'] = { width: 300, delay: 0 };
fixtures['x://slow-800.png'] = { width: 800, delay: 30 };
fixtures['x://nosize.svg'] = { width: 0, delay: 0 };

(async () => {
  // I4
  check('load → naturalWidth', (await measureImageWidth('x://1200.png')) === 1200);
  check('error → null', (await measureImageWidth('x://missing.png')) === null);
  check('zero natural width (SVG without intrinsic size) → null', (await measureImageWidth('x://nosize.svg')) === null);
  check('empty URL → null', (await measureImageWidth('')) === null && (await measureImageWidth(null)) === null);

  const measure = createImageWidthMeasurer();
  const first = measure('x://slow-800.png'); // resolves after the second
  const second = measure('x://300.png');
  const [r1, r2] = await Promise.all([first, second]);
  check('superseded call resolves null (result ignored)', r1 === null, String(r1));
  check('latest call resolves its own width', r2 === 300, String(r2));
  check('a later call after both settled measures normally', (await measure('x://1200.png')) === 1200);

  // I5 — cap
  check('cap = CANVAS_WIDTH − left − right (24/24 → 552)', autoFillCap({ top: 16, bottom: 16, left: 24, right: 24 }) === 552);
  check('cap with no padding object = CANVAS_WIDTH', autoFillCap(null) === 600 && autoFillCap(undefined) === 600);
  check('cap with asymmetric padding', autoFillCap({ left: 10, right: 40 }) === 550);

  // I5 — rule
  check('empty width, 1200 measured, cap 552 → 552', decideWidth(null, null, 1200, 552) === 552);
  check('undefined width behaves as empty', decideWidth(undefined, null, 1200, 552) === 552);
  check('empty width, 300 measured, cap 552 → 300 (never upscaled)', decideWidth(null, null, 300, 552) === 300);
  check('authored width kept (no write)', decideWidth(400, null, 1200, 552) === null);
  check('authored width kept even with a previous auto-fill of a different value', decideWidth(400, 552, 1200, 552) === null);
  check('width equal to the previous auto-fill is re-filled', decideWidth(552, 552, 300, 552) === 300);
  check('measured null (failure/superseded) → no write, even when empty', decideWidth(null, null, null, 552) === null);
  check('measured 0 → no write', decideWidth(null, null, 0, 552) === null);
  check('autoPrev null never matches a stored width', decideWidth(0, null, 300, 552) === null);

  console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
  process.exit(failed ? 1 : 0);
})();
