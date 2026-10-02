// BIBLE-OUTLOOK-FIXES-SPEC I7 and I14 (§4.4, defect F). Word ignores table-layout:fixed and shares
// a column row by content width, so with the Outlook flag on every auto-width cell of a builder
// ColumnsContainer (table-layout:fixed) carries the class `lm-cw-<n>`, n its content-box share,
// and <head> carries ONE Word-only style block — a Safe payload holding
// `<!--[if mso]><style>td.lm-cw-<n>{width:<n>px}…</style><![endif]-->`, one flat rule per distinct
// n, ascending. An explicit-width cell carries no class; no column cell gains a width attribute
// or an inline width (a px-pinned cell breaks Outlook mobile and the Gmail apps, hazard 91); a
// fixed-layout table inside a user Html fence gains nothing. I14: the payload holds no `{{` and
// no `}}` but its own Safe delimiters (the review's D2.2 template check). Whether Word honours the
// rule is client behaviour: gates G1/G6.
const { JSDOM, pp, canvas, decodeSafe, makeChecker } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

// The builder's ColumnsContainer shape (vendored block-columns-container through the reader),
// inside its block's padded wrapper div. `cells` is a list of [cell style, cell contents].
function columns(cells, wrapper = 'padding:16px 24px 16px 24px') {
  return `<div style="${wrapper}"><table align="center" width="100%" cellpadding="0" border="0" style="table-layout:fixed;border-collapse:collapse"><tbody style="width:100%"><tr style="width:100%">`
    + cells.map(([style, inner]) => `<td style="box-sizing:content-box;vertical-align:top;${style}">${inner}</td>`).join('')
    + '</tr></tbody></table></div>';
}
const text = (t) => `<div style="font-weight:normal;padding:16px 8px 16px 8px"><p>${t}</p></div>`;

function compile(inner, outlook = true) {
  const raw = pp.postProcess(canvas(inner), { outlook });
  return { raw, doc: new JSDOM(raw).window.document };
}
// The innermost COLUMN cell (the builder's content-box cell) holding text `t`.
const cellWith = (doc, t) => Array.from(doc.querySelectorAll('td')).filter((td) => /box-sizing:content-box/.test(td.getAttribute('style') || '') && td.textContent.includes(t)).pop();
const cwClasses = (td) => ((td && td.getAttribute('class')) || '').split(/\s+/).filter((c) => c.startsWith('lm-cw-'));
// The head's Word column payload, decoded, and its raw Safe string.
function headRules(raw) {
  const head = raw.slice(raw.indexOf('<head>'), raw.indexOf('</head>'));
  const safes = [...head.matchAll(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g)].filter((m) => /lm-cw-/.test(m[0]));
  return { count: safes.length, raw: safes.map((m) => m[0]), decoded: safes.map((m) => decodeSafe(m[0])) };
}
function noPinnedWidth(doc) {
  return Array.from(doc.querySelectorAll('td[class*="lm-cw-"]')).every((td) => !td.hasAttribute('width') && !/(^|;)\s*width\s*:/.test(td.getAttribute('style') || ''));
}

