// CAMPAIGN-52-HARDENING T3 (I4), superseded for the shipped variant by BIBLE-OUTLOOK-FIXES-SPEC
// I5. postProcess.ts ships VML_LABEL_VARIANT = 'textpath': the Word label is VML text
// (v:textpath) whose colour is a VML attribute Word's dark transform never inverts (hazard 54).
// A Button outside spec S5 compiles to the §4.3 group — the fill on a self-closed roundrect, the
// Button's TEXT colour (never its border colour) as the text shape's fillcolor, the escaped label
// in `string`, no <center>, no anchorlock, no v:textbox. Each S5 case compiles to exactly the
// 'border' output erinos.188 (b5ceb3ce) shipped — the literals below were captured from that
// commit's postProcess.ts. Behaviour (Word's dark transform) is external-client rendering —
// gates G1/G6, not this suite.
const { pp, canvas, decodeSafe, makeChecker, inlineButton, fullWidthButton } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

check('VML_LABEL_VARIANT ships textpath (spec §4.3)', pp.VML_LABEL_VARIANT === 'textpath', String(pp.VML_LABEL_VARIANT));

const word = (html) => (decodeSafe(pp.postProcess(canvas(html), { outlook: true })).match(/<!--\[if mso\]><v:[\s\S]*?<!\[endif\]-->/) || [''])[0];

// ---- the textpath shape (the inline fixture: #FFFFFF label, #000000 fill, 2px #fbf00b border) ----
const vml = word(inlineButton);
check('one v:group, inside one [if mso] conditional', /^<!--\[if mso\]><v:group [^>]*>[\s\S]*<\/v:group><!\[endif\]-->$/.test(vml), vml);
check('group sized in pt with a px x 10 coordinate space',
  /<v:group xmlns:v="urn:schemas-microsoft-com:vml" xmlns:w="urn:schemas-microsoft-com:office:word" style="width:113.25pt;height:33.75pt" coordorigin="0,0" coordsize="1510,450">/.test(vml), vml);
const roundrect = (vml.match(/<v:roundrect [^>]*\/>/) || [''])[0];
check('roundrect is self-closed and covers the box', /style="position:absolute;left:0;top:0;width:1510;height:450"/.test(roundrect), roundrect);
check('fill stays the button colour', /fillcolor="#000000"/.test(roundrect), roundrect);
check('stroke is the border colour, weight as today', /strokecolor="#fbf00b" strokeweight="1.5pt"/.test(roundrect), roundrect);
const shape = (vml.match(/<v:shape [^>]*>[\s\S]*?<\/v:shape>/) || [''])[0];
check('text shape: the Button\'s TEXT colour as fillcolor, never the border colour (U8)',
  /<v:shape href="https:\/\/x\.test\/go" style="position:absolute;left:0;top:0;width:1510;height:450" coordsize="21600,21600" path="m0,10800l21600,10800e" fillcolor="#FFFFFF" stroked="f">/.test(shape)
    && !/fillcolor="#fbf00b"/.test(shape), shape);
check('textpath carries the label, quoted Word font, pt size, weight, centred',
  /<v:path textpathok="t"\/><v:textpath on="t" fitpath="f" fitshape="f" string="Inline CTA" style="font-family:&quot;Arial&quot;;font-size:12pt;font-weight:bold;v-text-align:center"\/><\/v:shape>/.test(shape), shape);
check('no <center>, no anchorlock, no v:textbox', !/<center|anchorlock|v:textbox/.test(vml));
check('both shapes carry the href', (vml.match(/href="https:\/\/x\.test\/go"/g) || []).length === 2);

