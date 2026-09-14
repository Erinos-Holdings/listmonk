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

// I7 (BUTTON-DARK-MODE-SPEC): the accept/reject table is mirrored by Go's
// TestButtonDarkParseColor (internal/manager/button_dark_lint_test.go). A loosening on
// either side must fail here as well as there.
const parseOk = {
  '#fff': [255, 255, 255], '#FFF': [255, 255, 255], '#000000': [0, 0, 0], '#F5F5F5': [245, 245, 245],
  '  #e54582 ': [229, 69, 130], 'rgb(255, 255, 255)': [255, 255, 255], 'rgb(0,0,0)': [0, 0, 0],
  'rgb(1 2 3)': [1, 2, 3], 'rgba(0,0,0,0.9)': [0, 0, 0], 'rgba(10, 20, 30, .5)': [10, 20, 30],
  'RGB(0,0,0)': [0, 0, 0],
};
Object.entries(parseOk).forEach(([input, [r, g, b]]) => {
  const got = parseCssColor(input);
  check(`I7: parseCssColor accepts ${JSON.stringify(input)}`,
    got !== null && got.r === r && got.g === g && got.b === b, JSON.stringify(got));
});
['rgb(300,0,0)', '#00000080', '#0008', '#ff', 'black', 'transparent', 'inherit',
  'linear-gradient(#fff,#000)', '', '#gggggg'].forEach((input) => {
  check(`I7: parseCssColor rejects ${JSON.stringify(input)}`, parseCssColor(input) === null);
});
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
// BUTTON-DARK-MODE-SPEC I2. The previous form was `A || B` with B always true once the style
// block was stripped, so it passed on any output. Compare the parsed <body> of full against
// light (the input, parsed the same way) on a fixture that carries a dark border and colour --
// exactly what the partial pass rewrites -- and prove the comparison can fail by running it on
// partial too.
const bodyOf = (html) => new JSDOM(html).window.document.body.innerHTML;
const borderFixture = '<!doctype html><html><head><title>t</title></head><body>'
  + '<table bgcolor="#ffffff"><tr><td style="background-color:#f5f5f5;color:#000000;border:2px solid #000000">'
  + '<a style="color:#111111;border-bottom:1px solid rgb(0, 0, 0)" href="https://x.test">go</a></td></tr></table>'
  + '</body></html>';
check('full (I2): the document body is identical to light on a dark border/colour fixture',
  bodyOf(applyScheme(borderFixture, 'full')) === bodyOf(applyScheme(borderFixture, 'light')));
check('full (I2): identical to light on template 29 too',
  bodyOf(full) === bodyOf(applyScheme(fixture, 'light')));
check('full (I2): the comparison is not vacuous -- partial fails it',
  bodyOf(applyScheme(borderFixture, 'partial')) !== bodyOf(applyScheme(borderFixture, 'light')));

// ---- borders under partial (BUTTON-DARK-MODE-SPEC D1, I1) --------------------------------
// The style attribute of the single <td> after a partial pass.
function partialStyle(style) {
  const out = applyScheme(`<!doctype html><html><body><table><tr><td style="${style}">x</td></tr></table></body></html>`, 'partial');
  return new JSDOM(out).window.document.querySelector('td').getAttribute('style');
}
const borderCases = [
  ['border:2px solid #000000', `border:2px solid ${LIGHT_TEXT}`],
  ['border-color:#000', `border-color:${LIGHT_TEXT}`],
  ['border-top:1px solid rgb(0, 0, 0)', `border-top:1px solid ${LIGHT_TEXT}`],
  ['border-color:#000 #fff', `border-color:${LIGHT_TEXT} #fff`],
  ['border:1px solid transparent', 'border:1px solid transparent'],
  ['border:1px solid #00000080', 'border:1px solid #00000080'],
  ['border-color:#0008', 'border-color:#0008'],
  ['border-radius:64px', 'border-radius:64px'],
  ['border-radius:64px;border-collapse:collapse', 'border-radius:64px;border-collapse:collapse'],
  ['border-radius:64px;border:2px solid #000000', `border-radius:64px;border:2px solid ${LIGHT_TEXT}`],
  ['border:2px solid #777777', 'border:2px solid #777777'],
  ['border: 2px  dashed #000000 ', `border: 2px  dashed ${LIGHT_TEXT} `],
  ['border:none', 'border:none'],
  ['border:1px solid rgba(0,0,0,0.5)', `border:1px solid ${LIGHT_TEXT}`],
  ['border:1px solid rgb(300, 0, 0)', 'border:1px solid rgb(300, 0, 0)'],
  ['border-top:1px solid #000;border-top-color:#111', `border-top:1px solid ${LIGHT_TEXT};border-top-color:${LIGHT_TEXT}`],
  ['color:#000000;background-color:#ffffff;border:2px solid #000000',
    `color:${LIGHT_TEXT};background-color:${PAGE_DARK};border:2px solid ${LIGHT_TEXT}`],
];
['border-right', 'border-bottom', 'border-left'].forEach((p) => {
  borderCases.push([`${p}:1px solid #000`, `${p}:1px solid ${LIGHT_TEXT}`]);
});
['border-top-color', 'border-right-color', 'border-bottom-color', 'border-left-color'].forEach((p) => {
  borderCases.push([`${p}:#000000`, `${p}:${LIGHT_TEXT}`]);
});
borderCases.forEach(([input, want]) => {
  const got = partialStyle(input);
  check(`I1: partial "${input}" -> "${want}"`, got === want, got);
});

const borderDoc = '<!doctype html><html><body><table><tr><td style="border:2px solid #000000;border-color:#000 #fff">x</td></tr></table></body></html>';
check('I1: light returns a border fixture byte for byte', applyScheme(borderDoc, 'light') === borderDoc);
const borderOnce = applyScheme(borderDoc, 'partial');
check('I1: the border pass is idempotent', applyScheme(borderOnce, 'partial') === borderOnce, borderOnce);

if (failed) {
  console.log(`\n${failed} FAILURES`);
  process.exit(1);
}
console.log('\nALL PASS');
