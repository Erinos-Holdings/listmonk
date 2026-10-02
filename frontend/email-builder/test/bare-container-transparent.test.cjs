// integrations BIBLE-OUTLOOK-FIXES-SPEC §12 (Amendment A) IA18 — the compile half of SA3: a BARE
// Container (the editor's unstyled Container, structure.ts::isUnstyledContainer — style null,
// absent, or zero padding and nothing else) compiles to ONE wrapper `div` around its children and
// changes nothing else in the output. The review's descriptor version 2 reads a bare Container as
// transparent on the strength of this (and of the renders, IA16 — client behaviour, untestable
// here).
//
// Method: compile a document holding a bare Container and the same document with that Container
// unwrapped (its children spliced into its slot), Outlook flag on and off, with the BUILT bundle.
// Both outputs are parsed and serialized by the same jsdom; the wrapped one must equal the
// unwrapped one once exactly one element — a `div` with no attribute, or only
// `style="padding:0px 0px 0px 0px"` and/or a zero radius — is replaced by its children.
const { JSDOM } = require('jsdom');
const { loadUmd, compileInputs } = require('./_umd.cjs');

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail ? '  [' + String(detail).slice(0, 400) + ']' : ''}`); }

const { EB } = loadUmd();
const { context, refs } = compileInputs();

const P = (t, r, b, l) => ({ top: t, right: r, bottom: b, left: l });
const URL_ = 'https://canary.invalid/bible';
const IMG = 'https://email.curatedfor.you/uploads/Asset-2RUZE.png';
const BLOCKS = {
  text: { type: 'Text', data: { style: { padding: P(8, 24, 8, 24) }, props: { markdown: true, text: 'Bare wrapper text' } } },
  heading: { type: 'Heading', data: { style: { padding: P(8, 24, 8, 24) }, props: { level: 'h2', text: 'Bare wrapper heading' } } },
  heading2: { type: 'Heading', data: { style: { textAlign: 'center', padding: P(4, 24, 4, 24) }, props: { level: 'h3', text: 'Second heading' } } },
  button: { type: 'Button', data: { style: { textAlign: 'center', padding: P(8, 24, 8, 24) }, props: { text: 'Inline button', url: URL_, size: 'medium', fullWidth: false, buttonStyle: 'rounded', buttonBackgroundColor: '#003B4D', buttonTextColor: '#FFFFFF' } } },
  buttonFull: { type: 'Button', data: { style: { textAlign: 'center', padding: P(8, 24, 8, 24) }, props: { text: 'Full-width button', url: URL_, size: 'medium', fullWidth: true, buttonStyle: 'rectangle', buttonBackgroundColor: '#2563EB', buttonTextColor: '#FFFFFF' } } },
  buttonFallback: { type: 'Button', data: { style: { textAlign: 'center', padding: P(8, 24, 8, 24) }, props: { text: 'Fallback →', url: URL_, size: 'medium', fullWidth: false, buttonStyle: 'pill', buttonBackgroundColor: '#1F2937', buttonTextColor: '#FFFFFF' } } },
  imageRight: { type: 'Image', data: { style: { textAlign: 'right', padding: P(8, 24, 8, 24) }, props: { url: IMG, alt: 'Right', contentAlignment: 'middle', width: 200 } } },
  imageWide: { type: 'Image', data: { style: { textAlign: 'center', padding: P(8, 24, 8, 24) }, props: { url: IMG, alt: 'Wide', contentAlignment: 'middle', width: 700 } } },
  imageUnsized: { type: 'Image', data: { style: { padding: P(8, 24, 8, 24) }, props: { url: IMG, alt: 'Unsized', contentAlignment: 'middle' } } },
  html: { type: 'Html', data: { style: { padding: P(8, 24, 8, 24) }, props: { contents: '<table role="presentation" style="margin:0 auto"><tr><td style="color:#262626">Html cell</td></tr></table>' } } },
  divider: { type: 'Divider', data: { style: { padding: P(16, 24, 16, 24) }, props: { lineColor: '#CCCCCC', lineHeight: 1 } } },
  spacer: { type: 'Spacer', data: { props: { height: 24 } } },
  avatar: { type: 'Avatar', data: { style: { textAlign: 'center', padding: P(8, 24, 8, 24) }, props: { imageUrl: 'https://email.curatedfor.you/uploads/social-curated-instagram.png', shape: 'circle', size: 64, alt: 'Avatar' } } },
};
const SINGLE_KINDS = ['text', 'heading', 'button', 'buttonFull', 'buttonFallback', 'imageRight', 'imageWide', 'imageUnsized', 'html', 'divider', 'spacer', 'avatar'];
// Every bare style SA1 admits (implementation review F5): null, absent, empty, zero padding, a
// radius of the number 0, null padding, all keys falsy with zero padding, all keys null.
const WRAPPER_STYLES = {
  null: null,
  absent: undefined,
  empty: {},
  'zero padding': { padding: P(0, 0, 0, 0) },
  'radius 0': { borderRadius: 0 },
  'null padding': { padding: null },
  'all falsy, zero padding': { backgroundColor: '', borderColor: '', borderRadius: 0, padding: P(0, 0, 0, 0) },
  'all null': { backgroundColor: null, borderColor: null, borderRadius: null, padding: null },
};

const bare = (childrenIds, style) => (style === undefined
  ? { type: 'Container', data: { props: { childrenIds } } }
  : { type: 'Container', data: { style, props: { childrenIds } } });
const styled = (childrenIds) => ({ type: 'Container', data: { style: { padding: P(16, 16, 16, 16), backgroundColor: '#F0F0F0' }, props: { childrenIds } } });
const cols = (a, b) => ({ type: 'ColumnsContainer', data: { style: { padding: P(8, 24, 8, 24) }, props: { columnsCount: 2, columnsGap: 16, contentAlignment: 'middle', columns: [{ childrenIds: a }, { childrenIds: b }, { childrenIds: [] }] } } });
const layout = (ids, outlook) => ({ type: 'EmailLayout', data: { backdropColor: '#F5F5F5', canvasColor: '#FFFFFF', textColor: '#262626', fontFamily: 'MODERN_SANS', outlook, childrenIds: ids } });

/** Splice block `w`'s children into every slot that lists `w`, and drop `w`. */
function unwrap(doc, w) {
  const out = JSON.parse(JSON.stringify(doc));
  const kids = out[w].data.props.childrenIds;
  const splice = (ids) => ids.flatMap((k) => (k === w ? kids : [k]));
  for (const b of Object.values(out)) {
    if (b.data && Array.isArray(b.data.childrenIds)) b.data.childrenIds = splice(b.data.childrenIds);
    if (b.data && b.data.props && Array.isArray(b.data.props.childrenIds)) b.data.props.childrenIds = splice(b.data.props.childrenIds);
    if (b.data && b.data.props && Array.isArray(b.data.props.columns)) for (const c of b.data.props.columns) c.childrenIds = splice(c.childrenIds);
  }
  delete out[w];
  return out;
}

const serialize = (html) => new JSDOM(html).window.document.documentElement.outerHTML;
// The wrapper a bare Container compiles to: no attribute, or a style of zero padding and/or a zero
// radius only (in either order).
const WRAPPER_STYLE = /^(?:(?:padding:0px 0px 0px 0px|border-radius:0(?:px)?);?)+$/;
const isWrapperDiv = (el) => el.tagName === 'DIV' && (el.attributes.length === 0
  || (el.attributes.length === 1 && WRAPPER_STYLE.test(el.getAttribute('style') || '')));

/** True when removing exactly one bare wrapper div from `wrapped` (keeping its children) gives `plain`. */
function equalApartFromOneWrapper(wrapped, plain) {
  const target = serialize(plain);
  const dom = new JSDOM(wrapped);
  const candidates = Array.from(dom.window.document.querySelectorAll('div')).filter(isWrapperDiv);
  for (let i = 0; i < candidates.length; i++) {
    const copy = new JSDOM(wrapped);
    const el = Array.from(copy.window.document.querySelectorAll('div')).filter(isWrapperDiv)[i];
    el.replaceWith(...Array.from(el.childNodes));
    if (copy.window.document.documentElement.outerHTML === target) return true;
  }
  return false;
}

// Each case: a document holding one bare Container `w` (the wrapper under test).
function cases(style) {
  const out = {};
  const root = (ids, extra) => ({ root: layout(ids, true), ...extra });
  for (const k of SINGLE_KINDS) {
    out[`${k} at top level`] = root(['lead', 'w', 'tail'], { lead: BLOCKS.text, tail: BLOCKS.divider, w: bare(['x'], style), x: BLOCKS[k] });
    out[`${k} in a column`] = root(['c'], { c: cols(['w'], ['y']), w: bare(['x'], style), x: BLOCKS[k], y: BLOCKS.text });
    out[`${k} inside a styled Container, alone`] = root(['s'], { s: styled(['w']), w: bare(['x'], style), x: BLOCKS[k] });
    out[`${k} inside a styled Container, with a sibling`] = root(['s'], { s: styled(['y', 'w']), w: bare(['x'], style), x: BLOCKS[k], y: BLOCKS.text });
  }
  out['a group'] = root(['w'], { w: bare(['a', 'b', 'c2'], style), a: BLOCKS.heading, b: BLOCKS.text, c2: BLOCKS.button });
  out['a column row inside a bare Container'] = root(['w'], { w: bare(['c'], style), c: cols(['a'], ['b']), a: BLOCKS.imageUnsized, b: BLOCKS.text });
  out['Headings only'] = root(['w'], { w: bare(['a', 'b'], style), a: BLOCKS.heading, b: BLOCKS.heading2 });
  out['an empty one'] = root(['lead', 'w', 'tail'], { lead: BLOCKS.text, tail: BLOCKS.text, w: bare([], style) });
  return out;
}

let n = 0;
for (const [styleName, style] of Object.entries(WRAPPER_STYLES)) {
  for (const [name, doc] of Object.entries(cases(style))) {
    for (const outlook of [true, false]) {
      const d = JSON.parse(JSON.stringify(doc));
      d.root.data.outlook = outlook;
      if (style === undefined) delete d.w.data.style;
      const wrapped = EB.compileDocument(d, context, refs);
      const plain = EB.compileDocument(unwrap(d, 'w'), context, refs);
      check(`IA18 [${styleName} style, Outlook ${outlook ? 'on' : 'off'}] ${name}: the same output apart from one wrapper div`,
        equalApartFromOneWrapper(wrapped, plain) && wrapped !== plain, wrapped === plain ? 'no wrapper emitted' : '');
      n++;
    }
  }
}
check(`IA18 ran every case (12 kinds x 4 places + 4, x ${Object.keys(WRAPPER_STYLES).length} styles x 2 flags)`, n === (SINGLE_KINDS.length * 4 + 4) * Object.keys(WRAPPER_STYLES).length * 2, String(n));

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
