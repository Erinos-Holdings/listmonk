// Font-family parity: the dropdown's font list (src/documents/blocks/helpers/fontFamily.ts)
// and the @usewaypoint packages' embedded copies (extended by patches/*.patch via
// patch-package) MUST agree, or a selectable font silently renders as the client default —
// exactly what happened with the 12 fonts added 2026-08-17: the fork's list knew the keys,
// the npm blocks' hardcoded enum + resolver switch did not.
//
// Source of truth is fontFamily.ts; this suite fails if a key added there is missing from
// the patched packages (or resolves to a different stack), and if a patch is dropped —
// e.g. lost in a dependency bump or a rebase — every added key fails at once.
const fs = require('fs');
const path = require('path');

const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
const { Text, TextPropsSchema } = require('@usewaypoint/block-text');
const { Heading, HeadingPropsSchema } = require('@usewaypoint/block-heading');
const { HtmlPropsSchema } = require('@usewaypoint/block-html');
const { ButtonPropsSchema } = require('@usewaypoint/block-button');
const emailBuilder = require('@usewaypoint/email-builder');

let failed = 0;
function check(name, ok, detail) {
  if (!ok) {
    failed++;
    console.log(`FAIL  ${name}${detail ? ` — ${detail}` : ''}`);
  } else {
    console.log(`PASS  ${name}`);
  }
}

// Parse key/value pairs out of fontFamily.ts rather than importing it (TS).
const src = fs.readFileSync(
  path.join(__dirname, '..', 'src', 'documents', 'blocks', 'helpers', 'fontFamily.ts'),
  'utf8',
);
const fonts = [];
const re = /key: '([A-Z_]+)',\s*label: '[^']+',\s*value:\s*'((?:[^'\\]|\\.)*)'/g;
for (let m; (m = re.exec(src)); ) fonts.push({ key: m[1], value: m[2] });
check('fontFamily.ts parse found a plausible font count', fonts.length >= 21, `got ${fonts.length}`);

for (const { key, value } of fonts) {
  // 1. The npm packages' zod enum accepts the key.
  const textParse = TextPropsSchema.safeParse({ style: { fontFamily: key }, props: { text: 'x' } });
  check(`block-text schema accepts ${key}`, textParse.success);
  const headingParse = HeadingPropsSchema.safeParse({ style: { fontFamily: key }, props: { text: 'x' } });
  check(`block-heading schema accepts ${key}`, headingParse.success);

  // 2. The npm packages' resolver returns the exact same stack the dropdown promises.
  // A missing key resolves to undefined and emits no font-family at all.
  const html = renderToStaticMarkup(
    React.createElement(Text, { style: { fontFamily: key }, props: { text: 'x' } }),
  );
  const expected = `font-family:${value}`.replace(/"/g, '&quot;');
  check(
    `block-text renders ${key} stack`,
    html.includes(expected),
    `expected ${expected} in ${html}`,
  );
  const headingHtml = renderToStaticMarkup(
    React.createElement(Heading, { style: { fontFamily: key }, props: { text: 'x' } }),
  );
  check(`block-heading renders ${key} stack`, headingHtml.includes(expected));

  // 3. The other three patched packages. block-html and block-button ship schemas the fork
  // validates through (HtmlPropsSchema directly; ButtonPropsSchema as the base of the fork's
  // own), so an unpatched enum rejects the key. Their npm renderers are NOT the shipped
  // render path (the fork's local HtmlReader/Button resolve from fontFamily.ts), so schema
  // acceptance is the whole contract here.
  const htmlParse = HtmlPropsSchema.safeParse({ style: { fontFamily: key }, props: { contents: 'x' } });
  check(`block-html schema accepts ${key}`, htmlParse.success);
  const buttonParse = ButtonPropsSchema.safeParse({
    style: { fontFamily: key },
    props: { text: 'x', url: 'https://example.com' },
  });
  check(`block-button schema accepts ${key}`, buttonParse.success);

  // email-builder's bundled copy renders the TemplatePanel preview (schema + resolver),
  // so it gets the full render check like text/heading.
  const previewHtml = emailBuilder.renderToStaticMarkup(
    { root: { type: 'Text', data: { style: { fontFamily: key }, props: { text: 'x' } } } },
    { rootBlockId: 'root' },
  );
  check(
    `email-builder renders ${key} stack`,
    previewHtml.includes(expected),
    `expected ${expected} in ${previewHtml}`,
  );
}

if (failed) {
  console.log(`${failed} CHECKS FAILED`);
  process.exit(1);
}
console.log('ALL PASS');
