const path = require('path');
const fs = require('fs');
const { JSDOM } = require('jsdom');

const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;

const { postProcess } = require(path.join(__dirname, '.build', 'postProcess.cjs'));

const input = fs.readFileSync(path.join(__dirname, 'fixtures', 'campaign10-body.html'), 'utf8');
const output = postProcess(input, { outlook: true });
fs.writeFileSync(path.join(__dirname, '.build', 'campaign10-fixed.html'), output);

const checks = [];
function check(name, ok, detail) {
  checks.push({ name, ok, detail });
}

const MSO_OPEN = '{{ Safe "\\x3c!--[if\\x20mso]\\x3e" }}';
const MSO_CLOSE = '{{ Safe "\\x3c![endif]--\\x3e" }}';
// CAMPAIGN-52-HARDENING D1: the non-Word twin is no longer a downlevel-revealed
// conditional (T-Online strips those wholesale) but an unconditional mso-hide:all block.
const NONMSO_OPEN = '<div class="lm-nomso" style="mso-hide:all">';
const NONMSO_CLOSE = '</div>';

// 1. Ghost table wraps the canvas
const ghostOpen = output.match(/\{\{ Safe "\\x3c!--\[if\\x20mso\]\\x3e\\x3ctable\\x20[^}]*width=\\"600\\"[^}]*\}\}/);
check('mso ghost table opener (width=600) present', !!ghostOpen);
check('mso ghost table closer present', /\{\{ Safe "\\x3c!--\[if\\x20mso\]\\x3e\\x3c\/td\\x3e\\x3c\/tr\\x3e\\x3c\/table\\x3e\\x3c!\[endif\]--\\x3e" \}\}/.test(output));

// 2. Backdrop table carries bgcolor and padding
check('backdrop td with bgcolor #edd8d8 + padding', /<td align="center" bgcolor="#edd8d8" style="background-color:#edd8d8;padding:32px 0px 32px 0px">/.test(output));
check('backdrop div no longer carries padding', !/<div style="background-color:#edd8d8[^"]*padding:/.test(output));

// 3. Clamping: ONE copy per over-wide image, at the clamped width, for every client
// (2026-09-25: the unclamped non-Word twin made the Gmail apps overflow the row).
const widths = [...output.matchAll(/<img[^>]*width="(\d+)"/g)].map((m) => Number(m[1]));
const countOf = (w) => widths.filter((x) => x === w).length;
check('6 clamped 270px grid images', countOf(270) === 6, `270=${countOf(270)}`);
check('no 300px grid original survives', countOf(300) === 0, `300=${countOf(300)}`);
check('4 clamped 150px footer images', countOf(150) === 4, `150=${countOf(150)}`);
check('no 200px footer original survives', countOf(200) === 0, `200=${countOf(200)}`);
check('hero images stay single 600px (never clamped)', countOf(600) === 3, `600=${countOf(600)}`);

// 4. No image rides in a conditional or an mso-hide wrapper any more
// (D1 still holds: no downlevel-revealed conditional anywhere in the output).
check('no downlevel-revealed conditional in the output', !output.includes('!mso'));
check('no image inside an [if mso] block', output.indexOf(MSO_OPEN + '<img') === -1);
check('no image inside an mso-hide:all wrapper', output.indexOf(NONMSO_OPEN + '<img') === -1);

// 5. Clamped image: style width follows, max-width:100% kept as the fluid belt
check('clamped image style width:270px', /<img[^>]*width="270"[^>]*style="[^"]*width:270px/.test(output));
check('clamped image keeps max-width:100%', /<img[^>]*width="270"[^>]*style="[^"]*max-width:100%/.test(output));

// 5b. Padded table-wrapping divs convert to td-carried boxes (Word drops div padding)
check('grid-row wrapper div converted to padded td', /<td style="padding:16px 24px 8px 24px">/.test(output));
check('zero-padding structural wrappers stay divs', /<div style="padding:0px 0px 0px 0px"><table align="center"/.test(output));

// 6. Structure sanity
check('canvas max-width:600px still present once', (output.match(/max-width:600px/g) || []).length === 1);
check('button markup preserved', /border:1px solid #999999">COLLABS<\/a>/.test(output));
check('only the head [if mso] block is a literal comment', (output.match(/<!--\[if mso\]>/g) || []).length === 1);

// 7. Go-render simulation: substitute Safe blocks, then confirm balanced conditionals
const rendered = output.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_, s) =>
  s.replace(/\\x3c/g, '<').replace(/\\x3e/g, '>').replace(/\\x20/g, ' ').replace(/\\x09/g, '\t').replace(/\\x26/g, '&').replace(/\\"/g, '"').replace(/\\\\/g, '\\'));
const opens = (rendered.match(/<!--\[if mso\]>/g) || []).length;
const closes = (rendered.match(/<!\[endif\]-->/g) || []).length;
check('rendered conditional comments balance', opens === closes, `mso=${opens} end=${closes}`);
check('rendered output has no downlevel-revealed conditional', !/<!--\[if !mso\]>|<!--<!\[endif\]-->/.test(rendered));
check('rendered output has no leftover {{ Safe', !rendered.includes('{{ Safe'));
fs.writeFileSync(path.join(__dirname, '.build', 'campaign10-rendered.html'), rendered);

// 8. Gmail-visible content = strip downlevel-hidden comments; it sees the clamped widths
const gmailVisible = rendered.replace(/<!--\[if mso\]>[\s\S]*?<!\[endif\]-->/g, '');
check('gmail-visible content has all 6 clamped 270px imgs', (gmailVisible.match(/width="270"/g) || []).length === 6);
check('gmail-visible content has all 4 clamped 150px imgs', (gmailVisible.match(/width="150"/g) || []).length === 4);
check('gmail-visible content has no unclamped 300/200 originals', !/width="300"|width="200"/.test(gmailVisible));

let failed = 0;
for (const c of checks) {
  if (!c.ok) failed++;
  console.log(`${c.ok ? 'PASS' : 'FAIL'}  ${c.name}${c.detail ? '  [' + c.detail + ']' : ''}`);
}
console.log(failed === 0 ? '\nALL PASS' : `\n${failed} FAILURES`);
process.exit(failed === 0 ? 0 : 1);
