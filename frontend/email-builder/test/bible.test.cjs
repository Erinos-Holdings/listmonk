// integrations RENDERING-BIBLE-SPEC I13 -- the rendering bible (test/build-bible.cjs):
//
//   - regenerating reproduces the committed files byte for byte (and leaves no stale bible file);
//   - at most 7 sheets, at most 40 rows a sheet, each sheet compiles to at most 80,000 bytes;
//   - every cell of {Text, Heading, Button, Image} x {-, center, right} x {top level, Container,
//     each column of 2, each column of 3, Container in a column, column row in a Container} is
//     present -- read from the documents' structure, not from the manifest;
//   - explicit `left` for each of the four types at top level, and in every context when the
//     census found explicit left, except where manifest.json records the gap;
//   - the dropped values are spread (U7): in every context class the smallest and the largest font
//     size and horizontal padding occur;
//   - each sheet's EmailLayout is the census's most common (B6: its Outlook-off one);
//   - _context.json's context is { lang: "en", brand: "ruze" } (integrations BIBLE_CONTEXT);
//   - a height-only Image at -, center and right at top level (amendment A1);
//   - no row overflows its column at 600 or 360 px by the D4.4 estimate, images come from
//     test/bible/assets.json, and both OfficialFooter rows resolve ok under the canary context.
//
// integrations BIBLE-OUTLOOK-FIXES-SPEC §12 (Amendment A):
//   - IA6 / SA4 / SA5: exactly four bare Containers, all in B7's three evidence pairs, each wrapped
//     row directly after its twin and naming it, one with a null style; every other Container has
//     padding 8, 16 or 24 on all four sides by its row index, no background, no radius;
//   - IA14 / SA11-SA13: seven sheets; B7 holds the three bordered rows, the two fallback Buttons (the
//     only Buttons in the bible whose Word copy is the stamped table-cell fallback, causes `lines`
//     and `chars`), the SA5 evidence rows and every census row, most common first; B3 keeps its
//     whole grid (the `left` drop applies to B4 only); the manifest records every move and drop;
//   - IA21 / SA8: the generator's composition function agrees with the review's version 2 grammar on
//     the shared case table (integrations tests/lib/campaign-review/fingerprint-v2.test.ts,
//     TRANSPARENT_CASES and BOX_CASES; by copy).
const fs = require('fs');
const path = require('path');
const { loadUmd } = require('./_umd.cjs');
const { CANARY_DIR } = require('./build-canary.cjs');
const { BIBLE_DIR, MANIFEST, LIMITS, SIZES, PADS, CONTAINER_PADS, CONTEXTS, FALLBACK_LINES_LABEL, FALLBACK_CHARS_LABEL, bibleFiles, compiledBytes, compositionsOf, containerParts, isBareContainer } = require('./build-bible.cjs');

