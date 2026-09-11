// Blockquote render-side styling (EDITOR-POLISH-SPEC I3b): Markdown `> quoted text` must
// compile to a <blockquote> carrying the inline border/radius/padding style patched into
// @usewaypoint/block-text's CustomRenderer (patches/@usewaypoint+block-text+0.0.6.patch), and
// must not be stripped or altered by the sanitizer (blockquote is already in ALLOWED_TAGS,
// and style is already in GENERIC_ALLOWED_ATTRIBUTES for every allowlisted tag).
//
// renderMarkdownString is internal to block-text and not exported (the package exports only
// Text/TextPropsDefaults/TextPropsSchema), so this renders the exported Text component
// instead — same pattern as font-family-parity.test.cjs.
//
// DARK-MODE-SPEC D8 (I8) additionally pins the bottom margin. The quote's visual gap used to
// come only from the inner <p> margins; T-Online strips those, so two adjacent quotes sat
// ~2px apart and read as one rule (template 29 full matrix, 2026-09-10). The 12px collapses
// into the following <p>'s larger top margin in browser engines, so clients that keep <p>
// margins are unchanged — where they are stripped, the gap is now intrinsic. The exact
// string is pinned so a patch regenerated against a bumped block-text is a visible break.
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
const { Text } = require('@usewaypoint/block-text');

let failed = 0;
function check(name, ok, detail) {
  if (!ok) {
    failed++;
    console.log(`FAIL  ${name}${detail ? ` — ${detail}` : ''}`);
  } else {
    console.log(`PASS  ${name}`);
  }
}

const html = renderToStaticMarkup(
  React.createElement(Text, { style: {}, props: { markdown: true, text: '> quoted' } }),
);

const STYLE = 'border-left: 3px solid #bdbdbd; border-radius: 4px; padding-left: 12px; margin: 0 0 12px 0;';

check('renders a <blockquote> element', /<blockquote[ >]/.test(html), html);
check('<blockquote> carries an inline style attribute', /<blockquote style="/.test(html), html);
check('<blockquote> style includes a left border', html.includes('border-left'), html);
check('<blockquote> style includes a border radius', html.includes('border-radius'), html);
check('<blockquote> style includes left padding', html.includes('padding-left'), html);
check('quoted text survives into the output', html.includes('quoted'), html);

// I8: the exact style string, bottom margin included.
check('I8: the rendered style string is exactly the pinned one',
  html.includes(`<blockquote style="${STYLE}"`), html);

// I8: two consecutive `> ` blocks are two elements, not one — the gap between them is what
// the bottom margin exists for.
const two = renderToStaticMarkup(
  React.createElement(Text, { style: {}, props: { markdown: true, text: '> first\n\n> second' } }),
);
check('I8: two consecutive quote blocks render as two <blockquote> elements',
  (two.match(/<blockquote[ >]/g) || []).length === 2, two);
check('I8: both carry the bottom margin',
  (two.match(/margin: 0 0 12px 0;/g) || []).length === 2, two);

if (failed) {
  console.log(`${failed} CHECKS FAILED`);
  process.exit(1);
}
console.log('ALL PASS');
