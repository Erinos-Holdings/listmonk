// BIBLE-OUTLOOK-FIXES-SPEC I2-I4 (§4.2, defects C and D). Word ignores padding on a heading and
// the Word font fallback never reached a Heading block, so with the Outlook flag on every Heading
// block (top level, in a Container, in a column) compiles to ONE presentation table whose cell
// carries the padding, the background and the explicit alignment; the heading keeps margin:0 and
// carries no padding, and carries its own Word font wrapper INSIDE itself exactly when its
// effective stack leads with a family off WORD_FONT_ALLOWLIST. A hand-typed heading inside a Text
// block's markdown, and anything inside a user Html fence, is left alone. Client behaviour is
// gates G1/G5/G6, not this suite.
const { JSDOM, pp, canvas, decodeSafe, makeChecker, ARIAL } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

const OPEN = '<!--[if mso]><font face="Arial"><![endif]-->';
const CLOSE = '<!--[if mso]></font><![endif]-->';

function styleMap(el) {
  return (el && el.getAttribute('style') || '').split(';').map((d) => d.trim()).filter(Boolean).reduce((acc, d) => {
    const i = d.indexOf(':');
    acc[d.slice(0, i).trim().toLowerCase()] = d.slice(i + 1).trim();
    return acc;
  }, {});
}
// Compiled (Outlook flag on), then Safe payloads decoded, parsed: what Word's parser sees once
// Go has rendered the template.
function compile(inner, outlook = true) {
  const raw = pp.postProcess(canvas(inner), { outlook });
  const decoded = decodeSafe(raw);
  return { raw, decoded, doc: new JSDOM(raw).window.document };
}
const headingNamed = (doc, text) => Array.from(doc.querySelectorAll('h1, h2, h3, h4, h5, h6')).find((h) => h.textContent.includes(text)) || null;
// The heading's own cell and the presentation table around it, when it was converted.
function cellOf(h) {
  const td = h && h.parentElement;
  if (!td || td.tagName !== 'TD') return null;
  const table = td.closest('table');
  return { td, table, onlyChild: td.children.length === 1, rows: table.querySelectorAll(':scope > tbody > tr').length };
}

// The upstream Heading block's shape: React's inline style, margin:0 among it.
const heading = (text, style, level = 'h2') => `<${level} style="${style}">${text}</${level}>`;
const H_PADDED = 'background-color:#FFEECC;font-weight:bold;text-align:center;margin:0;font-size:24px;padding:16px 24px 8px 32px';