let failed = 0;
function check(name, ok, detail) {
  if (!ok) failed++;
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail ? `  [${String(detail).slice(0, 400)}]` : ''}`);
}

const CENSUS = JSON.parse(fs.readFileSync(path.join(BIBLE_DIR, 'census.json'), 'utf8'));
const ASSETS = JSON.parse(fs.readFileSync(path.join(BIBLE_DIR, 'assets.json'), 'utf8'));
const CELL_TYPES = ['Text', 'Heading', 'Button', 'Image'];
const HEADING_SIZE = { h1: 32, h2: 24, h3: 20 };

const read = (f) => JSON.parse(fs.readFileSync(f, 'utf8'));
const stems = fs.readdirSync(CANARY_DIR).filter((f) => /^bible-.*\.json$/.test(f)).map((f) => f.slice(0, -5)).sort();
const docs = Object.fromEntries(stems.map((s) => [s, read(path.join(CANARY_DIR, `${s}.json`))]));
const manifest = read(MANIFEST);

// Parent links: id -> { parent, column } (column index for a ColumnsContainer parent).
function parents(doc) {
  const out = {};
  for (const [id, b] of Object.entries(doc)) {
    const p = (b && b.data && (b.type === 'EmailLayout' ? b.data : b.data.props)) || {};
    for (const k of p.childrenIds || []) out[k] = { parent: id };
    (p.columns || []).forEach((c, i) => (c.childrenIds || []).forEach((k) => (out[k] = { parent: id, column: i })));
  }
  return out;
}
const typeOf = (doc, id) => (doc[id] || {}).type;
const colsOf = (doc, id) => (((doc[id] || {}).data || {}).props || {}).columnsCount || 2;

// The cell context of a block, from its ancestors (null: not a cell context).
function contextOf(doc, par, id) {
  const chain = [];
  for (let x = par[id]; x && x.parent !== 'root'; x = par[x.parent]) chain.push(x.parent);
  const t = chain.map((c) => (typeOf(doc, c) === 'ColumnsContainer' ? `Columns${colsOf(doc, c)}` : typeOf(doc, c))).join('/');
  if (!par[id]) return null;
  if (t === '') return 'top';
  if (t === 'Container') return 'container';
  if (t === 'Columns2') return 'col2';
  if (t === 'Columns3') return 'col3';
  if (t === 'Container/Columns2') return 'containerInColumn';
  if (t === 'Columns2/Container') return 'columnRowInContainer';
  return null;
}
const alignOf = (b) => ((b.data && b.data.style && b.data.style.textAlign) || '-');
const hpadOf = (b) => {
  const p = (b.data && b.data.style && b.data.style.padding) || {};
  return (p.left || 0) + (p.right || 0);
};

// A cell is present for a column context only when EVERY used column of that row holds it.
function cellsOf(doc) {
  const par = parents(doc);
  const out = new Map(); // key -> [block ids]
  for (const [id, b] of Object.entries(doc)) {
    if (!b || !CELL_TYPES.includes(b.type) || id.endsWith('-label')) continue;
    const ctx = contextOf(doc, par, id);
    if (!ctx) continue;
    const key = `${b.type}@${alignOf(b)}@${ctx}`;
    if (!out.has(key)) out.set(key, []);
    out.get(key).push(id);
  }
  // Column contexts: every used column of the innermost ColumnsContainer must hold the cell.
  for (const [key, ids] of [...out]) {
    if (!/@(col2|col3|containerInColumn|columnRowInContainer)$/.test(key)) continue;
    const rows = new Map();
    for (const id of ids) {
      let x = par[id];
      while (x && typeOf(doc, x.parent) !== 'ColumnsContainer') x = par[x.parent];
      if (!x) continue;
      if (!rows.has(x.parent)) rows.set(x.parent, new Set());
      rows.get(x.parent).add(x.column);
    }
    const full = [...rows].some(([cid, cols]) => cols.size === colsOf(doc, cid));
    if (!full) out.delete(key);
  }
  return out;
}

// ---------------------------------------------------------------------------------------------
// The D4.4 estimate (mirrors integrations lib/campaign-review/value-rules.ts availableWidths/neededWidth)
// ---------------------------------------------------------------------------------------------

const PRESET = { 'x-small': 8, small: 12, medium: 20, large: 32 };
const run = (s) => String(s || '').replace(/\{\{[\s\S]*?\}\}/g, '').replace(/<[^>]*>/g, '').split(/[ \t\r\n]+|-|\//).reduce((a, r) => ([...r].length > [...a].length ? r : a), '');
function widths(doc, v) {
  const out = {};
  const visit = (id, avail) => {
    const b = doc[id];
    if (!b) return;
    out[id] = avail;
    const p = (b.data && b.data.props) || {};
    if (b.type === 'Container') (p.childrenIds || []).forEach((k) => visit(k, avail - hpadOf(b)));
    if (b.type === 'ColumnsContainer') {
      const inner = avail - hpadOf(b);
      const n = p.columnsCount === 3 ? 3 : 2;
      const gap = typeof p.columnsGap === 'number' ? p.columnsGap : 0;
      const fixed = [0, 1, 2].slice(0, n).map((i) => (Array.isArray(p.fixedWidths) && typeof p.fixedWidths[i] === 'number' ? p.fixedWidths[i] : null));
      const free = fixed.filter((w) => w === null).length;
      const share = free ? (inner - gap * (n - 1) - fixed.reduce((a, w) => a + (w || 0), 0)) / free : 0;
      (p.columns || []).slice(0, n).forEach((c, i) => (c.childrenIds || []).forEach((k) => visit(k, fixed[i] === null ? share : fixed[i])));
    }
  };
  doc.root.data.childrenIds.forEach((k) => visit(k, Math.min(600, v)));
  return out;
}
function needed(b) {
  const s = (b.data && b.data.style) || {};
  const p = (b.data && b.data.props) || {};
  if (b.type === 'Heading') return [...run(p.text)].length * HEADING_SIZE[p.level || 'h2'] * (s.fontWeight === 'normal' ? 0.6 : 0.65) + hpadOf(b);
  if (b.type === 'Button') {
    // Re-review R1: the custom-width label is one nowrap line — the whole label plus twice the border must fit inside it.
    if (!p.fullWidth && p.customWidth > 0) {
      const label = [...String(p.text || '').replace(/\{\{[\s\S]*?\}\}/g, '').replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim()].length * (s.fontSize || 16) * (s.fontWeight === 'normal' ? 0.6 : 0.65) + 2 * (p.borderSize || 0);
      return label > p.customWidth ? Infinity : p.customWidth + hpadOf(b);
    }
    return [...run(p.text)].length * (s.fontSize || 16) * (s.fontWeight === 'normal' ? 0.6 : 0.65) + hpadOf(b) + 2 * (PRESET[p.size || 'medium'] || 20) + 2 * (p.borderSize || 0);
  }
  if (b.type === 'Text') return [...run(p.text)].length * (s.fontSize || 16) * (s.fontWeight === 'bold' ? 0.65 : 0.6) + hpadOf(b);
  return 0;
}

// ---------------------------------------------------------------------------------------------

const { dom, EB } = loadUmd();
try {
  // Determinism: a fresh generation equals the committed files, and nothing else is committed.
  const files = bibleFiles(EB);
  const differs = Object.entries(files).filter(([f, c]) => !fs.existsSync(f) || fs.readFileSync(f, 'utf8') !== c).map(([f]) => path.basename(f));
  check('regenerating reproduces the committed files byte for byte', differs.length === 0, differs.join(', '));
  const generated = Object.keys(files).filter((f) => f !== MANIFEST).map((f) => path.basename(f, '.json')).sort();
  check('no stale bible-*.json beside the generated ones', JSON.stringify(generated) === JSON.stringify(stems), `${generated} vs ${stems}`);

  // Limits.
  check(`at most ${LIMITS.sheets} sheets`, stems.length > 0 && stems.length <= LIMITS.sheets, stems.length);
  for (const s of stems) {
    const rows = docs[s].root.data.childrenIds.filter((k) => !k.endsWith('-label')).length;
    const labels = docs[s].root.data.childrenIds.filter((k) => k.endsWith('-label')).length;
    const bytes = compiledBytes(EB, docs[s]);
    check(`${s}: ${rows} rows (<= ${LIMITS.rows}), each preceded by its label`, rows <= LIMITS.rows && labels === rows && docs[s].root.data.childrenIds.every((k, i) => (i % 2 === 0) === k.endsWith('-label')));
    check(`${s}: compiles to ${bytes} bytes (<= ${LIMITS.bytes})`, bytes <= LIMITS.bytes);
    // BIBLE-OUTLOOK-FIXES-SPEC S5 / §12 SA13 (I10 as amended): no Word copy is the old
    // <w:anchorlock/> + <center> shape, and the only stamped table-cell fallbacks are B7's two.
    const { context, refs } = JSON.parse(fs.readFileSync(path.join(CANARY_DIR, '_context.json'), 'utf8'));
    const html = EB.compileDocument(JSON.parse(JSON.stringify(docs[s])), context, refs);
    check(`${s}: no Button compiles to the old fallback Word shape`, !/anchorlock/.test(html));
    // The stamp rides inside a Safe payload: `data-lm-btn-fallback=\"width\x20chars\"`.
    const stamps = [...html.matchAll(/data-lm-btn-fallback=\\"(.*?)\\"/g)].map((m) => m[1].replace(/\\x20/g, ' '));
    if (s === 'bible-07-real-arrangements') check(`${s}: exactly the two fallback Buttons, causes lines and chars (SA13)`, JSON.stringify(stamps) === JSON.stringify(['lines', 'chars']), JSON.stringify(stamps));
    else check(`${s}: no Button falls back (SA13)`, stamps.length === 0, JSON.stringify(stamps));
  }
  check(`exactly ${LIMITS.sheets} sheets (SA11)`, stems.length === LIMITS.sheets && LIMITS.sheets === 7 && stems.includes('bible-07-real-arrangements'), stems.join(', '));

  // Cells.
  const present = new Set();
  for (const s of stems) for (const k of cellsOf(docs[s]).keys()) present.add(k);
  const gapCtx = (manifest.gaps || []).join(' ');
  const missing = [];
  for (const type of CELL_TYPES) for (const align of ['-', 'center', 'right']) for (const ctx of CONTEXTS) if (!present.has(`${type}@${align}@${ctx}`)) missing.push(`${type}@${align}@${ctx}`);
  check('every cell {Text, Heading, Button, Image} x {-, center, right} x six contexts is present', missing.length === 0, missing.join(', '));
  const leftTop = CELL_TYPES.filter((t) => !present.has(`${t}@left@top`));
  check('explicit left for each of the four types at top level', leftTop.length === 0, leftTop.join(', '));
  if (CENSUS.explicitLeft.blocks > 0) {
    const leftMissing = [];
    for (const type of CELL_TYPES) for (const ctx of CONTEXTS) if (!present.has(`${type}@left@${ctx}`) && !gapCtx.includes(ctx)) leftMissing.push(`${type}@left@${ctx}`);
    check('the census found explicit left: it is a cell in every context, or the gap is recorded in manifest.json', leftMissing.length === 0, leftMissing.join(', '));
  }

  // Amendment A1: a height-only Image (a px height, no width) at -, center and right at top level.
  const heightOnly = new Set();
  for (const s of stems) {
    const par = parents(docs[s]);
    for (const [id, b] of Object.entries(docs[s])) {
      const p = (b && b.type === 'Image' && b.data.props) || null;
      if (p && typeof p.height === 'number' && typeof p.width !== 'number' && contextOf(docs[s], par, id) === 'top') heightOnly.add(alignOf(b));
    }
  }
  check('a height-only Image at -, center and right at top level (amendment A1)', ['-', 'center', 'right'].every((a) => heightOnly.has(a)), [...heightOnly].join(','));

  // Review L6: a plain Button (auto width, auto height, no border) at -, left and right at top level.
  const plain = new Set();
  for (const s of stems) {
    const par = parents(docs[s]);
    for (const [id, b] of Object.entries(docs[s])) {
      const p = (b && b.type === 'Button' && b.data.props) || null;
      if (p && !(p.customWidth > 0) && !(p.customHeight > 0) && !(p.borderSize > 0) && !p.fullWidth && contextOf(docs[s], par, id) === 'top') plain.add(alignOf(b));
    }
  }
  check('a plain Button at -, left and right at top level (review L6)', ['-', 'left', 'right'].every((a) => plain.has(a)), [...plain].join(','));

  // Re-review R2/R3: the Button width x height x border combinations, and the plain shapes.
  const buttons = stems.flatMap((s) => {
    const par = parents(docs[s]);
    return Object.entries(docs[s]).filter(([id, b]) => b && b.type === 'Button' && contextOf(docs[s], par, id) === 'top').map(([, b]) => b);
  });
  const has = (pred) => buttons.some((b) => pred(b.data.props || {}, b));
  check('R2: custom width, auto height, no border', has((p) => p.customWidth > 0 && !(p.customHeight > 0) && !(p.borderSize > 0)));
  check('R2: auto width, custom height', has((p) => !(p.customWidth > 0) && p.customHeight > 0));
  check('R2: pill, custom width and height, bordered', has((p) => p.buttonStyle === 'pill' && p.customWidth > 0 && p.customHeight > 0 && p.borderSize > 0));
  for (const shape of ['pill', 'rectangle']) {
    const aligns = new Set(buttons.filter((b) => (b.data.props || {}).buttonStyle === shape && !(b.data.props.customWidth > 0) && !(b.data.props.customHeight > 0) && !(b.data.props.borderSize > 0) && !b.data.props.fullWidth).map(alignOf));
    check(`R3: a plain ${shape} Button at -, left and right at top level`, ['-', 'left', 'right'].every((a) => aligns.has(a)), [...aligns].join(','));
  }

  // Spread (U7): per context class, the smallest and the largest of each dropped value.
  for (const ctx of CONTEXTS) {
    const sizes = new Set();
    const levels = new Set();
    const pads = new Set();
    for (const s of stems) {
      for (const [key, ids] of cellsOf(docs[s])) {
        if (!key.endsWith(`@${ctx}`)) continue;
        for (const id of ids) {
          const b = docs[s][id];
          if (b.type === 'Heading') levels.add(b.data.props.level);
          else if (b.type !== 'Image') sizes.add(b.data.style.fontSize);
          pads.add(hpadOf(b));
        }
      }
    }
    const lo = SIZES[0];
    const hi = SIZES[SIZES.length - 1];
    check(`${ctx}: font sizes ${lo} and ${hi}, heading levels h3 and h1, paddings ${PADS[0]} and ${PADS[PADS.length - 1]} all occur`,
      sizes.has(lo) && sizes.has(hi) && levels.has('h3') && levels.has('h1') && pads.has(PADS[0]) && pads.has(PADS[PADS.length - 1]),
      `sizes ${[...sizes]} levels ${[...levels]} pads ${[...pads]}`);
  }

  // EmailLayout per sheet.
  const luma = (h) => {
    const m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(h);
    return (0.2126 * parseInt(m[1], 16) + 0.7152 * parseInt(m[2], 16) + 0.0722 * parseInt(m[3], 16)) / 255;
  };
  const band = (h) => (luma(h) <= 0.3 ? 'dark' : luma(h) >= 0.6 ? 'light' : 'mid');
  const describe = (d) => `EmailLayout|outlook=${!!d.outlook}|radius=${d.borderRadius > 0 ? 'rounded' : 'square'}|backdrop=${band(d.backdropColor || '#F5F5F5')}|canvas=${band(d.canvasColor || '#FFFFFF')}`;
  const offLayout = CENSUS.layouts.find((l) => l.value.includes('outlook=false')).value;
  for (const s of stems) {
    const want = s === 'bible-06-layout-variants' ? offLayout : CENSUS.layouts[0].value;
    check(`${s}: EmailLayout ${want}`, describe(docs[s].root.data) === want, describe(docs[s].root.data));
  }

  // The context integrations' BIBLE_CONTEXT must equal.
  const ctx = read(path.join(CANARY_DIR, '_context.json')).context;
  check('_context.json context is { lang: "en", brand: "ruze" }', JSON.stringify(ctx) === JSON.stringify({ lang: 'en', brand: 'ruze' }), JSON.stringify(ctx));

  // Nothing overflows by the D4.4 estimate, at 600 and 360 px.
  const over = [];
  for (const s of stems) {
    for (const v of [600, 360]) {
      const w = widths(docs[s], v);
      for (const [id, b] of Object.entries(docs[s])) if (b && w[id] !== undefined && needed(b) > w[id]) over.push(`${s}:${id}@${v}`);
    }
  }
  check('no row overflows its column at 600 or 360 px by the D4.4 estimate', over.length === 0, over.join(', '));

  // Images: only listed assets.
  const urls = new Set(Object.values(ASSETS).filter((a) => a && a.url).map((a) => a.url));
  const stray = [];
  for (const s of stems) for (const b of Object.values(docs[s])) {
    const u = b && b.data && b.data.props && (b.type === 'Image' ? b.data.props.url : b.type === 'Avatar' ? b.data.props.imageUrl : null);
    if (u && !urls.has(u)) stray.push(`${s}:${u}`);
  }
  check('every image and avatar is an asset listed in test/bible/assets.json', stray.length === 0, stray.join(', '));

  // OfficialFooter brand and corporate resolve ok under the canary context.
  const { context, refs } = read(path.join(CANARY_DIR, '_context.json'));
  const footers = stems.filter((s) => Object.values(docs[s]).some((b) => b && b.type === 'OfficialFooter'));
  for (const s of footers) {
    const html = EB.compileDocument(JSON.parse(JSON.stringify(docs[s])), context, refs);
    const kinds = Object.values(docs[s]).filter((b) => b && b.type === 'OfficialFooter').map((b) => b.data.props.kind).sort();
    const ok = (html.match(/<!--\s*official:(corporate|brand):[a-z]*:[^:\s]*:[0-9a-f]{16}\s*-->/g) || []).map((m) => /official:(\w+)/.exec(m)[1]).sort();
    check(`${s}: every OfficialFooter block resolves ok (${kinds.join(', ')})`, JSON.stringify(ok) === JSON.stringify(kinds), `${ok} vs ${kinds}`);
  }
  check('OfficialFooter brand and corporate both occur', footers.some((s) => Object.values(docs[s]).some((b) => b && b.type === 'OfficialFooter' && b.data.props.kind === 'brand')) && footers.some((s) => Object.values(docs[s]).some((b) => b && b.type === 'OfficialFooter' && b.data.props.kind === 'corporate')));

  // ---- integrations BIBLE-OUTLOOK-FIXES-SPEC §12 (Amendment A) ----

  // IA6 / SA4 / SA5: the bare Containers and the styled ones.
  const bare = [];
  const badStyled = [];
  for (const s of stems) {
    for (const [id, b] of Object.entries(docs[s])) {
      if (!b || b.type !== 'Container') continue;
      if (isBareContainer(b)) {
        bare.push({ s, id, nullStyle: b.data.style === null });
        continue;
      }
      // The row a Container belongs to: its row id is the prefix `b<n>-r<nn>`; its index is nn - 1.
      const m = /^b\d+-r(\d\d)/.exec(id);
      const want = m ? CONTAINER_PADS[(Number(m[1]) - 1) % CONTAINER_PADS.length] : null;
      const st = b.data.style || {};
      const p = st.padding || {};
      const ok = want !== null && ['top', 'right', 'bottom', 'left'].every((k) => p[k] === want) && !st.backgroundColor && !st.borderRadius;
      if (!ok) badStyled.push(`${s}:${id} ${JSON.stringify(st)}`);
    }
  }
  check('IA6: exactly four bare Containers in the bible, all on B7', bare.length === 4 && bare.every((x) => x.s === 'bible-07-real-arrangements'), JSON.stringify(bare));
  check('IA6: one bare Container has a null style, the others zero padding (SA5)', bare.filter((x) => x.nullStyle).length === 1, JSON.stringify(bare));
  check('SA4: every other Container has padding 8, 16 or 24 on all four sides by its row index, no background, no radius', badStyled.length === 0, badStyled.join(' | '));

  // SA5: three twin pairs on B7 at (c), after the bordered rows and the fallback Buttons; each wrapped
  // row directly follows its twin, names it, holds the twin's content inside bare Containers and
  // reads as the twin does.
  {
    const d3 = docs['bible-07-real-arrangements'];
    const rows = d3.root.data.childrenIds.filter((k) => !k.endsWith('-label'));
    const labelOf = (k) => d3[d3.root.data.childrenIds[d3.root.data.childrenIds.indexOf(k) - 1]].data.props.text;
    const evidence = rows.map((k, i) => ({ k, i, label: labelOf(k) })).filter((r) => / · evidence/.test(r.label));
    check('SA5: six evidence rows on B7 (three twin pairs)', evidence.length === 6, evidence.map((r) => r.label).join(' | '));
    const firstEvidence = evidence.length ? evidence[0].i : -1;
    check('SA5: the evidence rows are B7 rows 6-11, after the three bordered rows and the two fallback Buttons', firstEvidence === 5
      && rows.slice(firstEvidence, firstEvidence + 6).every((k) => / · evidence/.test(labelOf(k))), String(firstEvidence));
    // Strip bare Containers from a subtree: the block types, alignments and props it renders.
    const shape = (doc, id) => {
      const b = doc[id];
      if (isBareContainer(b)) return (b.data.props.childrenIds || []).map((x) => shape(doc, x)).join('+');
      const p = b.data.props || {};
      const kids = b.type === 'ColumnsContainer' ? p.columns.map((c) => (c.childrenIds || []).map((x) => shape(doc, x)).join('+')).join(';') : (p.childrenIds || []).map((x) => shape(doc, x)).join('+');
      const { childrenIds: _c, columns: _k, ...own } = p;
      return `${b.type}${JSON.stringify(b.data.style || null)}${JSON.stringify(own)}[${kids}]`;
    };
    for (let k = 0; k < 3; k++) {
      const twin = evidence[2 * k];
      const wrapped = evidence[2 * k + 1];
      if (!twin || !wrapped) continue;
      const twinNo = /^B7\.(\d+) /.exec(twin.label)[1];
      check(`SA5 pair ${k + 1}: the wrapped row directly follows its twin and names it`, wrapped.i === twin.i + 1 && / · evidence twin/.test(twin.label) && wrapped.label.includes(`redundant wrapper, must render as B7.${twinNo})`), `${twin.label} / ${wrapped.label}`);
      check(`SA5 pair ${k + 1}: the same content once its bare Containers are looked through`, shape(d3, twin.k) === shape(d3, wrapped.k), `${shape(d3, twin.k)}\n  vs ${shape(d3, wrapped.k)}`);
      const wrappers = Object.keys(d3).filter((id) => isBareContainer(d3[id]) && (id === wrapped.k || id.startsWith(`${wrapped.k.replace(/-w$/, '')}-`)));
      check(`SA5 pair ${k + 1}: the wrapped row holds ${k === 2 ? 'two group' : 'one single-block'} wrapper(s)`, wrappers.length === (k === 2 ? 2 : 1)
        && wrappers.every((id) => d3[id].data.props.childrenIds.length === (k === 2 ? 2 : 1)), wrappers.join(','));
      check(`SA5 pair ${k + 1}: twin and wrapped row read as the same compositions`, JSON.stringify(compositionsOf(Object.fromEntries(Object.entries(d3).filter(([id]) => id === twin.k || id.startsWith(`${twin.k.replace(/-x$/, '')}-`))))) === JSON.stringify(compositionsOf(Object.fromEntries(Object.entries(d3).filter(([id]) => id === wrapped.k || id.startsWith(`${wrapped.k.replace(/-w$/, '')}-`))))));
    }
    const p1 = evidence[0] && d3[evidence[0].k];
    const p2 = evidence[2] && d3[evidence[2].k];
    check('SA5: pair 1 is a right-aligned Image sized by width; pair 2 a centred plain Button',
      !!p1 && p1.type === 'Image' && p1.data.style.textAlign === 'right' && typeof p1.data.props.width === 'number'
      && !!p2 && p2.type === 'Button' && p2.data.style.textAlign === 'center' && !(p2.data.props.customWidth > 0) && !(p2.data.props.borderSize > 0) && !p2.data.props.fullWidth);
    const p3 = evidence[4] && d3[evidence[4].k];
    check('SA5: pair 3 is a two-column row whose columns each hold a Text and a Button',
      !!p3 && p3.type === 'ColumnsContainer' && p3.data.props.columns.slice(0, 2).every((c) => JSON.stringify((c.childrenIds || []).map((x) => d3[x].type)) === '["Text","Button"]'));
    const b3doc = docs['bible-03-three-columns'];
    const b3labels = b3doc.root.data.childrenIds.filter((k) => k.endsWith('-label')).map((k) => b3doc[k].data.props.text);
    check('B3 holds no census rows and no evidence rows (SA11, SA5)', b3labels.every((l) => !/census composition| · evidence/.test(l)), b3labels.join(' | '));
  }

  // IA14 / SA11 / SA13: B7, the real arrangements.
  {
    const d7 = docs['bible-07-real-arrangements'];
    const ids = d7.root.data.childrenIds;
    const labels = ids.filter((k) => k.endsWith('-label')).map((k) => d7[k].data.props.text);
    const rows = ids.filter((k) => !k.endsWith('-label'));
    check('B7 uses the census\'s most common EmailLayout with Outlook on', d7.root.data.outlook === true);
    const bordered = rows.filter((k) => d7[k].type === 'Container' && d7[k].data.style && d7[k].data.style.borderColor);
    check('SA11 (a): three bordered-Container rows, first on B7', bordered.length === 3 && JSON.stringify(rows.slice(0, 3)) === JSON.stringify(bordered), bordered.join(','));
    const kidTypes = (k) => d7[k].data.props.childrenIds.map((x) => d7[x].type);
    check('SA11 (a): they hold a two-column row, an Image sized by width at 600, and a Text then a Button',
      bordered.length === 3 && JSON.stringify(kidTypes(bordered[0])) === '["ColumnsContainer"]'
      && JSON.stringify(kidTypes(bordered[1])) === '["Image"]' && d7[d7[bordered[1]].data.props.childrenIds[0]].data.props.width === 600
      && JSON.stringify(kidTypes(bordered[2])) === '["Text","Button"]');
    check('SA11 (a): bordered rows carry no radius and pad by their row index',
      bordered.every((k, i) => !d7[k].data.style.borderRadius && d7[k].data.style.padding.top === CONTAINER_PADS[i % CONTAINER_PADS.length]));
    const buttons = rows.slice(3, 5).map((k) => d7[k]);
    check('SA13 (b): the next two rows are the fallback Buttons, labelled as such',
      buttons.length === 2 && buttons[0].type === 'Button' && buttons[0].data.props.fullWidth === true && buttons[0].data.props.text === FALLBACK_LINES_LABEL
      && buttons[1].type === 'Button' && !buttons[1].data.props.fullWidth && buttons[1].data.props.text === FALLBACK_CHARS_LABEL && FALLBACK_CHARS_LABEL.endsWith('→')
      && /fallback Button/.test(labels[3]) && /fallback Button/.test(labels[4]));
    // (d) census rows: most common first, after (a)-(c), none equal to a composition B1-B4 hold.
    const censusLabels = labels.map((l, i) => ({ l, i })).filter((x) => /census composition, (\d+) items/.test(x.l));
    const items = censusLabels.map((x) => Number(/census composition, (\d+) items/.exec(x.l)[1]));
    check('SA11 (d): census rows, most common first, at the end of B7', censusLabels.length > 0 && items.every((n, i) => i === 0 || items[i - 1] >= n)
      && censusLabels.every((x, j) => x.i === labels.length - censusLabels.length + j), JSON.stringify(items));
    const earlier = new Set(['bible-01-flat-alignment', 'bible-02-two-columns', 'bible-03-three-columns', 'bible-04-nesting'].flatMap((s) => compositionsOf(docs[s])));
    const censusValues = new Set(CENSUS.compositions.map((c) => c.value));
    const rowComps = rows.slice(ids.length / 2 - censusLabels.length).map((k) => {
      const prefix = k.replace(/-0$/, '');
      return compositionsOf(Object.fromEntries(Object.entries(d7).filter(([id]) => id.startsWith(`${prefix}-`))));
    });
    check('SA11 (d): each census row is a census composition not on B1-B4', rowComps.length === censusLabels.length && rowComps.every((cs) => cs.some((c) => censusValues.has(c) && !earlier.has(c))));
    // Every census composition is on some sheet B1-B4 or B7, or recorded as dropped on B7 (SA12 step 3).
    const all = new Set([...earlier, ...compositionsOf(d7)]);
    const b7Dropped = (manifest.sheets.find((x) => x.stem === 'bible-07-real-arrangements') || { dropped: [] }).dropped;
    const missingComps = CENSUS.compositions.filter((c) => !all.has(c.value) && !b7Dropped.some((w) => w.startsWith(`census composition, ${c.items} items:`)));
    check('IA14: every census composition is in the bible or recorded as dropped', missingComps.length === 0, missingComps.map((c) => c.value).join(' | '));
  }

  // IA14 / SA12: every move and drop is recorded. The full grid of cell rows a sheet was built from,
  // less what the manifest says was dropped or moved away, plus what it says was moved in, is
  // exactly what each sheet holds.
  {
    const moved = manifest.moved || [];
    for (const m of moved) {
      check(`IA14: the move of "${m.what}" is recorded from B4 and found on ${m.to}`, m.from === 'bible-04-nesting' && ['bible-03-three-columns', 'bible-07-real-arrangements'].includes(m.to)
        && Object.values(docs[m.to]).some((b) => b && b.type === 'Text' && typeof b.data.props.text === 'string' && b.data.props.text.endsWith(`${m.what} (moved from B4)`)));
      check(`IA14: a moved row is a Heading row (SA12 step 2)`, /^Heading, /.test(m.what));
    }
    const b4 = manifest.sheets.find((x) => x.stem === 'bible-04-nesting');
    const fullB4 = 4 * 4 * 2; // four types x four alignments x two nesting contexts
    check('IA14: B4\'s rows plus its recorded drops and moves are its whole grid', b4.rows + b4.dropped.length + moved.length === fullB4, `${b4.rows} + ${b4.dropped.length} + ${moved.length}`);
    const b3 = manifest.sheets.find((x) => x.stem === 'bible-03-three-columns');
    const b3Cells = b3.cells.filter((c) => c.endsWith('@col3')).length;
    check('IA14 / SA12: B3 holds its whole three-column grid, explicit left included, and drops nothing', b3Cells === 16 && b3.dropped.length === 0 && !(manifest.gaps || []).some((g) => /B3 /.test(g)), `${b3Cells} + ${b3.dropped.length}`);
    check('IA14: every dropped `left` row is covered by a recorded gap (B4 only)', [b4].every((x) => x.dropped.every((w) => / aligned left, /.test(w)) && (x.dropped.length === 0 || (manifest.gaps || []).some((g) => g.includes(`B${x.stem.slice(6, 8).replace(/^0/, '')} `)))));
  }

  // IA21 / SA8 / SA9: the generator's composition function and Container parts agree with the
  // review's version 2 grammar on the shared case tables (copied from integrations
  // tests/lib/campaign-review/fingerprint-v2.test.ts TRANSPARENT_CASES and BOX_CASES).
  const TRANSPARENT_CASES = [
    ["single", {"root":{"type":"EmailLayout","data":{"backdropColor":"#F5F5F5","canvasColor":"#FFFFFF","textColor":"#262626","fontFamily":"MODERN_SANS","outlook":true,"childrenIds":["w"]}},"w":{"type":"Container","data":{"style":null,"props":{"childrenIds":["t"]}}},"t":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"text":"x"}}}}, []],
    ["group", {"root":{"type":"EmailLayout","data":{"backdropColor":"#F5F5F5","canvasColor":"#FFFFFF","textColor":"#262626","fontFamily":"MODERN_SANS","outlook":true,"childrenIds":["w"]}},"w":{"type":"Container","data":{"style":{"padding":{"top":0,"right":0,"bottom":0,"left":0}},"props":{"childrenIds":["t","b"]}}},"t":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"text":"x"}}},"b":{"type":"Button","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24},"textAlign":"center"},"props":{"text":"Go","url":"https://x","buttonStyle":"pill","size":"medium","buttonBackgroundColor":"#003b4d","buttonTextColor":"#ffffff"}}}}, []],
    ["empty", {"root":{"type":"EmailLayout","data":{"backdropColor":"#F5F5F5","canvasColor":"#FFFFFF","textColor":"#262626","fontFamily":"MODERN_SANS","outlook":true,"childrenIds":["t","w"]}},"t":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"text":"x"}}},"w":{"type":"Container","data":{"style":null,"props":{"childrenIds":[]}}}}, []],
    ["bare in bare", {"root":{"type":"EmailLayout","data":{"backdropColor":"#F5F5F5","canvasColor":"#FFFFFF","textColor":"#262626","fontFamily":"MODERN_SANS","outlook":true,"childrenIds":["o"]}},"o":{"type":"Container","data":{"style":null,"props":{"childrenIds":["w"]}}},"w":{"type":"Container","data":{"style":{},"props":{"childrenIds":["t","i"]}}},"t":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"text":"x"}}},"i":{"type":"Image","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"url":"https://x/a.png","width":300}}}}, []],
    ["bare in a column", {"root":{"type":"EmailLayout","data":{"backdropColor":"#F5F5F5","canvasColor":"#FFFFFF","textColor":"#262626","fontFamily":"MODERN_SANS","outlook":true,"childrenIds":["c"]}},"c":{"type":"ColumnsContainer","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"columnsCount":2,"columnsGap":16,"columns":[{"childrenIds":["w"]},{"childrenIds":["y"]},{"childrenIds":[]}]}}},"w":{"type":"Container","data":{"style":null,"props":{"childrenIds":["t","b"]}}},"t":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"text":"x"}}},"b":{"type":"Button","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24},"textAlign":"center"},"props":{"text":"Go","url":"https://x","buttonStyle":"pill","size":"medium","buttonBackgroundColor":"#003b4d","buttonTextColor":"#ffffff"}}},"y":{"type":"Image","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"url":"https://x/a.png","width":300}}}}, ["Columns[Text@-+Button@center;Image:width@-;-]"]],
    ["an empty one alone in a column", {"root":{"type":"EmailLayout","data":{"backdropColor":"#F5F5F5","canvasColor":"#FFFFFF","textColor":"#262626","fontFamily":"MODERN_SANS","outlook":true,"childrenIds":["c"]}},"c":{"type":"ColumnsContainer","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"columnsCount":2,"columnsGap":16,"columns":[{"childrenIds":["w"]},{"childrenIds":["y"]},{"childrenIds":[]}]}}},"w":{"type":"Container","data":{"style":null,"props":{"childrenIds":[]}}},"y":{"type":"Image","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"url":"https://x/a.png","width":300}}}}, ["Columns[-;Image:width@-;-]"]],
    ["bare inside a styled Container", {"root":{"type":"EmailLayout","data":{"backdropColor":"#F5F5F5","canvasColor":"#FFFFFF","textColor":"#262626","fontFamily":"MODERN_SANS","outlook":true,"childrenIds":["s"]}},"s":{"type":"Container","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"childrenIds":["t","w"]}}},"w":{"type":"Container","data":{"style":{"padding":{"top":0,"right":0,"bottom":0,"left":0}},"props":{"childrenIds":["i","u"]}}},"t":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"text":"x"}}},"i":{"type":"Image","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"url":"https://x/a.png","width":300}}},"u":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"text":"x"}}}}, ["Container[Text@-+Image:width@-+Text@-]"]],
    ["a styled Container inside a bare one", {"root":{"type":"EmailLayout","data":{"backdropColor":"#F5F5F5","canvasColor":"#FFFFFF","textColor":"#262626","fontFamily":"MODERN_SANS","outlook":true,"childrenIds":["w"]}},"w":{"type":"Container","data":{"style":null,"props":{"childrenIds":["s","t"]}}},"s":{"type":"Container","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"childrenIds":["i"]}}},"i":{"type":"Image","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"url":"https://x/a.png","width":300}}},"t":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"text":"x"}}}}, ["Container[Image:width@-]"]],
    ["a bare Container that contains itself", {"root":{"type":"EmailLayout","data":{"backdropColor":"#F5F5F5","canvasColor":"#FFFFFF","textColor":"#262626","fontFamily":"MODERN_SANS","outlook":true,"childrenIds":["t","w"]}},"t":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24}},"props":{"text":"x"}}},"w":{"type":"Container","data":{"style":null,"props":{"childrenIds":["w","u"]}}},"u":{"type":"Text","data":{"style":{"padding":{"top":16,"bottom":16,"right":24,"left":24},"textAlign":"center"},"props":{"text":"x"}}}}, []],
  ];
  for (const [name, doc, want] of TRANSPARENT_CASES) {
    const got = compositionsOf(doc).sort();
    check(`IA21: ${name} reads as ${JSON.stringify(want)}`, JSON.stringify(got) === JSON.stringify([...want].sort()), JSON.stringify(got));
  }
  const ZERO = { padding: { top: 0, right: 0, bottom: 0, left: 0 } };
  const BOX_CASES = [
    ['background with no padding key', { backgroundColor: '#eeeeee' }, 'box=div|border=none'],
    ['background with null padding', { backgroundColor: '#eeeeee', padding: null }, 'box=div|border=none'],
    ['background and border with no padding key', { backgroundColor: '#eeeeee', borderColor: '#000000' }, 'box=div|border=some'],
    ['background with zero padding', { backgroundColor: '#eeeeee', ...ZERO }, 'box=cell|border=none'],
    ['padding above 0', { padding: { top: 8, right: 8, bottom: 8, left: 8 } }, 'box=cell|border=none'],
    ['padding above 0 with a border', { padding: { top: 0, right: 0, bottom: 16, left: 0 }, borderColor: '#000000' }, 'box=cell|border=some'],
    ['border alone', { borderColor: '#000000' }, 'box=div|border=some'],
    ['radius alone', { borderRadius: 8 }, 'box=div|border=none'],
    ['border with zero padding', { borderColor: '#000000', ...ZERO }, 'box=div|border=some'],
    ['radius with zero padding', { borderRadius: 8, ...ZERO }, 'box=div|border=none'],
    ['a bare Container', { ...ZERO }, 'bare'],
  ];
  for (const [name, style, want] of BOX_CASES) {
    const got = containerParts({ type: 'Container', data: { style, props: { childrenIds: [] } } });
    check(`IA21: Container parts, ${name} -> ${want}`, got === want, got);
  }
} finally {
  dom.window.close();
}

console.log(failed ? `${failed} FAILURE(S)` : 'ALL PASS');
process.exit(failed ? 1 : 0);
