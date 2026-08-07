const path = require('path');
const { JSDOM } = require('jsdom');
global.DOMParser = new JSDOM('<!doctype html>').window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// RAW pre-transform builder shape: wrapper div > display:block anchor.
// transformButtonBlocks must stamp the marker, the walker must then dual-emit.
const input = `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
<div style="text-align:center;padding:16px 24px 16px 24px">
<a href="https://x.test/go" target="_blank" style="color:#fff;font-size:16px;font-weight:bold;background-color:#0055d4;border-radius:4px;display:block;padding:12px 20px 12px 20px;text-decoration:none">SHOP NOW</a>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;

const out = postProcessForOutlook(input);
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }
const mso = out.match(/\{\{ Safe "[^}]*?v:roundrect[^}]*?width:([\d.]+)pt[^}]*?fillcolor=\\"#0055d4\\"[^}]*?\}\}/);
check('pipeline: mso VML copy emitted', !!mso, mso && `w=${mso[1]}pt`);
check('pipeline: width = (600 - 48 td padding = 552px) -> 414pt', mso && mso[1] === '414');
check('pipeline: height 43px + 1px fallback border -> 33pt', mso && /height:33pt/.test(mso[0]));
check('pipeline: marker only on the non-mso original', (out.match(/data-lm-full-width-button/g) || []).length === 1);
check('pipeline: original CSS anchor preserved', /display:block/.test(out) && (out.match(/SHOP NOW/g) || []).length === 2);
// Long label wraps in the CSS button -> VML height must cover the wrapped lines
const longLabel = 'Discover Everything New In The August Collection Now';
const wrapInput = input.replace('SHOP NOW', longLabel);
const wrapOut = postProcessForOutlook(wrapInput);
const wh = wrapOut.match(/height:([\d.]+)pt;v-text-anchor/);
check('wrapped 2-line label: 19*2+24+1 hairline=63px -> 47.25pt', wh && wh[1] === '47.25', wh && `h=${wh[1]}`);

// Non-canonical border shorthand still yields the stroke
const borderInput = input.replace('text-decoration:none">SHOP NOW', 'text-decoration:none;border:solid 5px #e01d1d !important">SHOP NOW');
const borderOut = postProcessForOutlook(borderInput);
check('reordered border + !important -> stroke color kept', /strokecolor=\\"#e01d1d\\"/.test(borderOut) && /strokeweight=\\"3.75pt\\"/.test(borderOut));
check('border width joins the height: 43+5=48px -> 36pt', /height:36pt;v-text-anchor/.test(borderOut));

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