// ---- I2: top level ---------------------------------------------------------------------------
{
  const { doc, raw } = compile(heading('Top heading', H_PADDED));
  const h = headingNamed(doc, 'Top heading');
  const c = cellOf(h);
  check('I2 top level: the heading is the only child of a presentation table cell', c && c.onlyChild && c.rows === 1
    && c.table.getAttribute('role') === 'presentation' && c.table.getAttribute('width') === '100%');
  check('I2 top level: the cell carries the padding (one shorthand), the background and the alignment',
    c && styleMap(c.td).padding === '16px 24px 8px 32px' && styleMap(c.td)['background-color'] === '#FFEECC'
      && styleMap(c.td)['text-align'] === 'center' && c.td.getAttribute('align') === 'center' && c.td.getAttribute('bgcolor') === '#FFEECC',
    c && c.td.outerHTML.slice(0, 200));
  check('I2 top level: the cell style is padding, then background, then alignment', c && c.td.getAttribute('style') === 'padding:16px 24px 8px 32px;background-color:#FFEECC;text-align:center');
  const hs = styleMap(h);
  check('I2 top level: the heading keeps margin:0, its text-align and its font size', hs.margin === '0' && hs['text-align'] === 'center' && hs['font-size'] === '24px', h && h.getAttribute('style'));
  check('I2 top level: the heading carries no padding and no background', !Object.keys(hs).some((k) => k.startsWith('padding')) && !('background-color' in hs), h && h.getAttribute('style'));
  check('I2 top level: nothing of the cell rides in a Safe payload', !/\{\{ Safe "[^}]*padding:16px/.test(raw));
}

// Longhand padding is read too; all-zero padding gives no padding; no alignment is never fabricated.
{
  const { doc } = compile(heading('Longhand heading', 'font-weight:bold;margin:0;font-size:20px;padding-top:4px;padding-left:12px', 'h3')
    + heading('Bare heading', 'font-weight:bold;margin:0;font-size:32px;padding:0px 0px 0px 0px', 'h1'));
  const lc = cellOf(headingNamed(doc, 'Longhand heading'));
  check('I2 longhands: padding-top/-left read into the shorthand', lc && styleMap(lc.td).padding === '4px 0px 0px 12px', lc && lc.td.getAttribute('style'));
  check('I2 longhands: no alignment on the cell when the heading states none', lc && !lc.td.hasAttribute('align') && !('text-align' in styleMap(lc.td)));
  const bc = cellOf(headingNamed(doc, 'Bare heading'));
  check('I2 zero padding: the heading is still converted (one shape)', !!bc);
  check('I2 zero padding: the cell carries no padding, no align, no bgcolor', bc && !bc.td.hasAttribute('style') && !bc.td.hasAttribute('align') && !bc.td.hasAttribute('bgcolor'), bc && bc.td.outerHTML.slice(0, 120));
  check('I2 zero padding: the heading carries no padding', !Object.keys(styleMap(headingNamed(doc, 'Bare heading'))).some((k) => k.startsWith('padding')));
}

// Review fix F1: only px or bare-0 padding moves to the cell, carried AS WRITTEN (never rounded);
// any other unit stays on the heading and the cell gets none.
{
  const { doc } = compile(heading('Em heading', 'font-weight:bold;margin:0;font-size:24px;padding:1em 2em')
    + heading('Fractional heading', 'font-weight:bold;margin:0;font-size:24px;padding:12.5px 24px')
    + heading('Mixed heading', 'font-weight:bold;margin:0;font-size:24px;padding:8px;padding-left:5%')
    + heading('Zero heading', 'font-weight:bold;margin:0;font-size:24px;padding:0 10px')
    + heading('Five heading', 'font-weight:bold;margin:0;font-size:24px;padding:1px 2px 3px 4px 5px')
    + heading('Both heading', 'font-weight:bold;margin:0;font-size:24px;padding:8px;padding-top:0')
    + heading('Upper heading', 'font-weight:bold;margin:0;font-size:24px;padding:10PX 0'));
  const em = headingNamed(doc, 'Em heading');
  const emCell = cellOf(em);
  check('F1: padding:1em 2em stays on the heading', styleMap(em).padding === '1em 2em', em && em.getAttribute('style'));
  check('F1: and that heading\'s cell carries no padding', emCell && !('padding' in styleMap(emCell.td)), emCell && emCell.td.outerHTML.slice(0, 160));
  const frac = cellOf(headingNamed(doc, 'Fractional heading'));
  check('F1: 12.5px is carried to the cell unrounded', frac && styleMap(frac.td).padding === '12.5px 24px 12.5px 24px', frac && frac.td.getAttribute('style'));
  check('F1: and removed from the heading', !Object.keys(styleMap(headingNamed(doc, 'Fractional heading'))).some((k) => k.startsWith('padding')));
  const mixed = headingNamed(doc, 'Mixed heading');
  check('F1: one non-px longhand keeps ALL the heading\'s padding on the heading', styleMap(mixed).padding === '8px' && styleMap(mixed)['padding-left'] === '5%'
    && !('padding' in styleMap(cellOf(mixed).td)), mixed && mixed.getAttribute('style'));
  const zero = cellOf(headingNamed(doc, 'Zero heading'));
  check('F1: a bare 0 token is accepted and written as written', zero && styleMap(zero.td).padding === '0 10px 0 10px', zero && zero.td.getAttribute('style'));
  // Re-review R3: the guards a mutant survived.
  const five = headingNamed(doc, 'Five heading');
  check('F1: a five-token shorthand stays on the heading', styleMap(five).padding === '1px 2px 3px 4px 5px' && !('padding' in styleMap(cellOf(five).td)), five && five.getAttribute('style'));
  const both = cellOf(headingNamed(doc, 'Both heading'));
  check('F1: a px longhand beside a px shorthand wins for its side', both && styleMap(both.td).padding === '0 8px 8px 8px', both && both.td.getAttribute('style'));
  const upper = cellOf(headingNamed(doc, 'Upper heading'));
  check('F1: an upper-case PX unit is a px value', upper && /^10px 0 10px 0$/i.test(styleMap(upper.td).padding || ''), upper && upper.td.getAttribute('style'));
}

// ---- I2: in a Container ----------------------------------------------------------------------
{
  const container = `<div style="border-radius:0;padding:8px 8px 8px 8px">${heading('Boxed one', H_PADDED)}${heading('Boxed two', 'font-weight:bold;margin:0;font-size:24px;padding:0px 24px 0px 24px')}</div>`;
  const { doc } = compile(container);
  for (const name of ['Boxed one', 'Boxed two']) {
    const c = cellOf(headingNamed(doc, name));
    check(`I2 in a Container: "${name}" is its own presentation table cell`, c && c.onlyChild && c.rows === 1);
  }
  check('I2 in a Container: the second heading\'s side padding is on its cell', styleMap(cellOf(headingNamed(doc, 'Boxed two')).td).padding === '0px 24px 0px 24px');
  check('I2 in a Container: the Container itself still converts to a padded cell',
    Array.from(doc.querySelectorAll('td')).some((td) => styleMap(td).padding === '8px 8px 8px 8px' && td.querySelector('h2')));
}

// ---- I2: in a column -------------------------------------------------------------------------
{
  const columns = `<div style="padding:0px 0px 0px 0px"><table align="center" width="100%" cellpadding="0" border="0" style="table-layout:fixed;border-collapse:collapse"><tbody style="width:100%"><tr style="width:100%">`
    + `<td style="box-sizing:content-box;vertical-align:top;padding-left:0;padding-right:8px">${heading('Column heading', 'font-weight:bold;text-align:right;margin:0;font-size:20px;padding:0px 8px 0px 8px', 'h3')}</td>`
    + `<td style="box-sizing:content-box;vertical-align:top;padding-left:8px;padding-right:0"><div style="padding:0px 0px 0px 0px"><p>Text</p></div></td>`
    + '</tr></tbody></table></div>';
  const { doc } = compile(columns);
  const h = headingNamed(doc, 'Column heading');
  const c = cellOf(h);
  check('I2 in a column: converted to its own cell', c && c.onlyChild && c.td.closest('table') !== h.closest('table[style*="table-layout"]'));
  check('I2 in a column: padding and explicit right alignment on the cell', c && styleMap(c.td).padding === '0px 8px 0px 8px' && c.td.getAttribute('align') === 'right');
  check('I2 in a column: heading keeps margin:0 and no padding', styleMap(h).margin === '0' && !('padding' in styleMap(h)));
}

// ---- I3: the Word font wrapper ---------------------------------------------------------------
{
  // Layout default MODERN_SANS (leads with Helvetica Neue, off the allowlist) inherited from the backdrop.
  const { decoded, doc } = compile(heading('Inherited stack', 'font-weight:bold;margin:0;font-size:24px;padding:0px 24px 0px 24px')
    + heading('Georgia heading', 'font-weight:bold;margin:0;font-family:Georgia, &quot;Times New Roman&quot;, Times, serif;font-size:24px;padding:0px')
    + heading('Organic heading', 'font-weight:bold;margin:0;font-family:Seravek, &quot;Gill Sans Nova&quot;, Ubuntu, Calibri, sans-serif;font-size:24px;padding:0px'));
  check('I3: an inherited off-list stack is wrapped INSIDE the heading, around its content',
    new RegExp(`<h2 style="[^"]*">${OPEN.replace(/[[\]()]/g, '\\$&')}Inherited stack${CLOSE.replace(/[[\]()]/g, '\\$&')}</h2>`).test(decoded), decoded.slice(decoded.indexOf('Inherited') - 200, decoded.indexOf('Inherited') + 80));
  check('I3: a heading on a compliant stack (Georgia) gets no wrapper', /<h2 style="[^"]*">Georgia heading<\/h2>/.test(decoded));
  check('I3: an own off-list stack falls back to its first allowlisted family (Calibri)',
    /<h2 style="[^"]*"><!--\[if mso\]><font face="Calibri"><!\[endif\]-->Organic heading<!--\[if mso\]><\/font><!\[endif\]--><\/h2>/.test(decoded));
  check('I3: the wrapper is not outside the heading (the cell holds only the heading)', cellOf(headingNamed(doc, 'Inherited stack')).td.childNodes.length === 1);
  const arial = compile(heading('Arial layout', 'font-weight:bold;margin:0;font-size:24px;padding:0px'), true);
  check('I3: under a compliant inherited stack no wrapper', !/<font face=/.test(decodeSafe(pp.postProcess(canvas(heading('Arial layout', 'font-weight:bold;margin:0;font-size:24px;padding:0px'), { backdropFont: ARIAL }), { outlook: true }))) && !!arial);
  // A Container holding only Headings no longer gets a block-level wrapper; each heading has its own.
  const box = decodeSafe(pp.postProcess(canvas(`<div style="padding:8px 8px 8px 8px">${heading('In box A', 'font-weight:bold;margin:0;font-size:24px')}${heading('In box B', 'font-weight:bold;margin:0;font-size:24px')}</div>`), { outlook: true }));
  check('I3: a Container of Headings carries one wrapper per heading, none around the Container', (box.split(OPEN).length - 1) === 2
    && /<h2 style="[^"]*"><!--\[if mso\]><font face="Arial"><!\[endif\]-->In box A/.test(box) && /<h2 style="[^"]*"><!--\[if mso\]><font face="Arial"><!\[endif\]-->In box B/.test(box));
}

// ---- I4: what is NOT a Heading block ---------------------------------------------------------
{
  const markdown = `<div style="padding:0px 24px 0px 24px"><p>Intro para</p>\n<h2 style="margin:0">Typed heading</h2>\n<h2>Markdown heading</h2>\n</div>`;
  const { doc, decoded } = compile(markdown);
  const typed = headingNamed(doc, 'Typed heading');
  check('I4: a hand-typed margin:0 heading beside other flow content stays in its Text block', typed && typed.parentElement.tagName === 'TD' && typed.parentElement.querySelector('p'));
  check('I4: a markdown heading (no inline margin) is not converted', headingNamed(doc, 'Markdown heading').parentElement === typed.parentElement);
  check('I4: the Text block keeps its own font wrapper around all its flow', new RegExp(`${OPEN.replace(/[[\]()]/g, '\\$&')}<p[^>]*>Intro para`).test(decoded) && (decoded.split(OPEN).length - 1) === 1);
  // Fenced user Html: never touched.
  const fenced = `<div data-lm-user-html="true" style="padding:8px 24px 8px 24px"><h2 style="margin:0;padding:12px">Fenced heading</h2></div>`;
  const f = compile(fenced);
  const fh = headingNamed(f.doc, 'Fenced heading');
  check('I4: a heading inside a user Html fence is not converted and keeps its padding', fh && styleMap(fh).padding === '12px' && !/<h2[^>]*>\{\{ Safe/.test(f.raw), fh && fh.outerHTML);
  // The accepted collateral: a hand-typed margin:0 heading that is a Text block's ONLY flow child.
  const solo = compile(`<div style="padding:0px 24px 0px 24px"><h2 style="margin:0">Solo typed</h2>\n</div>`);
  const sc = cellOf(headingNamed(solo.doc, 'Solo typed'));
  check('I4 collateral: a lone hand-typed margin:0 heading converts, carrying its own wrapper', sc && sc.onlyChild
    && /<h2 style="margin:0"><!--\[if mso\]><font face="Arial"><!\[endif\]-->Solo typed/.test(solo.decoded) && (solo.decoded.split(OPEN).length - 1) === 1);
}

// ---- the flag gates it -----------------------------------------------------------------------
{
  const input = heading('Flag off heading', H_PADDED);
  const out = pp.postProcess(canvas(input), { outlook: false });
  check('flag off: the heading is byte-identical, no table, no wrapper', out.includes(input) && !/\{\{ Safe/.test(out));
}

done();
