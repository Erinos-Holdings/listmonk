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
//   B3 three columns + real mixes the four types x every alignment in all three columns; then the
//                                census's most common compositions not already present, most common first
//   B4 nesting                   the four types x every alignment as a Container in each of two
//                                columns, and as a two-column row inside a Container
//   B5 colour bands, other blocks ink x ground pairings, Button label x fill, rows either side of both band
//                                thresholds, two hues per band, image assets on light and dark grounds,
//                                Button shape x full/inline (+ custom width, bordered), Divider, Spacer,
//                                Avatar, Image unsized, an Html block, OfficialFooter brand and corporate
//   B6 layout variants           under Outlook off: the key rows again (§3.10 "one factor at a time")
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
// Limits (asserted by bible.test.cjs): at most 6 sheets, 40 rows a sheet, and 80,000 compiled bytes
// a sheet. Over a limit: B3's census compositions are dropped least common first, then B4's Heading
// rows, and the drop is recorded in manifest.json. Bands, the other blocks and B6 are never dropped.
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

const LIMITS = { sheets: 6, rows: 40, bytes: 80000 };
const TYPES = ['Text', 'Heading', 'Button', 'Image'];
const ALIGNS = ['-', 'left', 'center', 'right'];
const SIZES = [11, 14, 16, 20, 28, 40];
const PADS = [0, 8, 24, 48];
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

const container = (childrenIds, style = {}) => ({ type: 'Container', data: { style: { padding: { top: 0, bottom: 0, left: 0, right: 0 }, ...style }, props: { childrenIds } } });
const columns = (cols, props = {}) => ({
  type: 'ColumnsContainer',
  data: {
    style: { padding: { top: 8, bottom: 8, left: 0, right: 0 } },
    props: { columnsCount: cols.length === 3 ? 3 : 2, columnsGap: GAP, contentAlignment: 'middle', ...props, columns: [0, 1, 2].map((k) => ({ childrenIds: cols[k] || [] })) },
  },
});

// A cell row: `type` x `align` placed in `ctx`, with `values` (a row index, or {size, pad, level}). Returns { id, blocks }.
function cellRow(prefix, type, align, ctx, values) {
  const blocks = {};
  const leaf = (suffix, avail) => {
    const id = `${prefix}-${suffix}`;
    blocks[id] = contentBlock(type, align, values, avail, type === 'Image' ? 'width' : undefined);
    return id;
  };
  const half = columnWidth(NARROW, 2);
  switch (ctx) {
    case 'top':
      return { id: leaf('x', NARROW), blocks };
    case 'container':
      blocks[prefix] = container([leaf('x', NARROW)]);
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
      blocks[`${prefix}-k0`] = container([leaf('c0', half)]);
      blocks[`${prefix}-k1`] = container([leaf('c1', half)]);
      blocks[prefix] = columns([[`${prefix}-k0`], [`${prefix}-k1`]]);
      return { id: prefix, blocks };
    case 'columnRowInContainer':
      blocks[`${prefix}-cols`] = columns([[leaf('c0', half)], [leaf('c1', half)]]);
      blocks[prefix] = container([`${prefix}-cols`]);
      return { id: prefix, blocks };
    default:
      throw new Error(`unknown context ${ctx}`);
  }
}

