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

// 8 full-width buttons in campaign 10 -> 8 mso copies
const msoButtons = [...out.matchAll(/<table role="presentation" width="(\d+)"[^>]*><tbody><tr><td bgcolor="#999999" align="center" style="([^"]*)"><a href="[^"]*"[^>]*style="([^"]*)">([^<]+)<\/a>/g)];
check('8 mso button copies with explicit px width', msoButtons.length === 8, `count=${msoButtons.length} widths=${[...new Set(msoButtons.map((m) => m[1]))]}`);
check('mso td centers and carries padding + border', msoButtons.every((m) => m[2].includes('text-align:center') && /padding:\d+px \d+px \d+px \d+px/.test(m[2]) && /border:\d+px solid/.test(m[2])), msoButtons.map((m) => m[2]).find((s2) => !/text-align:center/.test(s2)));
check('mso anchor stripped of block/padding/border/bg', msoButtons.every((m) => !/display:block|padding:|border:|background-color/.test(m[3])), msoButtons[0] && msoButtons[0][3]);
check('mso anchor keeps text styling', msoButtons.every((m) => /color:#220258/.test(m[3]) && /font-size:20px/.test(m[3])));

// originals preserved and only visible outside mso
const rendered = out.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_, s) =>
  s.replace(/\\x3c/g, '<').replace(/\\x3e/g, '>').replace(/\\"/g, '"').replace(/\\\\/g, '\\'));
const gmailVisible = rendered.replace(/<!--\[if mso\]>[\s\S]*?<!\[endif\]-->/g, '');
const gmailButtons = [...gmailVisible.matchAll(/<a href="https:\/\/listmonk.app"[^>]*style="[^"]*display:block[^"]*"/g)];
check('8 original CSS buttons in gmail-visible content', gmailButtons.length === 8, `count=${gmailButtons.length}`);
const gmailTables = [...gmailVisible.matchAll(/<table role="presentation" width="100%"[^>]*class="lm-gm-pin-(\d+)"><tbody><tr><td data-lm-full-width-button/g)];
check('8 originals stay fluid (width=100%) with per-width Gmail class', gmailTables.length === 8, `count=${gmailTables.length} w=${[...new Set(gmailTables.map((m) => m[1]))]}`);
check('no px pin on the original style', !/lm-gm-pin[^>]*style="[^"]*width:244px/.test(gmailVisible));
check('Gmail-only pin rule injected behind u + .body', /<style>u \+ \.body table\.lm-gm-pin-244\{width:244px!important;max-width:100%!important;margin:0 auto!important\}<\/style>/.test(out));
check('body carries the .body class for the Gmail selector', /<body class="[^"]*body[^"]*">/.test(out));
check('dead data-outlook-cycle rule is gone', !/data-outlook-cycle/.test(out.replace(/data-lm-full-width-button/g, '')));
check('no explicit-width button tables leak into gmail-visible content', !/width="2\d\d"[^>]*><tbody><tr><td bgcolor="#999999" align="center"/.test(gmailVisible));
// Negative: an unmarked lookalike (user HTML) must NOT be transformed
const lookalike = `<!doctype html><html><body><div style="margin:0;min-height:100%;width:100%"><table align="center" width="100%" style="max-width:600px"><tbody><tr><td><table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse"><tbody><tr><td bgcolor="#336699"><a href="https://x.test" style="background-color:#336699;display:block;padding:10px;color:#fff">FAKE</a></td></tr></tbody></table></td></tr></tbody></table></div></body></html>`;
const lk = postProcessForOutlook(lookalike);
check('unmarked lookalike button table left untouched', !/\[if mso\]\\x3e\\x3ctable role=\\"presentation\\" width=\\"\d+\\"/.test(lk) && (lk.match(/FAKE/g) || []).length === 1);
console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