// ---- the font, weight and escaping rules -------------------------------------------------------
const styled = (style, label = 'Go') => word(`<div style="padding:0px"><a href="https://x.test/go" style="color:#ffffff;font-size:20px;background-color:#123456;display:inline-block;padding:10px 10px 10px 10px;${style}">${label}</a></div>`);
const textpath = (v) => (v.match(/<v:textpath [^>]*\/>/) || [''])[0];
check('font: the stack\'s first allowlisted family (MODERN_SANS -> Arial)', /font-family:&quot;Arial&quot;/.test(textpath(styled('font-family:&quot;Helvetica Neue&quot;, &quot;Arial Nova&quot;, Arial, sans-serif'))));
check('font: GEOMETRIC_SANS -> Corbel', /font-family:&quot;Corbel&quot;/.test(textpath(styled('font-family:Avenir, &quot;Avenir Next LT Pro&quot;, Montserrat, Corbel, sans-serif'))));
check('font: an allowlisted lead family is used as is', /font-family:&quot;Georgia&quot;/.test(textpath(styled('font-family:Georgia, serif'))));
check('font: a stack with no allowlisted family -> Arial', /font-family:&quot;Arial&quot;/.test(textpath(styled('font-family:Roboto, sans-serif'))));
check('size: 20px -> 15pt', /font-size:15pt/.test(textpath(styled(''))));
for (const [w, want] of [['bold', 'bold'], ['bolder', 'bold'], ['600', 'bold'], ['900', 'bold'], ['500', 'normal'], ['normal', 'normal'], ['lighter', 'normal']]) {
  check(`weight: ${w} -> ${want}`, new RegExp(`font-weight:${want};`).test(textpath(styled(`font-weight:${w}`))), textpath(styled(`font-weight:${w}`)));
}
check('a #RGB text colour is passed through as written', /<v:shape [^>]*fillcolor="#ffffff"/.test(styled('')));
const escaped = textpath(styled('', 'Tom &amp; Jerry\'s &lt;50%&gt; "deal" é ü — €5'));
check('label is attribute-escaped in string', /string="Tom &amp; Jerry's &lt;50%&gt; &quot;deal&quot; é ü — €5"/.test(escaped), escaped);

