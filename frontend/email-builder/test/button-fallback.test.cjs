// BIBLE-OUTLOOK-FIXES-SPEC §12 (Amendment A) IA8: under VML_LABEL_VARIANT 'textpath', a Button
// for which VML text cannot carry the label (spec S5) compiles its Word copy to the table-cell
// button of §12.3 — no VML element, exactly one href marker, the stamp
// `data-lm-btn-fallback` naming every cause that holds in SA14's order (empty, lines, width,
// colour, chars), the cell widths and height by §12.3's arithmetic — while everything outside the
// Word copy compiles exactly as at erinos.190 (pinned by the non-Word twins below).
//
// integrations lib/campaign-review/value-rules.ts STAMP matches the table's opening attributes byte for
// byte up to the stamp: reordering or adding an attribute before data-lm-btn-fallback makes D4.7
// blind, and this fixture (copied to integrations) is what catches it.
// FIXTURE is the compiled Word copy (the stored form: Safe payloads and the href marker) of one
// fallback Button per cause. This file writes nothing. The SAME strings are copied into the
// integrations repository's tests/fixtures/campaign-review/button-fallback.json, which the
// review's D4.7 test reads, so the rule is tested on what this compile emits (SA15, IA11). A
// change to the shape changes both files.
const { pp, canvas, decodeSafe, makeChecker, inlineButton, fullWidthButton } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

const inline = (label, color = '#FFFFFF', extra = '') => `<div style="text-align:center;padding:0px 24px 20px 24px"><a href="https://x.test/go" target="_blank" style="color:${color};font-size:16px;font-weight:bold;background-color:#000000;border-radius:64px;display:inline-block;padding:12px 20px 12px 20px;text-decoration:none;border:2px solid #fbf00b${extra}">${label}</a></div>`;
const full = (label, color = '#FFFFFF', extra = '') => `<div style="text-align:center;padding:0px 24px 8px 24px"><a href="https://x.test/wide" style="color:${color};font-size:16px;font-weight:bold;background-color:#2563EB;border-radius:4px;display:block;padding:12px 20px 12px 20px;text-decoration:none;width:100%${extra}">${label}</a></div>`;

// One input per cause, plus several causes at once and a custom width and height.
const INPUTS = {
  empty: inline(''),
  lines: full('Discover everything new in the autumn collection, every single piece, today and tomorrow'),
  width: inline('Shop the whole autumn collection', '#FFFFFF', ';width:120px;box-sizing:border-box'),
  colour: inline('Inline CTA', 'rgb(255, 255, 255)'),
  chars: inline('Shop →'),
  several: inline('Shop 今日', 'white', ';width:60px;box-sizing:border-box'),
  sized: inline('Shop →', '#FFFFFF', ';width:120px;box-sizing:border-box;height:64px'),
  // Implementation review F9: a full-width Button with a CSS height; `empty` and `lines` each with
  // another cause; a full-width fallback carrying an author border.
  fullHeight: full('Shop now →', '#FFFFFF', ';height:64px'),
  emptyColour: inline('', 'white'),
  linesColour: full('Discover everything new in the autumn collection, every single piece, today and tomorrow', 'rgb(255, 255, 255)'),
  fullBorder: full('Discover everything new in the autumn collection, every single piece, today and tomorrow', '#FFFFFF', ';border:3px solid #FFCC00'),
};

