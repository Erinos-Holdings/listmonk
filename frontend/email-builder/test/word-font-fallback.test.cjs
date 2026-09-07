// CAMPAIGN-52-HARDENING T5 (I6). FONT_FAMILIES stays Mac-first (unchanged); Word gets a
// per-block <font face> fallback, inside downlevel-hidden conditionals only, naming the
// stack's first Windows/Office family — derived, never hand-picked. A block on a compliant
// stack gets no wrapper. A stack with no allowlisted family fails this suite (the build).
const fs = require('fs');
const path = require('path');
const { outlook, canvas, decodeSafe, makeChecker, modernText, arialText, rhythmModernText, borderedContainer, plainModernText, spacerDiv } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

// The allowlist pinned by the spec (D4).
const ALLOWLIST = ['Arial', 'Arial Rounded MT Bold', 'Bahnschrift', 'Bodoni MT', 'Bookman Old Style', 'Calibri', 'Cambria', 'Candara', 'Corbel', 'Courier New', 'Franklin Gothic Medium', 'Georgia', 'Palatino Linotype', 'Rockwell', 'Segoe Print', 'Segoe UI', 'Sitka Text', 'Tahoma', 'Times New Roman', 'Trebuchet MS', 'Verdana'];
check('WORD_FONT_ALLOWLIST is exactly the pinned set', JSON.stringify([...outlook.WORD_FONT_ALLOWLIST].sort()) === JSON.stringify([...ALLOWLIST].sort()));

