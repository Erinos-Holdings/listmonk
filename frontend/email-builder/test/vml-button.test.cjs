const path = require('path');
const { JSDOM } = require('jsdom');
global.DOMParser = new JSDOM('<!doctype html>').window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// Custom width+height button (VML branch): inline-block anchor, explicit width,
// line-height-based height, thick border — mirrors the "Outlook Test" button.
const input = `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
<div style="text-align:center;padding:16px 24px 16px 24px">
<a href="https://x.test/go" target="_blank" style="color:#FFFFFF;font-size:16px;font-weight:bold;background-color:#2563EB;border-radius:64px;display:inline-block;padding:0px 0px 0px 0px;text-decoration:none;width:200px;box-sizing:border-box;text-align:center;line-height:19px;border:5px solid #DC2626;white-space:nowrap">Outlook Test</a>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;

const out = postProcessForOutlook(input);
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

check('VML rides inside a Safe template (no raw v:roundrect in stored body)', !/<v:roundrect/.test(out));
check('anchorlock stays self-closing inside the Safe payload', /\\x3cw:anchorlock\/\\x3e\\x3cv:textbox/.test(out), (out.match(/anchorlock[^\\]*/) || [])[0]);
check('conditional markers and VML in ONE Safe block', /\{\{ Safe "\\x3c!--\[if mso\]\\x3e\\x3cv:roundrect[^}]*\\x3c!\[endif\]--\\x3e" \}\}/.test(out));
const h = out.match(/height:(\d+)px;v-text-anchor/);
check('height = max(19, 19+2 slack) + border 5 = 26', h && h[1] === '26', h && `h=${h[1]}`);
const w = out.match(/v-text-anchor:middle;width:(\d+)px/);
check('explicit width 200 preserved', w && w[1] === '200', w && `w=${w[1]}`);

// Simulated Go render: VML must decode intact
const rendered = out.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_, s) =>
  s.replace(/\\x3c/g, '<').replace(/\\x3e/g, '>').replace(/\\"/g, '"').replace(/\\\\/g, '\\'));
check('rendered VML has self-closing anchorlock then zero-inset textbox', /<w:anchorlock\/><v:textbox inset="0,0,0,0"><center/.test(rendered));
check('textbox closes before roundrect', /<\/center><\/v:textbox><\/v:roundrect>/.test(rendered));
check('center pins line-height exactly to inner height 26-5=21', /mso-line-height-rule:exactly;line-height:21px/.test(rendered));
check('rendered VML inside one [if mso] conditional', /<!--\[if mso\]><v:roundrect[\s\S]*?<\/v:roundrect><!\[endif\]-->/.test(rendered));
check('CSS anchor intact in [if !mso] branch', /<!--\[if !mso\]><!--><a href="https:\/\/x.test\/go"[^>]*white-space:nowrap[^>]*>Outlook Test<\/a><!--<!\[endif\]-->/.test(rendered));
console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
