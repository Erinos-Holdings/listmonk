const path = require('path');
const fs = require('fs');
const { createRequire } = require('module');

// The Image block's alt-text nudge (ImageSidebarPanel) fires on isAltMissing():
// alt NEVER SET (absent/null) nudges; alt="" is the explicit decorative form and
// must stay silent. That distinction only means anything if a fresh Add-menu
// Image block actually starts with no alt key — the old 'Sample product'
// placeholder both suppressed the nudge and shipped junk alt text into real
// mail. Both halves are pinned here.
const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));
const builderRequire = createRequire(path.join(builderRoot, 'package.json'));

function transpile(relSourcePath) {
  const src = fs.readFileSync(path.join(builderRoot, 'src', relSourcePath), 'utf8');
  return ts.transpileModule(src, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      esModuleInterop: true,
      jsx: ts.JsxEmit.React,
    },
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

// Half 1: the predicate distinguishes never-set from explicitly empty.
const { isAltMissing } = evaluate(transpile(path.join('documents', 'blocks', 'Img', 'altText.ts')));

check('absent alt (no key) is missing', isAltMissing({}) === true);
check('null alt is missing', isAltMissing({ alt: null }) === true);
check('absent props object is missing', isAltMissing(undefined) === true);
check('explicit alt="" (decorative) is NOT missing', isAltMissing({ alt: '' }) === false);
check('set alt is NOT missing', isAltMissing({ alt: 'logo' }) === false);

// Half 2: a fresh Add-menu Image block starts never-set. Icons are stubbed —
// only the block() constructors matter here.
const iconStub = new Proxy({}, { get: () => () => null });
const { BUTTONS } = evaluate(transpile(path.join(
  'documents', 'blocks', 'helpers', 'EditorChildrenIds', 'AddBlockMenu', 'buttons.tsx',
)), {
  react: builderRequire('react'),
  '@mui/icons-material': iconStub,
  '../../../../editor/core': {},
});

const imageButton = BUTTONS.find((b) => b.label === 'Image');
check('Add-menu has an Image block', Boolean(imageButton));
const fresh = imageButton ? imageButton.block() : null;
check('fresh Image block carries NO alt key (never-set, no placeholder alt)',
  fresh && !('alt' in fresh.data.props),
  fresh && JSON.stringify(fresh.data.props));
check('fresh Image block triggers the nudge',
  fresh && isAltMissing(fresh.data.props) === true);
// IMAGE-WIDTH-SPEC I6: no literal default width — the sidebar auto-fills Width from the
// image's natural size on URL set; a literal would stretch anything narrower than the
// canvas in every client.
check('fresh Image block carries NO width key (auto-fill, not a literal default)',
  fresh && !('width' in fresh.data.props),
  fresh && JSON.stringify(fresh.data.props));

// Fresh Image blocks are full-bleed by default (2026-09-04): zero padding on all four sides,
// so stacked marketing images show no seams and the auto-fill cap (canvas − horizontal
// padding) is the full 600. Only the Image constructor — Text/Heading/Button keep 16/24.
const pad = fresh && fresh.data.style && fresh.data.style.padding;
check('fresh Image block has zero padding on all four sides',
  pad && pad.top === 0 && pad.bottom === 0 && pad.left === 0 && pad.right === 0,
  JSON.stringify(pad));
const textButton = BUTTONS.find((b) => b.label === 'Text');
const textPad = textButton && textButton.block().data.style.padding;
check('Text block keeps its 16/24 padding (the change is Image-only)',
  textPad && textPad.left === 24 && textPad.top === 16, JSON.stringify(textPad));

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
