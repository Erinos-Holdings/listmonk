const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// Image with explicit height attr inside a fixed-layout 2-col row: clamp must scale height.
const input = `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
<div style="padding:0px 24px 0px 24px">
<table align="center" width="100%" cellpadding="0" border="0" style="table-layout:fixed;border-collapse:collapse"><tbody><tr>
<td style="box-sizing:content-box;vertical-align:middle;padding-left:0;padding-right:6px">
<div style="padding:0"><img alt="h" src="x://h.png" width="300" height="200" style="width:300px"></div>
</td>
<td style="box-sizing:content-box;vertical-align:middle;padding-left:6px;padding-right:0">
<div style="padding:0"><img alt="n" src="x://n.png" width="300" style="width:300px"></div>
</td>
</tr></tbody></table>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;

const out = postProcessForOutlook(input);
// available: 600-48=552; /2=276; -6 = 270. scale: 200 * 270/300 = 180
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }
const clampedH = out.match(/<img[^>]*alt="h"[^>]*width="270"[^>]*>/);
check('clamped copy exists at 270', !!clampedH);
check('clamped copy height attr scaled to 180', clampedH && /height="180"/.test(clampedH[0]), clampedH && clampedH[0].replace(/src="[^"]*"/,''));
const clampedStyle = clampedH && clampedH[0].match(/style="([^"]*)"/);
check('clamped copy style height (final value)', true, clampedStyle && clampedStyle[1]);
const origH = out.match(/<img[^>]*alt="h"[^>]*width="300"[^>]*>/);
check('original copy keeps height="200"', origH && /height="200"/.test(origH[0]));
const noH = out.match(/<img[^>]*alt="n"[^>]*width="270"[^>]*>/);
check('no-height image: clamped copy has no height attr', noH && !/height=/.test(noH[0]));
console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
