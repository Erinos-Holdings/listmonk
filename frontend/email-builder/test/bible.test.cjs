// integrations RENDERING-BIBLE-SPEC I13 -- the rendering bible (test/build-bible.cjs):
//
//   - regenerating reproduces the committed files byte for byte (and leaves no stale bible file);
//   - at most 6 sheets, at most 40 rows a sheet, each sheet compiles to at most 80,000 bytes;
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
const fs = require('fs');
const path = require('path');
const { loadUmd } = require('./_umd.cjs');
const { CANARY_DIR } = require('./build-canary.cjs');
const { BIBLE_DIR, MANIFEST, LIMITS, SIZES, PADS, CONTEXTS, bibleFiles, compiledBytes } = require('./build-bible.cjs');

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
    // BIBLE-OUTLOOK-FIXES-SPEC S5: no bible Button is a fallback — every Word copy is the
    // VML-text group, never the old <w:anchorlock/> + <center> shape (flag-off sheets have none).
    const { context, refs } = JSON.parse(fs.readFileSync(path.join(CANARY_DIR, '_context.json'), 'utf8'));
    const html = EB.compileDocument(JSON.parse(JSON.stringify(docs[s])), context, refs);
    check(`${s}: no Button compiles to the fallback Word shape`, !/anchorlock/.test(html));
  }

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
} finally {
  dom.window.close();
}

console.log(failed ? `${failed} FAILURE(S)` : 'ALL PASS');
process.exit(failed ? 1 : 0);
