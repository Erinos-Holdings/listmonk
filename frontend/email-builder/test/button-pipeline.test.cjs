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
const mso = out.match(/<table role="presentation" width="(\d+)"[^>]*><tbody><tr><td bgcolor="#0055d4" align="center" style="([^"]*)">/);
check('pipeline: mso copy emitted with px width', !!mso, mso && `w=${mso[1]}`);
check('pipeline: width = 600 - 48 td padding = 552', mso && mso[1] === '552');
check('pipeline: marker only on the non-mso original', (out.match(/data-lm-full-width-button/g) || []).length === 1);
check('pipeline: original CSS anchor preserved', /display:block/.test(out) && (out.match(/SHOP NOW/g) || []).length === 2);
console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
