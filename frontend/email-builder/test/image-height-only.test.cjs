const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// IMAGE-WIDTH-SPEC Part B (D3.4). hardenImages used to set height:auto on EVERY
// image, which deleted the only sizing of a height-only image (campaign 47's logo:
// Height 62, no Width) so CSS-honoring clients drew it at full canvas width. Pins:
//   I1  a height-only image keeps height:<h>px (style) and height="<h>" (attr) and
//       gains width:auto — for the attribute-only, style-only and both forms;
//   I3  an image with neither dimension has its style height untouched.
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

function render(imgs) {
  return postProcessForOutlook(`<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
${imgs.map((img) => `<div style="padding:16px 24px 16px 24px;text-align:center">${img}</div>`).join('\n')}
</td></tr></tbody></table>
</div>
</body></html>`);
}

function imgByAlt(out, alt) {
  const m = out.match(new RegExp(`<img[^>]*alt="${alt}"[^>]*>`, 'g')) || [];
  return m.map((tag) => ({ tag, style: (tag.match(/style="([^"]*)"/) || [, ''])[1] }));
}

const out = render([
  // The builder's emit for Height 62, no Width (attribute + px style).
  '<img alt="both" src="x://logo.png" height="62" style="outline:none;border:none;text-decoration:none;vertical-align:middle;display:inline-block;max-width:100%;height:62px">',
  // Attribute only.
  '<img alt="attr" src="x://logo2.png" height="40" style="max-width:100%">',
  // Style only — the attribute must be added for Word.
  '<img alt="style" src="x://logo3.png" style="max-width:100%;height:40px">',
  // Neither dimension.
  '<img alt="none" src="x://hero.png" style="max-width:100%;display:inline-block">',
  // Today's stored unsized block (D-2 baked height:auto in at an earlier save): no px
  // dimension resolves, so hardenImages must leave the declaration exactly as it is.
  '<img alt="baked" src="x://hero2.png" style="max-width:100%;height:auto;display:inline-block">',
  // A non-px height is not a resolvable size either; it must survive untouched.
  '<img alt="pct" src="x://hero3.png" style="max-width:100%;height:50%">',
]);

// I1
for (const [alt, h] of [['both', '62'], ['attr', '40'], ['style', '40']]) {
  const imgs = imgByAlt(out, alt);
  check(`${alt}: emitted exactly once (no dual emit without a width)`, imgs.length === 1, String(imgs.length));
  const { tag, style } = imgs[0] || { tag: '', style: '' };
  check(`${alt}: style keeps height:${h}px`, new RegExp(`(^|;)height:${h}px(;|$)`).test(style), style);
  check(`${alt}: style does NOT carry height:auto`, !/(^|;)height:auto(;|$)/.test(style), style);
  check(`${alt}: style gains width:auto`, /(^|;)width:auto(;|$)/.test(style), style);
  check(`${alt}: height="${h}" attribute present`, new RegExp(` height="${h}"`).test(tag), tag.replace(/src="[^"]*"/, ''));
  check(`${alt}: no width attribute invented`, !/ width=/.test(tag), tag.replace(/src="[^"]*"/, ''));
}

// I3
{
  const imgs = imgByAlt(out, 'none');
  check('none: emitted exactly once', imgs.length === 1, String(imgs.length));
  const { tag, style } = imgs[0] || { tag: '', style: '' };
  check('none: no height declaration of any kind in style', !/(^|;)height:/.test(style), style);
  check('none: no width declaration in style', !/(^|;)width:/.test(style), style);
  check('none: no height/width attribute', !/ (height|width)=/.test(tag), tag.replace(/src="[^"]*"/, ''));
  check('none: still hardened (border 0, display block)', / border="0"/.test(tag) && /(^|;)display:block(;|$)/.test(style), style);
}

{
  const imgs = imgByAlt(out, 'baked');
  const { tag, style } = imgs[0] || { tag: '', style: '' };
  check('baked: pre-existing height:auto is left in place (not deleted, not replaced)', /(^|;)height:auto(;|$)/.test(style), style);
  check('baked: no width declaration or attribute invented', !/(^|;)width:/.test(style) && !/ (width|height)=/.test(tag), style);
}
{
  const imgs = imgByAlt(out, 'pct');
  const { tag, style } = imgs[0] || { tag: '', style: '' };
  check('pct: height:50% survives untouched', /(^|;)height:50%(;|$)/.test(style), style);
  check('pct: no height attribute invented from a non-px height', !/ height=/.test(tag), tag.replace(/src="[^"]*"/, ''));
}

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