// ---- two columns: 600 canvas - 48 block padding = 552; two 276 tracks less the 8 px gap half ----
{
  const { raw, doc } = compile(columns([['padding-left:0;padding-right:8px', text('Left')], ['padding-left:8px;padding-right:0', text('Right side, with much longer content than the left')]]));
  check('I7 two columns: each auto cell carries lm-cw-268', cwClasses(cellWith(doc, 'Left')).join() === 'lm-cw-268' && cwClasses(cellWith(doc, 'Right side')).join() === 'lm-cw-268');
  const rules = headRules(raw);
  check('I7 two columns: one Word-only style payload in <head> with one rule', rules.count === 1
    && rules.decoded[0] === '<!--[if mso]><style>td.lm-cw-268{width:268px}</style><![endif]-->', rules.decoded.join(' | '));
  check('I7 two columns: no column cell gains a width attribute or inline width', noPinnedWidth(doc));
  check('I7: the class is the only change to the cell (style untouched)', cellWith(doc, 'Left').getAttribute('style') === 'box-sizing:content-box;vertical-align:top;padding-left:0;padding-right:8px');
  check('I7: the style block is NOT in the body', !/td\.lm-cw-/.test(raw.slice(raw.indexOf('<body'))));
  // I14
  const inner = rules.raw[0].slice('{{ Safe "'.length, -'" }}'.length);
  check('I14: the payload holds no {{ and no }} besides its own delimiters', !/\{\{|\}\}/.test(inner), inner);
  check('I14: the payload is one well-formed Safe action (single token, no raw space or quote)', /^\{\{ Safe "(?:[^"\\ \n]|\\.)*" \}\}$/.test(rules.raw[0]));
}

// ---- three columns: 552 / 3 = 184 a track; content-box shares 179 / 178 / 179 ----------------
{
  const { raw, doc } = compile(columns([
    ['padding-left:0;padding-right:5.333333333333333px', text('One')],
    ['padding-left:2.6666666666666665px;padding-right:2.6666666666666665px', text('Two')],
    ['padding-left:5.333333333333333px;padding-right:0', text('Three')],
  ]));
  check('I7 three columns: content-box shares 179 / 178 / 179',
    cwClasses(cellWith(doc, 'One')).join() === 'lm-cw-179' && cwClasses(cellWith(doc, 'Two')).join() === 'lm-cw-178' && cwClasses(cellWith(doc, 'Three')).join() === 'lm-cw-179');
  check('I7 three columns: one rule per distinct width, ascending',
    headRules(raw).decoded.join() === '<!--[if mso]><style>td.lm-cw-178{width:178px}td.lm-cw-179{width:179px}</style><![endif]-->', headRules(raw).decoded.join());
  check('I7 three columns: no pinned widths', noPinnedWidth(doc));
}

// ---- a nested row: a two-column row inside the left column of a two-column row ---------------
{
  const nested = `<table align="center" width="100%" cellpadding="0" border="0" style="table-layout:fixed;border-collapse:collapse"><tbody style="width:100%"><tr style="width:100%">`
    + `<td style="box-sizing:content-box;vertical-align:top;padding-left:0;padding-right:4px">${text('Inner A')}</td>`
    + `<td style="box-sizing:content-box;vertical-align:top;padding-left:4px;padding-right:0">${text('Inner B')}</td></tr></tbody></table>`;
  const { raw, doc } = compile(columns([['padding-left:0;padding-right:8px', nested], ['padding-left:8px;padding-right:0', text('Outer right')]]));
  const innerA = cellWith(doc, 'Inner A');
  check('I7 nested: the outer cells carry 268', cwClasses(cellWith(doc, 'Outer right')).join() === 'lm-cw-268');
  check('I7 nested: the inner cells carry their share of 268 (134 - 4 = 130)', cwClasses(innerA).join() === 'lm-cw-130', cwClasses(innerA).join());
  check('I7 nested: both widths have a rule', headRules(raw).decoded.join() === '<!--[if mso]><style>td.lm-cw-130{width:130px}td.lm-cw-268{width:268px}</style><![endif]-->', headRules(raw).decoded.join());
  check('I7 nested: no pinned widths', noPinnedWidth(doc));
}

// ---- a row mixing a fixed and an auto cell -----------------------------------------------------
{
  const { raw, doc } = compile(columns([['padding-left:0;padding-right:8px;width:200px', text('Fixed cell')], ['padding-left:8px;padding-right:0', text('Auto cell')]]));
  check('I7 mixed: the explicit-width cell carries no class', cwClasses(cellWith(doc, 'Fixed cell')).length === 0 && !cellWith(doc, 'Fixed cell').hasAttribute('class'));
  check('I7 mixed: the auto cell takes the rest (552 - 208 - 8 = 336)', cwClasses(cellWith(doc, 'Auto cell')).join() === 'lm-cw-336');
  check('I7 mixed: one rule, for the auto cell only', headRules(raw).decoded.join() === '<!--[if mso]><style>td.lm-cw-336{width:336px}</style><![endif]-->');
}

// ---- review fix F2: a bordered Container's border is inside no column's share ------------------
// Canvas 600; Container border 10 px a side + padding 20 px a side → 540; the row's own block
// padding 24 px a side → 492; two 246 tracks less the 8 px gap half → 238 (248 if the border
// were ignored).
{
  const container = (inner) => `<div style="border:10px solid #fbf00b;padding:20px 20px 20px 20px">${inner}</div>`;
  const { raw, doc } = compile(container(columns([['padding-left:0;padding-right:8px', text('Boxed left')], ['padding-left:8px;padding-right:0', text('Boxed right')]])));
  check('F2: a row inside a 10px-bordered, 20px-padded Container gets lm-cw-238', cwClasses(cellWith(doc, 'Boxed left')).join() === 'lm-cw-238' && cwClasses(cellWith(doc, 'Boxed right')).join() === 'lm-cw-238',
    `${cwClasses(cellWith(doc, 'Boxed left'))} ${cwClasses(cellWith(doc, 'Boxed right'))}`);
  check('F2: and its rule is 238', headRules(raw).decoded.join() === '<!--[if mso]><style>td.lm-cw-238{width:238px}</style><![endif]-->', headRules(raw).decoded.join());
  // An over-wide image in the same Container is clamped to the width inside the border: 540.
  const img = compile(container('<div style="padding:0px 0px 0px 0px"><img alt="wide" src="https://x.test/w.png" width="700" style="width:700px;max-width:100%"></div>'));
  const w = (img.raw.match(/<img[^>]*alt="wide"[^>]*>/) || [''])[0].match(/ width="(\d+)"/);
  check('F2: an over-wide image in a bordered Container is clamped inside the border (540)', w && w[1] === '540', w && w[1]);
}

// ---- review fix F5: a th never gets a class; a user-typed lm-cw- class makes no rule ------------
{
  const thRow = `<div style="padding:16px 24px 16px 24px"><table align="center" width="100%" cellpadding="0" border="0" style="table-layout:fixed;border-collapse:collapse"><tbody><tr>`
    + '<th style="padding-right:8px">Head cell</th><td style="box-sizing:content-box;vertical-align:top;padding-left:8px;padding-right:0">Data cell</td></tr></tbody></table></div>';
  const { raw, doc } = compile(thRow);
  const th = doc.querySelector('th');
  check('F5: a th cell gets no lm-cw- class', th && !th.hasAttribute('class'), th && th.outerHTML);
  check('F5: the td beside it still does', cwClasses(cellWith(doc, 'Data cell')).length === 1);
  const typed = compile(`<div data-lm-user-html="true" style="padding:16px 24px 16px 24px"><table width="100%"><tbody><tr><td class="lm-cw-777">typed</td></tr></tbody></table>`
    + '<table width="100%" style="table-layout:fixed"><tbody><tr><td class="lm-cw-777">typed fixed</td></tr></tbody></table></div>'
    + columns([['padding-left:0;padding-right:8px', text('Real left')], ['padding-left:8px;padding-right:0', text('Real right')]]));
  check('F5: a user-typed class="lm-cw-777" inside an Html block produces no lm-cw-777 rule', !/td\.lm-cw-777/.test(typed.raw) && headRules(typed.raw).decoded.join() === '<!--[if mso]><style>td.lm-cw-268{width:268px}</style><![endif]-->', headRules(typed.raw).decoded.join());
  check('F5: and the typed class itself is left as typed', (typed.raw.match(/class="lm-cw-777"/g) || []).length === 2);
  const typedOnly = compile('<div data-lm-user-html="true"><table width="100%"><tbody><tr><td class="lm-cw-777">only typed</td></tr></tbody></table></div>');
  check('F5: a body whose only lm-cw- class is user-typed gets no style payload', headRules(typedOnly.raw).count === 0);
}

// ---- no columns: no payload; a user Html fence: nothing; the flag off: nothing ------------------
{
  const { raw } = compile(text('Just text'));
  check('I7: a document without column rows gets no style payload', headRules(raw).count === 0 && !/lm-cw-/.test(raw));
  const fenced = compile(`<div data-lm-user-html="true" style="padding:16px 24px 16px 24px"><table width="100%" style="table-layout:fixed"><tbody><tr><td style="padding:0 8px 0 0">user a</td><td style="padding:0 0 0 8px">user b</td></tr></tbody></table></div>`);
  check('I7: a fixed-layout table inside a user Html fence gains no class and no rule', !/lm-cw-/.test(fenced.raw));
  check('I7: the temporary fence mark never reaches the output', !/data-lm-cw-fenced/.test(fenced.raw));
  const unpadded = compile(`<div data-lm-user-html="true"><table width="100%" style="table-layout:fixed"><tbody><tr><td>user c</td><td>user d</td></tr></tbody></table></div>`);
  check('I7: also when the fence wrapper stays a div', !/lm-cw-|data-lm-cw-fenced/.test(unpadded.raw));
  const off = compile(columns([['padding-left:0;padding-right:8px', text('Left')], ['padding-left:8px;padding-right:0', text('Right')]]), false);
  check('I7 flag off: no class, no payload', !/lm-cw-/.test(off.raw) && !/\{\{ Safe/.test(off.raw));
}

done();
