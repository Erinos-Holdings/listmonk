// integrations RENDER-CATALOG-SPEC §17.5 -- the four Word fixes of phase C, each under the Outlook
// flag only, each proved by one candidate render on outlook2024_win_lm_dt:
//
//   F1 Divider: a Word-only one-cell table whose top border is the line, before the <hr>; the
//      <hr> wrapped in <div class="lm-nomso" style="mso-hide:all"> for every other client.
//   F2 full-width Button: the Word box (VML group, or a full-width fallback's table) is 2 px
//      narrower than its slot, floor 1; the Gmail pin class and the non-Word twin keep the slot.
//   F3 bordered canvas: the Word-only wrapper cell draws the layout's border; the canvas table
//      (style unchanged, border-collapse:collapse) gains the class lm-cvb, switched off in Word by
//      `table.lm-cvb{border:none}` in the Word-only head block. (A first attempt, separate on the
//      canvas table, still lost the right edge in Word.)
//   F4 zero-padding bordered Container: the table-cell form (borders on the td) with
//      border-collapse:separate on its table.
//
// And, for each: a flag-off compile, and a document holding no fixed shape, are byte-identical to
// the bundle before the fixes (test/fixtures/compile-snapshot-word-fixes-before.json).
const fs = require('fs');
const path = require('path');
const { loadUmd, compileInputs, FIXTURES } = require('./_umd.cjs');
const { fixedDocs, cleanDocs } = require('./word-fixes-docs.cjs');

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail !== undefined ? '  [' + String(detail).slice(0, 400) + ']' : ''}`); }

// The markup inside {{ Safe "…" }} payloads, decoded (makeSafeTemplate's escapes).
const decode = (html) => html.replace(/\{\{ Safe "((?:[^"\\]|\\.)*)" \}\}/g, (_m, p) => p.replace(/\\x([0-9a-f]{2})/gi, (_x, h) => String.fromCharCode(parseInt(h, 16))).replace(/\\(["\\])/g, '$1'));

const before = JSON.parse(fs.readFileSync(path.join(FIXTURES, 'compile-snapshot-word-fixes-before.json'), 'utf8'));
const { dom, EB } = loadUmd();
const { context, refs } = compileInputs();
const compile = (d) => EB.compileDocument(d, context, refs);
try {
  const fixed = fixedDocs(true);

  // ---- F1 ----
  {
    const out = decode(compile(fixed.divider));
    const hrs = (out.match(/<hr\b/g) || []).length;
    const wrapped = (out.match(/<div class="lm-nomso" style="mso-hide:all"><hr style="[^"]*"><\/div>/g) || []).length;
    check('F1: every Divider <hr> is wrapped in the lm-nomso twin (3 Dividers)', hrs === 3 && wrapped === 3, `${hrs} hr, ${wrapped} wrapped`);
    const line = (h, c) => `<!--[if mso]><table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse"><tr><td style="border-top:${h}px solid ${c};font-size:1px;line-height:1px;mso-line-height-rule:exactly">&nbsp;</td></tr></table><![endif]--><div class="lm-nomso" style="mso-hide:all"><hr`;
    check('F1: the Word line carries the Divider\'s own height and colour, right before its twin', out.split(line(3, '#CCCCCC')).length - 1 === 2 && out.split(line(1, '#333333')).length - 1 === 1);
    check('F1: no downlevel-revealed conditional (hazard 53)', !/<!--\[if !mso\]>/i.test(out) && !/<!--<!\[endif\]/.test(out));
    const html = decode(compile(cleanDocs().htmlHr));
    check('F1: an <hr> inside an Html block is left alone', !/lm-nomso" style="mso-hide:all"><hr/.test(html) && /<hr style="border:none;border-top:2px solid #000000">/.test(html));
  }

  // ---- F2 ----
  {
    const out = decode(compile(fixed.fullWidthButtons));
    const pt = (px) => Math.round(px * 0.75 * 100) / 100;
    // Each full-width Button: its Word box (a VML group in pt, or a fallback table in px), then its
    // non-Word twin carrying the Gmail pin class with the SLOT width.
    const pairs = [...out.matchAll(/(?:<v:group [^>]*style="width:([\d.]+)pt|data-lm-btn-fallback="[^"]*" width="(\d+)")[\s\S]*?class="[^"]*\blm-gm-pin-(\d+)\b/g)]
      .map((m) => ({ vml: m[1] !== undefined ? Number(m[1]) : null, table: m[2] !== undefined ? Number(m[2]) : null, slot: Number(m[3]) }));
    check('F2: five full-width Buttons, three as VML and two as fallbacks', pairs.length === 5 && pairs.filter((p) => p.vml !== null).length === 3, JSON.stringify(pairs));
    check('F2: each VML group is 2 px narrower than its slot', pairs.filter((p) => p.vml !== null).every((p) => p.vml === pt(p.slot - 2)), JSON.stringify(pairs));
    check('F2: each full-width fallback table is 2 px narrower than its slot', pairs.filter((p) => p.table !== null).every((p) => p.table === p.slot - 2), JSON.stringify(pairs));
    check('F2: the slots include the canvas (552) and a column', pairs.some((p) => p.slot === 552) && pairs.some((p) => p.slot < 200), JSON.stringify(pairs));
  }

  // ---- F3 ----
  {
    const raw = compile(fixed.borderedCanvas);
    const out = decode(raw);
    const canvasTag = (/<table align="center" width="100%" style="[^"]*max-width:600px[^"]*"[^>]*>/.exec(out) || [''])[0];
    check('F3: the Word-only wrapper cell draws the layout\'s border', out.includes('<!--[if mso]><table role="presentation" align="center" width="600" cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse;mso-table-lspace:0pt;mso-table-rspace:0pt;"><tr><td style="border:1px solid #333333"><![endif]-->'));
    check('F3: the canvas table keeps its own border and collapse, and carries the class lm-cvb', /border:1px solid #333333;border-collapse:collapse/.test(canvasTag) && / class="lm-cvb"/.test(canvasTag) && !/separate/.test(canvasTag), canvasTag);
    const head = out.slice(0, out.indexOf('</head>'));
    check('F3: the Word-only head block switches the canvas border off in Word', /<!--\[if mso\]><style>[^<]*table\.lm-cvb\{border:none\}<\/style><!\[endif\]-->/.test(head), head.slice(-300));
    check('F3: the head rule is emitted even with no Word column width', !/td\.lm-cw-/.test(head) && /table\.lm-cvb\{border:none\}/.test(head));
    const plain = decode(compile(cleanDocs().plainCanvas));
    check('F3: a canvas with no border: no class, no rule, a bare wrapper cell', !/lm-cvb/.test(plain) && /border="0" style="border-collapse:collapse;mso-table-lspace:0pt;mso-table-rspace:0pt;"><tr><td><!\[endif\]-->/.test(plain));
  }

  // ---- F4 ----
  {
    const out = compile(fixed.borderedZeroBox);
    check('F4: no bordered div is left', !/<div style="[^"]*border-(?:top|right|bottom|left):\s*1px/.test(out));
    const cells = [...out.matchAll(/<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border-collapse:separate;mso-table-lspace:0pt;mso-table-rspace:0pt;"><tbody><tr><td style="((?:border-radius:8px;)?padding:0px 0px 0px 0px;[^"]*)">/g)].map((m) => m[1]);
    check('F4: both zero-padding bordered Containers are a table cell, borders on the td, the table separate', cells.length === 2 && cells.every((s) => /border-top:1px solid #333333/.test(s) && /border-right:1px solid #333333/.test(s)), cells.join(' | '));
    const padded = compile(cleanDocs().paddedBorderedBox);
    check('F4: a padded bordered Container keeps its collapsed table', /style="border-collapse:collapse;mso-table-lspace:0pt;mso-table-rspace:0pt;"><tbody><tr><td style="padding:16px 24px 16px 24px;border-top:1px solid #333333/.test(padded) && !/border-collapse:separate/.test(padded));
  }

  // ---- unchanged ----
  for (const [name, d] of Object.entries(fixedDocs(false))) {
    check(`flag off: ${name} compiles byte-identical to before the fixes`, compile(d) === before.outputs[`flag-off:${name}`]);
  }
  for (const [name, d] of Object.entries(cleanDocs())) {
    check(`no fixed shape: ${name} compiles byte-identical to before the fixes`, compile(d) === before.outputs[`clean:${name}`]);
  }
  check('the pin was taken with the same context and references', JSON.stringify(before.context) === JSON.stringify(context)
    && JSON.stringify(before.refs) === JSON.stringify(refs.map((r) => ({ id: r.id, name: r.name }))));
} finally {
  dom.window.close();
}
console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
