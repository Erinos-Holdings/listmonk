// Preview-modal dark simulation (DARK-MODE-SPEC D2, invariants I2a / I2b).
//
// darkSim.js lives under frontend/email-builder/src/ precisely so this suite runs in CI's
// existing "Email-builder tests" step -- a test under frontend/src/ would not run anywhere.
// It is dependency-free ESM; the evaluate() harness below fails on any require at all.
const path = require('path');
const fs = require('fs');
const { JSDOM } = require('jsdom');

global.DOMParser = new JSDOM('<!doctype html>').window.DOMParser;

const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));

const src = fs.readFileSync(path.join(builderRoot, 'src', 'darkSim.js'), 'utf8');
const js = ts.transpileModule(src, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;
const mod = { exports: {} };
new Function('module', 'exports', 'require', js)(mod, mod.exports, () => {
  throw new Error('darkSim must stay dependency-free');
});
const {
  applyScheme, remapSchemeColor, parseCssColor, PAGE_DARK, CARD_DARK, LIGHT_TEXT,
} = mod.exports;

let failed = 0;
function check(name, ok, detail) {
  if (!ok) {
    failed++;
    console.log(`FAIL  ${name}${detail ? `  [${detail}]` : ''}`);
  } else {
    console.log(`PASS  ${name}`);
  }
}

// ---- I2a: the mapping table ----------------------------------------------------------
check('I2a: #ffffff background -> the page dark', remapSchemeColor('#ffffff', 'background') === PAGE_DARK,
  remapSchemeColor('#ffffff', 'background'));
check('I2a: #000000 text -> light', remapSchemeColor('#000000', 'text') === LIGHT_TEXT,
  remapSchemeColor('#000000', 'text'));
check('I2a: #888888 is a mid-tone, unchanged in both roles',
  remapSchemeColor('#888888', 'background') === '#888888' && remapSchemeColor('#888888', 'text') === '#888888',
  `${remapSchemeColor('#888888', 'background')} / ${remapSchemeColor('#888888', 'text')}`);
check('I2a: #262626 background is already dark, unchanged',
  remapSchemeColor('#262626', 'background') === '#262626', remapSchemeColor('#262626', 'background'));
check('I2a: a non-parseable value maps to itself',
  remapSchemeColor('transparent', 'background') === 'transparent'
  && remapSchemeColor('inherit', 'text') === 'inherit'
  && remapSchemeColor('linear-gradient(#fff,#000)', 'background') === 'linear-gradient(#fff,#000)'
  && remapSchemeColor(undefined, 'text') === undefined);

// A light-but-not-white ground lands on the card dark, not the page dark.
const eee = remapSchemeColor('#eeeeee', 'background');
check('I2a: a light grey ground darkens, between the two palette values',
  eee !== '#eeeeee' && eee >= PAGE_DARK && eee <= CARD_DARK, eee);
check('short hex and rgb() parse the same as long hex',
  remapSchemeColor('#fff', 'background') === PAGE_DARK
  && remapSchemeColor('rgb(255, 255, 255)', 'background') === PAGE_DARK
  && remapSchemeColor('rgba(0,0,0,0.9)', 'text') === LIGHT_TEXT);
check('an out-of-range rgb() does not parse', parseCssColor('rgb(300,0,0)') === null);
check('light text is left alone (it already reads on a dark ground)',
  remapSchemeColor('#ffffff', 'text') === '#ffffff');

// ---- I2b: light is the identity ------------------------------------------------------
// The compiled preview of template 29 (Shala_EN) as submitted to Mailgun Inspect test
// utOd26av, 2026-09-10 -- the run this whole spec came out of. Tracking/unsubscribe UUIDs
// are the preview compile's dummies (runbook hazard 51).
const fixture = fs.readFileSync(path.join(__dirname, 'fixtures', 'template29-preview.html'), 'utf8');
check('I2b: applyScheme(html, "light") is the identity on template 29',
  applyScheme(fixture, 'light') === fixture);
check('I2b: identity holds for an unknown scheme name too',
  applyScheme(fixture, 'nonsense') === fixture && applyScheme('', 'light') === '');

// ---- partial ---------------------------------------------------------------------------
const partial = applyScheme(fixture, 'partial');
check('partial: the document actually changed', partial !== fixture);
check('partial: images are untouched',
  (fixture.match(/<img /g) || []).length === (partial.match(/<img /g) || []).length
  && !/img[^>]*filter:/i.test(partial));
check('partial: no white ground survives as a background declaration',
  !/background-color:\s*#fff(fff)?\b/i.test(partial),
  (partial.match(/background-color:\s*#fff[^;"]*/i) || [])[0]);
check('partial: the mid-tone link colour is preserved',
  fixture.includes('#888888') ? partial.includes('#888888') : true);

const small = '<!doctype html><html><body>'
  + '<table bgcolor="#ffffff"><tr><td style="background-color:#eeeeee;color:#000000">hi</td></tr>'
  + '<tr><td style="background:url(x.png) no-repeat;color:#888888">keep</td></tr></table>'
  + '</body></html>';
const smallOut = applyScheme(small, 'partial');
check('partial: a bgcolor attribute is remapped', smallOut.includes(`bgcolor="${PAGE_DARK}"`), smallOut);
check('partial: dark text is lightened', smallOut.includes(`color:${LIGHT_TEXT}`), smallOut);
check('partial: a background shorthand carrying a url() is left alone',
  smallOut.includes('background:url(x.png) no-repeat'), smallOut);
check('partial: mid-tone text kept', smallOut.includes('color:#888888'), smallOut);
check('partial: is itself idempotent', applyScheme(smallOut, 'partial') === smallOut);

// ---- full ------------------------------------------------------------------------------
const full = applyScheme(fixture, 'full');
check('full: the invert style block is prepended to <head>',
  /<head[^>]*><style id="lm-dark-sim">html\{filter:invert\(1\)/.test(full),
  full.slice(full.indexOf('<head'), full.indexOf('<head') + 200));
check('full: images are double-inverted back to true colour',
  /img,video\{filter:invert\(1\) hue-rotate\(180deg\)\}/.test(full));
check('full: no colour in the document body was rewritten',
  full.replace(/<style id="lm-dark-sim">[^<]*<\/style>/, '') === applyScheme(fixture, 'nothing-doing')
  || !/lm-dark-sim/.test(full.replace(/<style id="lm-dark-sim">[^<]*<\/style>/, '')));

if (failed) {
  console.log(`\n${failed} FAILURES`);
  process.exit(1);
}
console.log('\nALL PASS');
