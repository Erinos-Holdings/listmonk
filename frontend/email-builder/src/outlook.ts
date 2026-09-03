type TStyleMap = Record<string, string>;
type TPaddingValues = {
  top: number;
  right: number;
  bottom: number;
  left: number;
};

const PRESENTATION_TABLE_STYLE = 'border-collapse:collapse;mso-table-lspace:0pt;mso-table-rspace:0pt;';

function appendMissingStyles(style: string | null, declarations: Array<[string, string]>) {
  const current = (style || '').trim();
  const lower = current.toLowerCase();
  const missing = declarations
    .filter(([property]) => !lower.includes(`${property.toLowerCase()}:`))
    .map(([property, value]) => `${property}:${value}`);

  if (missing.length === 0) {
    return current;
  }

  return [current.replace(/;+\s*$/, ''), ...missing]
    .filter(Boolean)
    .join(';');
}

function setStyleValues(style: string | null, declarations: Array<[string, string | null]>) {
  const styleMap = parseStyleMap(style);

  declarations.forEach(([property, value]) => {
    const key = property.toLowerCase();
    if (value === null || value === '') {
      delete styleMap[key];
      return;
    }

    styleMap[key] = value;
  });

  return Object.entries(styleMap)
    .map(([property, value]) => `${property}:${value}`)
    .join(';');
}

function parseStyleMap(style: string | null) {
  return (style || '')
    .split(';')
    .map((entry) => entry.trim())
    .filter(Boolean)
    .reduce<TStyleMap>((acc, entry) => {
      const separator = entry.indexOf(':');
      if (separator === -1) {
        return acc;
      }

      const property = entry.slice(0, separator).trim().toLowerCase();
      const value = entry.slice(separator + 1).trim();
      if (property) {
        acc[property] = value;
      }
      return acc;
    }, {});
}

function getPixelValue(value?: string) {
  if (!value) {
    return null;
  }

  const match = value.trim().match(/^(-?\d+(?:\.\d+)?)px$/i);
  if (!match) {
    return null;
  }

  return Math.round(Number(match[1]));
}

function getPixelWidthFromImage(img: HTMLImageElement) {
  const attrWidth = img.getAttribute('width');
  if (attrWidth && /^\d+$/.test(attrWidth)) {
    return attrWidth;
  }

  const style = img.getAttribute('style') || '';
  const widthMatch = style.match(/(?:^|;)\s*width\s*:\s*(\d+)px(?:;|$)/i);
  if (widthMatch) {
    return widthMatch[1];
  }

  const maxWidthMatch = style.match(/(?:^|;)\s*max-width\s*:\s*(\d+)px(?:;|$)/i);
  if (maxWidthMatch) {
    return maxWidthMatch[1];
  }

  return null;
}

// A resolvable pixel height: the `height` attribute or a px `style.height`.
// Word reads the attribute; CSS-honoring clients read the style.
function getPixelHeightFromImage(img: HTMLImageElement) {
  const attrHeight = img.getAttribute('height');
  if (attrHeight && /^\d+$/.test(attrHeight)) {
    return attrHeight;
  }

  const height = getPixelValue(parseStyleMap(img.getAttribute('style')).height);
  if (height && height > 0) {
    return String(height);
  }

  return null;
}

function getPaddingValues(styleMap: TStyleMap): TPaddingValues {
  const shorthand = styleMap.padding?.trim().split(/\s+/) || [];

  const [
    topFromShorthand,
    rightFromShorthand = topFromShorthand,
    bottomFromShorthand = topFromShorthand,
    leftFromShorthand = rightFromShorthand,
  ] = shorthand;

  return {
    top: getPixelValue(styleMap['padding-top'] || topFromShorthand) || 0,
    right: getPixelValue(styleMap['padding-right'] || rightFromShorthand) || 0,
    bottom: getPixelValue(styleMap['padding-bottom'] || bottomFromShorthand) || 0,
    left: getPixelValue(styleMap['padding-left'] || leftFromShorthand) || 0,
  };
}

