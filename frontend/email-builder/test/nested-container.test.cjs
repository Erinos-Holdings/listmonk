const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// Campaign 28 shape: a Container embedded in a Container. The outer (48px
// padding) converts to a table; before innermost-first ordering, that
// conversion detached the inner blue Container and its Text wrappers from the
// wrappers snapshot, so they shipped as raw padded divs and Word dropped
// their padding and background.
const input = `<!doctype html><html><body>
<div style="background-color:#F5F5F5;color:#262626;font-size:16px;margin:0;padding:32px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#FFFFFF" role="presentation" cellspacing="0" cellpadding="0" border="0"><tbody><tr style="width:100%"><td>
<div style="max-width:100%;border-radius:0;padding:48px 48px 48px 48px;border:1px solid #FAFAFA">
<div style="background-color:#CCD6E9;padding:16px 0px 16px 0px">
<div style="font-size:17px;font-weight:normal;text-align:center;padding:0px 24px 0px 24px">FOR OUR CREATOR COMMUNITY</div>
<div style="font-size:16px;font-weight:normal;text-align:center;padding:0px 24px 0px 24px">Try the AirLuxe for yourself.</div>
<div style="font-size:21px;text-align:center;padding:4px 24px 16px 24px"><a href="https://example.com/airluxe" style="color:#FFFFFF;font-size:21px;font-weight:normal;background-color:#0A0A0A;border-radius:4px;display:inline-block;padding:12px 20px;text-decoration:none" target="_blank">GET YOUR AIRLUXE</a></div>
</div>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;

const out = postProcessForOutlook(input);

const checks = [];
function check(name, ok, detail) {
  checks.push({ name, ok, detail });
}

// The embedded Container must become a table cell, not remain a div.
check('inner container div is gone', !/<div[^>]*#CCD6E9/.test(out));
check(
  'inner container is a td with bgcolor + padding',
  /<td[^>]*bgcolor="#CCD6E9"[^>]*style="[^"]*background-color:#CCD6E9[^"]*padding:16px 0px 16px 0px/.test(out)
);

// Its Text wrappers must convert too (they were equally detached before).
check('text wrapper divs are gone', !/<div[^>]*padding:0px 24px 0px 24px/.test(out));
check(
  'text wrappers are tds with their padding',
  (out.match(/<td align="center"[^>]*style="[^"]*padding:0px 24px 0px 24px/g) || []).length === 2
);

// The outer container still converts as before.
check(
  'outer container is a td with 48px padding',
  /<td[^>]*style="[^"]*padding:48px 48px 48px 48px/.test(out)
);

// The button inside the nested container still gets the VML dual-emit.
check('nested button has mso VML copy', /v:roundrect[^}]*GET\\x20YOUR\\x20AIRLUXE/.test(out));
check('nested button keeps non-mso anchor', /<a href="https:\/\/example.com\/airluxe"[^>]*>GET YOUR AIRLUXE<\/a>/.test(out));

let failed = 0;
for (const { name, ok, detail } of checks) {
  if (!ok) failed++;
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? `  [${detail}]` : ''}`);
}
process.exit(failed === 0 ? 0 : 1);
