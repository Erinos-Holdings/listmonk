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

check('VML rides inside a Safe template (no raw v:group / v:roundrect in stored body)', !/<v:(group|roundrect)/.test(out));
// The self-closed VML elements of the textpath shape (BIBLE-OUTLOOK-FIXES-SPEC §4.3) must stay
// self-closing: emitted raw, the fragment parser would open them and swallow what follows.
check('self-closed VML elements stay self-closing inside the Safe payload',
  /fillcolor=\\"#2563EB\\"\/\\x3e/.test(out) && /\\x3cv:path\\x20textpathok=\\"t\\"\/\\x3e/.test(out), (out.match(/v:path[^\\]*/) || [])[0]);
// Click tracking: the href VALUE rides OUTSIDE the Safe payload as a marker. The textpath shape
// carries the href on BOTH shapes, so there are two markers between three Safe parts, adjacent.
check('conditional markers and VML in three Safe parts around two href markers',
  /\{\{ Safe "\\x3c!--\[if\\x20mso\]\\x3e\\x3cv:group[^}]*\\x3cv:roundrect\\x20href=\\"" \}\}<span data-lm-vml-href="https:\/\/x\.test\/go"><\/span>\{\{ Safe "\\"[^}]*\\x3cv:shape\\x20href=\\"" \}\}<span data-lm-vml-href="https:\/\/x\.test\/go"><\/span>\{\{ Safe "\\"[^}]*\\x3c!\[endif\]--\\x3e" \}\}/.test(out));
check('no raw href value inside any Safe payload', !/\{\{ Safe "[^}]*x\.test\/go[^}]*\}\}/.test(out));
const h = out.match(/;height:([\d.]+)pt\\"\\x20coordorigin/);
check('height floored at 2x font: 32px + 5 border = 37px -> 27.75pt', h && h[1] === '27.75', h && `h=${h[1]}`);
const w = out.match(/style=\\"width:([\d.]+)pt;height/);
check('explicit width 200px -> 150pt', w && w[1] === '150', w && `w=${w[1]}`);
check('coordinate space is the px box x 10 (2000 x 370)', /coordsize=\\"2000,370\\"/.test(out));

// Simulated Go render: Safe payloads decode, and the marker becomes its (entity-decoded)
// value — what the Go transform's TrackLink call renders to for a static URL with tracking
// off; with tracking on it is a /link/ URL, structurally identical for these assertions.
const rendered = out.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_, s) =>
  s.replace(/\\x3c/g, '<').replace(/\\x3e/g, '>').replace(/\\x20/g, ' ').replace(/\\x09/g, '\t').replace(/\\x26/g, '&').replace(/\\"/g, '"').replace(/\\\\/g, '\\'))
  .replace(/<span data-lm-vml-href="([^"]*)"><\/span>/g, (_, v) => v.replace(/&quot;/g, '"').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&'));
check('rendered VML carries the href on the roundrect AND the text shape',
  /<v:roundrect href="https:\/\/x\.test\/go"/.test(rendered) && /<v:shape href="https:\/\/x\.test\/go"/.test(rendered));
check('textpath label: no anchorlock, no center, NO textbox', /<v:textpath [^>]*string="Outlook Test"/.test(rendered) && !/anchorlock|<center|v:textbox/.test(rendered));
check('no line-height pin', !/mso-line-height-rule/.test(rendered));
check('stroke in pt: 5px -> 3.75pt', /strokeweight="3.75pt"/.test(rendered));
check('font in pt: 16px -> 12pt', /font-size:12pt/.test(rendered));
check('rendered VML inside one [if mso] conditional', /<!--\[if mso\]><v:group[\s\S]*?<\/v:group><!\[endif\]-->/.test(rendered));
// D1: the CSS anchor rides unconditionally inside an mso-hide:all block, never a
// downlevel-revealed conditional.
check('CSS anchor intact inside the mso-hide:all twin', /<div class="lm-nomso" style="mso-hide:all"><a href="https:\/\/x.test\/go"[^>]*white-space:nowrap[^>]*>Outlook Test<\/a><\/div>/.test(rendered));
check('no downlevel-revealed conditional', !rendered.includes('!mso'));
// Explicit sub-floor height must also be floored (invisible-label guard)
const tiny = input.replace('line-height:19px;', 'line-height:19px;height:20px;');
const tinyOut = postProcess(tiny, { outlook: true });
const th = tinyOut.match(/;height:([\d.]+)pt\\"\\x20coordorigin/);
check('explicit 20px height floored to 2x font: 32px -> 24pt', th && th[1] === '24', th && `h=${th[1]}`);

// BIBLE-OUTLOOK-FIXES-SPEC §12 (IA8): the same Button with a label outside the VML label set
// (U+2192) falls back to the table-cell button: one href marker between two Safe parts, no VML,
// the cell width content-box (200 - 0 padding - 2 x 5 border = 190), and — unlike the VML shape —
// no 2x-font floor on an explicit height (20 - 2 x 5 = 10).
const fb = postProcess(tiny.replace('Outlook Test', 'Outlook Test →'), { outlook: true });
check('fallback: the stamped table in two Safe parts around one href marker',
  /\{\{ Safe "\\x3c!--\[if\\x20mso\]\\x3e\\x3ctable\\x20[^}]*data-lm-btn-fallback=\\"chars\\"[^}]*\\x3ca\\x20href=\\"" \}\}<span data-lm-vml-href="https:\/\/x\.test\/go"><\/span>\{\{ Safe "\\"[^}]*\\x3c!\[endif\]--\\x3e" \}\}/.test(fb)
    && (fb.match(/<span data-lm-vml-href=/g) || []).length === 1);
check('fallback: no VML element anywhere', !/v:group|v:roundrect|v:textpath|anchorlock/.test(fb));
check('fallback: W 200, w 190, h 10 (no Word shape floor on a cell)', /width=\\"200\\"/.test(fb) && /width=\\"190\\"\\x20height=\\"10\\"\\x20valign=\\"middle\\"/.test(fb));
check('fallback: the mso-hide:all twin is unchanged', /<div class="lm-nomso" style="mso-hide:all"><a href="https:\/\/x.test\/go"[^>]*white-space:nowrap[^>]*>Outlook Test →<\/a><\/div>/.test(fb));

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
