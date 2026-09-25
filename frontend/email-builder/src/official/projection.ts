// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D5. The ONE projection implementation: a
// content fingerprint of a builder document (or one block of it) that ignores everything a
// re-save or a cosmetic edit may change, and a hash of it.
//
// Every consumer -- the marker the reader emits, the seed generator, the repair script, the
// Release 2 review -- computes it through the builder bundle (`EmailBuilder.officialProjection`),
// never from a second implementation.
//
// What it keeps, per block in document order (block ids are NEVER part of it):
//  - the block type (and the nesting, as `<Type>` ... `</Type>` around children);
//  - text, whitespace collapsed and HTML entities decoded: Text/Heading `text`, Button `text`,
//    Html `contents` (style="..." attributes stripped first, then tags and comments);
//  - every href, aria-label and img src in document order: Image url/linkHref/alt, Button url,
//    Avatar imageUrl, and links/images inside Text markdown or HTML and Html contents;
//  - style fontFamily, fontSize, fontWeight, textAlign when set.
// Colours, widths, heights, padding, lineColor, contentAlignment are excluded.
//
// The projection string is joined with "\n"; the hash is FNV-1a 64-bit over its UTF-8,
// 16 lower-case hex digits -- a change detector, not a credential.
//
// Import-free by construction: test/run.cjs compiles this file standalone.

type TBlock = { type?: string; data?: { style?: Record<string, unknown> | null; props?: Record<string, unknown> | null; childrenIds?: string[] | null } };
type TDocument = Record<string, TBlock | undefined>;

const NAMED_ENTITIES: Record<string, string> = {
  amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ', bull: '•', middot: '·',
  copy: '©', reg: '®', trade: '™', hellip: '…', mdash: '—', ndash: '–',
  rsquo: '’', lsquo: '‘', ldquo: '“', rdquo: '”', laquo: '«', raquo: '»',
  euro: '€', eacute: 'é', egrave: 'è', aacute: 'á', agrave: 'à', iacute: 'í',
  oacute: 'ó', uacute: 'ú', ntilde: 'ñ', ccedil: 'ç', uuml: 'ü', ouml: 'ö',
  auml: 'ä', szlig: 'ß', zwnj: '‌', zwj: '‍', ensp: ' ', emsp: ' ', thinsp: ' ',
};

export function decodeEntities(s: string): string {
  return s.replace(/&(#x[0-9a-fA-F]+|#[0-9]+|[A-Za-z][A-Za-z0-9]*);/g, (m, e: string) => {
    if (e[0] === '#') {
      const code = e[1] === 'x' || e[1] === 'X' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10);
      if (!Number.isFinite(code) || code < 0 || code > 0x10ffff) {
        return m;
      }
      return String.fromCodePoint(code);
    }
    const v = NAMED_ENTITIES[e.toLowerCase()];
    return v === undefined ? m : v;
  });
}

function collapse(s: string): string {
  return s.replace(/[\s ]+/g, ' ').trim();
}

function stripStyleAttributes(html: string): string {
  return html.replace(/\sstyle\s*=\s*("[^"]*"|'[^']*')/gi, '');
}

// Visible text of an HTML/markdown fragment: comments and tags removed, entities decoded,
// whitespace collapsed. A tag becomes a space so adjacent cells do not glue together.
function htmlText(html: string): string {
  const noComments = html.replace(/<!--[\s\S]*?-->/g, ' ');
  const noTags = noComments.replace(/<[^>]*>/g, ' ');
  return collapse(decodeEntities(noTags));
}

// Every href / aria-label / src attribute and every markdown link or image target, in the
// order it appears in the fragment.
function fragmentLinks(fragment: string, markdown: boolean): string[] {
  const found: { at: number; line: string }[] = [];
  const attr = /\b(href|aria-label|src)\s*=\s*("([^"]*)"|'([^']*)')/gi;
  let m: RegExpExecArray | null;
  while ((m = attr.exec(fragment)) !== null) {
    const name = m[1].toLowerCase();
    const value = decodeEntities(m[3] !== undefined ? m[3] : m[4] ?? '');
    found.push({ at: m.index, line: `${name}: ${value}` });
  }
  if (markdown) {
    const md = /(!?)\[([^\]]*)\]\(\s*([^)\s]+)(?:\s+"[^"]*")?\s*\)/g;
    while ((m = md.exec(fragment)) !== null) {
      if (m[1] === '!') {
        found.push({ at: m.index, line: `src: ${decodeEntities(m[3])}` });
        found.push({ at: m.index + 0.5, line: `alt: ${collapse(decodeEntities(m[2]))}` });
      } else {
        found.push({ at: m.index, line: `href: ${decodeEntities(m[3])}` });
      }
    }
  }
  found.sort((a, b) => a.at - b.at);
  return found.map((f) => f.line);
}

const STYLE_KEYS = ['fontFamily', 'fontSize', 'fontWeight', 'textAlign'];

function str(v: unknown): string | null {
  if (v === null || v === undefined) {
    return null;
  }
  return String(v);
}

