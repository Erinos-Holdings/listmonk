// integrations BIBLE-OUTLOOK-FIXES-SPEC §12 (Amendment A) IA20 — the compile side of SA9: with the
// Outlook flag on, a Container becomes a table CELL exactly when its style holds a padding object
// and one side is above 0 or a background is set (transformSimpleDivBlocks); otherwise it stays a
// `div`. The review's version 2 Container descriptor carries `box=cell|div` from that same rule
// (integrations lib/campaign-review/fingerprint.ts::containerBox), pinned against the same style
// combinations in tests/lib/campaign-review/fingerprint-v2.test.ts (BOX_CASES, IA7) — the two
// tables agree by copy; each file names the other.
//
// Method: a Container holding one Text block whose wrapper has zero padding (so the Text itself
// stays a div); the Container's compiled element is that wrapper's parent — TD or DIV.
const { JSDOM } = require('jsdom');
const { loadUmd, compileInputs } = require('./_umd.cjs');

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail ? '  [' + String(detail).slice(0, 300) + ']' : ''}`); }

const { EB } = loadUmd();
const { context, refs } = compileInputs();
const ZERO = { padding: { top: 0, right: 0, bottom: 0, left: 0 } };

const BOX_CASES = [
  ['background with no padding key', { backgroundColor: '#eeeeee' }, 'div'],
  ['background with null padding', { backgroundColor: '#eeeeee', padding: null }, 'div'],
  ['background and border with no padding key', { backgroundColor: '#eeeeee', borderColor: '#000000' }, 'div'],
  ['background with zero padding', { backgroundColor: '#eeeeee', ...ZERO }, 'cell'],
  ['padding above 0', { padding: { top: 8, right: 8, bottom: 8, left: 8 } }, 'cell'],
  ['padding above 0 with a border', { padding: { top: 0, right: 0, bottom: 16, left: 0 }, borderColor: '#000000' }, 'cell'],
  ['border alone', { borderColor: '#000000' }, 'div'],
  ['radius alone', { borderRadius: 8 }, 'div'],
  ['border with zero padding', { borderColor: '#000000', ...ZERO }, 'div'],
  ['radius with zero padding', { borderRadius: 8, ...ZERO }, 'div'],
];

const doc = (style) => ({
  root: { type: 'EmailLayout', data: { backdropColor: '#F5F5F5', canvasColor: '#FFFFFF', textColor: '#262626', fontFamily: 'MODERN_SANS', outlook: true, childrenIds: ['c'] } },
  c: { type: 'Container', data: { style, props: { childrenIds: ['t'] } } },
  t: { type: 'Text', data: { style: { ...ZERO }, props: { text: 'BOXCHILD' } } },
});

function containerTag(style) {
  const html = EB.compileDocument(doc(style), context, refs);
  const d = new JSDOM(html).window.document;
  const textWrapper = Array.from(d.querySelectorAll('div')).find((el) => el.getAttribute('style') === 'padding:0px 0px 0px 0px' && el.textContent.includes('BOXCHILD'));
  return textWrapper && textWrapper.parentElement ? textWrapper.parentElement.tagName : null;
}

for (const [name, style, box] of BOX_CASES) {
  const tag = containerTag(style);
  check(`IA20: ${name} -> box=${box} (compiled ${tag})`, tag === (box === 'cell' ? 'TD' : 'DIV'), tag);
}

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
