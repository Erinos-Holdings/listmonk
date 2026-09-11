// PARAGRAPH-SPACING-SPEC I1–I5 (+ the flag half of I9): text rhythm is stated inline.
//
// The compiled body used to rely on client-default paragraph margins it never stated, so
// Gmail added two margins where the editor collapsed them, T-Online's reset zeroed them,
// and two stacked Text blocks could never be single-spaced (runbook hazard 55).
// normalizeTextMargins (src/postProcess.ts) now gives every direct text-flow child of a
// builder text block `margin-top:0` and `margin-bottom:<block font size>px`, `0` on the
// last, for EVERY document — the Outlook flag gates only the Word idioms. Pins:
//   I1  the margins themselves: inner bottoms = the block's effective size, edges 0,
//       single spacing between zero-padding blocks, mixed content, <pre>
//   I2  the pass runs with outlook:false, and that output carries no Word idiom
//   I3  user-authored Html content is never touched
//   I4  the blockquote's `margin` shorthand is expanded first (its side 0s survive)
//   I5  the mso-padding-alt edge graft and its estimator are gone
// The mso head block's flag half of I9 is pinned in link-color-inline.test.cjs (I1c).
const fs = require('fs');
const path = require('path');
const { JSDOM, pp, canvas, makeChecker, inlineButton } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