// ---- S5: each fallback is exactly erinos.188's 'border' output --------------------------------
const inline = (label, color = '#FFFFFF') => `<div style="text-align:center;padding:0px 24px 20px 24px"><a href="https://x.test/go" target="_blank" style="color:${color};font-size:16px;font-weight:bold;background-color:#000000;border-radius:64px;display:inline-block;padding:12px 20px 12px 20px;text-decoration:none;border:2px solid #fbf00b">${label}</a></div>`;
const full = (label) => `<div style="text-align:center;padding:0px 24px 8px 24px"><a href="https://x.test/wide" style="color:#FFFFFF;font-size:16px;font-weight:bold;background-color:#2563EB;border-radius:4px;display:block;padding:12px 20px 12px 20px;text-decoration:none;width:100%">${label}</a></div>`;
const BORDER_188 = {
  empty: "<!--[if mso]><v:roundrect xmlns:v=\"urn:schemas-microsoft-com:vml\" xmlns:w=\"urn:schemas-microsoft-com:office:word\" href=\"https://x.test/go\" style=\"height:33.75pt;v-text-anchor:middle;width:32.25pt;\" arcsize=\"50%\" strokecolor=\"#fbf00b\" strokeweight=\"1.5pt\" fillcolor=\"#000000\"><w:anchorlock/><center style=\"color:#fbf00b;font-family:Arial, sans-serif;font-size:12pt;font-weight:bold;\"></center></v:roundrect><![endif]-->",
  multiline: "<!--[if mso]><v:roundrect xmlns:v=\"urn:schemas-microsoft-com:vml\" xmlns:w=\"urn:schemas-microsoft-com:office:word\" href=\"https://x.test/wide\" style=\"height:47.25pt;v-text-anchor:middle;width:414pt;\" arcsize=\"6%\" strokecolor=\"#2563EB\" strokeweight=\"0.75pt\" fillcolor=\"#2563EB\"><w:anchorlock/><center style=\"color:#2563EB;font-family:Arial, sans-serif;font-size:12pt;font-weight:bold;\">Discover everything new in the autumn collection, every single piece, today and tomorrow</center></v:roundrect><![endif]-->",
  nonhex: "<!--[if mso]><v:roundrect xmlns:v=\"urn:schemas-microsoft-com:vml\" xmlns:w=\"urn:schemas-microsoft-com:office:word\" href=\"https://x.test/go\" style=\"height:33.75pt;v-text-anchor:middle;width:113.25pt;\" arcsize=\"50%\" strokecolor=\"#fbf00b\" strokeweight=\"1.5pt\" fillcolor=\"#000000\"><w:anchorlock/><center style=\"color:#fbf00b;font-family:Arial, sans-serif;font-size:12pt;font-weight:bold;\">Inline CTA</center></v:roundrect><![endif]-->",
  outside: "<!--[if mso]><v:roundrect xmlns:v=\"urn:schemas-microsoft-com:vml\" xmlns:w=\"urn:schemas-microsoft-com:office:word\" href=\"https://x.test/go\" style=\"height:33.75pt;v-text-anchor:middle;width:88.5pt;\" arcsize=\"50%\" strokecolor=\"#fbf00b\" strokeweight=\"1.5pt\" fillcolor=\"#000000\"><w:anchorlock/><center style=\"color:#fbf00b;font-family:Arial, sans-serif;font-size:12pt;font-weight:bold;\">Shop 今日</center></v:roundrect><![endif]-->",
  emoji: "<!--[if mso]><v:roundrect xmlns:v=\"urn:schemas-microsoft-com:vml\" xmlns:w=\"urn:schemas-microsoft-com:office:word\" href=\"https://x.test/go\" style=\"height:33.75pt;v-text-anchor:middle;width:88.5pt;\" arcsize=\"50%\" strokecolor=\"#fbf00b\" strokeweight=\"1.5pt\" fillcolor=\"#000000\"><w:anchorlock/><center style=\"color:#fbf00b;font-family:Arial, sans-serif;font-size:12pt;font-weight:bold;\">Shop 🛍</center></v:roundrect><![endif]-->",
};
const cases = {
  empty: inline(''),
  multiline: full('Discover everything new in the autumn collection, every single piece, today and tomorrow'),
  nonhex: inline('Inline CTA', 'rgb(255, 255, 255)'),
  outside: inline('Shop 今日'),
  emoji: inline('Shop 🛍'),
};
for (const [name, html] of Object.entries(cases)) {
  const got = word(html);
  check(`S5 ${name}: exactly the erinos.188 'border' output`, got === BORDER_188[name], got);
  const raw = pp.postProcess(canvas(html), { outlook: true });
  check(`S5 ${name}: one href marker (one shape)`, (raw.match(/<span data-lm-vml-href=/g) || []).length === 1);
}
check('S5: a one-line full-width Button is NOT a fallback', /<v:group /.test(word(fullWidthButton)) && /string="Wide CTA"/.test(word(fullWidthButton)));
check('S5: a label of U+0020, U+024F, U+2014 and U+20AC is NOT a fallback',
  /<v:group /.test(word(inline('A ɏ — €'))));
check('S5: a non-hex keyword colour IS a fallback', /<center /.test(word(inline('Go', 'white'))));


