// integrations RENDERING-BIBLE-SPEC §3.6 -- the rendering bible's generator.
//
// Writes test/canary/bible-<nn>-<slug>.json (render-canary corpus documents: "proven" and
// "watched" are one list, U1) and test/bible/manifest.json from the matrix declared below. The
// generated files are committed; test/bible.test.cjs regenerates them and requires byte-for-byte
// equality (I13). A ROW is one top-level block of a sheet (a block, a Container or a column row),
// preceded by a small label Text block `B<sheet>.<row> · <what it shows>`.
//
//   B1 flat alignment            the four types x every alignment at top level, and each alone in a Container;
//                                a height-only Image at -, center and right at top level (amendment A1);
//                                a plain Button (auto width, no border) at -, left and right at top level (review L6)
//   B2 two columns               the four types x every alignment, the same block in both columns;
//                                gap none, valign top/bottom, one fixed-width row
//   B3 three columns             the four types x every alignment in all three columns (the whole grid,
//                                explicit left included); then any Heading rows moved from B4 (SA12)
//   B4 nesting                   the four types x every alignment as a Container in each of two
//                                columns, and as a two-column row inside a Container
//   B5 colour bands, other blocks ink x ground pairings, Button label x fill, rows either side of both band
//                                thresholds, two hues per band, image assets on light and dark grounds,
//                                Button shape x full/inline (+ custom width, bordered), Divider, Spacer,
//                                Avatar, Image unsized, an Html block, OfficialFooter brand and corporate
//   B6 layout variants           under Outlook off: the key rows again (§3.10 "one factor at a time")
//   B7 real arrangements         (BIBLE-OUTLOOK-FIXES-SPEC SA11) three bordered-Container rows, the two
//                                fallback Buttons (SA13), the redundant-wrapper evidence rows (SA5:
//                                three twin pairs, each a row followed by the same content in bare
//                                Containers), any B4 Heading rows B3 had no room for, then the census's
//                                compositions not already on B1-B4 or earlier on B7, most common first
//
// "Every alignment" is absent (`-`), center, right -- and explicit `left`, which the census found in
// stored bodies (test/bible/census.json), so `left` is a cell in every context too. The values the
// version 2 descriptor drops are SPREAD, not held constant (U7): font sizes cycle 11/14/16/20/28/40
// (a Heading's size is its level, which cycles h3/h2/h1 -- the block ignores style.fontSize) and
// the horizontal padding cycles 0/8/24/48 px (total, split evenly) by row index. A row's TEXT is
// chosen (and, only for a deep census composition, its padding stepped down) so nothing overflows
// its column at a 360 px viewport by the D4.4 estimate (integrations lib/campaign-review/
// value-rules.ts): a bible render must read clean.
//
// Containers (BIBLE-OUTLOOK-FIXES-SPEC SA4): every Container the generator builds is STYLED --
// padding on all four sides cycling 8, 16 and 24 px by the row's index on its sheet, no background,
// no radius -- because a bare Container (style-less, zero padding) is transparent to the review's
// version 2 matching and would vouch for nothing of its own. The fit estimate subtracts that padding
// and a bordered Container's two borders. The only bare Containers are B7's four evidence wrappers.
//
// Limits (asserted by bible.test.cjs): at most 7 sheets, 40 rows a sheet, and 80,000 compiled bytes
// a sheet. The overflow rule (SA12), in order: (1) explicit `left` that does not fit B4 is dropped
// and the gap recorded (B4 only: B3's grid is never dropped); (2) while B4 is still over a limit, its
// Heading rows move, last first, onto B3 after its grid while B3 stays within the limits, the rest
// onto B7; (3) while B7 is over
// a limit, its census rows are dropped least common first. manifest.json records every move and
// drop; nothing else moves or is dropped, and a sheet still over a limit makes the generator throw.
//
// Images are existing listmonk media with a current, clean dark-mode verdict (test/bible/assets.json).
// Every Image states its sizing mode explicitly (amendment A1 -- version 2 tells `width`,
// `height-only` and `unsized` apart): `imageProps` is the one place an Image's size props are set.
//
//   node test/build-bible.cjs [--umd <email-builder.umd.js>] [--check]
//
// --check writes nothing and exits 1 when a committed file differs from a fresh generation.
const fs = require('fs');
const path = require('path');

const { CANARY_DIR } = require('./build-canary.cjs');

const BIBLE_DIR = path.join(__dirname, 'bible');
const MANIFEST = path.join(BIBLE_DIR, 'manifest.json');
const CENSUS = JSON.parse(fs.readFileSync(path.join(BIBLE_DIR, 'census.json'), 'utf8'));
const ASSETS = JSON.parse(fs.readFileSync(path.join(BIBLE_DIR, 'assets.json'), 'utf8'));

const LIMITS = { sheets: 7, rows: 40, bytes: 80000 };
const TYPES = ['Text', 'Heading', 'Button', 'Image'];
const ALIGNS = ['-', 'left', 'center', 'right'];
const SIZES = [11, 14, 16, 20, 28, 40];
const PADS = [0, 8, 24, 48];
// SA4: a Container's padding on all four sides, by the row's index on its sheet (never 0).
const CONTAINER_PADS = [8, 16, 24];
const containerPadOf = (i) => CONTAINER_PADS[i % CONTAINER_PADS.length];
const LEVELS = ['h3', 'h2', 'h1'];
const HEADING_SIZE = { h1: 32, h2: 24, h3: 20 };
const GAP = 16;
const NARROW = 360;
const CANVAS = 600;
const LINK = 'https://canary.invalid/bible';
const CONTEXTS = ['top', 'container', 'col2', 'col3', 'containerInColumn', 'columnRowInContainer'];
const CONTEXT_LABEL = {
  top: 'top level',
  container: 'in a Container',
  col2: 'in both of two columns',
  col3: 'in all three columns',
  containerInColumn: 'as a Container in each of two columns',
  columnRowInContainer: 'as a two-column row in a Container',
};
// The text candidates, longest first: a row takes the longest whose run fits its column.
const WORDS = ['Go far', 'Go', 'G'];

const pad2 = (n) => String(n).padStart(2, '0');
const padding = (total, v = 8) => ({ top: v, bottom: v, left: total / 2, right: total / 2 });
const alignStyle = (a) => (a === '-' ? {} : { textAlign: a });
const alignLabel = (a) => (a === '-' ? 'no alignment' : `aligned ${a}`);