const FIXTURE = {
  empty: {
    causes: "empty",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"empty\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#000000\\\"\\x20style=\\\"background-color:#000000;padding:12px\\x2020px\\x2012px\\x2020px;border:2px\\x20solid\\x20#fbf00b;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/go\"></span>{{ Safe \"\\\"\\x20style=\\\"color:#FFFFFF;font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:#FFFFFF\\\"\\x3e\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  lines: {
    causes: "lines",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"lines\\\"\\x20width=\\\"550\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#2563EB\\\"\\x20width=\\\"508\\\"\\x20style=\\\"background-color:#2563EB;padding:12px\\x2020px\\x2012px\\x2020px;border:1px\\x20solid\\x20#2563EB;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/wide\"></span>{{ Safe \"\\\"\\x20style=\\\"color:#FFFFFF;font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:#FFFFFF\\\"\\x3eDiscover\\x20everything\\x20new\\x20in\\x20the\\x20autumn\\x20collection,\\x20every\\x20single\\x20piece,\\x20today\\x20and\\x20tomorrow\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  width: {
    causes: "width",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"width\\\"\\x20width=\\\"120\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#000000\\\"\\x20width=\\\"76\\\"\\x20style=\\\"background-color:#000000;padding:12px\\x2020px\\x2012px\\x2020px;border:2px\\x20solid\\x20#fbf00b;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/go\"></span>{{ Safe \"\\\"\\x20style=\\\"color:#FFFFFF;font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:#FFFFFF\\\"\\x3eShop\\x20the\\x20whole\\x20autumn\\x20collection\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  colour: {
    causes: "colour",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"colour\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#000000\\\"\\x20style=\\\"background-color:#000000;padding:12px\\x2020px\\x2012px\\x2020px;border:2px\\x20solid\\x20#fbf00b;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/go\"></span>{{ Safe \"\\\"\\x20style=\\\"color:rgb(255,\\x20255,\\x20255);font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:rgb(255,\\x20255,\\x20255)\\\"\\x3eInline\\x20CTA\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  chars: {
    causes: "chars",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"chars\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#000000\\\"\\x20style=\\\"background-color:#000000;padding:12px\\x2020px\\x2012px\\x2020px;border:2px\\x20solid\\x20#fbf00b;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/go\"></span>{{ Safe \"\\\"\\x20style=\\\"color:#FFFFFF;font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:#FFFFFF\\\"\\x3eShop\\x20→\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  several: {
    causes: "width colour chars",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"width\\x20colour\\x20chars\\\"\\x20width=\\\"60\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#000000\\\"\\x20width=\\\"16\\\"\\x20style=\\\"background-color:#000000;padding:12px\\x2020px\\x2012px\\x2020px;border:2px\\x20solid\\x20#fbf00b;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/go\"></span>{{ Safe \"\\\"\\x20style=\\\"color:white;font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:white\\\"\\x3eShop\\x20今日\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  sized: {
    causes: "chars",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"chars\\\"\\x20width=\\\"120\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#000000\\\"\\x20width=\\\"76\\\"\\x20height=\\\"36\\\"\\x20valign=\\\"middle\\\"\\x20style=\\\"background-color:#000000;padding:12px\\x2020px\\x2012px\\x2020px;border:2px\\x20solid\\x20#fbf00b;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/go\"></span>{{ Safe \"\\\"\\x20style=\\\"color:#FFFFFF;font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:#FFFFFF\\\"\\x3eShop\\x20→\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  fullHeight: {
    causes: "chars",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"chars\\\"\\x20width=\\\"550\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#2563EB\\\"\\x20width=\\\"508\\\"\\x20height=\\\"38\\\"\\x20valign=\\\"middle\\\"\\x20style=\\\"background-color:#2563EB;padding:12px\\x2020px\\x2012px\\x2020px;border:1px\\x20solid\\x20#2563EB;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/wide\"></span>{{ Safe \"\\\"\\x20style=\\\"color:#FFFFFF;font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:#FFFFFF\\\"\\x3eShop\\x20now\\x20→\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  emptyColour: {
    causes: "empty colour",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"empty\\x20colour\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#000000\\\"\\x20style=\\\"background-color:#000000;padding:12px\\x2020px\\x2012px\\x2020px;border:2px\\x20solid\\x20#fbf00b;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/go\"></span>{{ Safe \"\\\"\\x20style=\\\"color:white;font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:white\\\"\\x3e\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  linesColour: {
    causes: "lines colour",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"lines\\x20colour\\\"\\x20width=\\\"550\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#2563EB\\\"\\x20width=\\\"508\\\"\\x20style=\\\"background-color:#2563EB;padding:12px\\x2020px\\x2012px\\x2020px;border:1px\\x20solid\\x20#2563EB;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/wide\"></span>{{ Safe \"\\\"\\x20style=\\\"color:rgb(255,\\x20255,\\x20255);font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:rgb(255,\\x20255,\\x20255)\\\"\\x3eDiscover\\x20everything\\x20new\\x20in\\x20the\\x20autumn\\x20collection,\\x20every\\x20single\\x20piece,\\x20today\\x20and\\x20tomorrow\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
  fullBorder: {
    causes: "lines",
    word: "{{ Safe \"\\x3c!--[if\\x20mso]\\x3e\\x3ctable\\x20role=\\\"presentation\\\"\\x20border=\\\"0\\\"\\x20cellpadding=\\\"0\\\"\\x20cellspacing=\\\"0\\\"\\x20data-lm-btn-fallback=\\\"lines\\\"\\x20width=\\\"549\\\"\\x20style=\\\"border-collapse:separate\\\"\\x3e\\x3ctr\\x3e\\x3ctd\\x20align=\\\"center\\\"\\x20bgcolor=\\\"#2563EB\\\"\\x20width=\\\"503\\\"\\x20style=\\\"background-color:#2563EB;padding:12px\\x2020px\\x2012px\\x2020px;border:3px\\x20solid\\x20#FFCC00;\\\"\\x3e\\x3cfont\\x20face=\\\"Arial\\\"\\x3e\\x3ca\\x20href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/wide\"></span>{{ Safe \"\\\"\\x20style=\\\"color:#FFFFFF;font-family:Arial,\\x20sans-serif;font-size:16px;font-weight:bold;text-decoration:none\\\"\\x3e\\x3cspan\\x20style=\\\"color:#FFFFFF\\\"\\x3eDiscover\\x20everything\\x20new\\x20in\\x20the\\x20autumn\\x20collection,\\x20every\\x20single\\x20piece,\\x20today\\x20and\\x20tomorrow\\x3c/span\\x3e\\x3c/a\\x3e\\x3c/font\\x3e\\x3c/td\\x3e\\x3c/tr\\x3e\\x3c/table\\x3e\\x3c![endif]--\\x3e\" }}",
  },
};

// The Word copy: the Safe-wrapped [if mso] table that carries the stamp.
const WORD = /\{\{ Safe "\\x3c!--\[if\\x20mso\]\\x3e\\x3ctable\\x20role=\\"presentation\\"\\x20border=[\s\S]*?\\x3c!\[endif\]--\\x3e" \}\}/;
const compile = (html) => pp.postProcess(canvas(html), { outlook: true });
const SA14_ORDER = ['empty', 'lines', 'width', 'colour', 'chars'];

for (const [name, html] of Object.entries(INPUTS)) {
  const out = compile(html);
  const word = (out.match(WORD) || [''])[0];
  check(`${name}: the Word copy is the fixture, byte for byte`, word === FIXTURE[name].word, word.slice(0, 300));
  const decoded = decodeSafe(word);
  check(`${name}: no VML element`, !/<v:|<w:anchorlock|xmlns:v/.test(decoded));
  check(`${name}: exactly one href marker in the whole output`, (out.match(/<span data-lm-vml-href=/g) || []).length === 1);
  check(`${name}: one stamped table, inside one [if mso] conditional`, /^<!--\[if mso\]><table [^>]*data-lm-btn-fallback="[^"]*"[^>]*>[\s\S]*<\/table><!\[endif\]-->$/.test(decoded), decoded.slice(0, 200));
  const stamp = (/data-lm-btn-fallback="([^"]*)"/.exec(decoded) || [])[1];
  check(`${name}: the stamp names ${FIXTURE[name].causes}`, stamp === FIXTURE[name].causes, stamp);
  const causes = stamp ? stamp.split(' ') : [];
  check(`${name}: causes in SA14's order, each known`, causes.every((c) => SA14_ORDER.includes(c)) && causes.join(' ') === SA14_ORDER.filter((c) => causes.includes(c)).join(' '));
  check(`${name}: the link is the label, inside the Word font wrapper`, /<font face="Arial"><a href="[^"]*" style="color:[^"]*;text-decoration:none"><span style="color:[^"]*">[^<]*<\/span><\/a><\/font>/.test(decoded));
}

