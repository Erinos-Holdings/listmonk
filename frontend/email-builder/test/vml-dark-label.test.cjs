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
check('S5: a #RRGGBB / #RGB label at the range edges (U+0020, U+024F, U+2000-U+206F, U+20A0-U+20CF) is NOT a fallback',
  /<v:group /.test(word(inline('A ɏ — €'))));
check('S5: a non-hex keyword colour IS a fallback', /<center /.test(word(inline('Go', 'white'))));

done();