// Every stack in fontFamily.ts: the fallback is the first allowlisted family; a stack
// whose lead is allowlisted needs none; a stack with none is a build error.
const src = fs.readFileSync(path.join(__dirname, '..', 'src', 'documents', 'blocks', 'helpers', 'fontFamily.ts'), 'utf8');
const fonts = [];
const re = /key: '([A-Z_]+)',\s*label: '[^']+',\s*value:\s*'((?:[^'\\]|\\.)*)'/g;
for (let m; (m = re.exec(src)); ) fonts.push({ key: m[1], value: m[2] });
check('fontFamily.ts parsed', fonts.length >= 21, `got ${fonts.length}`);

const expected = {
  MODERN_SANS: 'Arial', BOOK_SANS: 'Candara', GEOMETRIC_SANS: 'Corbel', ORGANIC_SANS: 'Calibri',
  ROUNDED_SANS: 'Arial Rounded MT Bold', SYSTEM_UI: 'Segoe UI', ANTIQUE_SERIF: 'Bookman Old Style',
  BOOK_SERIF: 'Palatino Linotype', DIDONE_SERIF: 'Bodoni MT',
  // The spec's D4 table says Cambria for MODERN_SERIF, but the rule it states — the FIRST
  // allowlisted family — yields Sitka Text (third in the stack, allowlisted). The rule wins.
  MODERN_SERIF: 'Sitka Text', MONOSPACE: 'Courier New',
  HEAVY_SANS: '', HANDWRITTEN: '', SLAB_SERIF: '', ARIAL: '', COURIER_NEW: '', GEORGIA: '', TAHOMA: '',
  TIMES_NEW_ROMAN: '', TREBUCHET_MS: '', VERDANA: '',
};
const allow = new Set(ALLOWLIST.map((f) => f.toLowerCase()));
for (const { key, value } of fonts) {
  const got = outlook.wordFontFallback(value);
  check(`${key}: stack contains an allowlisted family (build error otherwise)`, got !== null, value);
  const families = outlook.parseFontStack(value);
  const derived = allow.has(families[0].toLowerCase()) ? '' : (families.find((f) => allow.has(f.toLowerCase())) || null);
  check(`${key}: fallback is the stack's first allowlisted family (${JSON.stringify(derived)})`, got === derived, `got ${JSON.stringify(got)}`);
  if (key in expected) check(`${key}: matches the spec's D4 table`, got === expected[key], `got ${JSON.stringify(got)}`);
}
check('a stack with no allowlisted family reports null', outlook.wordFontFallback('Roboto, "Open Sans", sans-serif') === null);
check('MODERN_SANS lead is not compliant; ARIAL is', outlook.wordFontFallback('"Helvetica Neue", Arial') === 'Arial' && outlook.wordFontFallback('Arial, Helvetica') === '');

// Compiled document: layout default MODERN_SANS (inherited from the backdrop) → the
// converted greeting, the unconverted rhythm block and the box heading inside a bordered
// Container all get the Word-only wrapper; the explicit ARIAL block gets none.
const out = outlook.postProcessForOutlook(canvas(modernText + arialText + rhythmModernText + borderedContainer));
const rendered = decodeSafe(out);
const OPEN = '<!--[if mso]><font face="Arial"><![endif]-->';
const CLOSE = '<!--[if mso]></font><![endif]-->';
check('three MODERN_SANS blocks get the Word-only <font face="Arial"> wrapper', (rendered.split(OPEN).length - 1) === 3, `opens=${rendered.split(OPEN).length - 1}`);
check('every wrapper is closed', (rendered.split(CLOSE).length - 1) === 3);
check('greeting (converted td) wrapped', /<td[^>]*>\s*<!--\[if mso\]><font face="Arial"><!\[endif\]--><p>Ciao \{\{ \.Subscriber\.FirstName \}\},<\/p><!--\[if mso\]><\/font><!\[endif\]-->\s*<\/td>/.test(rendered));
check('rhythm-only block (unconverted div) wrapped', /<div style="padding:0px 24px 0px 24px"[^>]*><!--\[if mso\]><font face="Arial"><!\[endif\]--><p>Hai sbloccato/.test(rendered));
check('box heading inside the bordered Container wrapped', /<!--\[if mso\]><font face="Arial"><!\[endif\]--><p><strong>Scade il 15 ottobre<\/strong><\/p>/.test(rendered));
// Markdown-off Text block (bare text node, no <p>) — I6 says EVERY text block.
const outPlain = decodeSafe(outlook.postProcessForOutlook(canvas(plainModernText + spacerDiv)));
check('markdown-off text block (bare text node) gets the wrapper', /<!--\[if mso\]><font face="Arial"><!\[endif\]-->Tutto a metà prezzo\.<!--\[if mso\]><\/font><!\[endif\]-->/.test(outPlain), outPlain.slice(0, 400));
check('nbsp-only spacer div gets no wrapper', (outPlain.split(OPEN).length - 1) === 1, `opens=${outPlain.split(OPEN).length - 1}`);
check('ARIAL block gets no wrapper', !/<!--\[if mso\]><font face="[^"]*"><!\[endif\]--><p>Le ricompense/.test(rendered));
check('wrapper only inside downlevel-hidden conditionals (no raw <font face> outside Word)', !/(^|[^>])<font face=/.test(rendered.replace(/<!--\[if mso\]>[\s\S]*?<!\[endif\]-->/g, '')));
check('stored body: wrapper rides in Safe payloads (never a raw comment the DOM could eat)', out.includes('{{ Safe "\\x3c!--[if\\x20mso]\\x3e\\x3cfont\\x20face=\\"Arial\\"\\x3e\\x3c![endif]--\\x3e" }}'));

// Editor/reader parity is font-family-parity.test.cjs; the stacks themselves are unchanged
// (Q5): the layout default still leads with Helvetica Neue.
check('FONT_FAMILIES MODERN_SANS still leads with Helvetica Neue (stacks not reordered)', /key: 'MODERN_SANS',\s*label: '[^']+',\s*value: '"Helvetica Neue"/.test(src));
check('EmailLayout reader default still MODERN_SANS via FONT_FAMILIES', /fontFamily \?\? 'MODERN_SANS'/.test(fs.readFileSync(path.join(__dirname, '..', 'src', 'documents', 'reader', 'core.tsx'), 'utf8')));

done();