// ---- review fix F3: a custom-width inline Button whose label does not fit wraps in CSS, so it
// keeps the wrapping <center> shape; a label that fits still gets the group. Literal captured
// from b5ceb3ce's postProcess.ts.
const custom = (label) => `<div style="text-align:center;padding:0px 24px 20px 24px"><a href="https://x.test/go" target="_blank" style="color:#FFFFFF;font-size:16px;font-weight:bold;background-color:#000000;border-radius:64px;display:inline-block;padding:12px 20px 12px 20px;text-decoration:none;border:2px solid #fbf00b;width:120px;box-sizing:border-box">${label}</a></div>`;
const NARROW_188 = "<!--[if mso]><v:roundrect xmlns:v=\"urn:schemas-microsoft-com:vml\" xmlns:w=\"urn:schemas-microsoft-com:office:word\" href=\"https://x.test/go\" style=\"height:33.75pt;v-text-anchor:middle;width:90pt;\" arcsize=\"50%\" strokecolor=\"#fbf00b\" strokeweight=\"1.5pt\" fillcolor=\"#000000\"><w:anchorlock/><center style=\"color:#fbf00b;font-family:Arial, sans-serif;font-size:12pt;font-weight:bold;\">Shop the whole autumn collection</center></v:roundrect><![endif]-->";
check('F3: a width:120px Button with a long label is exactly the erinos.188 output', word(custom('Shop the whole autumn collection')) === NARROW_188, word(custom('Shop the whole autumn collection')));
// Re-review R1/R3: the fit model is characters x 16 x 0.65 + side padding (40) + both borders (4)
// inside the 120 px box. "Shop now" (8 characters, 83.2) fits alone but not with padding.
check('F3: a label that fits alone but not with its padding and borders is a fallback', /^<!--\[if mso\]><v:roundrect [\s\S]*<center /.test(word(custom('Shop now'))), word(custom('Shop now')).slice(0, 60));
check('F3: a seven-character label (72.8 + 44 = 116.8) fits and gets the group', /^<!--\[if mso\]><v:group /.test(word(custom('Shop it'))), word(custom('Shop it')).slice(0, 60));
check('F3: a width:120px Button whose label fits still gets the group', /^<!--\[if mso\]><v:group [^>]*style="width:90pt;/.test(word(custom('Go'))) && /string="Go"/.test(word(custom('Go'))));

// ---- review fix F4: the label's printable set, and the colour forms ---------------------------
const cp = (n) => String.fromCodePoint(n);
const hex = (n) => `U+${n.toString(16).toUpperCase().padStart(4, '0')}`;
const FALLS_BACK = [0x007F, 0x0080, 0x0085, 0x009F, 0x00AD, 0x200C, 0x200D, 0x200F, 0x0250, 0x1FFF, 0x2000, 0x200B, 0x202E, 0x205F, 0x2060, 0x2070, 0x209F, 0x20D0];
const ALLOWED = [0x007E, 0x00A0, 0x00AC, 0x00AE, 0x024F, 0x2010, 0x2027, 0x2030, 0x205E, 0x20A0, 0x20CF];
for (const n of FALLS_BACK) check(`F4: ${hex(n)} is outside the label set`, pp.textpathLabelAllowed(`Go${cp(n)}far`) === false);
for (const n of ALLOWED) check(`F4: ${hex(n)} is inside the label set`, pp.textpathLabelAllowed(`Go${cp(n)}far`) === true);
// Through the compile: a label is whitespace-collapsed first (/\s+/ -> ' ', unchanged by this
// fix), so U+2000 and U+205F (JS whitespace) arrive as a plain space and never reach the check;
// every other character arrives as typed.
const JS_SPACE = new Set([0x00A0, 0x2000, 0x205F]);
for (const n of FALLS_BACK.filter((x) => !JS_SPACE.has(x))) {
  check(`F4: a label holding ${hex(n)} compiles to the fallback`, /^<!--\[if mso\]><v:roundrect [\s\S]*<center /.test(word(inline(`Go${cp(n)}far`))));
}
for (const n of ALLOWED.filter((x) => !JS_SPACE.has(x))) {
  check(`F4: a label holding ${hex(n)} compiles to the group`, /^<!--\[if mso\]><v:group /.test(word(inline(`Go${cp(n)}far`))));
}
for (const n of [0x2000, 0x205F]) {
  check(`F4: ${hex(n)} in a label is collapsed to a plain space before the check (never drawn)`, word(inline(`Go${cp(n)}far`)).includes('string="Go far"'));
}
check('F4: #ffff (4 digits) falls back', /<center /.test(word(inline('Go', '#ffff'))));
check('F4: #ffffffff (8 digits) falls back', /<center /.test(word(inline('Go', '#ffffffff'))));

done();
