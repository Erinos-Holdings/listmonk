const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// 600px canvas; columns [200px fixed, auto] with 12px gap (6px inner paddings).
// Auto column real content width = 600 - (200+6) - 6 = 388 -> 380px image must NOT clamp.
const input = `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
<div style="background-color:#ececec;padding:12px 8px 12px 8px"><table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse"><tbody><tr><td>band content</td></tr></tbody></table></div>
<div style="padding:0px 0px 0px 0px">
<table align="center" width="100%" cellpadding="0" border="0" style="table-layout:fixed;border-collapse:collapse"><tbody><tr>
<td style="box-sizing:content-box;vertical-align:middle;padding-left:0;padding-right:6px;width:200px">
<div style="padding:0"><img alt="a" src="x://a.png" width="240" style="width:240px;max-width:100%"></div>
</td>
<td style="box-sizing:content-box;vertical-align:middle;padding-left:6px;padding-right:0">
<div style="padding:0"><img alt="b" src="x://b.png" width="380" style="width:380px;max-width:100%"></div>
</td>
</tr></tbody></table>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;

const out = postProcessForOutlook(input);
const widths = [...out.matchAll(/<img[^>]*alt="([ab])"[^>]*width="(\d+)"/g)].map((m) => [m[1], Number(m[2])]);
const byAlt = {};
for (const [alt, w] of widths) (byAlt[alt] = byAlt[alt] || []).push(w);
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }
check('fixed 200px column dual-emits: mso 200 + original 240', JSON.stringify(byAlt.a) === '[200,240]', `a=${JSON.stringify(byAlt.a)}`);
check('auto column gets remainder 388 -> 380px image untouched (single copy)', JSON.stringify(byAlt.b) === '[380]', `b=${JSON.stringify(byAlt.b)}`);
check('bg-carrying table wrapper converts to bgcolor td', /<td bgcolor="#ececec" style="background-color:#ececec;padding:12px 8px 12px 8px">/.test(out));
check('no Gmail pin style injected when no buttons exist', !/lm-gm-pin/.test(out) && !/u \+ \.body/.test(out));
process.exit(failed ? 1 : 0);
