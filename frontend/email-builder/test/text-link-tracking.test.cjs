// integrations TEXT-LINK-TRACKING-SPEC I1/I2 against the BUILT bundle (run.cjs rebuilds it when
// absent or stale): a Text block's markdown link carrying the product-link expression compiles,
// through the REAL pipeline (block-text render + its insane sanitizer + postProcess + the editor
// quote hack -- EB.compileDocument, the stored body a UI Save writes), to an anchor whose href is
// exactly the shape the Go side wraps in TrackLink.
//
//   I1  The compiled anchor's href, entity-decoded the way models/campaigns.go::trackLinkArg
//       decodes it (html.UnescapeString), is the literal URL-with-expression; and the raw
//       compiled anchor is byte-identical to the input of the Go pin
//       models/tracking_test.go::TestProductLinkTrackedBackquoteFallback (`<a href="` + bq + `">`),
//       which asserts TransformTrackLinks wraps it. This is the test that would have caught the
//       stage-1 probe error (marked's DEFAULT renderer percent-encodes; block-text's does not).
//   I2  A literal %7B%7B x %7D%7D typed in a markdown URL survives the compile byte-identical --
//       nothing decodes it into an executing action.
//
// Plus the hazard-102 Text-path mode (spec M2) as a documented pin: a DOUBLE-quoted fallback in a
// markdown link makes the sanitizer delete the anchor AND its label. The picker never emits it
// (attributes.test.cjs TL I3); if this check ever fails, re-read hazard 102 before relaxing it.
//
// The document is built inline -- a fixture file under test/fixtures would join the
// compile-unchanged snapshot set.
const { JSDOM } = require('jsdom');
const { loadUmd, compileInputs } = require('./_umd.cjs');

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail ? '  [' + String(detail).slice(0, 400) + ']' : ''}`); }

const { EB } = loadUmd();
const { context, refs } = compileInputs();
const decoder = new JSDOM('<!doctype html>').window.document;

function compileText(markdown, outlook = true) {
  const doc = {
    root: { type: 'EmailLayout', data: { backdropColor: '#F5F5F5', canvasColor: '#FFFFFF', textColor: '#262626', fontFamily: 'MODERN_SANS', outlook, childrenIds: ['t'] } },
    t: { type: 'Text', data: { style: { padding: { top: 0, bottom: 0, left: 24, right: 24 } }, props: { markdown: true, text: markdown } } },
  };
  return EB.compileDocument(doc, context, refs);
}

// Entity-decode an attribute's raw text (html.UnescapeString's job on the Go side).
function decodeEntities(raw) {
  const ta = decoder.createElement('textarea');
  ta.innerHTML = raw;
  return ta.value;
}

// The raw href attribute texts of every <a> in the compiled body (the regexp shape the Go
// regVisualHref reads: href="…" with no raw quote inside).
const hrefs = (html) => [...html.matchAll(/<a\s[^>]*?href="([^"]*)"/g)].map((m) => m[1]);

// Copied from the Go pin (models/tracking_test.go::TestProductLinkTrackedBackquoteFallback, `bq`).
const BQ = 'https://ruzepouches.com/pk/X/?ref={{ or .Subscriber.Attribs.shop_links.ruze.slug `` }}';
const I1_MARKDOWN = '[Shop now](<https://ruzepouches.com/pk/X/?ref={{ or .Subscriber.Attribs.shop_links.ruze.slug `` }}>)';

for (const outlook of [true, false]) {
  const html = compileText(`Before ${I1_MARKDOWN} after`, outlook);
  const hs = hrefs(html);
  check(`I1 (outlook ${outlook}): exactly one anchor compiled`, hs.length === 1, JSON.stringify(hs));
  check(`I1 (outlook ${outlook}): the entity-decoded href is the literal expression URL`, decodeEntities(hs[0] || '') === BQ, hs[0]);
  check(`I1 (outlook ${outlook}): the raw anchor is the Go pin's tracked input <a href="bq"`, html.includes(`<a href="${BQ}"`), hs[0]);
  check(`I1 (outlook ${outlook}): the label and the surrounding text survive`,
    /<a href="[^"]*"[^>]*>Shop now<\/a>/.test(html) && html.includes('Before ') && html.includes(' after'));
  check(`I1 (outlook ${outlook}): balanced action, no raw quote, no percent-encoding of the braces`,
    (hs[0] || '').split('{{').length === 2 && (hs[0] || '').split('}}').length === 2 && !/%7B|%7D/i.test(hs[0] || ''));
}

// The picker's own output for a typical selection compiles the same way.
{
  const html = compileText('[Buy](<https://ruzepouches.com/pk/X/?utm_source=email&ref={{ or .Subscriber.Attribs.shop_links.ruze.slug `` }}#reviews>)');
  const h = decodeEntities(hrefs(html)[0] || '');
  check('I1: params + fragment around the expression decode to the literal',
    h === 'https://ruzepouches.com/pk/X/?utm_source=email&ref={{ or .Subscriber.Attribs.shop_links.ruze.slug `` }}#reviews', h);
}

// ---- I2: authored literals stay inert -------------------------------------------------------
for (const [name, md, want] of [
  ['bracketed, spaced', '[Lit](<https://x.test/p?q=%7B%7B x %7D%7D>)', 'https://x.test/p?q=%7B%7B x %7D%7D'],
  ['unbracketed, %20', '[Lit](https://x.test/p?q=%7B%7B%20x%20%7D%7D)', 'https://x.test/p?q=%7B%7B%20x%20%7D%7D'],
]) {
  const html = compileText(md);
  const hs = hrefs(html);
  check(`I2: ${name} -> the href is byte-identical to the typed URL`, hs.length === 1 && hs[0] === want, JSON.stringify(hs));
  check(`I2: ${name} -> no action manufactured`, !/\{\{\s*x\s*\}\}/.test(html));
}

// ---- hazard 102, Text-path mode (spec M2): the double-quoted form loses the anchor AND label --
{
  const dq = compileText('Before [Shop now](<https://ruzepouches.com/pk/X/?ref={{ or .Subscriber.Attribs.shop_links.ruze.slug "" }}>) after');
  check('M2: a double-quoted fallback deletes the anchor and its label (why the picker never emits `"`)',
    hrefs(dq).length === 0 && !dq.includes('Shop now') && dq.includes('Before'));
}

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