const cellSpecs = (ctx, types = TYPES) =>
  types.flatMap((type) => ALIGNS.map((align) => ({ what: `${type === 'Image' ? 'Image sized by width' : type}, ${alignLabel(align)}, ${CONTEXT_LABEL[ctx]}`, type, cell: { type, align, ctx }, build: (p, i, values) => cellRow(p, type, align, ctx, values) })));

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
function compositionRow(prefix, comp) {
  const blocks = {};
  let k = 0;
  const fixed = { size: 16, pad: 24, level: 'h2' };
  const build = (n, avail) => {
    const id = `${prefix}-${k++}`;
    if (n.kind === 'Container') {
      blocks[id] = container(n.children.map((c) => build(c, avail)));
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

const ALIGNED = new Set(['Text', 'Heading', 'Button', 'Image', 'Avatar', 'Html']);
function compositionsOf(doc) {
  const tok = (id, seen) => {
    const b = doc[id] || {};
    const p = (b.data && b.data.props) || {};
    const s = (b.data && b.data.style) || {};
    if ((b.type === 'Container' || b.type === 'ColumnsContainer') && !seen.has(id)) return comp(id, new Set(seen).add(id));
    const base = b.type === 'Image' ? `Image:${typeof p.width === 'number' ? 'width' : typeof p.height === 'number' ? 'height-only' : 'unsized'}` : b.type;
    return ALIGNED.has(b.type) ? `${base}@${typeof s.textAlign === 'string' && s.textAlign ? s.textAlign : '-'}` : base;
  };
  const comp = (id, seen) => {
    const p = (doc[id].data && doc[id].data.props) || {};
    if (doc[id].type === 'ColumnsContainer') return `Columns[${(p.columns || []).map((c) => (c.childrenIds || []).map((x) => tok(x, seen)).join('+') || '-').join(';')}]`;
    const t = (p.childrenIds || []).map((x) => tok(x, seen)).filter((x, i, all) => i === 0 || x !== all[i - 1]);
    return `Container[${t.join('+') || '-'}]`;
  };
  return Object.keys(doc)
    .filter((id) => doc[id] && (doc[id].type === 'Container' || doc[id].type === 'ColumnsContainer'))
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
    ...[['Text', 'center'], ['Heading', '-'], ['Button', 'center'], ['Image', '-']].map(([type, align]) => ({ what: `${type}, ${alignLabel(align)}, top level, Outlook off`, build: (p, i) => cellRow(p, type, align, 'top', i) })),
    { what: 'a Container, Outlook off', build: (p, i) => cellRow(p, 'Text', '-', 'container', i) },
    { what: 'two columns, Outlook off', build: (p, i) => columnsRow(p, i, [['Image', '-'], ['Text', 'center']], {}) },
    { what: 'three columns, Outlook off', build: (p, i) => cellRow(p, 'Text', 'center', 'col3', i) },
    ...['rectangle', 'rounded', 'pill'].map((shape) => ({ what: `Button, ${shape}, Outlook off`, build: single(button(shape, { buttonStyle: shape })) })),
  ];
}

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
    const { id, blocks } = spec.build(prefix, i, cycles[i] || i);
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
  // B4 before B3, so B3's census rows skip what B4 already holds. Over a limit, first the §3.6
  // `left` fallback (explicit left beyond top level does not fit: those rows go, the gap is
  // recorded), then B4's Heading rows.
  let b4 = [...cells('containerInColumn'), ...cells('columnRowInContainer')];
  let b4Dropped = [];
  if (!fits(buildDoc(common, b4, 4)) && leftEverywhere) {
    const left = b4.filter((s) => s.cell.align === 'left');
    b4 = b4.filter((s) => s.cell.align !== 'left');
    b4Dropped = left.map((s) => s.what);
    manifest.gaps.push(`explicit left does not fit B4 (${left.length} rows: ${[...new Set(left.map((s) => s.cell.ctx))].join(', ')}): those cells are not in the bible`);
  }
  if (!fits(buildDoc(common, b4, 4))) {
    b4Dropped = [...b4Dropped, ...b4.filter((s) => s.type === 'Heading').map((s) => s.what)];
    b4 = b4.filter((s) => s.type !== 'Heading');
  }
  const b5 = b5Specs();
  const b6 = b6Specs();
  const b4Doc = buildDoc(common, b4, 4);
  const b5Doc = buildDoc(common, b5, 5);
  const b6Doc = buildDoc(outlookOff, b6, 6);

  // B3: the three-column cells, then the census compositions not already present, most common first.
  const b3Cells = cells('col3');
  const present = new Set([...Object.values(out), b4Doc, b5Doc, b6Doc, buildDoc(common, b3Cells, 3)].flatMap(compositionsOf));
  const candidates = [];
  for (const c of CENSUS.compositions) {
    if (present.has(c.value)) continue;
    // A kept row also brings its nested containers' compositions: a later census entry equal to
    // one of them is already present.
    compositionsOf(compositionRow('probe', c.value).blocks).forEach((x) => present.add(x));
    // The label spells the composition with spaces, so it wraps like any text (and reads clean).
    const spelled = c.value.replace(/\[/g, ' ( ').replace(/\]/g, ' ) ').replace(/;/g, ' ; ').replace(/\+/g, ' + ').replace(/\s+/g, ' ').trim();
    candidates.push({ what: `census composition, ${c.items} items: ${spelled}`, composition: c.value, items: c.items, build: (p) => compositionRow(p, c.value) });
  }
  // Drop least common first = keep the longest most-common-first prefix that fits.
  let keep = candidates.length;
  while (keep > 0 && !fits(buildDoc(common, [...b3Cells, ...candidates.slice(0, keep)], 3))) keep -= 1;
  const b3 = [...b3Cells, ...candidates.slice(0, keep)];
  record(SHEETS[2], buildDoc(common, b3, 3), b3, candidates.slice(keep).map((c) => c.what));
  record(SHEETS[3], b4Doc, b4, b4Dropped);
  record(SHEETS[4], b5Doc, b5);
  record(SHEETS[5], b6Doc, b6);
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

module.exports = { BIBLE_DIR, MANIFEST, LIMITS, TYPES, ALIGNS, SIZES, PADS, CONTEXTS, GAP, NARROW, CANVAS, buildBible, bibleFiles, compiledBytes, compositionsOf, parseComposition, runWidth, columnWidth, fileOf, serialize };
