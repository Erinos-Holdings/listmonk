const path = require('path');
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.DOMParser = dom.window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// Image block alignment (campaign 48 footer logo, 2026-09-03). transformImageBlocks
// wraps the image in a shrink-wrap inner table inside a full-width outer table; the
// block's text-align became `align` on both TDs only. Word centers a nested table from
// the parent cell, but Gmail web, both Gmail apps and Outlook mobile lay the inner table
// out as a block at the left edge. Pins:
//   center        → the inner (shrink-wrap) table carries align="center" itself, and the
//                   outer td keeps align="center" for Word;
//   right         → the DOM inner table carries NO align (table align="right" is a float and
//                   Word drew the campaign 51 curling iron at the LEFT); the outer td keeps
//                   align="right" for Word, and a non-mso-only self-aligning wrapper table
//                   (align="right", in conditional Safe payloads) surrounds the inner table
//                   for Gmail web/apps and Outlook mobile;
//   left/unset    → no align attribute on the inner table (left is the default flow, and a
//                   stamped align="left" is the hazard commit a138c8f6 removed);
//   scope guards  → an oversized centered image still dual-emits inside the aligned table,
//                   and a Container holding a centered image does NOT derive align="center"
//                   on its own td (the self-centering-child rule sees the width="100%"
//                   outer table, never the aligned inner one).
let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

function render(inner) {
  return postProcessForOutlook(`<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
${inner}
</td></tr></tbody></table>
</div>
</body></html>`);
}
function block(wrapperStyle, inner) { return `<div style="${wrapperStyle}">${inner}</div>`; }

// Structural lookup, independent of attribute order: the inner table is the closest
// role=presentation table around the image; the outer td is its containing cell.
function imageTables(out, alt) {
  const doc = new JSDOM(out).window.document;
  const img = doc.querySelector(`img[alt="${alt}"]`);
  if (!img) return null;
  const inner = img.closest('table[role="presentation"]');
  const outerTd = inner && inner.parentElement.closest('td');
  return { img, inner, outerTd, innerAlign: inner && inner.getAttribute('align'), innerWidth: inner && inner.getAttribute('width') };
}

// Conditional markers ride in `{{ Safe "..." }}` payloads (same decoder as vml-button.test.cjs).
function decodeSafe(out) {
  return out.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_, s) =>
    s.replace(/\\x3c/g, '<').replace(/\\x3e/g, '>').replace(/\\x20/g, ' ').replace(/\\x09/g, '\t').replace(/\\x26/g, '&').replace(/\\"/g, '"').replace(/\\\\/g, '\\'));
}

const logo = '<img alt="logo" src="x://logo.png" height="62" style="max-width:100%;height:62px">';
const linked = `<a href="https://x.test" style="text-decoration:none;display:inline-block;border:0" target="_blank">${logo}</a>`;

for (const [label, inner] of [['bare img', logo], ['linked img', linked]]) {
  const c = imageTables(render(block('padding:16px 0px 0px 0px;text-align:center', inner)), 'logo');
  check(`${label} center: inner table is shrink-wrap (no width)`, c && c.innerWidth === null, c && c.innerWidth);
  check(`${label} center: inner table aligns itself (align="center")`, c && c.innerAlign === 'center', c && String(c.innerAlign));
  check(`${label} center: outer td still carries align="center" for Word`, c && c.outerTd && c.outerTd.getAttribute('align') === 'center');

  const rightOut = render(block('padding:0px;text-align:right', inner));
  const r = imageTables(rightOut, 'logo');
  check(`${label} right: DOM inner table carries NO align (Word positions it from the td)`, r && r.innerAlign === null, r && String(r.innerAlign));
  check(`${label} right: outer td carries align="right"`, r && r.outerTd && r.outerTd.getAttribute('align') === 'right');
  check(`${label} right: no table in the DOM carries align="right"`, !/<table role="presentation"[^>]* align="right"/.test(rightOut));
  const decoded = decodeSafe(rightOut);
  const wrapperRe = /<!--\[if !mso\]><!--><table role="presentation" align="right" cellpadding="0" cellspacing="0" border="0" style="[^"]*"><tr><td align="right"><!--<!\[endif\]--><table role="presentation" cellpadding="0" cellspacing="0" border="0" style="[^"]*"><tbody><tr><td align="right">[\s\S]*?<\/td><\/tr><\/tbody><\/table><!--\[if !mso\]><!--><\/td><\/tr><\/table><!--<!\[endif\]-->/;
  check(`${label} right: non-mso self-aligning wrapper (in Safe payloads) surrounds the inner table`, wrapperRe.test(decoded));
  check(`${label} right: the mso branch sees only td align="right" + plain inner table`, /<td align="right" style="[^"]*"><!--\[if !mso\]><!-->/.test(decoded));

  const l = imageTables(render(block('padding:0px;text-align:left', inner)), 'logo');
  check(`${label} left: inner table carries NO align attribute`, l && l.innerAlign === null, l && String(l.innerAlign));

  const u = imageTables(render(block('padding:0px', inner)), 'logo');
  check(`${label} unset: inner table carries NO align attribute`, u && u.innerAlign === null, u && String(u.innerAlign));
}

// A width-sized full-bleed hero (campaign 48 header) is left/unset and must be untouched.
{
  const out = render(block('padding:0px', '<img alt="hero" src="x://hero.jpg" width="600" style="width:600px;max-width:100%">'));
  const h = imageTables(out, 'hero');
  check('full-width hero: inner table carries NO align attribute', h && h.innerAlign === null, h && String(h.innerAlign));
  check('full-width hero: no image table anywhere carries align', !/<table role="presentation"[^>]* align=/.test(out));
}

// Scope guard 1: an oversized CENTERED image still dual-emits (mso clamped copy + non-mso
// original) inside the now-aligned inner table.
{
  const out = render(block('padding:16px 24px 16px 24px;text-align:center', '<img alt="wide" src="x://wide.jpg" width="900" style="width:900px;max-width:100%">'));
  const copies = out.match(/<img[^>]*alt="wide"[^>]*>/g) || [];
  const widths = copies.map((t) => (t.match(/ width="(\d+)"/) || [])[1]);
  check('oversized centered image: dual-emitted (2 copies)', copies.length === 2, String(copies.length));
  check('oversized centered image: one copy clamped to the block space (552), one original (900)', widths.includes('552') && widths.includes('900'), widths.join(','));
  const w = imageTables(out, 'wide');
  check('oversized centered image: the dual emit sits inside the aligned inner table', w && w.innerAlign === 'center', w && String(w.innerAlign));
}

// Scope guard 2: a padded Container holding a centered image must NOT derive align="center"
// on the Container's own td — the a138c8f6 self-centering-child rule only fires for a sole
// shrink-wrap table child, and the image's outer table is width="100%".
{
  const out = render(`<div style="padding:24px 24px 24px 24px;background-color:#fafafa">${block('padding:0px;text-align:center', logo)}</div>`);
  const doc = new JSDOM(out).window.document;
  const containerTd = Array.from(doc.querySelectorAll('td')).find((td) => /padding:24px 24px 24px 24px/.test(td.getAttribute('style') || ''));
  check('container around a centered image: container td exists', !!containerTd);
  check('container around a centered image: container td derives NO align', containerTd && containerTd.getAttribute('align') === null, containerTd && String(containerTd.getAttribute('align')));
  const c = imageTables(out, 'logo');
  check('container around a centered image: inner image table still align="center"', c && c.innerAlign === 'center');
}

if (failed) { console.error(`${failed} check(s) failed`); process.exit(1); }
