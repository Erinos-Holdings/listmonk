const path = require('path');
const fs = require('fs');

// The rebrand sweep's remap function (src/remapColors.ts). These pin the sweep's contract:
// exact same-role hex mapping with a count, the ambiguity skip, the key-constrained traversal
// (content that LOOKS like a hex is never rewritten), and the Keep-then-third-brand provenance
// case — a document kept on its original palette must still remap fully from that original
// palette later, which is why held provenance pins to the palette actually in the document.
const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));

function transpile(relSourcePath) {
  const src = fs.readFileSync(path.join(builderRoot, 'src', relSourcePath), 'utf8');
  return ts.transpileModule(src, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      esModuleInterop: true,
    },
  }).outputText;
}

function evaluate(js) {
  const mod = { exports: {} };
  new Function('module', 'exports', 'require', js)(mod, mod.exports, () => {
    throw new Error('remapColors must stay dependency-free');
  });
  return mod.exports;
}

const { remapColors } = evaluate(transpile('remapColors.ts'));

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

const LIYORA = {
  label: 'liyora',
  colors: [
    { role: 'bg', value: '#43273B' },
    { role: 'fg', value: '#F2EFEA' },
    { role: 'accent', value: '#C8D6EB' },
  ],
};
const THIRSTY = {
  label: 'thirstygirl',
  colors: [
    { role: 'bg', value: '#0A3450' },
    { role: 'fg', value: '#FFFFFF' },
    { role: 'accent', value: '#37B5FF' },
  ],
};
const THIRD = {
  label: 'third',
  colors: [
    { role: 'bg', value: '#111111' },
    { role: 'fg', value: '#EEEEEE' },
    { role: 'accent', value: '#FF00AA' },
  ],
};

// A realistic editor document: flat block map keyed by id, columns nesting via props arrays,
// role hexes in color-bearing keys (mixed case to pin case-insensitive matching), an ad-hoc
// color, and a text + markdown block whose CONTENT carries a bare role hex.
const DOC = {
  root: {
    type: 'EmailLayout',
    data: {
      backdropColor: '#43273b', // liyora bg, lowercase in the doc
      canvasColor: '#F2EFEA', // liyora fg
      textColor: '#333333', // ad-hoc: not a role hex
      childrenIds: ['block-text', 'block-cols', 'block-md'],
    },
  },
  'block-text': {
    type: 'Text',
    data: {
      style: { color: '#C8D6EB', backgroundColor: '#DDEEFF' }, // accent + ad-hoc
      props: { text: 'Our brand background is #43273B — do not change copy.' },
    },
  },
  'block-cols': {
    type: 'ColumnsContainer',
    data: {
      style: { backgroundColor: '#43273B' }, // liyora bg, uppercase
      props: {
        columns: [
          { childrenIds: ['block-btn'] },
          { childrenIds: [] },
        ],
      },
    },
  },
  'block-btn': {
    type: 'Button',
    data: {
      props: { buttonBackgroundColor: '#C8D6EB', buttonTextColor: '#43273B', borderColor: '#C8D6EB', text: 'Shop' },
    },
  },
  'block-md': {
    type: 'Markdown',
    data: {
      props: { markdown: 'Palette: `#F2EFEA` on `#43273B`.' },
    },
  },
};

// --- role mapping with count -----------------------------------------------------------------
{
  const { document: out, replaced } = remapColors(DOC, LIYORA, THIRSTY);
  // 7 rewrites: backdropColor, canvasColor, text style.color, cols backgroundColor,
  // buttonBackgroundColor, buttonTextColor, borderColor.
  check('replaced counts every rewrite', replaced === 7, `replaced=${replaced}`);
  check('bg role maps (lowercase doc hex)', out.root.data.backdropColor === '#0A3450');
  check('fg role maps', out.root.data.canvasColor === '#FFFFFF');
  check('accent role maps in nested style', out['block-text'].data.style.color === '#37B5FF');
  check('bg role maps under columns', out['block-cols'].data.style.backgroundColor === '#0A3450');
  check('button color props map', out['block-btn'].data.props.buttonBackgroundColor === '#37B5FF'
    && out['block-btn'].data.props.buttonTextColor === '#0A3450'
    && out['block-btn'].data.props.borderColor === '#37B5FF');
}

