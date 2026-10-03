const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcess } = require(path.join(__dirname, '.build', 'postProcess.cjs'));

// integrations BIBLE-OUTLOOK-FIXES-SPEC §16 SE1 / IE1 -- fluid images. `width:<n>px;max-width:100%`
// gives an auto-width table cell a minimum of <n> px: Android's WebView lays the mail out at the
// canvas width and the Gmail app zooms the whole mail out to about 65 % (bible sheet 7 and campaign
// 108, 2026-10-03). A width-sized image now compiles to `width:100%;max-width:<n>px` with its
// `width` attribute kept (the only thing Word reads). Pins:
//   a width-sized image, at its own width or clamped  -> fluid, attribute = the px width
//   a height-only image, an unsized image, an Avatar-like image with a px height,
//   an image with no style (the tracking pixel) and an author's Html-block image -> untouched
//   no mark attribute reaches the output; flag off -> nothing changes.
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

function render(inner, outlook = true) {
  return postProcess(`<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
${inner}
</td></tr></tbody></table>
</div>
</body></html>`, { outlook });
}
const block = (wrapperStyle, inner) => `<div style="${wrapperStyle}">${inner}</div>`;
function imgOf(out, alt) {
  const doc = new JSDOM(out).window.document;
  return Array.from(doc.querySelectorAll('img')).find((i) => i.getAttribute('alt') === alt);
}
const styleOf = (img) => Object.fromEntries((img.getAttribute('style') || '').split(';').filter(Boolean).map((d) => { const i = d.indexOf(':'); return [d.slice(0, i).trim(), d.slice(i + 1).trim()]; }));

// 1. A width-sized image at its own width.
{
  const out = render(block('padding:16px 24px 16px 24px;text-align:center', '<img alt="own" src="x://a.jpg" width="240" style="width:240px;max-width:100%">'));
  const img = imgOf(out, 'own');
  const s = styleOf(img);
  check('own width: width attribute kept', img.getAttribute('width') === '240', img.getAttribute('width'));
  check('own width: style width is 100%', s.width === '100%', s.width);
  check('own width: max-width is the px width', s['max-width'] === '240px', s['max-width']);
  check('own width: height stays auto', s.height === 'auto', s.height);
  check('own width: no px width left in the style', !/(?:^|;)width:240px/.test(img.getAttribute('style')));
}

// 2. An over-wide image: the clamp's width is what becomes the max-width.
{
  const out = render(block('padding:16px 24px 16px 24px', '<img alt="wide" src="x://w.jpg" width="900" style="width:900px;max-width:100%">'));
  const img = imgOf(out, 'wide');
  const s = styleOf(img);
  check('clamped: width attribute is the clamped width', img.getAttribute('width') === '552', img.getAttribute('width'));
  check('clamped: style width is 100%', s.width === '100%', s.width);
  check('clamped: max-width is the clamped px width', s['max-width'] === '552px', s['max-width']);
  check('clamped: exactly one copy of the image', (out.match(/alt="wide"/g) || []).length === 1);
}

// 3. Untouched shapes.
{
  const out = render([
    block('padding:0px', '<img alt="heightonly" src="x://logo.png" height="40" style="max-width:100%;height:40px">'),
    block('padding:0px', '<img alt="unsized" src="x://hero.png" style="max-width:100%">'),
    block('padding:0px', '<img alt="avatar" src="x://av.png" width="64" height="64" style="width:64px;height:64px;border-radius:64px">'),
  ].join('\n'));
  const ho = styleOf(imgOf(out, 'heightonly'));
  check('height-only: keeps its px height and width:auto', ho.height === '40px' && ho.width === 'auto', JSON.stringify(ho));
  const un = styleOf(imgOf(out, 'unsized'));
  check('unsized: no width declared, max-width:100% kept', un.width === undefined && un['max-width'] === '100%', JSON.stringify(un));
  // hardenImages gives every width-sized image height:auto, an Avatar included, so an Avatar is
  // width-sized like any other image and becomes fluid at its own size.
  const av = imgOf(out, 'avatar');
  const avs = styleOf(av);
  check('avatar: width attribute kept', av.getAttribute('width') === '64');
  check('avatar: fluid at its own size', avs.width === '100%' && avs['max-width'] === '64px', JSON.stringify(avs));
}

// 4. The tracking pixel shape (no style): untouched.
{
  const out = render('<img src="x://px.png" alt="" width="1" height="1">');
  const px = imgOf(out, '');
  check('pixel: no width:100%', !/width:100%/.test(px.getAttribute('style') || ''), px.getAttribute('style'));
}

// 5. An author's image inside an Html block is untouched and the mark never reaches the output.
{
  const out = render(`<div data-lm-user-html style="padding:16px 24px 16px 24px"><table><tbody><tr><td><img alt="author" src="x://icon.png" width="26" height="26" style="width:26px;height:auto;display:block;border:0"></td></tr></tbody></table></div>`);
  const img = imgOf(out, 'author');
  const s = styleOf(img);
  check('author Html image: px width kept', s.width === '26px', s.width);
  check('author Html image: no max-width added', s['max-width'] === undefined, s['max-width']);
  check('no fluid mark attribute in the output', !out.includes('data-lm-fluid-fenced'));
}
{
  const out = render(block('padding:0px', '<img alt="plain" src="x://a.jpg" width="240" style="width:240px;max-width:100%">'));
  check('no fluid mark attribute in an ordinary compile either', !out.includes('data-lm-fluid-fenced'));
}

// 6. Flag off: the image is not touched.
{
  const out = render(block('padding:0px', '<img alt="off" src="x://a.jpg" width="240" style="width:240px;max-width:100%">'), false);
  const s = styleOf(imgOf(out, 'off'));
  check('flag off: px width kept', s.width === '240px' && s['max-width'] === '100%', JSON.stringify(s));
}

console.log(failed ? `\n${failed} FAILED` : '\nALL PASS');
process.exit(failed ? 1 : 0);
