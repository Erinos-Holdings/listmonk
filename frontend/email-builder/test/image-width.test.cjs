const path = require('path');
const fs = require('fs');

// IMAGE-WIDTH-SPEC Part A, the pure module behind the sidebar's Width auto-fill
// (src/documents/blocks/Img/imageWidth.ts). run.cjs compiles only postProcess.ts, so this
// suite transpiles and evaluates the module directly (the alt-text-nudge pattern).
//   I4  measureImageSize resolves {naturalWidth, naturalHeight} on load, null on error / zero natural
//       width, and a superseded measurer call resolves null (its result is ignored);
//   I5  decideWidth: empty → min(measured, cap); authored → kept; equal to the previous
//       auto-fill → re-filled; the cap is CANVAS_WIDTH − padding.left − padding.right;
//   I6  decideWidthForHeight (2026-10-11): a Height typed on an auto-filled Width re-derives
//       the Width proportionally from the natural size, capped; an authored Width is kept.
const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));

function transpile(relSourcePath) {
  const src = fs.readFileSync(path.join(builderRoot, 'src', relSourcePath), 'utf8');
  return ts.transpileModule(src, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, esModuleInterop: true, jsx: ts.JsxEmit.React },
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

const { measureImageSize, createImageSizeMeasurer, decideWidth, decideWidthForHeight, autoFillCap } = evaluate(
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
      this.naturalHeight = f.height === undefined ? Math.round(f.width / 3) : f.height;
      this.onload && this.onload();
    }, f.delay);
  }
};
fixtures['x://1200.png'] = { width: 1200, delay: 0 };
fixtures['x://300.png'] = { width: 300, delay: 0 };
fixtures['x://slow-800.png'] = { width: 800, delay: 30 };
fixtures['x://nosize.svg'] = { width: 0, delay: 0 };
fixtures['x://noheight.svg'] = { width: 300, height: 0, delay: 0 };

(async () => {
  // I4
  check('load → {naturalWidth, naturalHeight}', JSON.stringify(await measureImageSize('x://1200.png')) === '{"width":1200,"height":400}');
  check('error → null', (await measureImageSize('x://missing.png')) === null);
  check('zero natural width (SVG without intrinsic size) → null', (await measureImageSize('x://nosize.svg')) === null);
  check('zero natural height → null', (await measureImageSize('x://noheight.svg')) === null);
  check('empty URL → null', (await measureImageSize('')) === null && (await measureImageSize(null)) === null);

  const measure = createImageSizeMeasurer();
  const first = measure('x://slow-800.png'); // resolves after the second
  const second = measure('x://300.png');
  const [r1, r2] = await Promise.all([first, second]);
  check('superseded call resolves null (result ignored)', r1 === null, String(r1));
  check('latest call resolves its own size', r2 && r2.width === 300, JSON.stringify(r2));
  check('a later call after both settled measures normally', (await measure('x://1200.png')).width === 1200);

  // I5 — cap
  check('cap = CANVAS_WIDTH − left − right (24/24 → 552)', autoFillCap({ top: 16, bottom: 16, left: 24, right: 24 }) === 552);
  check('cap with no padding object = CANVAS_WIDTH', autoFillCap(null) === 600 && autoFillCap(undefined) === 600);
  check('cap with asymmetric padding', autoFillCap({ left: 10, right: 40 }) === 550);
  // The Add-menu's fresh Image block (zero padding, 2026-09-04) must fill to the FULL canvas —
  // ties the constructor default to the cap formula, so a change to either alone fails here.
  {
    const { createRequire } = require('module');
    const req = createRequire(path.join(builderRoot, 'package.json'));
    const iconStub = new Proxy({}, { get: () => () => null });
    const { BUTTONS } = evaluate(
      transpile(path.join('documents', 'blocks', 'helpers', 'EditorChildrenIds', 'AddBlockMenu', 'buttons.tsx')),
      { react: req('react'), '@mui/icons-material': iconStub, '../../../../editor/core': {} },
    );
    const fresh = BUTTONS.find((b) => b.label === 'Image').block();
    check('fresh Image block auto-fills to the full canvas (cap 600)', autoFillCap(fresh.data.style.padding) === 600, JSON.stringify(fresh.data.style.padding));
  }

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

  // I6
  const nat = { width: 1200, height: 400 };
  check('auto-filled 600, height 100 → 300 (proportional)', decideWidthForHeight(600, 600, nat, 100, 600) === 300);
  check('auto-filled 552, height 150 → 450 under cap 552', decideWidthForHeight(552, 552, nat, 150, 552) === 450);
  check('scaled width over the cap → the cap (no Word overflow)', decideWidthForHeight(600, 600, nat, 300, 600) === 600);
  check('authored width kept (no write)', decideWidthForHeight(400, 600, nat, 100, 600) === null);
  check('no previous auto-fill → no write', decideWidthForHeight(600, null, nat, 100, 600) === null);
  check('no natural size → no write', decideWidthForHeight(600, 600, null, 100, 600) === null);
  check('height cleared → no write', decideWidthForHeight(600, 600, nat, null, 600) === null);
  check('rounds (1200×400, height 33 → 99)', decideWidthForHeight(600, 600, nat, 33, 600) === 99);
  check('sub-pixel result → no write', decideWidthForHeight(600, 600, { width: 1, height: 1000 }, 1, 600) === null);

  console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
  process.exit(failed ? 1 : 0);
})();