// The arithmetic (§12.3): {W} is the custom width (inline) or the column budget (full width); {w}
// is {W} less the horizontal padding and both borders; {h} is the CSS height less the vertical
// padding and both borders; an auto-width Button carries neither.
const dec = (name) => decodeSafe(FIXTURE[name].word);
check('width: W 120, w 120 - 2 x 20 - 2 x 2 = 76, no height', /<table [^>]* width="120" /.test(dec('width')) && /<td [^>]* width="76" style=/.test(dec('width')) && !/height=/.test(dec('width')));
// integrations RENDER-CATALOG-SPEC §17.5 F2: a full-width Word box is the column budget less 2 px.
check('lines: W is the column budget 552 less 2 (F2) = 550, w = 550 - 40 - 2 x 1 (the full-width hairline) = 508', /<table [^>]* width="550" /.test(dec('lines')) && /<td [^>]* width="508" style=/.test(dec('lines')));
check('sized: h 64 - 2 x 12 - 2 x 2 = 36, valign middle', /<td [^>]* width="76" height="36" valign="middle" style=/.test(dec('sized')));
check('several: w never below 1 (60 - 44 = 16)', /<td [^>]* width="16" style=/.test(dec('several')));
check('an auto-width Button carries no width', !/width=/.test(dec('chars').replace(/<a [\s\S]*$/, '')));
check('the border part appears with a border of width above 0', /border:2px solid #fbf00b;/.test(dec('chars')));
check('no border part without a border', !/border:\d/.test(decodeSafe((compile(inline('Shop →').replace(';border:2px solid #fbf00b', '')).match(WORD) || [''])[0])));
check('fullHeight: a full-width CSS height gives h = 64 - 2 x 12 - 2 x 1 (the hairline) = 38', /<table [^>]* width="550" /.test(dec('fullHeight')) && /<td [^>]* width="508" height="38" valign="middle" style=/.test(dec('fullHeight')));
check('fullBorder: the author border replaces the hairline, W = 552 - max(2, 3) (F2) = 549, w = 549 - 40 - 2 x 3 = 503', /<table [^>]* width="549" /.test(dec('fullBorder')) && /<td [^>]* width="503" style="[^"]*border:3px solid #FFCC00;"/.test(dec('fullBorder')));
check('emptyColour / linesColour: empty and lines combine with another cause, in SA14 order', /data-lm-btn-fallback="empty colour"/.test(dec('emptyColour')) && /data-lm-btn-fallback="lines colour"/.test(dec('linesColour')));
check('w and h never go below 1', /<td [^>]* width="1" height="1" valign="middle"/.test(decodeSafe((compile(inline('Shop →', '#FFFFFF', ';width:10px;box-sizing:border-box;height:10px')).match(WORD) || [''])[0])));
check('the label is HTML-escaped; colours and the stack are attribute-escaped',
  /<span style="color:#FFFFFF">Tom &amp; Jerry &lt;3 →<\/span>/.test(decodeSafe((compile(inline('Tom &amp; Jerry &lt;3 →')).match(WORD) || [''])[0])));
check('the font wrapper names the stack\'s first Word family (Georgia)', /<font face="Georgia">/.test(decodeSafe((compile(inline('Shop →').replace('font-weight:bold;', 'font-weight:bold;font-family:Georgia, serif;')).match(WORD) || [''])[0])));
check('an empty label compiles to the same shape with an empty span', /<span style="color:#FFFFFF"><\/span><\/a>/.test(dec('empty')));

// The non-Word twins are erinos.190's: the literals below were captured from 04459ffb's
// postProcess.ts (the commit erinos.190 builds) for the `chars` and `lines` inputs. The rest of
// each output outside the Word copy was compared with that commit in the implementation's scratch
// run; compile-unchanged.test.cjs pins whole documents.
const TWIN_190 = {
  chars: '<div class="lm-nomso" style="mso-hide:all"><a href="https://x.test/go" target="_blank" style="color:#FFFFFF;font-size:16px;font-weight:bold;background-color:#000000;border-radius:64px;display:inline-block;padding:12px 20px 12px 20px;text-decoration:none;border:2px solid #fbf00b">Shop →</a></div>',
  lines: '<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse;mso-table-lspace:0pt;mso-table-rspace:0pt;mso-hide:all" class="lm-gm-pin-552 lm-nomso"><tbody><tr><td data-lm-full-width-button="true" bgcolor="#2563EB" style="background-color:#2563EB;border-radius:4px;mso-hide:all"><a href="https://x.test/wide" style="color:#FFFFFF;font-size:16px;font-weight:bold;background-color:#2563EB;border-radius:4px;display:block;padding:12px 20px 12px 20px;text-decoration:none;width:100%;text-align:center;border:1px solid #2563EB">Discover everything new in the autumn collection, every single piece, today and tomorrow</a></td></tr></tbody></table>',
};
// §14: the lines twin is erinos.190's plus mso-hide:all on its anchor (the one declaration Word
// honours for the full-width twin, GC2) and the block wrapper around the table.
TWIN_190.lines = TWIN_190.lines.replace('border:1px solid #2563EB">Discover', 'border:1px solid #2563EB;mso-hide:all">Discover');
check('chars: the non-Word twin is erinos.190\'s', (compile(INPUTS.chars).match(/<div class="lm-nomso"[^>]*>[\s\S]*?<\/div>/) || [''])[0] === TWIN_190.chars);
check('lines: the non-Word twin is erinos.190\'s, inside the §14 block wrapper', (compile(INPUTS.lines).match(/<div class="lm-nomso" style="mso-hide:all"><table[^>]*lm-gm-pin[\s\S]*?<\/table><\/div>/) || [''])[0] === `<div class="lm-nomso" style="mso-hide:all">${TWIN_190.lines}</div>`);
check('a Button outside S5 is no fallback: the VML-text group, no stamp', /<v:group /.test(decodeSafe(compile(inlineButton))) && !/data-lm-btn-fallback/.test(compile(inlineButton)));
check('a one-line full-width Button is no fallback', /<v:group /.test(decodeSafe(compile(fullWidthButton))) && !/data-lm-btn-fallback/.test(compile(fullWidthButton)));
check('fallbackCauses is exported and agrees with the stamp', pp.fallbackCauses({ text: 'Shop 今日', textColor: 'white', narrow: true }).join(' ') === 'width colour chars');

done();
