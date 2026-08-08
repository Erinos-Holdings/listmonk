const path = require('path');
const { createRequire } = require('module');
const { JSDOM } = require('jsdom');
global.DOMParser = new JSDOM('<!doctype html>').window.DOMParser;
const { postProcessForOutlook } = require(path.join(__dirname, '.build', 'outlook.cjs'));

// Safe payloads must be SINGLE UNBREAKABLE TOKENS (spaces/tabs encoded \x20/\x09).
// listmonk's format-switch path (Editor.vue convertContentType) runs campaign
// bodies through js-beautify, which line-wraps long text nodes at whitespace —
// and a raw newline inside a Go template string is a parse error at campaign
// save ("template: content:N: unexpected ... in operand", hit live 2026-08-08).
// Assert the encoding structurally, then round-trip through BOTH beautifiers
// that touch campaign HTML (js-beautify via Editor.vue, prettier via the code
// view users copy from) and Go-lex the results.

const builderRequire = createRequire(path.join(__dirname, '..', 'package.json'));
const frontendRequire = createRequire(path.join(__dirname, '..', '..', 'package.json'));

// The button carries a QUOTED font stack (BOOK_SANS): escapeAttribute turns
// the quotes into &quot;, and without &-encoding the fragment-parser round
// trip decodes them back into raw quotes inside the Go string — the exact
// live failure of 2026-08-08 ('unexpected "Noto" in operand').
const input = `<!doctype html><html><body>
<div style="background-color:#eee;margin:0;padding:20px 0;min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#fff"><tbody><tr><td>
<div style="font-family:Avenir, &quot;Avenir Next LT Pro&quot;, Montserrat;padding:16px 24px 16px 24px">
<p>Some <strong>text</strong> with <span style="color: #E11D48">color</span>.</p>
</div>
<div style="text-align:center;padding:16px 24px 16px 24px">
<a href="https://x.test/go?a=1&amp;b=2" target="_blank" style="color:#fff;font-size:16px;font-weight:bold;font-family:Optima, Candara, &quot;Noto Sans&quot;, source-sans-pro, sans-serif;background-color:#0055d4;border-radius:4px;display:block;padding:12px 20px 12px 20px;text-decoration:none">SHOP THE NEW COLLECTION</a>
</div>
</td></tr></tbody></table>
</div>
</body></html>`;

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

// Walk a Go template string literal starting after its opening quote; returns
// index of the closing quote, or -1 on raw newline / unterminated.
function walkGoString(s, i) {
  while (i < s.length) {
    if (s[i] === '\\') { i += 2; continue; }
    if (s[i] === '\n') return -1;
    if (s[i] === '"') return i;
    i += 1;
  }
  return -1;
}

// Strict: every Safe action must be exactly `{{ Safe "<string>" }}` — the
// string closes once and ` }}` follows IMMEDIATELY. A lexer that merely scans
// to `}}` pairs up early-terminated strings and misses raw interior quotes
// (how the &quot; decode bug slipped past the first version of this check).
function lexActions(html, label) {
  let bad = 0;
  let idx = 0;
  while ((idx = html.indexOf('{{ Safe "', idx)) !== -1) {
    const close = walkGoString(html, idx + 9);
    if (close === -1 || html.slice(close, close + 4) !== '" }}') {
      bad += 1;
      idx += 9;
      continue;
    }
    idx = close + 4;
  }
  check(`${label}: no broken Go template strings`, bad === 0, `broken=${bad}`);
}

const out = postProcessForOutlook(input);

// 1. Structural: no whitespace and no raw & inside any Safe string payload.
// Whitespace invites beautifier line-wrapping; & invites entity decode on the
// fragment-parser round trip (both classes break the Go string at save).
let raw = 0, m;
const safeRe = /\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g;
let count = 0;
while ((m = safeRe.exec(out)) !== null) {
  count += 1;
  if (/[ \t\r\n&]/.test(m[1])) raw += 1;
}
check('every Safe payload is whitespace- and entity-free', count > 0 && raw === 0, `payloads=${count} withRaw=${raw}`);
lexActions(out, 'raw pipeline output');

// 2. js-beautify round trip, replicating Editor.vue beautifyHTML: tag-padding
// regex, then js-beautify with the same options.
const beautify = frontendRequire('js-beautify').html;
const padded = out.replace(/(<(?!(\/)?a|span)([^>]+)>)/ig, '\n$1\n').replace(/\n+/g, '\n');
const beautified = beautify(padded, {
  indent_size: 4,
  indent_char: ' ',
  max_preserve_newlines: 2,
  inline: ['h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'b', 'strong', 'span', 'em', 'i', 'code', 'a'],
});
lexActions(beautified, 'js-beautify (Editor.vue settings)');

// 3. prettier round trip (the code-view formatting users copy from).
(async () => {
  const { format } = builderRequire('prettier/standalone');
  const pluginHtml = builderRequire('prettier/plugins/html');
  const pretty = await format(out, { parser: 'html', plugins: [pluginHtml] });
  lexActions(pretty, 'prettier html');

  console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
  process.exit(failed ? 1 : 0);
})();