// The EmailLayout of a census descriptor `EmailLayout|outlook=..|radius=..|backdrop=..|canvas=..`.
function layoutOf(descriptor) {
  const kv = Object.fromEntries(descriptor.split('|').slice(1).map((p) => p.split('=')));
  const band = { light: ['#F5F5F5', '#FFFFFF'], mid: ['#8A8A8A', '#8A8A8A'], dark: ['#262626', '#262626'] };
  return {
    backdropColor: band[kv.backdrop][0],
    canvasColor: band[kv.canvas][1],
    textColor: '#262626',
    fontFamily: 'MODERN_SANS',
    outlook: kv.outlook === 'true',
    ...(kv.radius === 'rounded' ? { borderRadius: 8 } : {}),
  };
}

// ---------------------------------------------------------------------------------------------
// Widths -- the D4.4 estimate (mirrors integrations lib/campaign-review/value-rules.ts)
// ---------------------------------------------------------------------------------------------

const runOf = (s) => String(s).split(/[ \t\r\n]+|-|\//).reduce((a, r) => ([...r].length > [...a].length ? r : a), '');
const runWidth = (s, size, bold) => [...runOf(s)].length * size * (bold ? 0.65 : 0.6);
// A custom-width Button's label is ONE nowrap line (Button.tsx: white-space: nowrap when
// customWidth > 0) -- the whole label is measured, never its longest word (re-review R1).
const labelWidth = (s, size, bold) => [...String(s).replace(/\s+/g, ' ').trim()].length * size * (bold ? 0.65 : 0.6);
const columnWidth = (w, n, gap = GAP) => (w - gap * (n - 1)) / n;

// ---------------------------------------------------------------------------------------------
// Blocks
// ---------------------------------------------------------------------------------------------

function labelBlock(text) {
  return { type: 'Text', data: { style: { fontSize: 12, fontWeight: 'normal', padding: { top: 12, bottom: 4, left: 24, right: 24 } }, props: { markdown: true, text } } };
}

// The cycled values of row index `i`.
const cycleOf = (i) => ({ size: SIZES[i % SIZES.length], pad: PADS[i % PADS.length], level: LEVELS[i % LEVELS.length] });

// One content block of `type`, `align`, with `values` ({size, pad, level}; a row index is cycled),
// fitted to `avail` px (the 360 px viewport).
function contentBlock(type, align, values, avail, sizing = 'width') {
  const v = typeof values === 'number' ? cycleOf(values) : values;
  let size = v.size;
  let pad = v.pad;
  const level = v.level;
  for (;;) {
    const b = tryBlock(type, align, size, pad, level, avail, sizing);
    if (b) return b;
    // Only a deep census composition reaches here: step the padding down, then the size.
    const pi = PADS.indexOf(pad);
    const si = SIZES.indexOf(size);
    if (pi > 0) pad = PADS[pi - 1];
    else if (si > 0) size = SIZES[si - 1];
    else throw new Error(`no ${type} fits ${avail.toFixed(1)} px`);
  }
}

function tryBlock(type, align, size, pad, level, avail, sizing = 'width') {
  const style = { ...alignStyle(align), padding: padding(pad) };
  switch (type) {
    case 'Text': {
      const text = WORDS.find((w) => runWidth(w, size, false) + pad <= avail);
      return text && { type: 'Text', data: { style: { ...style, fontSize: size, fontWeight: 'normal' }, props: { markdown: true, text } } };
    }
    case 'Heading': {
      const hs = HEADING_SIZE[level];
      const text = WORDS.find((w) => runWidth(w, hs, true) + pad <= avail);
      return text && { type: 'Heading', data: { style, props: { level, text } } };
    }
    case 'Button': {
      // The census's most common Button: pill, inline, custom width, bordered (label nowrap inside it).
      const room = Math.min(140, Math.floor(avail - pad));
      // The whole label plus twice the 1 px border must fit inside the custom width (D4.4's model).
      const text = WORDS.find((w) => labelWidth(w, size, true) + 2 <= room);
      return text && {
        type: 'Button',
        data: {
          style: { ...style, fontSize: size },
          // The border reads against the fill (D4.3, amendment A2: a border the fill's colour is an empty pill in Word).
          props: { text, url: LINK, buttonStyle: 'pill', fullWidth: false, size: 'medium', customWidth: room, borderSize: 1, borderColor: '#FFFFFF', buttonBackgroundColor: '#003B4D', buttonTextColor: '#FFFFFF' },
        },
      };
    }
    case 'Image': {
      if (sizing === 'height-only') return avail - pad >= 100 && { type: 'Image', data: { style, props: imageProps('height-only', ASSETS.logoOnTransparent, LOGO_HEIGHT, 'Bible logo') } };
      if (sizing === 'unsized') return { type: 'Image', data: { style, props: imageProps('unsized', ASSETS.photo, null, 'Bible photo, unsized') } };
      const want = avail >= CANVAS / 2 ? 240 : avail >= 150 ? 140 : 80;
      const width = Math.min(want, Math.floor(avail - pad));
      return width >= 24 && { type: 'Image', data: { style, props: imageProps('width', ASSETS.photo, width, 'Bible photo') } };
    }
    case 'Avatar':
      return { type: 'Avatar', data: { style: { ...style }, props: { imageUrl: ASSETS.icon.url, shape: 'circle', size: 48, alt: 'Bible avatar' } } };
    case 'Html':
      return { type: 'Html', data: { style: { ...style }, props: { contents: '<p style="margin:0">Html</p>' } } };
    case 'Divider':
      return { type: 'Divider', data: { style: { padding: padding(pad) }, props: { lineColor: '#CCCCCC' } } };
    case 'Spacer':
      return { type: 'Spacer', data: { props: { height: 16 } } };
    case 'OfficialFooter':
      return { type: 'OfficialFooter', data: { props: { kind: 'corporate' } } };
    default:
      throw new Error(`unknown block type ${type}`);
  }
}

// An Image's props in one sizing mode: `width` (a px width), `height-only` (a px height and no
// width -- a small wide logo, the shape Word overflows when it is large) or `unsized` (neither).
function imageProps(mode, asset, size, alt) {
  const base = { url: asset.url, alt, linkHref: LINK, contentAlignment: 'middle' };
  if (mode === 'width') return { ...base, width: size };
  if (mode === 'height-only') return { ...base, height: size };
  if (mode === 'unsized') return base;
  throw new Error(`unknown image sizing mode ${mode}`);
}
// The height-only rows' height: the logo (1200 x 322) renders about 89 px wide, so it fits any column.
const LOGO_HEIGHT = 24;

// SA4: a styled Container -- `pad` px on all four sides (the row's cycle value), no background, no
// radius; `style` adds a border colour for B7's bordered rows. `inner(avail, pad, style)` is the
// width the fit estimate gives a block inside it: less both paddings and, when bordered, both borders.
const container = (childrenIds, pad, style = {}) => {
  if (!(pad > 0)) throw new Error('every generated Container is padded (SA4)');
  return { type: 'Container', data: { style: { padding: { top: pad, bottom: pad, left: pad, right: pad }, ...style }, props: { childrenIds } } };
};
const inner = (avail, pad, style = {}) => avail - 2 * pad - (style.borderColor ? 2 : 0);
// SA5: a BARE Container (the editor's unstyled one): a null style, or zero padding and nothing else.
const bareContainer = (childrenIds, nullStyle) => ({ type: 'Container', data: { style: nullStyle ? null : { padding: { top: 0, bottom: 0, left: 0, right: 0 } }, props: { childrenIds } } });
const columns = (cols, props = {}) => ({
  type: 'ColumnsContainer',
  data: {
    style: { padding: { top: 8, bottom: 8, left: 0, right: 0 } },
    props: { columnsCount: cols.length === 3 ? 3 : 2, columnsGap: GAP, contentAlignment: 'middle', ...props, columns: [0, 1, 2].map((k) => ({ childrenIds: cols[k] || [] })) },
  },
});

// A cell row: `type` x `align` placed in `ctx`, with `values` (a row index, or {size, pad, level});
// `rowIndex` (the row's index on its sheet) sets its Containers' padding (SA4). Returns { id, blocks }.
function cellRow(prefix, type, align, ctx, values, rowIndex) {
  const blocks = {};
  const leaf = (suffix, avail) => {
    const id = `${prefix}-${suffix}`;
    blocks[id] = contentBlock(type, align, values, avail, type === 'Image' ? 'width' : undefined);
    return id;
  };
  const half = columnWidth(NARROW, 2);
  const cp = containerPadOf(rowIndex);
  switch (ctx) {
    case 'top':
      return { id: leaf('x', NARROW), blocks };
    case 'container':
      blocks[prefix] = container([leaf('x', inner(NARROW, cp))], cp);
      return { id: prefix, blocks };
    case 'col2':
      blocks[prefix] = columns([[leaf('c0', half)], [leaf('c1', half)]]);
      return { id: prefix, blocks };
    case 'col3': {
      const w = columnWidth(NARROW, 3);
      blocks[prefix] = columns([[leaf('c0', w)], [leaf('c1', w)], [leaf('c2', w)]]);
      return { id: prefix, blocks };
    }
    case 'containerInColumn':
      blocks[`${prefix}-k0`] = container([leaf('c0', inner(half, cp))], cp);
      blocks[`${prefix}-k1`] = container([leaf('c1', inner(half, cp))], cp);
      blocks[prefix] = columns([[`${prefix}-k0`], [`${prefix}-k1`]]);
      return { id: prefix, blocks };
    case 'columnRowInContainer': {
      const w = columnWidth(inner(NARROW, cp), 2);
      blocks[`${prefix}-cols`] = columns([[leaf('c0', w)], [leaf('c1', w)]]);
      blocks[prefix] = container([`${prefix}-cols`], cp);
      return { id: prefix, blocks };
    }
    default:
      throw new Error(`unknown context ${ctx}`);
  }
}

const cellSpecs = (ctx, types = TYPES) =>
  types.flatMap((type) => ALIGNS.map((align) => ({ what: `${type === 'Image' ? 'Image sized by width' : type}, ${alignLabel(align)}, ${CONTEXT_LABEL[ctx]}`, type, cell: { type, align, ctx }, build: (p, i, values) => cellRow(p, type, align, ctx, values, i) })));

// U7 per context class: within one class the padding cycles by its row index, the font size by
// its index among the class's Text and Button rows, and the Heading level by its index among the
// class's Heading rows -- so every class shows the smallest and the largest of each, whatever
// rows a limit dropped.
function cellCycles(specs) {
  const n = {};
  return specs.map((s) => {
    if (!s.cell) return null;
    const c = (n[s.cell.ctx] = n[s.cell.ctx] || { row: 0, sized: 0, heading: 0 });
    const v = { pad: PADS[c.row % PADS.length], size: SIZES[c.sized % SIZES.length], level: LEVELS[c.heading % LEVELS.length] };
    c.row += 1;
    if (s.type === 'Text' || s.type === 'Button') c.sized += 1;
    if (s.type === 'Heading') c.heading += 1;
    return v;
  });
}

// ---------------------------------------------------------------------------------------------
// Census compositions (B3): `Container[a+b]`, `Columns[a;b;c]`, leaves `Type`, `Type@align`, `Image:sized@align`
// ---------------------------------------------------------------------------------------------

function parseComposition(s) {
  let pos = 0;
  const peek = (str) => s.startsWith(str, pos);
  function list(stop) {
    const out = [];
    if (s[pos] === '-' && (s[pos + 1] === stop || s[pos + 1] === ';')) {
      pos += 1;
      return out;
    }
    for (;;) {
      out.push(node());
      if (s[pos] === '+') pos += 1;
      else return out;
    }
  }
  function node() {
    if (peek('Container[')) {
      pos += 'Container['.length;
      const children = list(']');
      pos += 1;
      return { kind: 'Container', children };
    }
    if (peek('Columns[')) {
      pos += 'Columns['.length;
      const cols = [];
      for (;;) {
        cols.push(list(']'));
        if (s[pos] === ';') pos += 1;
        else break;
      }
      pos += 1;
      return { kind: 'Columns', cols };
    }
    const m = /^([A-Za-z]+)(?::(width|height-only|unsized))?(?:@([a-z-]+))?/.exec(s.slice(pos));
    if (!m) throw new Error(`cannot parse composition ${s} at ${pos}`);
    pos += m[0].length;
    return { kind: 'leaf', type: m[1], sizing: m[2] || null, align: m[3] || '-' };
  }
  const n = node();
  if (pos !== s.length) throw new Error(`trailing input in composition ${s}`);
  return n;
}

// Build a census composition as one row. `fixed` values: realistic, not cycled (they are not cells).
// Every Container in it takes the row's padding (SA4, `rowIndex` on its sheet).
function compositionRow(prefix, comp, rowIndex) {
  const blocks = {};
  let k = 0;
  const fixed = { size: 16, pad: 24, level: 'h2' };
  const cp = containerPadOf(rowIndex);
  const build = (n, avail) => {
    const id = `${prefix}-${k++}`;
    if (n.kind === 'Container') {
      blocks[id] = container(n.children.map((c) => build(c, inner(avail, cp))), cp);
    } else if (n.kind === 'Columns') {
      const used = n.cols.length === 3 && n.cols[2].length ? 3 : 2;
      const w = columnWidth(avail, used);
      blocks[id] = columns(n.cols.slice(0, used).map((col) => col.map((c) => build(c, w))));
    } else {
      blocks[id] = contentBlock(n.type, n.align, fixed, avail, n.type === 'Image' ? n.sizing || 'width' : undefined);
    }
    return id;
  };
  return { id: build(parseComposition(comp), NARROW), blocks };
}

// ---------------------------------------------------------------------------------------------
// The v2 composition tokens (mirrors integrations lib/campaign-review/fingerprint.ts compositionsOfV2)
// ---------------------------------------------------------------------------------------------

// BIBLE-OUTLOOK-FIXES-SPEC SA1: the editor's unstyled Container (src/documents/structure.ts
// isUnstyledContainer), ported here because this generator runs without the bundle's modules.
function isBareContainer(b) {
  if (!b || b.type !== 'Container') return false;
  const style = b.data ? b.data.style : null;
  if (style === null || style === undefined) return true;
  if (typeof style !== 'object') return false;
  const emptyPad = (p) => p === null || p === undefined || (typeof p === 'object' && p.top === 0 && p.right === 0 && p.bottom === 0 && p.left === 0);
  return Object.entries(style).every(([k, v]) => {
    if (k === 'backgroundColor' || k === 'borderColor') return !v;
    if (k === 'borderRadius') return v === null || v === undefined || v === 0;
    if (k === 'padding') return emptyPad(v);
    return !v;
  });
}

// SA9: the Container descriptor's box and border parts -- `cell` when style.padding is an object
// and a side is above 0 or a background is set (the compile's transformSimpleDivBlocks), else `div`;
// `border=some` when style.borderColor is truthy. A bare Container has none (`bare`).
function containerParts(b) {
  if (isBareContainer(b)) return 'bare';
  const s = (b && b.data && b.data.style && typeof b.data.style === 'object') ? b.data.style : {};
  const p = s.padding;
  const box = p && typeof p === 'object' && (['top', 'right', 'bottom', 'left'].some((k) => typeof p[k] === 'number' && p[k] > 0) || !!s.backgroundColor) ? 'cell' : 'div';
  return `box=${box}|border=${s.borderColor ? 'some' : 'none'}`;
}

// SA2/SA8: a bare Container is TRANSPARENT -- no composition of its own; inside a parent its
// children's tokens stand in its place, recursively; one met again on the same path yields nothing.
// Consecutive identical tokens in a Container collapse after the splice. Shares IA2's case table
// with integrations fingerprint-v2.test.ts (bible.test.cjs, IA21).
const ALIGNED = new Set(['Text', 'Heading', 'Button', 'Image', 'Avatar', 'Html']);
function compositionsOf(doc) {
  const toks = (id, seen) => {
    const b = doc[id] || {};
    const p = (b.data && b.data.props) || {};
    const s = (b.data && b.data.style) || {};
    if (isBareContainer(b)) return seen.has(id) ? [] : (p.childrenIds || []).flatMap((x) => toks(x, new Set(seen).add(id)));
    if ((b.type === 'Container' || b.type === 'ColumnsContainer') && !seen.has(id)) return [comp(id, new Set(seen).add(id))];
    const base = b.type === 'Image' ? `Image:${typeof p.width === 'number' ? 'width' : typeof p.height === 'number' ? 'height-only' : 'unsized'}` : b.type;
    return [ALIGNED.has(b.type) ? `${base}@${typeof s.textAlign === 'string' && s.textAlign ? s.textAlign : '-'}` : base];
  };
  const comp = (id, seen) => {
    const p = (doc[id].data && doc[id].data.props) || {};
    if (doc[id].type === 'ColumnsContainer') return `Columns[${(p.columns || []).map((c) => (c.childrenIds || []).flatMap((x) => toks(x, seen)).join('+') || '-').join(';')}]`;
    const t = (p.childrenIds || []).flatMap((x) => toks(x, seen)).filter((x, i, all) => i === 0 || x !== all[i - 1]);
    return `Container[${t.join('+') || '-'}]`;
  };
  return Object.keys(doc)
    .filter((id) => doc[id] && (doc[id].type === 'ColumnsContainer' || (doc[id].type === 'Container' && !isBareContainer(doc[id]))))
    .map((id) => comp(id, new Set([id])));
}

// ---------------------------------------------------------------------------------------------
// B5 -- colour bands and the other blocks
// ---------------------------------------------------------------------------------------------

const ownText = (text, ink, ground, extra = {}) => ({
  type: 'Text',
  data: { style: { color: ink, backgroundColor: ground, fontSize: 20, fontWeight: 'bold', textAlign: 'center', padding: padding(48, 12), ...extra }, props: { markdown: true, text } },
});
const button = (text, props, style = {}) => ({
  type: 'Button',
  data: { style: { textAlign: 'center', padding: padding(48), ...style }, props: { text, url: LINK, size: 'medium', fullWidth: false, buttonStyle: 'rounded', buttonBackgroundColor: '#003B4D', buttonTextColor: '#FFFFFF', ...props } },
});
const single = (b) => (p) => ({ id: `${p}-x`, blocks: { [`${p}-x`]: b } });

function b5Specs() {
  const pair = (what, text, ink, ground) => ({ what, build: single(ownText(text, ink, ground)) });
  const imgOn = (what, asset, ground, width) => ({
    what,
    build: single({ type: 'Image', data: { style: { textAlign: 'center', backgroundColor: ground, padding: padding(48, 16) }, props: imageProps('width', asset, width, 'Bible logo') } }),
  });
  return [
    // ink x ground pairings (dark <= 0.30 < mid < 0.60 <= light, Rec. 709 gamma-space luma)
    pair('Text, dark ink on a light ground', 'Dark on light', '#262626', '#F9F9F9'),
    pair('Text, light ink on a dark ground', 'Light on dark', '#FFFFFF', '#262626'),
    pair('Text, mid ink on a light ground', 'Mid on light', '#555555', '#FFFFFF'),
    pair('Text, mid ink on a dark ground', 'Mid on dark', '#8A8A8A', '#1A1A1A'),
    pair('Text, dark ink on a mid ground', 'Dark on mid', '#000000', '#8A8A8A'),
    pair('Text, light ink on a mid ground', 'Light on mid', '#FFFFFF', '#5A5A5A'),
    // Button label x fill
    { what: 'Button, light label on a dark fill', build: single(button('Light on dark', { buttonTextColor: '#FFFFFF', buttonBackgroundColor: '#003B4D' })) },
    { what: 'Button, dark label on a light fill', build: single(button('Dark on light', { buttonTextColor: '#262626', buttonBackgroundColor: '#F2F2F2', borderSize: 1, borderColor: '#262626' })) },
    { what: 'Button, light label on a mid fill', build: single(button('Light on mid', { buttonTextColor: '#FFFFFF', buttonBackgroundColor: '#5A5A5A' })) },
    { what: 'Button, dark label on a mid fill', build: single(button('Dark on mid', { buttonTextColor: '#000000', buttonBackgroundColor: '#8A8A8A' })) },
    // Just either side of both thresholds (luma 0.290 / 0.310 and 0.588 / 0.612), as grounds
    pair('Text on a ground just inside dark (luma 0.290)', 'Edge dark', '#FFFFFF', '#4A4A4A'),
    pair('Text on a ground just inside mid (luma 0.310)', 'Edge mid low', '#FFFFFF', '#4F4F4F'),
    pair('Text on a ground just inside mid (luma 0.588)', 'Edge mid high', '#000000', '#969696'),
    pair('Text on a ground just inside light (luma 0.612)', 'Edge light', '#000000', '#9C9C9C'),
    // Two hues per band
    pair('Text on a dark teal ground', 'Teal', '#FFFFFF', '#003B4D'),
    pair('Text on a dark purple ground', 'Purple', '#FFFFFF', '#220258'),
    pair('Text on a mid pink ground', 'Pink', '#FFFFFF', '#E54582'),
    pair('Text on a mid amber ground', 'Amber', '#000000', '#CA8A04'),
    pair('Text on a light yellow ground', 'Yellow', '#262626', '#FDE68A'),
    pair('Text on a light blue ground', 'Blue', '#262626', '#E0F2FE'),
    // Image assets on light and dark grounds
    imgOn('Logo on transparency, on a light ground', ASSETS.logoOnTransparent, '#FFFFFF', 200),
    imgOn('Logo on transparency, on a dark ground', ASSETS.logoOnTransparent, '#262626', 200),
    imgOn('Logo on white, on a light ground', ASSETS.logoOnWhite, '#FFFFFF', 240),
    imgOn('Logo on white, on a dark ground', ASSETS.logoOnWhite, '#262626', 240),
    // Button shape x full/inline, a custom-width and a bordered Button
    ...['rectangle', 'rounded', 'pill'].flatMap((shape) => [false, true].map((full) => ({ what: `Button, ${shape}, ${full ? 'full width' : 'inline'}`, build: single(button(`${shape} ${full ? 'full' : 'inline'}`, { buttonStyle: shape, fullWidth: full })) }))),
    // The custom-size row takes the census's most common unbordered Button: pill, custom width AND
    // custom height (version 2 tells the two apart -- review L3).
    { what: 'Button, pill, custom width and height', build: single(button('Custom', { customWidth: 200, customHeight: 48, buttonStyle: 'pill' })) },
    // Re-review R2: custom width, auto height, no border.
    { what: 'Button, custom width, auto height, no border', build: single(button('Wide', { customWidth: 200 })) },
    { what: 'Button, bordered', build: single(button('Bordered', { borderSize: 2, borderColor: '#262626', buttonBackgroundColor: '#FFFFFF', buttonTextColor: '#262626' })) },
    // The other blocks
    { what: 'Divider', build: single({ type: 'Divider', data: { style: { padding: padding(48, 16) }, props: { lineColor: '#CCCCCC', lineHeight: 1 } } }) },
    { what: 'Spacer', build: single({ type: 'Spacer', data: { props: { height: 32 } } }) },
    { what: 'Avatar, centred', build: single({ type: 'Avatar', data: { style: { textAlign: 'center', padding: padding(48, 16) }, props: { imageUrl: ASSETS.icon.url, shape: 'circle', size: 64, alt: 'Bible avatar' } } }) },
    { what: 'Image, unsized', build: single({ type: 'Image', data: { style: { padding: padding(0, 0) }, props: imageProps('unsized', ASSETS.photo, null, 'Bible photo, unsized') } }) },
    {
      what: 'Html, a table with a style element and a link',
      build: single({
        type: 'Html',
        data: {
          style: { padding: padding(48, 16) },
          props: {
            contents:
              '<style>.bible-cell{padding:8px;border:1px solid #CCCCCC;}</style>\n<table role="presentation" border="0" cellpadding="0" cellspacing="0" style="margin:0 auto;">\n  <tr>\n    <td class="bible-cell" style="color:#262626;">Html cell</td>\n    <td class="bible-cell"><a href="https://canary.invalid/bible-html" style="color:#003B4D;">Html link</a></td>\n  </tr>\n</table>',
          },
        },
      }),
    },
    { what: 'OfficialFooter, brand', build: single({ type: 'OfficialFooter', data: { props: { kind: 'brand' } } }) },
    { what: 'OfficialFooter, corporate', build: single({ type: 'OfficialFooter', data: { props: { kind: 'corporate' } } }) },
  ];
}

// ---------------------------------------------------------------------------------------------
// B2 / B6 extras
// ---------------------------------------------------------------------------------------------

function columnsRow(prefix, i, leaves, props) {
  const blocks = {};
  const n = leaves.length;
  // §3.3's column width: the fixed width when set, else (W - gap x (n - 1) - the fixed widths) / the columns without one.
  const fixed = (props.fixedWidths || []).slice(0, n);
  const fixedSum = fixed.reduce((a, w) => a + (typeof w === 'number' ? w : 0), 0);
  const free = n - fixed.filter((w) => typeof w === 'number').length;
  const gap = props.columnsGap ?? GAP;
  const avail = (k) => (typeof fixed[k] === 'number' ? fixed[k] : (NARROW - gap * (n - 1) - fixedSum) / free);
  const cols = leaves.map(([type, align], k) => {
    const id = `${prefix}-c${k}`;
    blocks[id] = contentBlock(type, align, i, avail(k), type === 'Image' ? 'width' : undefined);
    return [id];
  });
  blocks[prefix] = columns(cols, props);
  return { id: prefix, blocks };
}

function b2Extras() {
  return [
    { what: 'two columns, no gap', build: (p, i) => columnsRow(p, i, [['Text', '-'], ['Text', '-']], { columnsGap: 0 }) },
    { what: 'two columns, vertical alignment top', build: (p, i) => columnsRow(p, i, [['Image', '-'], ['Text', '-']], { contentAlignment: 'top' }) },
    { what: 'two columns, vertical alignment bottom', build: (p, i) => columnsRow(p, i, [['Image', '-'], ['Text', '-']], { contentAlignment: 'bottom' }) },
    { what: 'two columns, fixed first column width', build: (p, i) => columnsRow(p, i, [['Image', '-'], ['Text', '-']], { fixedWidths: [120, null, null] }) },
  ];
}

function b6Specs() {
  return [
    ...[['Text', 'center'], ['Heading', '-'], ['Button', 'center'], ['Image', '-']].map(([type, align]) => ({ what: `${type}, ${alignLabel(align)}, top level, Outlook off`, build: (p, i) => cellRow(p, type, align, 'top', i, i) })),
    { what: 'a Container, Outlook off', build: (p, i) => cellRow(p, 'Text', '-', 'container', i, i) },
    { what: 'two columns, Outlook off', build: (p, i) => columnsRow(p, i, [['Image', '-'], ['Text', 'center']], {}) },
    { what: 'three columns, Outlook off', build: (p, i) => cellRow(p, 'Text', 'center', 'col3', i, i) },
    ...['rectangle', 'rounded', 'pill'].map((shape) => ({ what: `Button, ${shape}, Outlook off`, build: single(button(shape, { buttonStyle: shape })) })),
  ];
}

// ---------------------------------------------------------------------------------------------
// BIBLE-OUTLOOK-FIXES-SPEC §12: B7's bordered, fallback and evidence rows (SA11, SA13, SA5)
// ---------------------------------------------------------------------------------------------

// The row values held fixed on these rows (they are not cells, so nothing is cycled).
const FIXED = { size: 16, pad: 24, level: 'h2' };

// SA5: three twin pairs, each a row directly followed by the same content with bare Containers at
// top level, on B7 at (c). `base` is the number of B7 rows before them, so the wrapped row names its twin.
function evidenceRows(base) {
  const twinNo = (k) => `B7.${base + 2 * k + 1}`;
  const image = () => contentBlock('Image', 'right', FIXED, NARROW, 'width');
  const plain = () => button('Go', { buttonStyle: 'rounded' });
  const half = columnWidth(NARROW, 2);
  const textIn = () => contentBlock('Text', 'center', FIXED, half);
  const buttonIn = () => button('Go', { buttonStyle: 'rounded' }, { padding: padding(24) });
  const pairRow = (p, wrapped) => {
    const blocks = { [`${p}-t0`]: textIn(), [`${p}-b0`]: buttonIn(), [`${p}-t1`]: textIn(), [`${p}-b1`]: buttonIn() };
    if (!wrapped) {
      blocks[p] = columns([[`${p}-t0`, `${p}-b0`], [`${p}-t1`, `${p}-b1`]]);
    } else {
      blocks[`${p}-w0`] = bareContainer([`${p}-t0`, `${p}-b0`], false);
      blocks[`${p}-w1`] = bareContainer([`${p}-t1`, `${p}-b1`], false);
      blocks[p] = columns([[`${p}-w0`], [`${p}-w1`]]);
    }
    return { id: p, blocks };
  };
  return [
    { what: 'evidence twin: Image sized by width, aligned right, top level', evidence: 'twin', build: single(image()) },
    {
      what: `evidence: the same Image alone in a Container with no style (redundant wrapper, must render as ${twinNo(0)})`,
      evidence: 'wrapped',
      build: (p) => ({ id: `${p}-w`, blocks: { [`${p}-w`]: bareContainer([`${p}-x`], true), [`${p}-x`]: image() } }),
    },
    { what: 'evidence twin: Button, plain (auto width, no border), centred, top level', evidence: 'twin', build: single(plain()) },
    {
      what: `evidence: the same Button alone in a Container with zero padding (redundant wrapper, must render as ${twinNo(1)})`,
      evidence: 'wrapped',
      build: (p) => ({ id: `${p}-w`, blocks: { [`${p}-w`]: bareContainer([`${p}-x`], false), [`${p}-x`]: plain() } }),
    },
    { what: 'evidence twin: two columns, each a Text and a Button', evidence: 'twin', build: (p) => pairRow(p, false) },
    {
      what: `evidence: the same row with each column's two blocks grouped in a Container with zero padding (redundant wrapper, must render as ${twinNo(2)})`,
      evidence: 'wrapped',
      build: (p) => pairRow(p, true),
    },
  ];
}

// SA11 (a): three bordered-Container rows -- a border colour, padding from SA4's cycle, no radius.
const BORDER = { borderColor: '#CCCCCC' };
function borderedRows() {
  return [
    {
      what: 'a bordered Container holding a two-column row (Text centred, Image sized by width)',
      bordered: true,
      build: (p, i) => {
        const cp = containerPadOf(i);
        const w = columnWidth(inner(NARROW, cp, BORDER), 2);
        return {
          id: p,
          blocks: {
            [p]: container([`${p}-cols`], cp, BORDER),
            [`${p}-cols`]: columns([[`${p}-c0`], [`${p}-c1`]]),
            [`${p}-c0`]: contentBlock('Text', 'center', FIXED, w),
            [`${p}-c1`]: contentBlock('Image', '-', FIXED, w, 'width'),
          },
        };
      },
    },
    {
      what: 'a bordered Container holding an Image sized by width at 600 (the compile clamps it inside the padding and borders)',
      bordered: true,
      build: (p, i) => ({
        id: p,
        blocks: {
          [p]: container([`${p}-x`], containerPadOf(i), BORDER),
          [`${p}-x`]: { type: 'Image', data: { style: { padding: padding(0, 0) }, props: imageProps('width', ASSETS.photo, 600, 'Bible photo, over-wide') } },
        },
      }),
    },
    {
      what: 'a bordered Container holding a Text and then a Button',
      bordered: true,
      build: (p, i) => {
        const cp = containerPadOf(i);
        return {
          id: p,
          blocks: {
            [p]: container([`${p}-t`, `${p}-b`], cp, BORDER),
            [`${p}-t`]: contentBlock('Text', 'center', FIXED, inner(NARROW, cp, BORDER)),
            [`${p}-b`]: button('Go', { buttonStyle: 'rounded' }, { padding: padding(24) }),
          },
        };
      },
    },
  ];
}

// SA13 (b): the two Buttons that fall back in Outlook for Windows -- a full-width label the compile
// estimates at two lines (cause `lines`) and an inline label ending in U+2192, outside the VML label
// set (cause `chars`). No other bible Button falls back (bible.test.cjs, IA14).
const FALLBACK_LINES_LABEL = 'Two lines in Outlook: this full-width label wraps onto a second line';
const FALLBACK_CHARS_LABEL = 'Shop now \u2192';
function fallbackRows() {
  return [
    { what: 'fallback Button: full width, a label on two lines (Outlook for Windows: the table-cell button)', fallback: 'lines', build: single(button(FALLBACK_LINES_LABEL, { fullWidth: true })) },
    { what: 'fallback Button: inline, a label ending in an arrow (Outlook for Windows: the table-cell button)', fallback: 'chars', build: single(button(FALLBACK_CHARS_LABEL, {})) },
  ];
}

// Pin each cell row's cycled values (U7) to the row, so a row that later moves sheets keeps them.
const pinCycles = (specs) => {
  const cycles = cellCycles(specs);
  return specs.map((s, i) => (cycles[i] ? { ...s, values: cycles[i] } : s));
};

// ---------------------------------------------------------------------------------------------
// Sheets
// ---------------------------------------------------------------------------------------------

function buildDoc(layout, specs, sheetNo) {
  const doc = { root: { type: 'EmailLayout', data: { ...layout, childrenIds: [] } } };
  const cycles = cellCycles(specs);
  specs.forEach((spec, i) => {
    const prefix = `b${sheetNo}-r${pad2(i + 1)}`;
    const labelId = `${prefix}-label`;
    doc[labelId] = labelBlock(`B${sheetNo}.${i + 1} · ${spec.what}`);
    const { id, blocks } = spec.build(prefix, i, spec.values || cycles[i] || i);
    Object.assign(doc, blocks);
    doc.root.data.childrenIds.push(labelId, id);
  });
  return doc;
}

const SHEETS = [
  { n: 1, slug: 'flat-alignment' },
  { n: 2, slug: 'two-columns' },
  { n: 3, slug: 'three-columns' },
  { n: 4, slug: 'nesting' },
  { n: 5, slug: 'colour-and-blocks' },
  { n: 6, slug: 'layout-variants' },
  { n: 7, slug: 'real-arrangements' },
];
const stemOf = (s) => `bible-${pad2(s.n)}-${s.slug}`;

// Compile with the canary's own context and refs; bytes of the compiled html.
function compiledBytes(EB, doc) {
  const { context, refs } = JSON.parse(fs.readFileSync(path.join(CANARY_DIR, '_context.json'), 'utf8'));
  return Buffer.byteLength(EB.compileDocument(JSON.parse(JSON.stringify(doc)), context, refs), 'utf8');
}

// Build every sheet with the loaded bundle `EB` (sizes are measured, so drops are deterministic per bundle).
function buildBible(EB) {
  const common = layoutOf(CENSUS.layouts[0].value);
  const outlookOff = layoutOf(CENSUS.layouts.find((l) => l.value.includes('outlook=false')).value);
  const leftEverywhere = CENSUS.explicitLeft.blocks > 0;
  const aligns = leftEverywhere ? ALIGNS : ALIGNS.filter((a) => a !== 'left');
  const cells = (ctx, types) => cellSpecs(ctx, types).filter((s) => aligns.includes(s.cell.align) || (s.cell.align === 'left' && ctx === 'top'));
  const fits = (doc) => doc.root.data.childrenIds.length / 2 <= LIMITS.rows && compiledBytes(EB, doc) <= LIMITS.bytes;
  const manifest = { census: CENSUS.source, layouts: { common: CENSUS.layouts[0].value, b6: CENSUS.layouts.find((l) => l.value.includes('outlook=false')).value }, sheets: [], gaps: [] };
  const out = {};
  const record = (sheet, doc, specs, dropped = []) => {
    if (!fits(doc)) throw new Error(`${stemOf(sheet)} exceeds a limit (${specs.length} rows, ${compiledBytes(EB, doc)} bytes) and holds nothing droppable`);
    out[stemOf(sheet)] = doc;
    manifest.sheets.push({ stem: stemOf(sheet), rows: specs.length, cells: specs.filter((s) => s.cell).map((s) => `${s.cell.type}@${s.cell.align}@${s.cell.ctx}`), dropped });
  };

  // Amendment A1: B1 also carries a height-only Image at -, center and right, top level.
  const heightOnly = ['-', 'center', 'right'].map((align) => ({
    what: `Image sized by height only, ${alignLabel(align)}, top level`,
    build: (p, i) => ({ id: `${p}-x`, blocks: { [`${p}-x`]: contentBlock('Image', align, i, NARROW, 'height-only') } }),
  }));
  // Review L6: a plain Button (auto width, no border) at -, left and right, top level (centre is in B5).
  const plainButtons = ['-', 'left', 'right'].map((align) => ({
    what: `Button, plain (auto width, no border), ${alignLabel(align)}, top level`,
    build: single(button('Go', { buttonStyle: 'rounded' }, align === '-' ? { textAlign: undefined } : { textAlign: align })),
  }));
  // Re-review R2: the width/height split leaves these Button combinations in no other document.
  const sizedButtons = [
    { what: 'Button, auto width, custom height, centred, top level', build: single(button('Tall', { customHeight: 56 })) },
    { what: 'Button, pill, custom width and height, bordered, centred, top level', build: single(button('Boxed', { buttonStyle: 'pill', customWidth: 180, customHeight: 52, borderSize: 2, borderColor: '#FFFFFF' })) },
  ];
  const b1 = [...cells('top'), ...cells('container'), ...heightOnly, ...plainButtons, ...sizedButtons];
  record(SHEETS[0], buildDoc(common, b1, 1), b1);
  // Re-review R3: the plain Button (auto width, no border) in its other two shapes at -, left, right.
  const plainShapes = ['pill', 'rectangle'].flatMap((shape) =>
    ['-', 'left', 'right'].map((align) => ({
      what: `Button, plain ${shape} (auto width, no border), ${alignLabel(align)}, top level`,
      build: single(button('Go', { buttonStyle: shape }, align === '-' ? { textAlign: undefined } : { textAlign: align })),
    })),
  );
  const b2 = [...cells('col2'), ...b2Extras(), ...plainShapes];
  record(SHEETS[1], buildDoc(common, b2, 2), b2);
  // SA12 step 1 (the parent's §3.6 `left` rule, B4 only): explicit `left` beyond top level that
  // does not fit B4 is dropped and the gap recorded. B3's grid is never dropped (it throws).
  const dropLeft = (specs, sheetNo) => {
    if (fits(buildDoc(common, specs, sheetNo)) || !leftEverywhere) return { specs, dropped: [] };
    const left = specs.filter((s) => s.cell && s.cell.align === 'left');
    manifest.gaps.push(`explicit left does not fit B${sheetNo} (${left.length} rows: ${[...new Set(left.map((s) => s.cell.ctx))].join(', ')}): those cells are not in the bible`);
    return { specs: specs.filter((s) => !left.includes(s)), dropped: left.map((s) => s.what) };
  };

  // B4 (SA12 steps 1 and 2). Its rows' cycled values are pinned before anything moves, so a moved
  // row is the same row on its new sheet (only its Containers' padding follows its new index, SA4).
  const b4Left = dropLeft([...cells('containerInColumn'), ...cells('columnRowInContainer')], 4);
  let b4 = pinCycles(b4Left.specs);
  const b4Dropped = b4Left.dropped;
  const movedOut = [];
  while (!fits(buildDoc(common, b4, 4))) {
    const headings = b4.filter((s) => s.type === 'Heading');
    if (!headings.length) break; // record() throws: B4 holds nothing more that may move
    const last = headings[headings.length - 1];
    movedOut.unshift(last);
    b4 = b4.filter((s) => s !== last);
  }
  const movedRow = (spec, to) => ({ ...spec, what: `${spec.what} (moved from B4)`, movedTo: to });

  // B3: the whole three-column grid, then the moved Heading rows (document order) while B3 fits.
  const b3 = pinCycles(cells('col3'));
  const toB7 = [];
  for (const spec of movedOut) {
    const row = movedRow(spec, 3);
    if (!toB7.length && fits(buildDoc(common, [...b3, row], 3))) b3.push(row);
    else toB7.push(movedRow(spec, 7));
  }
  record(SHEETS[2], buildDoc(common, b3, 3), b3);
  record(SHEETS[3], buildDoc(common, b4, 4), b4, b4Dropped);
  manifest.moved = movedOut.map((spec, k) => ({ what: spec.what, from: stemOf(SHEETS[3]), to: stemOf(k < movedOut.length - toB7.length ? SHEETS[2] : SHEETS[6]) }));

  // B5, B6.
  const b5 = b5Specs();
  record(SHEETS[4], buildDoc(common, b5, 5), b5);
  const b6 = b6Specs();
  record(SHEETS[5], buildDoc(outlookOff, b6, 6), b6);

  // B7 (SA11): (a) bordered rows, (b) the fallback Buttons, (c) the SA5 evidence rows, (d) the moved
  // rows B3 had no room for, then (e) the census compositions not present on B1-B4 (B5 and B6 do not
  // count) or earlier on B7,
  // most common first. Over a limit, census rows go least common first (SA12 step 3).
  const b7Lead = [...borderedRows(), ...fallbackRows()];
  const b7Head = [...b7Lead, ...evidenceRows(b7Lead.length), ...toB7];
  const present = new Set([...['bible-01-flat-alignment', 'bible-02-two-columns', 'bible-03-three-columns', 'bible-04-nesting'].map((st) => out[st]), buildDoc(common, b7Head, 7)].flatMap(compositionsOf));
  const candidates = [];
  for (const c of CENSUS.compositions) {
    if (present.has(c.value)) continue;
    // A kept row also brings its nested containers' compositions: a later census entry equal to
    // one of them is already present.
    compositionsOf(compositionRow('probe', c.value, 0).blocks).forEach((x) => present.add(x));
    // The label spells the composition with spaces, so it wraps like any text (and reads clean).
    const spelled = c.value.replace(/\[/g, ' ( ').replace(/\]/g, ' ) ').replace(/;/g, ' ; ').replace(/\+/g, ' + ').replace(/\s+/g, ' ').trim();
    candidates.push({ what: `census composition, ${c.items} items: ${spelled}`, composition: c.value, items: c.items, build: (p, i) => compositionRow(p, c.value, i) });
  }
  // Drop least common first = keep the longest most-common-first prefix that fits.
  let keep = candidates.length;
  while (keep > 0 && !fits(buildDoc(common, [...b7Head, ...candidates.slice(0, keep)], 7))) keep -= 1;
  const b7 = [...b7Head, ...candidates.slice(0, keep)];
  record(SHEETS[6], buildDoc(common, b7, 7), b7, candidates.slice(keep).map((c) => c.what));
  manifest.sheets.sort((a, b) => (a.stem < b.stem ? -1 : 1));
  if (!leftEverywhere) manifest.gaps.push('the census found no explicit `left`: it is a cell at top level only');
  return { documents: out, manifest };
}

const fileOf = (stem) => path.join(CANARY_DIR, `${stem}.json`);
const serialize = (v) => `${JSON.stringify(v, null, 2)}\n`;

// The files a generation writes: { absolute path: contents }.
function bibleFiles(EB) {
  const { documents, manifest } = buildBible(EB);
  const files = {};
  for (const stem of Object.keys(documents).sort()) files[fileOf(stem)] = serialize(documents[stem]);
  files[MANIFEST] = serialize(manifest);
  return files;
}

function arg(name, fallback) {
  const i = process.argv.indexOf(name);
  return i >= 0 && process.argv[i + 1] ? path.resolve(process.argv[i + 1]) : fallback;
}

if (require.main === module) {
  const { loadUmd, DEFAULT_UMD } = require('./_umd.cjs');
  const { dom, EB } = loadUmd(arg('--umd', DEFAULT_UMD));
  try {
    const files = bibleFiles(EB);
    const check = process.argv.includes('--check');
    let differs = 0;
    for (const [file, contents] of Object.entries(files)) {
      const now = fs.existsSync(file) ? fs.readFileSync(file, 'utf8') : null;
      if (check) {
        if (now !== contents) {
          differs++;
          console.log(`DIFFERS  ${path.relative(__dirname, file)}`);
        }
      } else if (now !== contents) {
        fs.writeFileSync(file, contents);
        console.log(`wrote ${path.relative(__dirname, file)}`);
      }
    }
    const stale = fs.readdirSync(CANARY_DIR).filter((f) => /^bible-.*\.json$/.test(f) && !files[path.join(CANARY_DIR, f)]);
    for (const f of stale) {
      if (check) {
        differs++;
        console.log(`STALE    canary/${f}`);
      } else {
        fs.unlinkSync(path.join(CANARY_DIR, f));
        console.log(`removed canary/${f}`);
      }
    }
    for (const s of JSON.parse(files[MANIFEST]).sheets) {
      const bytes = compiledBytes(EB, JSON.parse(files[fileOf(s.stem)]));
      console.log(`  ${s.stem.padEnd(30)} ${String(s.rows).padStart(2)} rows  ${String(bytes).padStart(6)} bytes${s.dropped.length ? `  dropped ${s.dropped.length}` : ''}`);
    }
    if (check && differs) process.exitCode = 1;
  } finally {
    dom.window.close();
  }
}

module.exports = { BIBLE_DIR, MANIFEST, LIMITS, TYPES, ALIGNS, SIZES, PADS, CONTAINER_PADS, CONTEXTS, GAP, NARROW, CANVAS, FALLBACK_LINES_LABEL, FALLBACK_CHARS_LABEL, buildBible, bibleFiles, compiledBytes, compositionsOf, containerParts, isBareContainer, parseComposition, runWidth, columnWidth, fileOf, serialize };
