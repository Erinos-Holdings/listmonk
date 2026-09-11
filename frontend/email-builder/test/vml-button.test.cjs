const path = require('path');
const { JSDOM } = require('jsdom');
global.DOMParser = new JSDOM('<!doctype html>').window.DOMParser;
const { postProcess } = require(path.join(__dirname, '.build', 'postProcess.cjs'));

// Custom width+height button (VML branch): inline-block anchor, explicit width,
// line-height-based height, thick border — mirrors the "Outlook Test" button.
const input = `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
<div style="text-align:center;padding:16px 24px 16px 24px">
<a href="https://x.test/go" target="_blank" style="color:#FFFFFF;font-size:16px;font-weight:bold;background-color:#2563EB;border-radius:64px;display:inline-block;padding:0px 0px 0px 0px;text-decoration:none;width:200px;box-sizing:border-box;text-align:center;line-height:19px;border:5px solid #DC2626;white-space:nowrap">Outlook Test</a>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;

const out = postProcess(input, { outlook: true });
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

check('VML rides inside a Safe template (no raw v:roundrect in stored body)', !/<v:roundrect/.test(out));
check('anchorlock stays self-closing inside the Safe payload', /\\x3cw:anchorlock\/\\x3e\\x3ccenter/.test(out), (out.match(/anchorlock[^\\]*/) || [])[0]);
// Click tracking: the href VALUE rides OUTSIDE the Safe payload as a marker between two
// halves, so the Go compile transform can wrap it in TrackLink (static → tracked; a merge
// field → resolved per subscriber). Both halves and the marker must be adjacent.
check('conditional markers and VML in two Safe halves around the href marker',
  /\{\{ Safe "\\x3c!--\[if\\x20mso\]\\x3e\\x3cv:roundrect[^}]*href=\\"" \}\}<span data-lm-vml-href="https:\/\/x\.test\/go"><\/span>\{\{ Safe "\\"[^}]*\\x3c!\[endif\]--\\x3e" \}\}/.test(out));
check('no raw href value inside either Safe payload', !/\{\{ Safe "[^}]*x\.test\/go[^}]*\}\}/.test(out));
const h = out.match(/height:([\d.]+)pt;v-text-anchor/);
check('height floored at 2x font: 32px + 5 border = 37px -> 27.75pt', h && h[1] === '27.75', h && `h=${h[1]}`);
const w = out.match(/v-text-anchor:middle;width:([\d.]+)pt/);
check('explicit width 200px -> 150pt', w && w[1] === '150', w && `w=${w[1]}`);

// Simulated Go render: Safe payloads decode, and the marker becomes its (entity-decoded)
// value — what the Go transform's TrackLink call renders to for a static URL with tracking
// off; with tracking on it is a /link/ URL, structurally identical for these assertions.
const rendered = out.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_, s) =>
  s.replace(/\\x3c/g, '<').replace(/\\x3e/g, '>').replace(/\\x20/g, ' ').replace(/\\x09/g, '\t').replace(/\\x26/g, '&').replace(/\\"/g, '"').replace(/\\\\/g, '\\'))
  .replace(/<span data-lm-vml-href="([^"]*)"><\/span>/g, (_, v) => v.replace(/&quot;/g, '"').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&'));
check('rendered VML carries the href on the shape', /<v:roundrect[^>]*href="https:\/\/x\.test\/go"/.test(rendered));
check('canonical: anchorlock then center, NO textbox', /<w:anchorlock\/><center/.test(rendered) && !/v:textbox/.test(rendered));
check('canonical: no line-height pin on the center', !/mso-line-height-rule/.test(rendered));
check('v-text-anchor:middle present for vertical centering', /v-text-anchor:middle/.test(rendered));
check('stroke in pt: 5px -> 3.75pt', /strokeweight="3.75pt"/.test(rendered));
check('font in pt: 16px -> 12pt', /font-size:12pt/.test(rendered));
check('rendered VML inside one [if mso] conditional', /<!--\[if mso\]><v:roundrect[\s\S]*?<\/v:roundrect><!\[endif\]-->/.test(rendered));
// D1: the CSS anchor rides unconditionally inside an mso-hide:all block, never a
// downlevel-revealed conditional.
check('CSS anchor intact inside the mso-hide:all twin', /<div class="lm-nomso" style="mso-hide:all"><a href="https:\/\/x.test\/go"[^>]*white-space:nowrap[^>]*>Outlook Test<\/a><\/div>/.test(rendered));
check('no downlevel-revealed conditional', !rendered.includes('!mso'));
// Explicit sub-floor height must also be floored (invisible-label guard)
const tiny = input.replace('line-height:19px;', 'line-height:19px;height:20px;');
const tinyOut = postProcess(tiny, { outlook: true });
const th = tinyOut.match(/height:([\d.]+)pt;v-text-anchor/);
check('explicit 20px height floored to 2x font: 32px -> 24pt', th && th[1] === '24', th && `h=${th[1]}`);

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