function escapeHtml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function escapeAttribute(value: string) {
  return escapeHtml(value).replace(/"/g, '&quot;');
}

function createFragmentFromHtml(node: Element, html: string) {
  const range = node.ownerDocument.createRange();
  range.selectNode(node);
  return range.createContextualFragment(html);
}

function replaceNodeWithHtml(node: Element, html: string) {
  node.replaceWith(createFragmentFromHtml(node, html));
}

function escapeTemplateString(value: string) {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"');
}

function makeSafeTemplate(raw: string) {
  // Encode angle brackets so DOMParser does not consume Outlook conditional comments
  // before the Go template expression is evaluated. Encode spaces/tabs too: the
  // payload must be a single unbreakable token, because listmonk's format-switch
  // beautifier (Editor.vue convertContentType -> js-beautify) line-wraps long
  // text nodes at whitespace, and a raw newline inside a Go template string is
  // a parse error at campaign save ("unexpected ... in operand").
  // Encode & as well: the payload rides the fragment-parser round trip as a
  // text node, where entities DECODE — an &quot; (e.g. escapeAttribute'd font
  // names like "Noto Sans") becomes a raw quote that terminates the Go string
  // ("unexpected ... in operand" at save; hit live 2026-08-08 on the first
  // VML button with a quoted font stack). With & encoded, nothing in the
  // payload is an entity and the round trip is a no-op.
  const escaped = escapeTemplateString(raw)
    .replace(/&/g, '\\x26')
    .replace(/</g, '\\x3c')
    .replace(/>/g, '\\x3e')
    .replace(/ /g, '\\x20')
    .replace(/\t/g, '\\x09')
    .replace(/\n/g, '\\x0a')
    .replace(/\r/g, '\\x0d');

  return `{{ Safe "${escaped}" }}`;
}

function getWrapperOptions(style: string | null) {
  const styleValue = style || '';
  const styleMap = parseStyleMap(styleValue);
  const align = styleMap['text-align'] || 'left';
  const backgroundColor = styleMap['background-color'];
  const bgcolorAttr = backgroundColor ? ` bgcolor="${escapeAttribute(backgroundColor)}"` : '';

  return { styleValue, styleMap, align, bgcolorAttr };
}

function buildPresentationTable(contents: string, width: string = '100%', align: string | null = null) {
  const widthAttr = width && width !== 'auto' ? ` width="${escapeAttribute(width)}"` : '';
  const alignAttr = align ? ` align="${escapeAttribute(align)}"` : '';

  return `<table role="presentation"${widthAttr}${alignAttr} cellpadding="0" cellspacing="0" border="0" style="${PRESENTATION_TABLE_STYLE}">${contents}</table>`;
}

function hasSingleChildMatching(div: HTMLDivElement, predicate: (child: Element) => boolean) {
  const children = Array.from(div.children);
  return children.length === 1 && predicate(children[0]);
}

function addTableDefaults(doc: Document) {
  doc.querySelectorAll('table[role="presentation"]').forEach((table) => {
    if (!table.getAttribute('cellpadding')) {
      table.setAttribute('cellpadding', '0');
    }
    if (!table.getAttribute('cellspacing')) {
      table.setAttribute('cellspacing', '0');
    }
    if (!table.getAttribute('border')) {
      table.setAttribute('border', '0');
    }

    table.setAttribute(
      'style',
      appendMissingStyles(table.getAttribute('style'), [
        ['border-collapse', 'collapse'],
        ['mso-table-lspace', '0pt'],
        ['mso-table-rspace', '0pt'],
      ])
    );
  });
}

function isStandaloneImage(img: HTMLImageElement) {
  const parent = img.parentElement;
  if (!parent) {
    return false;
  }

  if (parent.tagName === 'DIV') {
    return hasSingleChildMatching(parent as HTMLDivElement, (child) => child.tagName === 'IMG');
  }

  if (parent.tagName === 'A' && parent.children.length === 1) {
    const grandparent = parent.parentElement;
    return grandparent?.tagName === 'DIV'
      && hasSingleChildMatching(grandparent as HTMLDivElement, (child) => child.tagName === 'A');
  }

  return false;
}

function hardenImages(doc: Document) {
  doc.querySelectorAll('img').forEach((img) => {
    img.setAttribute('border', '0');

    const width = getPixelWidthFromImage(img);
    if (width && !img.getAttribute('width')) {
      img.setAttribute('width', width);
    }

    const standaloneImage = isStandaloneImage(img);
    const declarations: Array<[string, string | null]> = [
      ['border', '0'],
      ['outline', 'none'],
      ['text-decoration', 'none'],
    ];

    // IMAGE-WIDTH-SPEC D3.4. `height:auto` keeps the aspect ratio of a
    // WIDTH-sized image (the clamp rewrites its width). On a height-only image
    // it deletes the only sizing there is — every CSS-honoring client then draws
    // the image at full width — so such an image keeps `height:<h>px`, gains
    // `width:auto`, and carries the height attribute for Word. An image with
    // neither dimension is left alone (the render warning names it).
    if (width) {
      declarations.push(['height', 'auto']);
    } else {
      const height = getPixelHeightFromImage(img);
      if (height) {
        if (!img.getAttribute('height')) {
          img.setAttribute('height', height);
        }
        declarations.push(['height', `${height}px`], ['width', 'auto']);
      }
    }
    declarations.push(['-ms-interpolation-mode', 'bicubic']);

    if (standaloneImage) {
      declarations.unshift(['display', 'block']);
      declarations.push(['vertical-align', null]);
    }

    img.setAttribute('style', setStyleValues(img.getAttribute('style'), declarations));

    const parent = img.parentElement;
    if (standaloneImage && parent?.tagName === 'A') {
      parent.setAttribute('style', setStyleValues(parent.getAttribute('style'), [
        ['display', 'inline-block'],
        ['border', '0'],
        ['text-decoration', 'none'],
      ]));
    }
  });
}

function transformImageBlocks(doc: Document) {
  const wrappers = Array.from(doc.querySelectorAll('div')).filter((div) => hasSingleChildMatching(div as HTMLDivElement, (child) => {
    if (child.tagName === 'IMG') {
      return true;
    }

    return child.tagName === 'A' && child.children.length === 1 && child.querySelector('img') !== null;
  })) as HTMLDivElement[];

  wrappers.forEach((div) => {
    const { styleValue, align, bgcolorAttr } = getWrapperOptions(div.getAttribute('style'));
    const content = div.innerHTML;

    // The inner table is shrink-wrap (no width). A parent td's align/text-align only
    // positions INLINE content; Gmail (web and both apps) and Outlook mobile lay a nested
    // table out as a block and leave it at the left edge — only Word centers it from the
    // parent cell. A centered/right image therefore needs the table to align ITSELF, the
    // same self-aligning idiom converted wrappers already recognize (campaign 48 footer
    // logo, 2026-09-03). `left` is the default flow; never stamp it.
    const innerTable = buildPresentationTable(
      `<tbody><tr><td align="${escapeAttribute(align)}">${content}</td></tr></tbody>`,
      'auto',
      align === 'center' || align === 'right' ? align : null
    );
    const html = buildPresentationTable(
      `<tbody><tr><td align="${escapeAttribute(align)}"${bgcolorAttr} style="${escapeAttribute(styleValue)}">${innerTable}</td></tr></tbody>`
    );

    replaceNodeWithHtml(div, html);
  });
}

const EDGE_MARGIN_TAGS = /^(P|H[1-6]|UL|OL|BLOCKQUOTE)$/;

// Client-default <p>/<h*> margins (≈1em of the governing font size) supply
// vertical spacing in every browser-engined client but die at Word's
// table-cell edges. Estimate what Word loses at a converted cell's top or
// bottom edge so the conversion can graft it back via mso-padding-alt —
// which Word reads in place of padding and every other client ignores as an
// unknown property. The value is an estimate of a client default, so Outlook
// lands within a few px of Gmail here, not byte-exact.
function estimateEdgeMarginPx(container: Element, side: 'first' | 'last', inheritedSize: number): number {
  const child = side === 'first' ? container.firstElementChild : container.lastElementChild;
  if (!child) {
    return 0;
  }

  const size = getPixelValue(parseStyleMap(child.getAttribute('style'))['font-size']) || inheritedSize;
  if (EDGE_MARGIN_TAGS.test(child.tagName)) {
    return size;
  }
  // An unconverted wrapper div (rhythm text block, structural zero-box div)
  // is transparent in flow — its own edge decides. User-authored Html content
  // is never guessed at.
  if (child.tagName === 'DIV' && !child.getAttribute('data-lm-user-html')) {
    return estimateEdgeMarginPx(child, side, size);
  }
  return 0;
}

function transformSimpleDivBlocks(doc: Document) {
  const wrappers = Array.from(doc.querySelectorAll('div')).filter((div) => {
    // Anything strictly inside an Html block's wrapper (marked by the reader,
    // see HtmlReader in documents/reader/core.tsx) is user-authored markup —
    // rewriting its divs into table cells drops div-only semantics (auto
    // margins, inline-block, floats) in EVERY client. The marked wrapper
    // itself carries builder-owned padding and stays convertible.
    if (div.parentElement?.closest('[data-lm-user-html]')) {
      return false;
    }

    const { styleMap } = getWrapperOptions(div.getAttribute('style'));

    if (!styleMap.padding && !styleMap.height) {
      return false;
    }

    if (styleMap['min-height'] && styleMap.width === '100%') {
      return false;
    }

    if (div.children.length > 0) {
      const firstChild = div.children[0];
      if (firstChild.tagName === 'A' || firstChild.tagName === 'IMG') {
        return false;
      }
      if (firstChild.tagName === 'TABLE' || firstChild.tagName === 'DIV') {
        // Every builder block arrives wrapped in a padded div, and Word drops
        // div padding and backgrounds — which silently strips the spacing the
        // author set in the editor (seen live on an Html icons block,
        // 2026-08-07). Convert block-wrapping divs (tables and nested block
        // wrappers alike) when they carry a real box, so zero-padding
        // structural wrappers stay divs.
        const padding = getPaddingValues(styleMap);
        return padding.top > 0 || padding.right > 0 || padding.bottom > 0 || padding.left > 0
          || Boolean(styleMap['background-color']);
      }
    }

    // Text-flow content (markdown paragraphs, headings) gets its vertical
    // rhythm from client-default <p>/<h*> margins — Word drops those margins
    // at table-cell edges, so converting a rhythm-only block destroys more
    // spacing than it preserves (campaign 28, 2026-08-10: inter-block gaps
    // vanished in Outlook desktop). Convert only when the div carries a box
    // Word would otherwise drop: vertical padding, a background, or a height.
    // Horizontal-only padding is the accepted loss — Word renders the text
    // uninset, which beats collapsing the rhythm.
    const padding = getPaddingValues(styleMap);
    return padding.top > 0 || padding.bottom > 0
      || Boolean(styleMap['background-color'])
      || Boolean(styleMap.height);
  }) as HTMLDivElement[];

  // Innermost-first: converting an ancestor re-parses its subtree, which
  // detaches every not-yet-converted descendant in this snapshot — an embedded
  // Container then shipped as a raw padded div and Word dropped its padding
  // and background (campaign 28, 2026-08-10). querySelectorAll is document
  // order (ancestors first), so the reverse converts children before their
  // ancestor snapshots them into innerHTML.
  [...wrappers].reverse().forEach((div) => {
    // Safety net only — with innermost-first ordering nothing here should be
    // detached; skip rather than corrupt the document if one ever is.
    if (!div.isConnected) {
      return;
    }
    const { styleValue, styleMap, bgcolorAttr } = getWrapperOptions(div.getAttribute('style'));
    const height = getPixelValue(styleMap.height);
    const isSpacer = div.children.length === 0 && (div.textContent || '').trim() === '' && height !== null;

    if (isSpacer) {
      const spacerHtml = buildPresentationTable(
        `<tbody><tr><td${bgcolorAttr} height="${height}" style="${escapeAttribute(styleValue)};line-height:${height}px;font-size:${height}px;">&nbsp;</td></tr></tbody>`
      );

      replaceNodeWithHtml(div, spacerHtml);
      return;
    }

    // Graft the edge margins Word drops back onto the cell, Word-only. Fenced
    // user content gets no guessing. Children already converted (button/image/
    // nested-container tables) resolve to 0 — no double counting.
    const ownSize = getPixelValue(styleMap['font-size']) || 16;
    const extraTop = div.getAttribute('data-lm-user-html') ? 0 : estimateEdgeMarginPx(div, 'first', ownSize);
    const extraBottom = div.getAttribute('data-lm-user-html') ? 0 : estimateEdgeMarginPx(div, 'last', ownSize);
    let tdStyle = styleValue;
    if (extraTop > 0 || extraBottom > 0) {
      const padding = getPaddingValues(styleMap);
      tdStyle = `${styleValue.replace(/;+\s*$/, '')};mso-padding-alt:${padding.top + extraTop}px ${padding.right}px ${padding.bottom + extraBottom}px ${padding.left}px`;
    }

    // Never fabricate alignment. An explicit text-align becomes the td
    // attribute; without one the attribute is omitted (left is every client's
    // default anyway) — a stamped align="left" overrode self-centering user
    // content in the Gmail app (campaign 10 social icons, 2026-08-11). When
    // the sole element child is a table that centers itself, encode that
    // intent as td align="center" — the idiom every client, Word included,
    // honors.
    let tdAlign: string | null = styleMap['text-align'] || null;
    if (!tdAlign && div.children.length === 1 && div.children[0].tagName === 'TABLE') {
      const childTable = div.children[0];
      const childStyleMap = parseStyleMap(childTable.getAttribute('style'));
      // Only a shrink-wrap table can visually center — builder ColumnsContainer
      // tables carry align="center" as boilerplate while being width:100%.
      const fullWidth = childTable.getAttribute('width') === '100%' || childStyleMap.width === '100%';
      const selfCentering = (childTable.getAttribute('align') || '').toLowerCase() === 'center'
        || /\bauto\b/i.test(childStyleMap.margin || '')
        || (/^auto$/i.test(childStyleMap['margin-left'] || '') && /^auto$/i.test(childStyleMap['margin-right'] || ''));
      if (!fullWidth && selfCentering) {
        tdAlign = 'center';
      }
    }
    const alignAttr = tdAlign ? ` align="${escapeAttribute(tdAlign)}"` : '';

    const blockHtml = buildPresentationTable(
      `<tbody><tr><td${alignAttr}${bgcolorAttr} style="${escapeAttribute(tdStyle)}">${div.innerHTML}</td></tr></tbody>`
    );

    replaceNodeWithHtml(div, blockHtml);
  });
}

// The Button block emits `border: <n>px solid <color>`, but user-authored
// markup may use any token order (`solid 5px red`), other line styles, or
// `!important` — parse tokens independently so a valid border never silently
// loses its stroke in the VML copy. Non-solid styles draw solid (VML has no
// 1:1 mapping; a visible stroke beats a missing one).
function parseButtonBorder(styleMap: TStyleMap): { color: string | null; width: number } {
  const raw = (styleMap.border || '').replace(/!important/gi, '').trim();
  if (!raw) {
    return { color: null, width: 0 };
  }

  const tokens = raw.split(/\s+/);
  const styleKeywords = /^(solid|dashed|dotted|double|groove|ridge|inset|outset|none|hidden)$/i;
  let width = 0;
  const colorTokens: string[] = [];
  tokens.forEach((token) => {
    const px = getPixelValue(token);
    if (px !== null) {
      width = px;
      return;
    }
    if (!styleKeywords.test(token)) {
      colorTokens.push(token);
    }
  });

  const color = colorTokens.join(' ') || null;
  if (!color || width <= 0 || /^(none|hidden)$/i.test(tokens.find((t) => styleKeywords.test(t)) || '')) {
    return { color: null, width: 0 };
  }

  return { color, width };
}

function estimateTextWidth(text: string, fontSize: number, fontWeight: string) {
  return Math.max(1, Math.round(text.length * fontSize * (fontWeight.toLowerCase() === 'bold' ? 0.68 : 0.62)));
}

type TVmlButtonOptions = {
  href: string;
  text: string;
  buttonColor: string;
  textColor: string;
  fontSize: number;
  fontWeight: string;
  fontFamily: string;
  borderColor: string | null;
  borderWidth: number;
  borderRadius: number;
  width: number;
  height: number;
};

// Canonical bulletproof VML button — the WHOLE shape is the link (href on the
// roundrect + anchorlock), unlike a td-based button where Word makes only the
// text run clickable (seen live 2026-08-07). Vertical centering is
// v-text-anchor:middle alone: a v:textbox with an exact line-height rides the
// text HIGH in real Word; the canonical form centers correctly at heights >=
// the 2x-font floor the callers apply, which makes tight shapes impossible.
// All dimensions are emitted in POINTS, not pixels — Word scales pt with the
// Windows display-scaling factor exactly like text, while px shapes stay
// fixed and clip or hide their own label at 125/150 % scaling
// (matrix-verified 2026-08-07). Width/height are border-box px.
function buildVmlButton(options: TVmlButtonOptions) {
  const pt = (px: number) => String(Math.round(px * 0.75 * 100) / 100);
  const arcsize = Math.max(0, Math.min(50, Math.round((options.borderRadius / options.height) * 100)));
  const strokeAttrs = options.borderColor
    ? `strokecolor="${escapeAttribute(options.borderColor)}" strokeweight="${pt(options.borderWidth)}pt"`
    : `strokecolor="${escapeAttribute(options.buttonColor)}"`;

  return `<v:roundrect xmlns:v="urn:schemas-microsoft-com:vml" xmlns:w="urn:schemas-microsoft-com:office:word" href="${escapeAttribute(options.href)}" style="height:${pt(options.height)}pt;v-text-anchor:middle;width:${pt(options.width)}pt;" arcsize="${arcsize}%" ${strokeAttrs} fillcolor="${escapeAttribute(options.buttonColor)}"><w:anchorlock/><center style="color:${escapeAttribute(options.textColor)};font-family:${escapeAttribute(options.fontFamily)};font-size:${pt(options.fontSize)}pt;font-weight:${escapeAttribute(options.fontWeight)};">${escapeHtml(options.text)}</center></v:roundrect>`;
}

function buildBulletproofButton(anchor: HTMLAnchorElement, wrapperStyle: string) {
  const anchorStyleMap = parseStyleMap(anchor.getAttribute('style'));
  const wrapperStyleMap = parseStyleMap(wrapperStyle);
  const text = anchor.textContent?.replace(/\s+/g, ' ').trim() || '';
  const href = anchor.getAttribute('href') || '#';
  const target = anchor.getAttribute('target');
  const align = wrapperStyleMap['text-align'] || 'left';
  const buttonColor = anchorStyleMap['background-color'] || '#0055d4';
  const textColor = anchorStyleMap.color || '#ffffff';
  const fontSize = getPixelValue(anchorStyleMap['font-size']) || 16;
  const fontWeight = anchorStyleMap['font-weight'] || 'bold';
  const fontFamily = anchorStyleMap['font-family'] || 'Arial, sans-serif';
  const borderRadius = getPixelValue(anchorStyleMap['border-radius']) || 0;
  const { color: borderColor, width: borderWidth } = parseButtonBorder(anchorStyleMap);
  const paddingValues = getPaddingValues(anchorStyleMap);
  const lineHeightPx = getPixelValue(anchorStyleMap['line-height']);
  const lineHeight = lineHeightPx ?? Math.round(fontSize * 1.2);
  const display = (anchorStyleMap.display || '').toLowerCase();
  const fullWidth = display === 'block' || anchorStyleMap.width === '100%';

  const targetAttr = target ? ` target="${escapeAttribute(target)}"` : '';

  if (fullWidth) {
    // A real border from the Button block wins; the hairline in the button
    // colour is only a fallback that keeps Outlook from drawing a white seam.
    const anchorStyle = setStyleValues(anchor.getAttribute('style'), [
      ['display', 'block'],
      ['text-align', 'center'],
      ['border', anchorStyleMap.border || '1px solid ' + buttonColor],
    ]);

    return [
      buildPresentationTable(
        `<tbody><tr><td align="${escapeAttribute(align)}" style="${escapeAttribute(wrapperStyle)}">${buildPresentationTable(
          `<tbody><tr><td data-lm-full-width-button="true" bgcolor="${escapeAttribute(buttonColor)}" style="background-color:${escapeAttribute(buttonColor)};border-radius:${borderRadius}px;"><a href="${escapeAttribute(href)}"${targetAttr} style="${escapeAttribute(anchorStyle)}">${escapeHtml(text)}</a></td></tr></tbody>`
        )}</td></tr></tbody>`
      ),
    ].join('');
  }

  // The Button block's "Custom button width/height" sliders emit real px on the
  // anchor. Where they are set, VML gets the actual box instead of the guess
  // below — which is the difference between Outlook matching every other client
  // and merely approximating it.
  const explicitWidth = getPixelValue(anchorStyleMap.width);
  const explicitHeight = getPixelValue(anchorStyleMap.height);
  const estimatedTextWidth = estimateTextWidth(text, fontSize, fontWeight);
  // An auto-sized CSS button grows by the border on every edge, while a VML
  // strokeweight straddles the shape edge — half in, half out — so adding one
  // border width per axis is what matches the drawn outer size. Explicit boxes
  // are border-box in CSS and need no adjustment.
  const estimatedWidth = explicitWidth ?? Math.max(40, estimatedTextWidth + paddingValues.left + paddingValues.right) + borderWidth;
  // The 32px floor guards the text-length guess when no line-height is
  // available; a real line-height + padding is the actual CSS box and must not
  // be inflated, or short custom-height buttons render taller in Outlook.
  const measuredHeight = lineHeight + paddingValues.top + paddingValues.bottom;
  // Word cannot render a shape as tight as CSS renders a cramped line-height:
  // browsers let glyphs overflow the line box, Word clips them to the shape.
  // Matrix-verified 2026-08-07 (150 % display scaling): a 26px-equivalent
  // shape clips a 16px label even in pt, 32px renders clean — so the shape
  // never goes below 2× the font size. Outlook buttons render slightly taller
  // than a deliberately-cramped CSS design; that is the achievable fidelity.
  // Calibrated at 16px font (the only matrix-tested size); for much larger
  // fonts 2× over-floors and ~1.4×font + slack is the likely true bound —
  // re-run the matrix (campaign 22 method) before loosening.
  const wordMinHeight = fontSize * 2;
  const cssHeight = lineHeightPx !== null ? measuredHeight : Math.max(measuredHeight, 32);
  // The floor applies to explicit heights too (border-box, so no border
  // adjustment): a sub-floor shape renders NO label at all in Word, which is
  // strictly worse than a taller-than-specified button.
  const estimatedHeight = explicitHeight !== null
    ? Math.max(explicitHeight, wordMinHeight)
    : Math.max(cssHeight, wordMinHeight) + borderWidth;
  const cleanAnchorStyle = anchor.getAttribute('style') || '';
  const vml = buildVmlButton({
    href,
    text,
    buttonColor,
    textColor,
    fontSize,
    fontWeight,
    fontFamily,
    borderColor,
    borderWidth,
    borderRadius,
    width: estimatedWidth,
    height: estimatedHeight,
  });
  // The VML must ride inside the Safe template WITH its conditional markers:
  // emitted raw, the fragment parser rewrites <w:anchorlock/> into an OPEN tag
  // that swallows <center> (self-closing syntax is ignored on unknown
  // elements), and Word then drops the vertical text anchoring — the label
  // renders clipped. Upstream PR #2978 has this latent.
  const msoBlock = makeSafeTemplate(`<!--[if mso]>${vml}<![endif]-->`);
  const nonMsoStart = makeSafeTemplate('<!--[if !mso]><!-->');
  const nonMsoEnd = makeSafeTemplate('<!--<![endif]-->');

  return buildPresentationTable(
    `<tbody><tr><td align="${escapeAttribute(align)}" style="${escapeAttribute(wrapperStyle)}">${msoBlock}${nonMsoStart}<a href="${escapeAttribute(href)}"${targetAttr} style="${escapeAttribute(cleanAnchorStyle)}">${escapeHtml(text)}</a>${nonMsoEnd}</td></tr></tbody>`
  );
}

function transformButtonBlocks(doc: Document) {
  const wrappers = Array.from(doc.querySelectorAll('div')).filter((div) => hasSingleChildMatching(div as HTMLDivElement, (child) => {
    if (child.tagName !== 'A' || child.querySelector('img')) {
      return false;
    }

    const styleMap = parseStyleMap((child as HTMLAnchorElement).getAttribute('style'));
    return Boolean(styleMap['background-color'] && styleMap.padding);
  })) as HTMLDivElement[];

  wrappers.forEach((div) => {
    const anchor = div.children[0] as HTMLAnchorElement;
    replaceNodeWithHtml(div, buildBulletproofButton(anchor, div.getAttribute('style') || ''));
  });
}

function getBorderWidths(styleMap: TStyleMap): Pick<TPaddingValues, 'top' | 'right' | 'bottom' | 'left'> {
  const shorthand = styleMap.border?.trim() || '';
  const shorthandMatch = shorthand.match(/^(-?\d+(?:\.\d+)?)px\b/i);
  const fromShorthand = shorthandMatch ? Math.round(Number(shorthandMatch[1])) : 0;
  const all = getPixelValue(styleMap['border-width']) ?? fromShorthand;

  return {
    top: getPixelValue(styleMap['border-top-width']) ?? all,
    right: getPixelValue(styleMap['border-right-width']) ?? all,
    bottom: getPixelValue(styleMap['border-bottom-width']) ?? all,
    left: getPixelValue(styleMap['border-left-width']) ?? all,
  };
}

function getHorizontalInset(styleMap: TStyleMap) {
  const padding = getPaddingValues(styleMap);
  const border = getBorderWidths(styleMap);
  return padding.left + padding.right + border.left + border.right;
}

function formatPaddingShorthand(padding: TPaddingValues) {
  return `${padding.top}px ${padding.right}px ${padding.bottom}px ${padding.left}px`;
}

function clearPaddingStyles(style: string | null) {
  return setStyleValues(style, [
    ['padding', null],
    ['padding-top', null],
    ['padding-right', null],
    ['padding-bottom', null],
    ['padding-left', null],
  ]);
}

function getCellColspan(cell: Element) {
  const raw = cell.getAttribute('colspan');
  if (!raw || !/^\d+$/.test(raw)) {
    return 1;
  }

  return Math.max(1, Number(raw));
}

function stripInlineEventHandlers(element: Element) {
  Array.from(element.attributes).forEach((attr) => {
    if (/^on/i.test(attr.name)) {
      element.removeAttribute(attr.name);
    }
  });

  Array.from(element.children).forEach((child) => stripInlineEventHandlers(child));
}

function findCanvasTable(doc: Document): { table: HTMLTableElement; width: number } | null {
  // Prefer the EmailLayout shape: body > backdrop div > canvas table.
  // Avoids mistaking an earlier Html-block table that happens to use width=100% + max-width.
  const candidates: HTMLTableElement[] = [];

  const body = doc.body;
  if (body) {
    Array.from(body.children).forEach((child) => {
      if (child.tagName !== 'DIV') {
        return;
      }

      Array.from(child.children).forEach((grandChild) => {
        if (grandChild.tagName === 'TABLE') {
          candidates.push(grandChild as HTMLTableElement);
        }
      });
    });
  }

  const tables = candidates.length > 0
    ? candidates
    : Array.from(doc.querySelectorAll('table')) as HTMLTableElement[];

  for (const table of tables) {
    if (table.getAttribute('width') !== '100%') {
      continue;
    }

    const width = getPixelValue(parseStyleMap(table.getAttribute('style'))['max-width']);
    if (width !== null) {
      return { table, width };
    }
  }

  return null;
}

function constrainCanvasForOutlook(doc: Document) {
  const found = findCanvasTable(doc);
  if (!found) {
    return;
  }

  const { table: canvas, width: canvasWidth } = found;

  // Word ignores max-width, so without this the white canvas table spans the
  // full reading pane. Pin it to its real width with a conditional ghost table.
  const ghostStart = makeSafeTemplate(
    `<!--[if mso]><table role="presentation" align="center" width="${canvasWidth}" cellpadding="0" cellspacing="0" border="0" style="${PRESENTATION_TABLE_STYLE}"><tr><td><![endif]-->`
  );
  const ghostEnd = makeSafeTemplate('<!--[if mso]></td></tr></table><![endif]-->');

  const backdrop = canvas.parentElement;
  if (backdrop && backdrop.tagName === 'DIV' && backdrop.parentElement?.tagName === 'BODY') {
    // Word drops div padding and the 100%-wide canvas hides the backdrop
    // color entirely, so move both onto a real table + td around the canvas.
    // This wrapper is intentional for all clients (not mso-only): keep visual
    // parity with the original backdrop padding/background outside Outlook.
    const styleMap = parseStyleMap(backdrop.getAttribute('style'));
    const backgroundColor = styleMap['background-color'];
    const padding = getPaddingValues(styleMap);
    const hasPadding = padding.top !== 0 || padding.right !== 0 || padding.bottom !== 0 || padding.left !== 0;
    const bgcolorAttr = backgroundColor ? ` bgcolor="${escapeAttribute(backgroundColor)}"` : '';
    const tdStyle = [
      backgroundColor ? `background-color:${backgroundColor}` : '',
      hasPadding ? `padding:${formatPaddingShorthand(padding)}` : '',
    ].filter(Boolean).join(';');

    backdrop.setAttribute('style', clearPaddingStyles(backdrop.getAttribute('style')));

    replaceNodeWithHtml(canvas, buildPresentationTable(
      `<tbody><tr><td align="center"${bgcolorAttr} style="${escapeAttribute(tdStyle)}">${ghostStart}${canvas.outerHTML}${ghostEnd}</td></tr></tbody>`
    ));
    return;
  }

  replaceNodeWithHtml(canvas, `${ghostStart}${canvas.outerHTML}${ghostEnd}`);
}

function getDirectRows(table: Element) {
  const rows: Element[] = [];
  Array.from(table.children).forEach((section) => {
    if (section.tagName === 'TR') {
      rows.push(section);
    } else if (section.tagName === 'TBODY' || section.tagName === 'THEAD' || section.tagName === 'TFOOT') {
      rows.push(...Array.from(section.children).filter((row) => row.tagName === 'TR'));
    }
  });
  return rows;
}

function getSingleButtonCell(table: Element) {
  const rows = getDirectRows(table);
  if (rows.length !== 1) {
    return null;
  }

  const cells = Array.from(rows[0].children).filter((cell) => cell.tagName === 'TD');
  if (cells.length !== 1) {
    return null;
  }

  const td = cells[0];
  const children = Array.from(td.children);
  if (children.length !== 1 || children[0].tagName !== 'A' || children[0].querySelector('img')) {
    return null;
  }

  return { td, anchor: children[0] as HTMLAnchorElement };
}

function isFullWidthButtonTable(table: Element) {
  // The fullWidth branch of buildBulletproofButton: a 100%-wide presentation
  // table whose single td carries bgcolor and whose single child is a
  // display:block anchor styled as the button.
  if (table.getAttribute('width') !== '100%' || table.getAttribute('role') !== 'presentation') {
    return false;
  }

  const found = getSingleButtonCell(table);
  if (!found || found.td.getAttribute('data-lm-full-width-button') !== 'true') {
    return false;
  }

  const styleMap = parseStyleMap(found.anchor.getAttribute('style'));
  return (styleMap.display || '').toLowerCase() === 'block' && Boolean(styleMap['background-color']);
}

function transformFullWidthButtonForMso(table: Element, available: number) {
  const found = getSingleButtonCell(table);
  if (!found || available <= 0) {
    return;
  }

  const { td, anchor } = found;
  const anchorStyleMap = parseStyleMap(anchor.getAttribute('style'));
  const width = Math.floor(available);
  const buttonColor = td.getAttribute('bgcolor') || anchorStyleMap['background-color'] || '#0055d4';
  const href = anchor.getAttribute('href') || '#';
  const text = anchor.textContent?.replace(/\s+/g, ' ').trim() || '';

  // The mso copy is a VML shape so the WHOLE surface is clickable — a
  // td-based button leaves only the text run as the link in Word (seen live
  // 2026-08-07). Height mirrors the CSS box (line-height + padding + border)
  // under the same 2x-font floor as the custom-size branch; width is the
  // column budget, border-box.
  const fontSize = getPixelValue(anchorStyleMap['font-size']) || 16;
  const fontWeight = anchorStyleMap['font-weight'] || 'bold';
  const { color: borderColor, width: borderWidth } = parseButtonBorder(anchorStyleMap);
  const padding = getPaddingValues(anchorStyleMap);
  const lineHeightPx = getPixelValue(anchorStyleMap['line-height']);
  const lineHeight = lineHeightPx ?? Math.round(fontSize * 1.2);
  // A full-width CSS button WRAPS long labels (no nowrap on this path) while
  // a VML shape clips what does not fit — estimate the wrapped line count at
  // the column width with the same calibrated heuristic the custom-size
  // branch uses. Overestimating degrades safely (a slightly taller button,
  // text still centered); underestimating hides text in Word.
  const contentWidth = Math.max(1, width - padding.left - padding.right - borderWidth * 2);
  const lines = Math.max(1, Math.ceil(estimateTextWidth(text, fontSize, fontWeight) / contentWidth));
  const measuredHeight = lineHeight * lines + padding.top + padding.bottom;
  const cssHeight = lineHeightPx !== null ? measuredHeight : Math.max(measuredHeight, 32);
  const height = Math.max(cssHeight, fontSize * 2) + borderWidth;

  const vml = buildVmlButton({
    href,
    text,
    buttonColor,
    textColor: anchorStyleMap.color || '#ffffff',
    fontSize,
    fontWeight,
    fontFamily: anchorStyleMap['font-family'] || 'Arial, sans-serif',
    borderColor,
    borderWidth,
    borderRadius: getPixelValue(anchorStyleMap['border-radius']) || 0,
    width,
    height,
  });

  // The non-mso copy stays width="100%" — the fluid form is the only one
  // Outlook mobile handles (a px-pinned table flipped its whole-email layout
  // into overflow, seen 2026-08-07). The Gmail app is the one client that
  // shrink-wraps a width=100% table in an auto-sized column cell into a
  // text-width pill, so the computed width is applied to Gmail ONLY, via a
  // per-width class and a head rule behind Gmail's `u + .body` selector hook
  // (see addGmailButtonPinStyles).
  table.setAttribute('class', `${table.getAttribute('class') || ''} lm-gm-pin-${width}`.trim());

  replaceNodeWithHtml(table, [
    makeSafeTemplate(`<!--[if mso]>${vml}<![endif]-->`),
    makeSafeTemplate('<!--[if !mso]><!-->'),
    table.outerHTML,
    makeSafeTemplate('<!--<![endif]-->'),
  ].join(''));
}

function clampImageWidths(node: Element, available: number) {
  if (available <= 0) {
    return;
  }

  if (node.tagName === 'IMG') {
    const img = node as HTMLImageElement;
    const width = getPixelWidthFromImage(img);
    if (width && Number(width) > available) {
      const originalWidth = Number(width);
      const clampedWidth = Math.floor(available);
      const clamped = String(clampedWidth);
      const clampedImage = img.cloneNode(true) as HTMLImageElement;
      const originalImage = img.cloneNode(true) as HTMLImageElement;
      // Dual-emitting multiplies any inline handlers; strip them from both
      // copies. Rendering (src/size/styles) stays the same for non-MSO.
      stripInlineEventHandlers(clampedImage);
      stripInlineEventHandlers(originalImage);
      clampedImage.setAttribute('width', clamped);

      const styleMap = parseStyleMap(clampedImage.getAttribute('style'));
      const heightAttr = clampedImage.getAttribute('height');
      const originalHeight = (heightAttr && /^\d+$/.test(heightAttr))
        ? Number(heightAttr)
        : getPixelValue(styleMap.height);
      // Word honors the height ATTRIBUTE; the style stays height:auto (the
      // trailing hardenImages pass enforces it anyway). Scale the attribute
      // when we know both axes; otherwise drop it so width clamping is not
      // distorted.
      const styleUpdates: Array<[string, string | null]> = [['width', `${clamped}px`]];
      if (originalHeight && originalWidth > 0) {
        const scaledHeight = String(Math.max(1, Math.round(originalHeight * (clampedWidth / originalWidth))));
        clampedImage.setAttribute('height', scaledHeight);
      } else {
        clampedImage.removeAttribute('height');
      }
      styleUpdates.push(['height', 'auto']);

      clampedImage.setAttribute('style', setStyleValues(clampedImage.getAttribute('style'), styleUpdates));

      replaceNodeWithHtml(img, [
        makeSafeTemplate('<!--[if mso]>'),
        clampedImage.outerHTML,
        makeSafeTemplate('<![endif]-->'),
        makeSafeTemplate('<!--[if !mso]><!-->'),
        originalImage.outerHTML,
        makeSafeTemplate('<!--<![endif]-->'),
      ].join(''));
    }
    return;
  }

  if (node.tagName === 'TABLE') {
    const widthAttr = node.getAttribute('width');
    const tableWidth = widthAttr && /^\d+$/.test(widthAttr)
      ? Math.min(available, Number(widthAttr))
      : available;

    if (isFullWidthButtonTable(node)) {
      transformFullWidthButtonForMso(node, tableWidth);
      return;
    }

    // Only the builder's ColumnsContainer tables (table-layout:fixed) use the
    // content-box + gap-padding column model below. Other tables (image
    // wrappers, Html-block markup, the canvas itself) pass their full share of
    // the canvas through unchanged — no per-cell content-box subtraction — so
    // user markup is not rewritten from a model it does not follow.
    const isFixedLayout = (parseStyleMap(node.getAttribute('style'))['table-layout'] || '').toLowerCase() === 'fixed';

    if (!isFixedLayout) {
      // No column-share model: each cell gets the full table width budget.
      // Still walk through the cell node so TD/TH padding is subtracted once
      // (image-block wrappers put padding on the td).
      getDirectRows(node).forEach((row) => {
        Array.from(row.children)
          .filter((cell) => cell.tagName === 'TD' || cell.tagName === 'TH')
          .forEach((cell) => clampImageWidths(cell, tableWidth));
      });
      return;
    }

    getDirectRows(node).forEach((row) => {
      const cells = Array.from(row.children)
        .filter((cell) => cell.tagName === 'TD' || cell.tagName === 'TH')
        .map((cell) => {
          const styleMap = parseStyleMap(cell.getAttribute('style'));
          return {
            cell,
            padding: getPaddingValues(styleMap),
            explicit: getPixelValue(styleMap.width),
            colspan: getCellColspan(cell),
          };
        });
      // Cells are content-box: an explicit width IS the content width and the
      // gap padding sits outside it. Under table-layout:fixed the remaining
      // width is split evenly across auto column tracks (colspan counts as N).
      const fixedTotal = cells.reduce(
        (sum, { padding, explicit }) => (explicit !== null ? sum + explicit + padding.left + padding.right : sum),
        0
      );
      const autoTracks = cells.reduce(
        (sum, { explicit, colspan }) => (explicit === null ? sum + colspan : sum),
        0
      );
      const trackShare = autoTracks > 0 ? Math.floor(Math.max(0, tableWidth - fixedTotal) / autoTracks) : 0;

      cells.forEach(({ cell, padding, explicit, colspan }) => {
        const innerWidth = explicit !== null
          ? explicit
          : trackShare * colspan - padding.left - padding.right;
        Array.from(cell.children).forEach((child) => clampImageWidths(child, innerWidth));
      });
    });
    return;
  }

  let innerWidth = available;
  if (node.tagName === 'DIV' || node.tagName === 'TD' || node.tagName === 'TH') {
    const padding = getPaddingValues(parseStyleMap(node.getAttribute('style')));
    innerWidth = available - padding.left - padding.right;
  }

  Array.from(node.children).forEach((child) => clampImageWidths(child, innerWidth));
}

function clampImagesToCanvas(doc: Document) {
  const found = findCanvasTable(doc);
  if (!found) {
    return;
  }

  // Word ignores the max-width:100% shrink-to-fit on images, so a fixed-width
  // image wider than its column blows the column out. Clamp each image's width
  // to its column's share of the canvas content box (max-width minus canvas
  // border/padding).
  const canvasStyle = parseStyleMap(found.table.getAttribute('style'));
  const contentWidth = Math.max(0, found.width - getHorizontalInset(canvasStyle));
  clampImageWidths(found.table, contentWidth);
}

function addGmailButtonPinStyles(doc: Document) {
  const widths = new Set<string>();
  doc.querySelectorAll('table[class*="lm-gm-pin-"]').forEach((table) => {
    const match = (table.getAttribute('class') || '').match(/lm-gm-pin-(\d+)/);
    if (match) {
      widths.add(match[1]);
    }
  });

  if (widths.size === 0 || !doc.head || !doc.body) {
    return;
  }

  // Gmail's renderer replaces <body> with a div carrying its class, preceded
  // by a <u> sibling — `u + .body` therefore matches in Gmail and nowhere
  // else. It is the only client that shrink-wraps the fluid button, so it is
  // the only client that gets the computed width. The body needs the .body
  // class for the selector to have something to match.
  // (Outlook mobile's data-outlook-cycle hook was tried first and is NOT
  // stamped in our tenant's app — verified 2026-08-07 via the matrix
  // campaign; do not reintroduce it.)
  doc.body.setAttribute('class', `${doc.body.getAttribute('class') || ''} body`.trim());

  const sorted = Array.from(widths).sort((a, b) => Number(a) - Number(b));
  const rules = sorted.map((w) =>
    `u + .body table.lm-gm-pin-${w}{width:${w}px!important;max-width:100%!important;margin:0 auto!important}`
  ).join('');

  // The pinned width is the DESKTOP column budget (canvas-relative). The
  // Gmail apps honor only absolute px widths — % in any form (attr, inline,
  // !important head rule), table-layout:fixed, and display:block anchors are
  // all ignored/shrink-wrapped — and Gmail iOS additionally ignores the
  // max-width:100% cap, so the desktop px overflows and overlaps phone-width
  // columns (campaign 29 matrix, rounds 1-5, 2026-08-11). Both apps DO honor
  // @media, so phones get the pin rescaled to its share of the narrowest
  // real viewport (320px vs the 600px canvas). Buttons render slightly
  // inset on larger phones; margin:0 auto keeps them centered.
  const phoneRules = sorted.map((w) =>
    `u + .body table.lm-gm-pin-${w}{width:${Math.max(1, Math.floor(Number(w) * 320 / 600))}px!important}`
  ).join('');

  const style = doc.createElement('style');
  style.textContent = `${rules}@media (max-width:480px){${phoneRules}}`;
  doc.head.appendChild(style);
}

export function postProcessForOutlook(html: string) {
  if (typeof DOMParser === 'undefined') {
    return html;
  }

  const doc = new DOMParser().parseFromString(html, 'text/html');

  addTableDefaults(doc);
  hardenImages(doc);
  transformButtonBlocks(doc);
  transformImageBlocks(doc);
  transformSimpleDivBlocks(doc);
  clampImagesToCanvas(doc);
  constrainCanvasForOutlook(doc);
  addGmailButtonPinStyles(doc);
  addTableDefaults(doc);
  hardenImages(doc);

  return `<!doctype html>\n${doc.documentElement.outerHTML}`;
}
