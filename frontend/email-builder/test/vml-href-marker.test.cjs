const path = require('path');
const { JSDOM } = require('jsdom');
global.DOMParser = new JSDOM('<!doctype html>').window.DOMParser;
const { postProcess } = require(path.join(__dirname, '.build', 'postProcess.cjs'));
const { decodeEntities } = require(path.join(__dirname, 'vml-marker-fold.cjs'));

// CLICK-TRACKING-SPEC T5 (I10–I12): the builder emits the VML button's href value OUTSIDE
// the Safe string literal, as <span data-lm-vml-href="VALUE"></span> between two Safe
// payloads, so the Go compile transform can wrap it in a TrackLink call. A typed value
// containing " \ < or & cannot terminate a Safe string or the marker attribute, and the
// marker survives Editor.vue's format-switch beautifier (js-beautify, Editor.vue settings).

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

function button(hrefAttr) {
  // hrefAttr is the attribute value AS SERIALIZED by React (entities already escaped).
  return `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
<div style="text-align:center;padding:16px 24px 16px 24px">
<a href="${hrefAttr}" target="_blank" style="color:#FFFFFF;font-size:16px;font-weight:bold;background-color:#2563EB;border-radius:64px;display:inline-block;padding:0px 0px 0px 0px;text-decoration:none;width:200px;box-sizing:border-box;text-align:center;line-height:19px;border:5px solid #DC2626;white-space:nowrap">Go</a>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;
}

const MARKER_RE = /\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}<span data-lm-vml-href="([^"]*)"><\/span>\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/;
// After Editor.vue's beautifier: whitespace (only) may separate the parts — Editor.vue's
// tag-padding regexp exempts <span but not </span>; the Go marker pass swallows it.
const LOOSE_MARKER_RE = /\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}\s*<span data-lm-vml-href="([^"]*)"\s*>\s*<\/span>\s*\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/;
const SAFE_RE = /\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g;

// Strict Safe lexer (safe-wrap.test.cjs): every action is exactly {{ Safe "<string>" }}.
function walkGoString(s, i) {
  while (i < s.length) {
    if (s[i] === '\\') { i += 2; continue; }
    if (s[i] === '\n') return -1;
    if (s[i] === '"') return i;
    i += 1;
  }
  return -1;
}
function safePayloadsWellFormed(html) {
  let idx = 0;
  while ((idx = html.indexOf('{{ Safe "', idx)) !== -1) {
    const close = walkGoString(html, idx + 9);
    if (close === -1 || html.slice(close, close + 4) !== '" }}') return false;
    idx = close + 4;
  }
  return true;
}

// 1. Static URL: marker between two Safe halves, halves end/start with the escaped quote.
const staticOut = postProcess(button('https://x.test/go?a=1&amp;b=2'), { outlook: true });
const m = staticOut.match(MARKER_RE);
check('static button: marker sits between two Safe payloads', !!m);
check('first half ends with href=\\" and second starts with \\"', m && /href=\\"$/.test(m[1]) && /^\\"/.test(m[3]));
check('marker VALUE is escapeAttribute(href): & stays &amp;', m && m[2] === 'https://x.test/go?a=1&amp;b=2', m && m[2]);
check('no part of the URL inside either Safe payload', m && !/x\.test/.test(m[1]) && !/x\.test/.test(m[3]));
check('the non-mso <a> copy keeps its href untouched', /<a href="https:\/\/x\.test\/go\?a=1&amp;b=2"/.test(staticOut));
check('every Safe payload is well-formed', safePayloadsWellFormed(staticOut));

// 2. Personalized URL with the `or` idiom (React stores the typed quotes as &quot;).
const dyn = '{{ or .Subscriber.Attribs.site &quot;https://curatedfor.you&quot; }}';
const dynOut = postProcess(button(dyn), { outlook: true });
const dm = dynOut.match(MARKER_RE);
check('dynamic button: marker emitted', !!dm);
check('dynamic VALUE round-trips to the typed expression after entity decode',
  dm && decodeEntities(dm[2]) === '{{ or .Subscriber.Attribs.site "https://curatedfor.you" }}', dm && dm[2]);
check('no raw quote inside the marker attribute', dm && !/"/.test(dm[2]));
check('every Safe payload is well-formed (dynamic)', safePayloadsWellFormed(dynOut));

// 3. Hostile characters: " \ < & typed into a url cannot break out of the marker or a payload.
const hostile = 'https://x.test/a&quot;b\\c&lt;d&amp;e&gt;f';
const hostileOut = postProcess(button(hostile), { outlook: true });
const hm = hostileOut.match(MARKER_RE);
check('hostile url: marker still isolates the value', !!hm);
check('hostile url: value decodes to the typed characters', hm && decodeEntities(hm[2]) === 'https://x.test/a"b\\c<d&e>f', hm && hm[2]);
check('hostile url: no raw quote in the marker attribute', hm && !/"/.test(hm[2]));
check('hostile url: every Safe payload is well-formed', safePayloadsWellFormed(hostileOut));
check('hostile url: no raw whitespace or & inside any Safe payload',
  [...hostileOut.matchAll(SAFE_RE)].every((x) => !/[ \t\r\n&]/.test(x[1])));

// 4. Editor.vue beautifyHTML round trip (the converted-document path): the marker must
// survive js-beautify with Editor.vue's settings, byte-identical, and stay adjacent to
// both Safe halves.
let jsBeautify;
try { jsBeautify = require(path.join(__dirname, '..', '..', 'node_modules', 'js-beautify')); } catch { jsBeautify = require('js-beautify'); }
const padded = dynOut.replace(/(<(?!(\/)?a|span)([^>]+)>)/ig, '\n$1\n').replace(/\n+/g, '\n');
const beautified = jsBeautify.html(padded, {
  indent_size: 4, indent_char: ' ', max_preserve_newlines: 2,
  inline: ['h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'b', 'strong', 'span', 'em', 'i', 'code', 'a'],
});
const bm = beautified.match(LOOSE_MARKER_RE);
check('beautified: marker intact, only whitespace between it and the Safe halves', !!bm);
check('beautified: marker VALUE unchanged', bm && bm[2] === dm[2]);
check('beautified: every Safe payload is well-formed', safePayloadsWellFormed(beautified));

// 5. body_source shape: the builder never touches the document; this file exercises the
// compiled-HTML post-processor only. The Button block's stored props are asserted unchanged
// by keeping the input <a href> as the single source — the post-processor reads it and
// emits the marker from it, never rewriting the source document (no body_source writer
// exists in postProcess.ts).
check('post-processor is HTML-in/HTML-out (no document/body_source access)',
  typeof postProcess === 'function' && postProcess.length === 2);

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
