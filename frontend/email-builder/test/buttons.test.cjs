const path = require('path');
const fs = require('fs');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// Buttons in this fixture predate the data-lm-full-width-button marker that
// buildBulletproofButton now stamps; inject it to mirror a current re-save.
const input = fs.readFileSync(path.join(__dirname, 'fixtures', 'campaign10-body.html'), 'utf8')
  .replace(/<td bgcolor="#999999" style="background-color:#999999;border-radius:4px;">/g,
    '<td data-lm-full-width-button="true" bgcolor="#999999" style="background-color:#999999;border-radius:4px;">');
const out = postProcessForOutlook(input);
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

// 8 full-width buttons in campaign 10 -> 8 mso VML copies (whole surface clickable)
const msoVml = [...out.matchAll(/\{\{ Safe "[^}]*?v:roundrect[^}]*?href=\\"https:\/\/listmonk\.app[^}]*?\}\}/g)];
check('8 mso VML button copies with href on the shape', msoVml.length === 8, `count=${msoVml.length}`);
check('all sized to the column budget 244px -> 183pt', msoVml.every((m) => /width:183pt/.test(m[0])));
const h3675 = msoVml.filter((m) => /height:36.75pt/.test(m[0])).length;
const h3375 = msoVml.filter((m) => /height:33.75pt/.test(m[0])).length;
check('7 standard shapes at 49px -> 36.75pt', h3675 === 7, `count=${h3675}`);
check('EYES shape floored to 40px + 5 border -> 33.75pt', h3375 === 1, `count=${h3375}`);
check('7 strokes #999999 + 1 stroke #e01d1d', msoVml.filter((m) => /strokecolor=\\"#999999\\"/.test(m[0])).length === 7 && msoVml.filter((m) => /strokecolor=\\"#e01d1d\\"/.test(m[0])).length === 1);
check('all fills #999999', msoVml.every((m) => /fillcolor=\\"#999999\\"/.test(m[0])));

// originals preserved and only visible outside mso
const rendered = out.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_, s) =>
  s.replace(/\\x3c/g, '<').replace(/\\x3e/g, '>').replace(/\\"/g, '"').replace(/\\\\/g, '\\'));
const gmailVisible = rendered.replace(/<!--\[if mso\]>[\s\S]*?<!\[endif\]-->/g, '');
const gmailButtons = [...gmailVisible.matchAll(/<a href="https:\/\/listmonk.app"[^>]*style="[^"]*display:block[^"]*"/g)];
check('8 original CSS buttons in gmail-visible content', gmailButtons.length === 8, `count=${gmailButtons.length}`);
const gmailTables = [...gmailVisible.matchAll(/<table role="presentation" width="100%"[^>]*class="lm-gm-pin-(\d+)"><tbody><tr><td data-lm-full-width-button/g)];
check('8 originals stay fluid (width=100%) with per-width Gmail class', gmailTables.length === 8, `count=${gmailTables.length} w=${[...new Set(gmailTables.map((m) => m[1]))]}`);
check('no px pin on the original style', !/lm-gm-pin[^>]*style="[^"]*width:244px/.test(gmailVisible));
check('Gmail-only pin rule injected behind u + .body', /<style>u \+ \.body table\.lm-gm-pin-244\{width:244px!important;max-width:100%!important;margin:0 auto!important\}/.test(out));
// Gmail apps honor px-only widths and iOS ignores the max-width cap, so the
// desktop-budget pin overlaps phone columns without this phone-share rescale
// (campaign 29 matrix, 2026-08-11). 244 * 320/600 = 130.
check('phone breakpoint rescales the pin (244 -> 130)', /@media \(max-width:480px\)\{u \+ \.body table\.lm-gm-pin-244\{width:130px!important\}\}<\/style>/.test(out));
check('body carries the .body class for the Gmail selector', /<body class="[^"]*body[^"]*">/.test(out));
check('dead data-outlook-cycle rule is gone', !/data-outlook-cycle/.test(out.replace(/data-lm-full-width-button/g, '')));
check('no explicit-width button tables leak into gmail-visible content', !/width="2\d\d"[^>]*><tbody><tr><td bgcolor="#999999" align="center"/.test(gmailVisible));
// Negative: an unmarked lookalike (user HTML) must NOT be transformed
const lookalike = `<!doctype html><html><body><div style="margin:0;min-height:100%;width:100%"><table align="center" width="100%" style="max-width:600px"><tbody><tr><td><table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse"><tbody><tr><td bgcolor="#336699"><a href="https://x.test" style="background-color:#336699;display:block;padding:10px;color:#fff">FAKE</a></td></tr></tbody></table></td></tr></tbody></table></div></body></html>`;
const lk = postProcessForOutlook(lookalike);
check('unmarked lookalike button table left untouched', !/\[if mso\]\\x3e\\x3ctable role=\\"presentation\\" width=\\"\d+\\"/.test(lk) && (lk.match(/FAKE/g) || []).length === 1);
console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
