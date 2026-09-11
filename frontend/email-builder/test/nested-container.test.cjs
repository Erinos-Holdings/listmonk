const fs = require('fs');
const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcess } = require(path.join(__dirname, '.build', 'postProcess.cjs'));

// Campaign 28 shape: a Container embedded in a Container. The outer (48px
// padding) converts to a table; before innermost-first ordering, that
// conversion detached the inner blue Container and its Text wrappers from the
// wrappers snapshot, so they shipped as raw padded divs and Word dropped
// their padding and background. The fixture is shared with text-margins.test.cjs.
const input = fs.readFileSync(path.join(__dirname, 'fixtures', 'campaign28-nested.html'), 'utf8');

const { foldVmlMarkers } = require(path.join(__dirname, 'vml-marker-fold.cjs'));
const out = foldVmlMarkers(postProcess(input, { outlook: true }));

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

// Horizontal-padding-only Text wrappers convert like every other padded block
// (PARAGRAPH-SPACING-SPEC D5/I6): their margins are stated inline, so there is
// no client-default edge margin left for Word to drop — the old rhythm-only
// carve-out, which kept them divs and let Word render them uninset, is gone.
check(
  'horizontal-padding-only text wrappers are no longer divs',
  (out.match(/<div style="font-size:1[67]px[^"]*padding:0px 24px 0px 24px">/g) || []).length === 0
);
check(
  'horizontal-padding-only text wrappers convert to tds',
  (out.match(/<td align="center"[^>]*style="font-size:1[67]px[^"]*padding:0px 24px 0px 24px">/g) || []).length === 2
);

// A text wrapper with real vertical padding carries an authored box — convert.
check(
  'vertically padded text wrapper converts to td',
  /<td align="center"[^>]*style="[^"]*padding:8px 24px 8px 24px/.test(out)
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