function projectBlock(doc: TDocument, id: string, out: string[], seen: Set<string>): void {
  const block = doc[id];
  if (!block || typeof block !== 'object') {
    return;
  }
  // A cycle in childrenIds would otherwise recurse forever.
  if (seen.has(id)) {
    return;
  }
  seen.add(id);

  const type = String(block.type ?? '');
  const data = block.data ?? {};
  const props = (data.props ?? {}) as Record<string, unknown>;
  const style = (data.style ?? {}) as Record<string, unknown>;
  out.push(`<${type}>`);

  switch (type) {
    case 'Text':
    case 'Heading': {
      const raw = str(props.text) ?? '';
      out.push(`text: ${htmlText(raw)}`);
      out.push(...fragmentLinks(raw, type === 'Text' && Boolean(props.markdown)));
      break;
    }
    case 'Button': {
      out.push(`text: ${collapse(decodeEntities(str(props.text) ?? ''))}`);
      const url = str(props.url);
      if (url !== null) {
        out.push(`href: ${url}`);
      }
      break;
    }
    case 'Html': {
      const raw = stripStyleAttributes(str(props.contents) ?? '');
      out.push(`text: ${htmlText(raw)}`);
      out.push(...fragmentLinks(raw, false));
      break;
    }
    case 'Image': {
      const url = str(props.url);
      const href = str(props.linkHref);
      const alt = str(props.alt);
      if (url !== null) {
        out.push(`src: ${url}`);
      }
      if (href !== null) {
        out.push(`href: ${href}`);
      }
      if (alt !== null) {
        out.push(`alt: ${collapse(decodeEntities(alt))}`);
      }
      break;
    }
    case 'Avatar': {
      const url = str(props.imageUrl);
      if (url !== null) {
        out.push(`src: ${url}`);
      }
      const alt = str(props.alt);
      if (alt !== null) {
        out.push(`alt: ${collapse(decodeEntities(alt))}`);
      }
      break;
    }
    case 'OfficialFooter': {
      out.push(`kind: ${str(props.kind) ?? ''}`);
      break;
    }
    default:
      break;
  }

  for (const k of STYLE_KEYS) {
    const v = str(style[k]);
    if (v !== null && v !== '') {
      out.push(`${k}: ${v}`);
    }
  }

  const children: string[] = [];
  if (type === 'Container' && Array.isArray(props.childrenIds)) {
    children.push(...(props.childrenIds as string[]));
  } else if (type === 'ColumnsContainer' && Array.isArray(props.columns)) {
    (props.columns as { childrenIds?: string[] | null }[]).forEach((c, i) => {
      // A column boundary is content structure (which cell a block sits in).
      if (i > 0) {
        children.push('\u0000column');
      }
      children.push(...((c && c.childrenIds) || []));
    });
  } else if (type === 'EmailLayout' && Array.isArray(data.childrenIds)) {
    children.push(...data.childrenIds);
  }
  for (const c of children) {
    if (c === '\u0000column') {
      out.push('|column');
    } else {
      projectBlock(doc, c, out, seen);
    }
  }
  out.push(`</${type}>`);
}

// UTF-8 bytes of a JS string (surrogate pairs combined), without TextEncoder.
function utf8(s: string): number[] {
  const bytes: number[] = [];
  for (const ch of s) {
    const cp = ch.codePointAt(0) as number;
    if (cp < 0x80) {
      bytes.push(cp);
    } else if (cp < 0x800) {
      bytes.push(0xc0 | (cp >> 6), 0x80 | (cp & 0x3f));
    } else if (cp < 0x10000) {
      bytes.push(0xe0 | (cp >> 12), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
    } else {
      bytes.push(0xf0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3f), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
    }
  }
  return bytes;
}

const FNV_OFFSET = BigInt('0xcbf29ce484222325');
const FNV_PRIME = BigInt('0x100000001b3');
const MASK_64 = BigInt('0xffffffffffffffff');

export function fnv1a64(s: string): string {
  let h = FNV_OFFSET;
  for (const b of utf8(s)) {
    h ^= BigInt(b);
    h = (h * FNV_PRIME) & MASK_64;
  }
  return h.toString(16).padStart(16, '0');
}

// The projection of a whole document (its root's children, root props ignored) or, with
// `blockId`, of that one block and its descendants. Accepts the parsed document or its JSON.
export function officialProjection(
  document: unknown,
  blockId?: string | null
): { projection: string; hash: string } {
  let doc: TDocument = {};
  if (typeof document === 'string') {
    try {
      doc = JSON.parse(document) as TDocument;
    } catch (e) {
      doc = {};
    }
  } else if (document && typeof document === 'object') {
    doc = document as TDocument;
  }

  const out: string[] = [];
  const seen = new Set<string>();
  if (blockId) {
    projectBlock(doc, blockId, out, seen);
  } else {
    const root = doc.root;
    seen.add('root');
    const ids = (root && root.data && Array.isArray(root.data.childrenIds) ? root.data.childrenIds : []) as string[];
    for (const id of ids) {
      projectBlock(doc, id, out, seen);
    }
  }
  const projection = out.join('\n');
  return { projection, hash: fnv1a64(projection) };
}
