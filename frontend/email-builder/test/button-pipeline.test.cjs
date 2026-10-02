const path = require('path');
const { JSDOM } = require('jsdom');
global.DOMParser = new JSDOM('<!doctype html>').window.DOMParser;
const { postProcess } = require(path.join(__dirname, '.build', 'postProcess.cjs'));

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

const { foldVmlMarkers } = require(path.join(__dirname, 'vml-marker-fold.cjs'));
const out = foldVmlMarkers(postProcess(input, { outlook: true }));
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }
// BIBLE-OUTLOOK-FIXES-SPEC §4.3: a one-line full-width Button is the textpath group, sized in pt.
const mso = out.match(/\{\{ Safe "[^}]*?v:group[^}]*?style=\\"width:([\d.]+)pt;height:[\d.]+pt[^}]*?v:roundrect[^}]*?fillcolor=\\"#0055d4\\"[^}]*?\}\}/);
check('pipeline: mso VML copy emitted', !!mso, mso && `w=${mso[1]}pt`);
check('pipeline: width = (600 - 48 td padding = 552px) -> 414pt', mso && mso[1] === '414');
check('pipeline: height 43px + 1px fallback border -> 33pt', mso && /;height:33pt/.test(mso[0]));
check('pipeline: marker only on the non-mso original', (out.match(/data-lm-full-width-button/g) || []).length === 1);
check('pipeline: original CSS anchor preserved', /display:block/.test(out) && (out.match(/SHOP NOW/g) || []).length === 1 && (out.match(/SHOP\\x20NOW/g) || []).length === 1);
// Long label wraps in the CSS button -> VML height must cover the wrapped lines
const longLabel = 'Discover Everything New In The August Collection Now';
const wrapInput = input.replace('SHOP NOW', longLabel);
const wrapOut = foldVmlMarkers(postProcess(wrapInput, { outlook: true }));
// Two lines select the S5 fallback (VML text draws one line). Since BIBLE-OUTLOOK-FIXES-SPEC §12
// (DA3) that is the table-cell button stamped "lines": no VML, the cell as wide as the column
// budget (552) less the padding and the 1px hairline border on both sides (552 - 40 - 2 = 510),
// and no height — the cell grows with the wrapped label (the old roundrect was 63px = 47.25pt).
check('wrapped 2-line label is the S5 fallback (stamped table cell, no VML)', /data-lm-btn-fallback=\\"lines\\"/.test(wrapOut) && !/v:textpath|v:roundrect|anchorlock/.test(wrapOut));
check('wrapped 2-line label: table 552, cell 510, no height', /width=\\"552\\"\\x20style=\\"border-collapse:separate\\"/.test(wrapOut) && /width=\\"510\\"\\x20style=\\"background-color:#0055d4;padding:12px\\x2020px\\x2012px\\x2020px;border:1px\\x20solid\\x20#0055d4;\\"/.test(wrapOut) && !/height=\\"/.test(wrapOut));

// Non-canonical border shorthand still yields the stroke
const borderInput = input.replace('text-decoration:none">SHOP NOW', 'text-decoration:none;border:solid 5px #e01d1d !important">SHOP NOW');
const borderOut = foldVmlMarkers(postProcess(borderInput, { outlook: true }));
check('reordered border + !important -> stroke color kept', /strokecolor=\\"#e01d1d\\"/.test(borderOut) && /strokeweight=\\"3.75pt\\"/.test(borderOut));
check('border width joins the height: 43+5=48px -> 36pt', /;height:36pt\\"\\x20coordorigin/.test(borderOut));

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
