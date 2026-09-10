// Blockquote render-side styling (EDITOR-POLISH-SPEC I3b): Markdown `> quoted text` must
// compile to a <blockquote> carrying the inline border/radius/padding style patched into
// @usewaypoint/block-text's CustomRenderer (patches/@usewaypoint+block-text+0.0.6.patch), and
// must not be stripped or altered by the sanitizer (blockquote is already in ALLOWED_TAGS,
// and style is already in GENERIC_ALLOWED_ATTRIBUTES for every allowlisted tag).
//
// renderMarkdownString is internal to block-text and not exported (the package exports only
// Text/TextPropsDefaults/TextPropsSchema), so this renders the exported Text component
// instead — same pattern as font-family-parity.test.cjs.
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

check('renders a <blockquote> element', /<blockquote[ >]/.test(html), html);
check('<blockquote> carries an inline style attribute', /<blockquote style="/.test(html), html);
check('<blockquote> style includes a left border', html.includes('border-left'), html);
check('<blockquote> style includes a border radius', html.includes('border-radius'), html);
check('<blockquote> style includes left padding', html.includes('padding-left'), html);
check('quoted text survives into the output', html.includes('quoted'), html);

if (failed) {
  console.log(`${failed} CHECKS FAILED`);
  process.exit(1);
}
console.log('ALL PASS');
