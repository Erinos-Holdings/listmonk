// integrations RENDER-CATALOG-SPEC §17.5 -- the documents word-fixes.test.cjs compiles, and
// capture-word-fixes-before.cjs pins with the bundle from BEFORE the four Word fixes F1-F4.
// Not a suite (no .test.cjs suffix).
//
//   FIXED: a document holding each fixed shape (a Divider, full-width Buttons in a fractional
//          column and at full width, a bordered canvas, a zero-padding bordered Container).
//          Pinned with the Outlook flag OFF: a flag-off compile must not change.
//   CLEAN: documents holding none of the fixed shapes with the flag ON (inline Buttons, a padded
//          bordered Container, an unbordered zero-padding Container, a canvas with no border,
//          an Html block holding its own <hr>). Their compile must not change at all.
const PAD = { top: 16, bottom: 16, left: 24, right: 24 };
const ZERO = { top: 0, bottom: 0, left: 0, right: 0 };
const L = 'https://canary.invalid/catalog';

const text = (t) => ({ type: 'Text', data: { props: { text: t, markdown: true }, style: { padding: PAD } } });
const button = (props = {}) => ({
  type: 'Button',
  data: { props: { text: 'Go', url: L, buttonBackgroundColor: '#0B6E6E', buttonTextColor: '#FFFFFF', ...props }, style: { padding: PAD, textAlign: 'center' } },
});
const divider = (style = {}, props = {}) => ({ type: 'Divider', data: { props: { lineColor: '#CCCCCC', lineHeight: 3, ...props }, style: { padding: PAD, ...style } } });
const cols = (count, kids, props = {}, style = {}) => ({
  type: 'ColumnsContainer',
  data: { style: { padding: PAD, ...style }, props: { columnsCount: count, columnsGap: 16, contentAlignment: 'middle', columns: [0, 1, 2].map((i) => ({ childrenIds: kids[i] || [] })), ...props } },
});
const box = (kids, style = {}) => ({ type: 'Container', data: { style: { padding: PAD, ...style }, props: { childrenIds: kids } } });
const html = (contents) => ({ type: 'Html', data: { props: { contents }, style: { padding: PAD } } });

function doc(blocks, top, layout = {}, outlook = true) {
  return { root: { type: 'EmailLayout', data: { ...layout, outlook, childrenIds: top } }, ...blocks };
}

// Each fixed shape, flag on.
function fixedDocs(outlook = true) {
  return {
    divider: doc({
      d1: divider(),
      d0: divider({ padding: ZERO }, { lineHeight: 1, lineColor: '#333333' }),
      dc: divider(),
      r: cols(3, [['t1'], ['dc'], ['t2']], { fixedWidths: [60, null, 60] }),
      t1: text('a'),
      t2: text('b'),
    }, ['d1', 'd0', 'r'], {}, outlook),
    fullWidthButtons: doc({
      b0: button({ fullWidth: true }),
      bf: button({ fullWidth: true, text: '' }),
      r: cols(3, [['b1'], ['b2'], ['b3']]),
      b1: button({ fullWidth: true }),
      b2: button({ fullWidth: true, text: '' }),
      b3: button({ fullWidth: true, borderSize: 2, borderColor: '#FFFFFF' }),
    }, ['b0', 'bf', 'r'], {}, outlook),
    // §17.5 F2 (amended): the box is the slot less max(2, stroke px) — border 0 (the 1 px hairline), 2 and 4.
    strokeButtons: doc({
      s0: button({ fullWidth: true }),
      s2: button({ fullWidth: true, borderSize: 2, borderColor: '#FFFFFF' }),
      s4: button({ fullWidth: true, borderSize: 4, borderColor: '#FFFFFF' }),
      r: cols(3, [['c0'], ['c2'], ['c4']]),
      c0: button({ fullWidth: true }),
      c2: button({ fullWidth: true, borderSize: 2, borderColor: '#FFFFFF' }),
      c4: button({ fullWidth: true, borderSize: 4, borderColor: '#FFFFFF' }),
    }, ['s0', 's2', 's4', 'r'], {}, outlook),
    borderedCanvas: doc({ t: text('Inside a bordered canvas.') }, ['t'], { borderColor: '#333333' }, outlook),
    borderedZeroBox: doc({
      c: box(['t'], { borderColor: '#333333', padding: ZERO }),
      t: text('In a zero-padding bordered Container.'),
      cr: box(['tr'], { borderColor: '#333333', padding: ZERO, borderRadius: 8 }),
      tr: text('Rounded.'),
    }, ['c', 'cr'], {}, outlook),
  };
}

// No fixed shape anywhere, flag on.
function cleanDocs() {
  return {
    inlineButtons: doc({
      b0: button(),
      b1: button({ customWidth: 200 }),
      b2: button({ text: '' }),
      r: cols(3, [['b3'], ['b4'], ['t']]),
      b3: button(),
      b4: button({ borderSize: 2, borderColor: '#FFFFFF' }),
      t: text('Cell'),
    }, ['b0', 'b1', 'b2', 'r']),
    paddedBorderedBox: doc({ c: box(['t'], { borderColor: '#333333' }), t: text('Padded and bordered.') }, ['c']),
    zeroUnborderedBox: doc({ c: box(['t'], { padding: ZERO }), t: text('Zero padding, no border.') }, ['c']),
    plainCanvas: doc({ t: text('A plain canvas.'), r: cols(2, [['a'], ['b']]), a: text('a'), b: text('b') }, ['t', 'r'], { backdropPadding: 32 }),
    htmlHr: doc({ h: html('<p>Above</p><hr style="border:none;border-top:2px solid #000000"><p>Below</p>') }, ['h']),
  };
}

module.exports = { fixedDocs, cleanDocs };
