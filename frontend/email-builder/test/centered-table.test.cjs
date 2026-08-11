const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// Conversion must not fabricate alignment: a wrapper without text-align gets
// NO align attribute (a stamped align="left" overrode self-centering user
// content in the Gmail app — campaign 10 social icons, 2026-08-11), and a
// wrapper whose sole child is a self-centering table gets align="center".
const input = `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff" role="presentation" cellspacing="0" cellpadding="0" border="0"><tbody><tr><td>
<div style="font-size:16px;padding:16px 24px 16px 24px"><!-- icons -->
<table role="presentation" align="center" border="0" cellpadding="0" cellspacing="0" style="margin:0 auto;border-collapse:collapse"><tbody><tr><td style="padding:0 3px;">icons</td></tr></tbody></table>
</div>
<div style="font-size:16px;padding:8px 0px 8px 0px"><p>plain text block</p></div>
<div style="padding:4px 6px 4px 6px">
<table role="presentation" width="100%" style="border-collapse:collapse"><tbody><tr><td>left table</td></tr></tbody></table>
</div>
<div style="text-align:center;font-size:16px;padding:8px 0px 8px 0px"><p>centered text block</p></div>
<div style="font-size:16px;padding:2px 4px 2px 4px">
<table role="presentation" style="margin-left:auto;margin-right:auto;border-collapse:collapse"><tbody><tr><td>longhand centered</td></tr></tbody></table>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;

const out = postProcessForOutlook(input);

const checks = [];
function check(name, ok, detail) {
  checks.push({ name, ok, detail });
}

// Self-centering table child → derived td align="center".
check(
  'self-centering table wrapper gets td align=center',
  /<td align="center"[^>]*style="[^"]*padding:16px 24px 16px 24px/.test(out)
);

// No text-align, text-flow content → converted td carries NO align attribute.
check(
  'unaligned text wrapper td has no align attribute',
  /<td style="font-size:16px;padding:8px 0px 8px 0px/.test(out)
);

// Non-centering table child → no fabricated alignment either.
check(
  'non-centering table wrapper td has no align attribute',
  /<td style="padding:4px 6px 4px 6px/.test(out)
);
check('no fabricated align=left anywhere', !/<td align="left"/.test(out));

// Explicit text-align still converts to the attribute.
check(
  'explicit text-align:center still becomes td align=center',
  /<td align="center"[^>]*style="text-align:center;font-size:16px;padding:8px 0px 8px 0px/.test(out)
);

// Longhand auto margins are recognized as self-centering too.
check(
  'longhand margin-left/right:auto table derives td align=center',
  /<td align="center"[^>]*style="[^"]*padding:2px 4px 2px 4px/.test(out)
);

let failed = 0;
for (const { name, ok, detail } of checks) {
  if (!ok) failed++;
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? `  [${detail}]` : ''}`);
}
process.exit(failed === 0 ? 0 : 1);
