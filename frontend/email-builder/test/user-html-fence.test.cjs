const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// User-authored markup inside an Html block (marked data-lm-user-html by the
// reader) must NOT have its divs rewritten into table cells — td drops
// div-only semantics like margin:0 auto. The marked wrapper itself carries
// builder-owned padding and must still convert (the 2026-08-07 icons-block
// behavior), as must builder blocks outside the fence.
const input = `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff" role="presentation" cellspacing="0" cellpadding="0" border="0"><tbody><tr><td>
<div data-lm-user-html="true" style="background-color:#FFF8F0;padding:16px 24px 16px 24px">
<div style="max-width:480px;margin:0 auto;padding:12px 8px 12px 8px;background-color:#E8F0FE">user card</div>
<div style="height:20px;padding:1px 0 1px 0"></div>
</div>
<div style="background-color:#CCD6E9;padding:16px 0px 16px 0px">
<div style="text-align:center;padding:0px 24px 0px 24px">builder text</div>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;

const out = postProcessForOutlook(input);

const checks = [];
function check(name, ok, detail) {
  checks.push({ name, ok, detail });
}

// Fenced user divs survive untouched (margin:0 auto card, empty spacer-shaped div).
check('user card div is untouched', /<div style="max-width:480px;margin:0 auto;padding:12px 8px 12px 8px;background-color:#E8F0FE">user card<\/div>/.test(out));
check('user card not converted to td', !/<td[^>]*#E8F0FE/.test(out));
check('user spacer-shaped div not converted', !/<td[^>]*height="20"/.test(out));

// The Html wrapper itself (builder-owned padding) still converts.
check(
  'html wrapper converts to td with bgcolor + padding',
  /<td[^>]*bgcolor="#FFF8F0"[^>]*style="[^"]*padding:16px 24px 16px 24px/.test(out)
);

// Builder blocks outside the fence still convert (nested-container behavior).
check(
  'builder container outside fence converts',
  /<td[^>]*bgcolor="#CCD6E9"[^>]*style="[^"]*padding:16px 0px 16px 0px/.test(out)
);
check('builder text wrapper converts', /<td align="center"[^>]*style="[^"]*padding:0px 24px 0px 24px/.test(out));

let failed = 0;
for (const { name, ok, detail } of checks) {
  if (!ok) failed++;
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? `  [${detail}]` : ''}`);
}
process.exit(failed === 0 ? 0 : 1);
