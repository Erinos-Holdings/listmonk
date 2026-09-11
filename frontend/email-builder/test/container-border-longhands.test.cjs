// CAMPAIGN-52-HARDENING T4 (I5). A bordered Container compiles to four border-<side>
// longhands of equal width/style/color plus border-color and NO bare `border` shorthand
// on the converted cell; getBorderWidths reads the longhand form, so the canvas width
// budget (getHorizontalInset) still subtracts the border.
const { pp, canvas, makeChecker, JSDOM, borderedContainer, wideImage } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

const out = pp.postProcess(canvas(borderedContainer), { outlook: true });
const doc = new JSDOM(out).window.document;
const td = Array.from(doc.querySelectorAll('td')).find((cell) => /border-top/.test(cell.getAttribute('style') || ''));
check('bordered Container converted to a td carrying the longhands', !!td);
const style = td ? td.getAttribute('style') : '';
const map = Object.fromEntries(style.split(';').filter(Boolean).map((e) => e.split(':').map((s) => s.trim())).map(([k, ...v]) => [k, v.join(':')]));
for (const side of ['top', 'right', 'bottom', 'left']) {
  check(`border-${side} is 1px solid #fbf00b`, map[`border-${side}`] === '1px solid #fbf00b', map[`border-${side}`]);
}
check('border-color carried', map['border-color'] === '#fbf00b', map['border-color']);
check('no bare border shorthand on the cell', !('border' in map));
check('padding and radius untouched', map.padding === '0px 24px 0px 24px' && map['border-radius'] === '0');
check('no bare `border:` shorthand survives on any converted block cell', !/<td[^>]*style="[^"]*(?:^|;)border:1px/.test(out));

// getBorderWidths reads longhands (and still the shorthand / -width forms).
const bw = pp.getBorderWidths;
const long = { 'border-top': '1px solid #fbf00b', 'border-right': '1px solid #fbf00b', 'border-bottom': '1px solid #fbf00b', 'border-left': '1px solid #fbf00b', 'border-color': '#fbf00b' };
const got = bw(long);
check('getBorderWidths: longhand form → 1 on all four sides', got.top === 1 && got.right === 1 && got.bottom === 1 && got.left === 1, JSON.stringify(got));
const mixed = bw({ 'border-top': '3px solid red', 'border-left-width': '2px', border: '1px solid blue' });
check('getBorderWidths: per-side wins over shorthand', mixed.top === 3 && mixed.left === 2 && mixed.right === 1 && mixed.bottom === 1, JSON.stringify(mixed));
check('getBorderWidths: shorthand alone still read', JSON.stringify(bw({ border: '2px solid #000' })) === JSON.stringify({ top: 2, right: 2, bottom: 2, left: 2 }));

// Canvas inset from longhands: a 600px canvas with 1px longhand borders and a 700px image
// clamps to 598 (600 − 2 × 1), exactly as the shorthand form did.
const longCanvas = 'margin:0 auto;max-width:600px;background-color:#fff;border-top:1px solid #000;border-right:1px solid #000;border-bottom:1px solid #000;border-left:1px solid #000';
const clamp = (html) => (html.match(/<img[^>]*width="(\d+)"[^>]*>/g) || []).map((m) => Number(m.match(/width="(\d+)"/)[1])).filter((w) => w !== 700)[0];
check('canvas longhand borders reduce the clamp budget: 700 → 598', clamp(pp.postProcess(canvas(wideImage.replace('padding:0px 24px 0px 24px', 'padding:0'), { canvasStyle: longCanvas }), { outlook: true })) === 598);
check('canvas shorthand border: 700 → 598 (unchanged behavior)', clamp(pp.postProcess(canvas(wideImage.replace('padding:0px 24px 0px 24px', 'padding:0'), { canvasStyle: 'margin:0 auto;max-width:600px;border:1px solid #000' }), { outlook: true })) === 598);

done();
