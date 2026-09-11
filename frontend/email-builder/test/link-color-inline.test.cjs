// Compile-time link-color inlining + the MSO document-settings head block
// (DARK-MODE-SPEC D1, invariants I1a / I1b / I1c).
//
// Six clients drop BOTH <style> copies of the link color (template 29 full matrix,
// 2026-09-10), so renderHtmlWithMeta additionally inlines it onto every anchor that does
// not declare one. This suite pins:
//   I1a  every <a> carries an inline color; anchors that declared one keep theirs verbatim
//   I1b  the pass is idempotent (a re-save of an already-inlined body changes nothing)
//   I1c  the head's document-settings block is a `{{ Safe "..." }}` action, not a raw
//        comment -- Go's html/template elides raw comments, so the raw form never reached
//        a recipient
// plus: no linkColor => byte-identical output, `background-color` is not a color
// declaration, and the Button VML builder still reads the anchor colors it always did.
//
// renderHtmlWithMeta lives in utils.tsx, which pulls the whole React reader. The suite
// transpiles it standalone and stubs that single import with a fixture renderer, so the
// assertions are about the compile PASSES, not about React's markup.
const path = require('path');
const fs = require('fs');
const { JSDOM } = require('jsdom');

global.DOMParser = new JSDOM('<!doctype html>').window.DOMParser;

const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));
const outlook = require(path.join(__dirname, '.build', 'outlook.cjs'));
const { foldVmlMarkers } = require(path.join(__dirname, 'vml-marker-fold.cjs'));

// makeSafeTemplate encodes `& < > space tab CR LF` as \xNN and escapes \ and " — decode a
// payload back to the markup Go will emit.
function decodeSafe(payload) {
  return payload
    .replace(/\\x([0-9a-f]{2})/gi, (_, h) => String.fromCharCode(parseInt(h, 16)))
    .replace(/\\"/g, '"')
    .replace(/\\\\/g, '\\');
}

function transpile(relSourcePath) {
  const src = fs.readFileSync(path.join(builderRoot, 'src', relSourcePath), 'utf8');
  return ts.transpileModule(src, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.React },
  }).outputText;
}

function evaluate(js, requireMap) {
  const mod = { exports: {} };
  new Function('module', 'exports', 'require', js)(mod, mod.exports, (id) => {
    if (!(id in requireMap)) {
      throw new Error(`unexpected require(${id})`);
    }
    return requireMap[id];
  });
  return mod.exports;
}

// inlineLinkColor must stay dependency-free -- any require at all is a failure.
const { inlineLinkColor } = evaluate(transpile('inlineLinkColor.ts'), {});

// A fixture document stands in for the React reader's output.
let fixtureBody = '';
const { renderHtmlWithMeta } = evaluate(transpile('utils.tsx'), {
  './documents/reader/renderToStaticMarkup': { default: () => `<!DOCTYPE html><html><body>${fixtureBody}</body></html>` },
  './documents/editor/core': {},
  './outlook': outlook,
  './inlineLinkColor': { inlineLinkColor },
});

let failed = 0;
function check(name, ok, detail) {
  if (!ok) {
    failed++;
    console.log(`FAIL  ${name}${detail ? `  [${detail}]` : ''}`);
  } else {
    console.log(`PASS  ${name}`);
  }
}

const LINK = '#888888';

// The six fixture anchors the spec names, in one document: two plain text-block links, a
// Button anchor with its own color, an Html-block link, an anchor that declares a color,
// and an anchor carrying only background-color.
// The canvas wrapper is what postProcessForOutlook's ghost table and VML button builder
// key off, so the fixture carries it — the I1c/VML assertions below run the real pipeline.
const CANVAS_OPEN = '<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">'
  + '<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>';
const CANVAS_CLOSE = '</td></tr></tbody></table></div>';

fixtureBody = [
  CANVAS_OPEN,
  '<div style="padding:16px 24px"><p>Read the ',
  '<a href="https://a.test/one" target="_blank">first</a> and the ',
  '<a href="https://a.test/two" target="_blank">second</a>.</p></div>',
  '<div style="text-align:center;padding:16px 24px">',
  '<a href="https://a.test/go" target="_blank" style="color:#ffffff;font-size:16px;font-weight:bold;background-color:#0055d4;border-radius:4px;display:inline-block;padding:12px 20px 12px 20px;text-decoration:none">SHOP NOW</a>',
  '</div>',
  '<div data-lm-user-html="true"><a href="https://a.test/html">html block link</a></div>',
  '<p><a href="https://a.test/hand" style="color:#123456">hand coloured</a></p>',
  '<p><a href="https://a.test/bg" style="background-color:#eeeeee">background only</a></p>',
  CANVAS_CLOSE,
].join('');

const out = renderHtmlWithMeta({}, { rootBlockId: 'root', outlook: false, linkColor: LINK });

// ---- I1a ----------------------------------------------------------------------------
const anchors = [...new JSDOM(out).window.document.querySelectorAll('a')];
check('I1a: the fixture six anchors all survive', anchors.length === 6, `got ${anchors.length}`);

function styleOf(href) {
  const a = anchors.find((el) => el.getAttribute('href') === href);
  return a ? (a.getAttribute('style') || '') : null;
}
function colorIn(style) {
  const m = /(^|;)\s*color\s*:\s*([^;]+)/.exec(style || '');
  return m ? m[2].trim() : null;
}