// --- non-palette colors untouched ------------------------------------------------------------
{
  const { document: out } = remapColors(DOC, LIYORA, THIRSTY);
  check('ad-hoc textColor untouched', out.root.data.textColor === '#333333');
  check('ad-hoc backgroundColor untouched', out['block-text'].data.style.backgroundColor === '#DDEEFF');
}

// --- key-constrained traversal: content hexes never rewritten --------------------------------
{
  const { document: out } = remapColors(DOC, LIYORA, THIRSTY);
  check('text content with bare role hex untouched',
    out['block-text'].data.props.text === 'Our brand background is #43273B — do not change copy.');
  check('markdown content with bare role hexes untouched',
    out['block-md'].data.props.markdown === 'Palette: `#F2EFEA` on `#43273B`.');
}

// --- zero-match -------------------------------------------------------------------------------
{
  const { replaced } = remapColors(DOC, THIRD, THIRSTY); // doc contains no THIRD hexes
  check('no old-palette hexes -> replaced: 0', replaced === 0, `replaced=${replaced}`);
}

// --- ambiguity skip ---------------------------------------------------------------------------
{
  const ambiguous = {
    label: 'amb',
    colors: [
      { role: 'bg', value: '#43273B' },
      { role: 'fg', value: '#43273b' }, // same hex, different case, different role
      { role: 'accent', value: '#C8D6EB' },
    ],
  };
  const { document: out, replaced } = remapColors(DOC, ambiguous, THIRSTY);
  check('duplicate old hex skipped entirely', out.root.data.backdropColor === '#43273b'
    && out['block-cols'].data.style.backgroundColor === '#43273B');
  check('unambiguous role still maps alongside skip', out['block-text'].data.style.color === '#37B5FF');
  check('replaced counts only real rewrites', replaced === 3, `replaced=${replaced}`);
}

// --- role missing from new palette -----------------------------------------------------------
{
  const twoRole = { label: 'two', colors: [{ role: 'bg', value: '#0A3450' }, { role: 'fg', value: '#FFFFFF' }] };
  const { document: out } = remapColors(DOC, LIYORA, twoRole);
  check('old hex with no same-role target left alone', out['block-text'].data.style.color === '#C8D6EB');
  check('roles with targets still map', out.root.data.backdropColor === '#0A3450');
}

// --- purity -----------------------------------------------------------------------------------
{
  const before = JSON.stringify(DOC);
  remapColors(DOC, LIYORA, THIRSTY);
  check('input document not mutated', JSON.stringify(DOC) === before);
}

// --- Keep-then-third-brand provenance --------------------------------------------------------
// A campaign kept on liyora colors after a swap to thirstygirl must, on a later swap to a
// third brand, map from LIYORA (the palette actually in the document) — mapping from the
// kept-over brand would be a silent no-op. This is why held provenance pins on Keep.
{
  const keptNoOp = remapColors(DOC, THIRSTY, THIRD); // wrong source: kept-over brand
  check('mapping from kept-over palette is a no-op', keptNoOp.replaced === 0, `replaced=${keptNoOp.replaced}`);

  const fromOriginal = remapColors(DOC, LIYORA, THIRD); // right source: pinned provenance
  check('mapping from pinned original palette fully maps', fromOriginal.replaced === 7
    && fromOriginal.document.root.data.backdropColor === '#111111'
    && fromOriginal.document['block-text'].data.style.color === '#FF00AA', `replaced=${fromOriginal.replaced}`);
}

console.log(failed === 0 ? 'ALL PASS' : `${failed} FAILED`);
process.exit(failed === 0 ? 0 : 1);
