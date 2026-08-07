const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// User-pasted table inside an Html block: percentage cells, NOT table-layout:fixed.
// The 400px image fits its 80% column; it must not be rewritten.
const input = `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
<table width="100%" style="border-collapse:collapse"><tbody><tr>
<td style="width:10%"></td>
<td style="width:10%"></td>
<td style="width:80%"><img alt="u" src="x://u.png" width="400" style="width:400px"></td>
</tr></tbody></table>
</td></tr></tbody></table>
</div>
</body></html>`;

const out = postProcessForOutlook(input);
const m = out.match(/<img[^>]*alt="u"[^>]*width="(\d+)"/);
const ok = m && Number(m[1]) === 400;
console.log(`${ok ? 'PASS' : 'FAIL'}  non-fixed-layout (user HTML) table passes width through  [u=${m && m[1]}]`);
process.exit(ok ? 0 : 1);
