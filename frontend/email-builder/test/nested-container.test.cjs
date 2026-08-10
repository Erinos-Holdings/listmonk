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
<div style="font-size:17px;font-weight:normal;text-align:center;padding:0px 24px 0px 24px"><p>FOR OUR CREATOR COMMUNITY</p></div>
<div style="font-size:16px;font-weight:normal;text-align:center;padding:0px 24px 0px 24px"><p>Try the AirLuxe for yourself.</p></div>
<div style="font-size:16px;font-weight:normal;text-align:center;padding:8px 24px 8px 24px"><p>RSVP TODAY</p></div>
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

// Rhythm-only Text wrappers (zero vertical padding) must STAY divs — their
// spacing comes from client-default <p> margins, which Word drops at
// table-cell edges if they are boxed into per-block tables.
check(
  'rhythm-only text wrappers remain divs',
  (out.match(/<div style="font-size:1[67]px[^"]*padding:0px 24px 0px 24px">/g) || []).length === 2
);
check('rhythm-only text wrappers not converted to tds', !/<td[^>]*style="[^"]*padding:0px 24px 0px 24px/.test(out));

// A text wrapper with real vertical padding carries an authored box — convert.
check(
  'vertically padded text wrapper converts to td',
  /<td align="center"[^>]*style="[^"]*padding:8px 24px 8px 24px/.test(out)
);

// Word-only edge-margin grafts (mso-padding-alt): the blue container's first
// flow content is a 17px rhythm block (p margin ≈ 1em → 16+17 top); its last
// child is the converted button table (no margin → bottom stays 16). The
// converted 16px text block grafts 1em on both its own edges (8+16).
check(
  'container td grafts top edge margin via mso-padding-alt',
  /bgcolor="#CCD6E9"[^>]*style="[^"]*mso-padding-alt:33px 0px 16px 0px/.test(out)
);
check(
  'converted text td grafts both edges via mso-padding-alt',
  /style="[^"]*padding:8px 24px 8px 24px[^"]*mso-padding-alt:24px 24px 24px 24px/.test(out)
);
check(
  'outer container (table-edged) gets no graft',
  !/padding:48px 48px 48px 48px[^"]*mso-padding-alt/.test(out)
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