check('I1a: plain link 1 inlined', colorIn(styleOf('https://a.test/one')) === LINK, styleOf('https://a.test/one'));
check('I1a: plain link 2 inlined', colorIn(styleOf('https://a.test/two')) === LINK, styleOf('https://a.test/two'));
check('I1a: Html-block link inlined', colorIn(styleOf('https://a.test/html')) === LINK, styleOf('https://a.test/html'));
check('I1a: hand-coloured link untouched', styleOf('https://a.test/hand') === 'color:#123456', styleOf('https://a.test/hand'));
check('I1a: Button anchor keeps its own colour', colorIn(styleOf('https://a.test/go')) === '#ffffff', styleOf('https://a.test/go'));
check('I1a: Button anchor keeps every other declaration',
  /background-color:#0055d4/.test(styleOf('https://a.test/go')) && /border-radius:4px/.test(styleOf('https://a.test/go')),
  styleOf('https://a.test/go'));
// The bug the parseStyleMap rule exists to prevent: a substring test would see
// `background-color:` and skip this anchor.
const bg = styleOf('https://a.test/bg');
check('I1a: background-color alone is NOT a colour declaration -> inlined',
  colorIn(bg) === LINK && /background-color:#eeeeee/.test(bg), bg);
check('I1a: every anchor ends up with an inline colour',
  anchors.every((a) => colorIn(a.getAttribute('style'))),
  anchors.map((a) => a.getAttribute('style')).join(' | '));
check('I1a: the <style> belt-and-braces copies stay',
  (out.match(/a\{color:#888888;\}/g) || []).length >= 1, out.slice(0, 200));

// ---- I1b ----------------------------------------------------------------------------
check('I1b: pass is idempotent over its own output', inlineLinkColor(out, LINK) === out);
const twice = inlineLinkColor(inlineLinkColor(fixtureBody, LINK), LINK);
check('I1b: idempotent over a bare fragment too', twice === inlineLinkColor(fixtureBody, LINK));

// ---- no linkColor: byte-identical, no DOMParser round trip ---------------------------
const plain = renderHtmlWithMeta({}, { rootBlockId: 'root', outlook: false });
check('no linkColor: output is the untouched render + viewport meta',
  plain === `<!DOCTYPE html><html><head><meta name="viewport" content="width=device-width, initial-scale=1.0"></head><body>${fixtureBody}</body></html>`,
  plain.slice(0, 160));
check('no linkColor: inlineLinkColor is a strict string no-op',
  inlineLinkColor(fixtureBody, null) === fixtureBody && inlineLinkColor(fixtureBody, '') === fixtureBody
  && inlineLinkColor(fixtureBody, undefined) === fixtureBody);

// ---- the round trip alone changes nothing else ---------------------------------------
// Same document, a colour every anchor already declares: the only difference from the
// untouched render must be the serializer's normalisations, not content.
const preColoured = fixtureBody
  .replace(/<a href="(https:\/\/a\.test\/(?:one|two|html))"/g, '<a style="color:#888888" href="$1"')
  .replace('style="background-color:#eeeeee"', 'style="background-color:#eeeeee;color:#888888"');
const before = inlineLinkColor(`<!DOCTYPE html><html><body>${preColoured}</body></html>`, null);
const after = inlineLinkColor(`<!DOCTYPE html><html><body>${preColoured}</body></html>`, LINK);
const strip = (s) => s.replace(/^<!doctype html>\n?/i, '').replace('<head></head>', '');
check('round trip alone: no anchor content or attribute changes', strip(before) === strip(after), `\n  ${strip(before)}\n  ${strip(after)}`);

// ---- I1c ----------------------------------------------------------------------------
const outlookOut = renderHtmlWithMeta({}, { rootBlockId: 'root', outlook: true, linkColor: LINK });
const headSafe = /\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/.exec(
  outlookOut.slice(outlookOut.indexOf('<head'), outlookOut.indexOf('</head>')),
);
check('I1c: the document-settings block is a Safe template action',
  !!headSafe && decodeSafe(headSafe[1]).includes('OfficeDocumentSettings'),
  outlookOut.slice(outlookOut.indexOf('<head'), outlookOut.indexOf('</head>')));
check('I1c: it is NOT emitted as a raw comment (html/template elides those)',
  !outlookOut.includes('<!--[if mso]><noscript>'),
  'raw <!--[if mso]><noscript> still present');
check('I1c: the Safe payload decodes to the PixelsPerInch block',
  !!headSafe && decodeSafe(headSafe[1]).includes('<o:PixelsPerInch>96</o:PixelsPerInch>'),
  headSafe && decodeSafe(headSafe[1]));
check('I1c: not emitted for outlook:false documents', !/OfficeDocumentSettings/.test(out));

// ---- the VML button builder still sees the colours it always did ---------------------
// The inline pass runs first, so this is the shape postProcessForOutlook receives.
const vml = decodeSafe(foldVmlMarkers(outlookOut));
check('VML: a roundrect was emitted at all (the pipeline really ran)', vml.includes('v:roundrect'));
check('VML: button fill and label colours unchanged by the pass',
  /fillcolor="#0055d4"/.test(vml) && /<center style="color:#ffffff/.test(vml),
  (vml.match(/fillcolor="[^"]*"/) || [])[0]);
check('VML: the link colour never reached the button label',
  !/<center style="color:#888888/.test(vml));

if (failed) {
  console.log(`\n${failed} FAILURES`);
  process.exit(1);
}
console.log('\nALL PASS');
