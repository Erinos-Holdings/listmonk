const path = require('path');
const fs = require('fs');
const { JSDOM } = require('jsdom');

const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;

const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

const input = fs.readFileSync(path.join(__dirname, 'fixtures', 'campaign10-body.html'), 'utf8');
const output = postProcessForOutlook(input);
fs.writeFileSync(path.join(__dirname, '.build', 'campaign10-fixed.html'), output);

const checks = [];
function check(name, ok, detail) {
  checks.push({ name, ok, detail });
}

const MSO_OPEN = '{{ Safe "\\x3c!--[if mso]\\x3e" }}';
const MSO_CLOSE = '{{ Safe "\\x3c![endif]--\\x3e" }}';
const NONMSO_OPEN = '{{ Safe "\\x3c!--[if !mso]\\x3e\\x3c!--\\x3e" }}';
const NONMSO_CLOSE = '{{ Safe "\\x3c!--\\x3c![endif]--\\x3e" }}';

// 1. Ghost table wraps the canvas
const ghostOpen = output.match(/\{\{ Safe "\\x3c!--\[if mso\]\\x3e\\x3ctable [^}]*width=\\"600\\"[^}]*\}\}/);
check('mso ghost table opener (width=600) present', !!ghostOpen);
check('mso ghost table closer present', /\{\{ Safe "\\x3c!--\[if mso\]\\x3e\\x3c\/td\\x3e\\x3c\/tr\\x3e\\x3c\/table\\x3e\\x3c!\[endif\]--\\x3e" \}\}/.test(output));

// 2. Backdrop table carries bgcolor and padding
check('backdrop td with bgcolor #edd8d8 + padding', /<td align="center" bgcolor="#edd8d8" style="background-color:#edd8d8;padding:32px 0px 32px 0px">/.test(output));
check('backdrop div no longer carries padding', !/<div style="background-color:#edd8d8[^"]*padding:/.test(output));

// 3. Dual-emit clamping: mso copy clamped, non-mso copy original
const widths = [...output.matchAll(/<img[^>]*width="(\d+)"/g)].map((m) => Number(m[1]));
const countOf = (w) => widths.filter((x) => x === w).length;
check('6 clamped 270px grid copies (mso)', countOf(270) === 6, `270=${countOf(270)}`);
check('6 original 300px grid copies (non-mso)', countOf(300) === 6, `300=${countOf(300)}`);
check('4 clamped 150px footer copies (mso)', countOf(150) === 4, `150=${countOf(150)}`);
check('4 original 200px footer copies (non-mso)', countOf(200) === 4, `200=${countOf(200)}`);
check('hero images stay single 600px (not dual-emitted)', countOf(600) === 3, `600=${countOf(600)}`);

// 4. Pairing structure: [if mso]<img clamped>[endif] [if !mso]<img original>[endif]
const pair = new RegExp(
  [
    MSO_OPEN, '<img[^>]*width="270"[^>]*>', MSO_CLOSE,
    NONMSO_OPEN, '<img[^>]*width="300"[^>]*>', NONMSO_CLOSE,
  ].map((s) => s.replace(/[.*+?^${}()|[\]\\]/g, (c) => ('<img'.includes(c) ? c : '\\' + c))).join('')
);
// simpler literal scan: find one full dual block
const idx = output.indexOf(MSO_OPEN + '<img');
const block = idx >= 0 ? output.slice(idx, idx + 1600) : '';
check('dual block order: mso-clamped then !mso-original',
  idx >= 0 && block.indexOf('width="270"') !== -1
    && block.indexOf(MSO_CLOSE) > block.indexOf('width="270"')
    && block.indexOf(NONMSO_OPEN) > block.indexOf(MSO_CLOSE)
    && block.indexOf('width="300"') > block.indexOf(NONMSO_OPEN)
    && block.indexOf(NONMSO_CLOSE) > block.indexOf('width="300"'));

// 5. Clamped copy: style width follows, non-mso copy style untouched
check('clamped copy style width:270px', /<img[^>]*width="270"[^>]*style="[^"]*width:270px/.test(output));
check('original copy keeps style width:300px', /<img[^>]*width="300"[^>]*style="[^"]*width:300px/.test(output));
check('original copy keeps max-width:100%', /<img[^>]*width="300"[^>]*style="[^"]*max-width:100%/.test(output));

// 6. Structure sanity
check('canvas max-width:600px still present once', (output.match(/max-width:600px/g) || []).length === 1);
check('button markup preserved', /border:1px solid #999999">COLLABS<\/a>/.test(output));
check('only the head [if mso] block is a literal comment', (output.match(/<!--\[if mso\]>/g) || []).length === 1);

// 7. Go-render simulation: substitute Safe blocks, then confirm balanced conditionals
const rendered = output.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_, s) =>
  s.replace(/\\x3c/g, '<').replace(/\\x3e/g, '>').replace(/\\"/g, '"').replace(/\\\\/g, '\\'));
const opens = (rendered.match(/<!--\[if mso\]>/g) || []).length;
const closes = (rendered.match(/<!\[endif\]-->/g) || []).length;
const nonMsoOpens = (rendered.match(/<!--\[if !mso\]><!-->/g) || []).length;
const nonMsoCloses = (rendered.match(/<!--<!\[endif\]-->/g) || []).length;
check('rendered conditional comments balance', opens === closes - nonMsoCloses && nonMsoOpens === nonMsoCloses, `mso=${opens} !mso=${nonMsoOpens} end=${closes - nonMsoCloses}+${nonMsoCloses}`);
check('rendered output has no leftover {{ Safe', !rendered.includes('{{ Safe'));
fs.writeFileSync(path.join(__dirname, '.build', 'campaign10-rendered.html'), rendered);

// 8. Gmail-visible content = strip downlevel-hidden comments, must contain NO clamped widths
const gmailVisible = rendered.replace(/<!--\[if mso\]>[\s\S]*?<!\[endif\]-->/g, '');
check('gmail-visible content has no 270px imgs', !/width="270"/.test(gmailVisible));
check('gmail-visible content keeps all 300px originals', (gmailVisible.match(/width="300"/g) || []).length === 6);
check('gmail-visible content keeps all 200px originals', (gmailVisible.match(/width="200"/g) || []).length === 4);

let failed = 0;
for (const c of checks) {
  if (!c.ok) failed++;
  console.log(`${c.ok ? 'PASS' : 'FAIL'}  ${c.name}${c.detail ? '  [' + c.detail + ']' : ''}`);
}
console.log(failed === 0 ? '\nALL PASS' : `\n${failed} FAILURES`);
process.exit(failed === 0 ? 0 : 1);