function styleMap(el) {
  return (el && el.getAttribute('style') || '').split(';').map((d) => d.trim()).filter(Boolean).reduce((acc, d) => {
    const i = d.indexOf(':');
    acc[d.slice(0, i).trim().toLowerCase()] = d.slice(i + 1).trim();
    return acc;
  }, {});
}
// The element whose own text starts with `text` (first match in document order).
function byText(doc, selector, text) {
  return Array.from(doc.querySelectorAll(selector)).find((el) => el.textContent.trim().startsWith(text)) || null;
}
function margins(el) {
  const m = styleMap(el);
  return `${m['margin-top']}/${m['margin-bottom']}`;
}
const WORD_IDIOM = /mso-|urn:schemas-microsoft-com|v:roundrect|<!--\[if|\{\{ Safe/;

// Text-block shapes as the vendored block-text reader emits them (marked output: element
// children separated by newlines).
const threeAt12 = '<div style="font-size:12px;font-weight:normal;padding:0px 24px 0px 24px"><p>Alpha one</p>\n<p>Alpha two</p>\n<p>Alpha three</p>\n</div>';
const singleAt14 = '<div style="font-size:14px;padding:16px 24px 16px 24px"><p>Solo para</p>\n</div>';
const inherits16 = '<div style="padding:0px 24px 0px 24px"><p>Inherit one</p>\n<p>Inherit two</p>\n</div>';
const stackedA = '<div style="padding:0px 0px 0px 0px"><p>Stacked first</p>\n</div>';
const stackedB = '<div style="padding:0px 0px 0px 0px"><p>Stacked second</p>\n</div>';
const withHr = '<div style="padding:0px 24px 0px 24px"><p>Before rule</p>\n<hr>\n<p>After rule</p>\n<hr>\n</div>';
const withPre = '<div style="font-size:15px;padding:0px 24px 0px 24px"><p>Code follows</p>\n<pre><code>x = 1</code></pre>\n<p>Code done</p>\n</div>';
const withList = '<div style="font-size:18px;padding:0px 24px 0px 24px"><p>List intro</p>\n<ul>\n<li><p>Nested item</p></li>\n</ul>\n<h1>Markdown heading</h1>\n</div>';
const withQuote = '<div style="padding:0px 24px 0px 24px"><blockquote style="border-left: 3px solid #bdbdbd; border-radius: 4px; padding-left: 12px; margin: 0 0 12px 0;">\n<p>Quoted para</p>\n</blockquote>\n<p>After quote</p>\n</div>';
const lastQuote = '<div style="padding:0px 24px 0px 24px"><p>Lead para</p>\n<blockquote style="border-left: 3px solid #bdbdbd; border-radius: 4px; padding-left: 12px; margin: 0 0 12px 0;">\n<p>Closing quote</p>\n</blockquote>\n</div>';
// Html blocks: the fence wrapper holding bare paragraphs, and a fenced subtree with a div.
const userHtmlFlat = '<div data-lm-user-html="true"><p>User flat one</p><p>User flat two</p></div>';
const userHtmlDeep = '<div data-lm-user-html="true" style="padding:8px 24px 8px 24px"><div><p>User deep</p></div></div>';
// A Container holding only Heading blocks: each <h2> is a block (inline margin:0), not a
// paragraph of a Text block, and must not gain an inner bottom margin.
const headingA = '<h2 style="font-weight:bold;margin:0;font-size:24px;padding:16px 24px 16px 24px">Heading block A</h2>';
const headingB = '<h2 style="font-weight:bold;margin:0;font-size:24px;padding:16px 24px 16px 24px">Heading block B</h2>';
const headingsOnly = `<div style="border-radius:0">${headingA}${headingB}</div>`;

const body = [threeAt12, singleAt14, inherits16, stackedA, stackedB, withHr, withPre, withList, withQuote, lastQuote, userHtmlFlat, userHtmlDeep, headingsOnly, inlineButton].join('\n');

for (const outlook of [true, false]) {
  const tag = `outlook:${outlook}`;
  const out = pp.postProcess(canvas(body), { outlook });
  const doc = new JSDOM(out).window.document;

  // ---- I1 -------------------------------------------------------------------------------
  check(`I1 ${tag}: three paragraphs at 12px → 0/12, 0/12, 0/0`,
    margins(byText(doc, 'p', 'Alpha one')) === '0/12px' && margins(byText(doc, 'p', 'Alpha two')) === '0/12px'
      && margins(byText(doc, 'p', 'Alpha three')) === '0/0',
    ['Alpha one', 'Alpha two', 'Alpha three'].map((t) => margins(byText(doc, 'p', t))).join(' '));
  check(`I1 ${tag}: a single-paragraph block → 0/0`, margins(byText(doc, 'p', 'Solo para')) === '0/0', margins(byText(doc, 'p', 'Solo para')));
  check(`I1 ${tag}: a block with no own size inherits 16 from the layout`,
    margins(byText(doc, 'p', 'Inherit one')) === '0/16px' && margins(byText(doc, 'p', 'Inherit two')) === '0/0',
    `${margins(byText(doc, 'p', 'Inherit one'))} ${margins(byText(doc, 'p', 'Inherit two'))}`);
  check(`I1 ${tag}: two stacked zero-padding single-paragraph blocks → both 0/0 (single spacing)`,
    margins(byText(doc, 'p', 'Stacked first')) === '0/0' && margins(byText(doc, 'p', 'Stacked second')) === '0/0');
  check(`I1 ${tag}: the zero-padding blocks stay plain divs (no box to convert)`,
    byText(doc, 'p', 'Stacked first').parentElement.tagName === 'DIV' && byText(doc, 'p', 'Stacked second').parentElement.tagName === 'DIV');
  check(`I1 ${tag}: an <hr> between paragraphs → the paragraphs stated, the last TEXT child zeroed`,
    margins(byText(doc, 'p', 'Before rule')) === '0/16px' && margins(byText(doc, 'p', 'After rule')) === '0/0',
    `${margins(byText(doc, 'p', 'Before rule'))} ${margins(byText(doc, 'p', 'After rule'))}`);
  const hrs = Array.from(byText(doc, 'p', 'Before rule').parentElement.querySelectorAll('hr'));
  check(`I1 ${tag}: the <hr> children are untouched`, hrs.length === 2 && hrs.every((hr) => !hr.hasAttribute('style')));
  check(`I1 ${tag}: a <pre> child is treated like any other`,
    margins(byText(doc, 'p', 'Code follows')) === '0/15px' && margins(doc.querySelector('pre')) === '0/15px'
      && margins(byText(doc, 'p', 'Code done')) === '0/0',
    margins(doc.querySelector('pre')));
  check(`I1 ${tag}: lists and markdown headings use the BLOCK size (a markdown <h1> is not 2em)`,
    margins(byText(doc, 'p', 'List intro')) === '0/18px' && margins(doc.querySelector('ul')) === '0/18px'
      && margins(byText(doc, 'h1', 'Markdown heading')) === '0/0',
    `${margins(doc.querySelector('ul'))} ${margins(byText(doc, 'h1', 'Markdown heading'))}`);
  check(`I1 ${tag}: only direct children are touched (nested <li><p> keeps marked's structure)`,
    !byText(doc, 'p', 'Nested item').hasAttribute('style'));
  check(`I1 ${tag}: side margins are never written on lists`, !/margin-(left|right)/.test(doc.querySelector('ul').getAttribute('style')));
  check(`I1 ${tag}: a Container holding only Heading blocks leaves them byte-identical`,
    out.includes(headingA) && out.includes(headingB));

  // ---- I3 -------------------------------------------------------------------------------
  check(`I3 ${tag}: fenced Html block with bare paragraphs compiles byte-identical inside the fence`,
    out.includes('<p>User flat one</p><p>User flat two</p>'));
  check(`I3 ${tag}: fenced Html subtree compiles byte-identical`, out.includes('<div><p>User deep</p></div>'));

  // ---- I4 -------------------------------------------------------------------------------
  for (const [label, text, bottom] of [['inner', 'Quoted para', '16px'], ['last', 'Closing quote', '0']]) {
    const bq = byText(doc, 'blockquote', text);
    const raw = bq.getAttribute('style');
    check(`I4 ${tag} (${label} blockquote): no margin shorthand remains`, !/(^|;)\s*margin\s*:/.test(raw), raw);
    check(`I4 ${tag} (${label} blockquote): margin-left:0 and margin-right:0 survive the expansion`,
      styleMap(bq)['margin-left'] === '0' && styleMap(bq)['margin-right'] === '0', raw);
    check(`I4 ${tag} (${label} blockquote): exactly one margin-top and one margin-bottom`,
      (raw.match(/margin-top\s*:/g) || []).length === 1 && (raw.match(/margin-bottom\s*:/g) || []).length === 1, raw);
    check(`I4 ${tag} (${label} blockquote): top 0, bottom ${bottom}`, margins(bq) === `0/${bottom}`, margins(bq));
    check(`I4 ${tag} (${label} blockquote): its own border/padding kept`, /border-left:3px solid #bdbdbd/.test(raw) && /padding-left:12px/.test(raw), raw);
  }
  check(`I4 ${tag}: the quoted paragraph (nested) is untouched`, !byText(doc, 'p', 'Quoted para').hasAttribute('style'));

  // ---- I5 -------------------------------------------------------------------------------
  check(`I5 ${tag}: no mso-padding-alt in the compiled output`, !/mso-padding-alt/.test(out));

  if (outlook) {
    // I9 (flag half): the Word idioms still run when the flag is on.
    check('I9 outlook:true: the VML button is emitted', /v:roundrect/.test(out));
  } else {
    // ---- I2 -----------------------------------------------------------------------------
    check('I2 outlook:false: output carries the margins', /margin-bottom:12px/.test(out) && /margin-top:0/.test(out));
    check('I2 outlook:false: output carries no Word idiom (mso-, VML, conditional, Safe payload)',
      !WORD_IDIOM.test(out), (out.match(WORD_IDIOM) || [])[0]);
    check('I2 outlook:false: the button stays a plain anchor (no conversion)', /<div style="text-align:center;padding:0px 24px 20px 24px"><a /.test(out));
  }
}

// ---- I5: the estimator is gone, and so is the graft on the campaign-28 fixture ---------
const campaign28 = fs.readFileSync(path.join(__dirname, 'fixtures', 'campaign28-nested.html'), 'utf8');
const c28 = pp.postProcess(campaign28, { outlook: true });
check('I5: campaign-28 nested-container fixture compiles with no mso-padding-alt', !/mso-padding-alt/.test(c28));
check('I5: its text paragraphs carry explicit margins instead', (c28.match(/<p style="margin-top:0;margin-bottom:0">/g) || []).length === 3,
  String((c28.match(/<p style="margin-top:0;margin-bottom:0">/g) || []).length));
check('I5: the built module does not contain the estimator or the graft at all',
  // (the source's comments still NAME the retired graft; only its code shape is pinned)
  !/estimateEdgeMarginPx|EDGE_MARGIN_TAGS|mso-padding-alt:\$\{/.test(fs.readFileSync(path.join(__dirname, '.build', 'postProcess.cjs'), 'utf8')));

// ---- I9: the export --------------------------------------------------------------------
check('I9: postProcess(html, { outlook }) is the export, arity 2', typeof pp.postProcess === 'function' && pp.postProcess.length === 2);
check('I9: the old export name is gone', !('postProcessForOutlook' in pp));

done();
