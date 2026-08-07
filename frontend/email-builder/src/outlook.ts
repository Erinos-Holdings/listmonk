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
  // before the Go template expression is evaluated.
  const escaped = escapeTemplateString(raw)
    .replace(/</g, '\\x3c')
    .replace(/>/g, '\\x3e');

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

function buildPresentationTable(contents: string, width: string = '100%') {
  const widthAttr = width && width !== 'auto' ? ` width="${escapeAttribute(width)}"` : '';

  return `<table role="presentation"${widthAttr} cellpadding="0" cellspacing="0" border="0" style="${PRESENTATION_TABLE_STYLE}">${contents}</table>`;
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
      ['height', 'auto'],
      ['-ms-interpolation-mode', 'bicubic'],
    ];

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

    const innerTable = buildPresentationTable(`<tbody><tr><td align="${escapeAttribute(align)}">${content}</td></tr></tbody>`, 'auto');
    const html = buildPresentationTable(
      `<tbody><tr><td align="${escapeAttribute(align)}"${bgcolorAttr} style="${escapeAttribute(styleValue)}">${innerTable}</td></tr></tbody>`
    );

    replaceNodeWithHtml(div, html);
  });
}

function transformSimpleDivBlocks(doc: Document) {
  const wrappers = Array.from(doc.querySelectorAll('div')).filter((div) => {
    const { styleMap } = getWrapperOptions(div.getAttribute('style'));

    if (!styleMap.padding && !styleMap.height) {
      return false;
    }

    if (div.children.length > 0) {
      const firstChild = div.children[0];
      if (firstChild.tagName === 'A' || firstChild.tagName === 'IMG' || firstChild.tagName === 'TABLE') {
        return false;
      }
    }

    if (styleMap['min-height'] && styleMap.width === '100%') {
      return false;
    }

    return true;
  }) as HTMLDivElement[];

  wrappers.forEach((div) => {
    const { styleValue, styleMap, align, bgcolorAttr } = getWrapperOptions(div.getAttribute('style'));
    const height = getPixelValue(styleMap.height);
    const isSpacer = div.children.length === 0 && (div.textContent || '').trim() === '' && height !== null;

    if (isSpacer) {
      const spacerHtml = buildPresentationTable(
        `<tbody><tr><td${bgcolorAttr} height="${height}" style="${escapeAttribute(styleValue)};line-height:${height}px;font-size:${height}px;">&nbsp;</td></tr></tbody>`
      );

      replaceNodeWithHtml(div, spacerHtml);
      return;
    }

    const blockHtml = buildPresentationTable(
      `<tbody><tr><td align="${escapeAttribute(align)}"${bgcolorAttr} style="${escapeAttribute(styleValue)}">${div.innerHTML}</td></tr></tbody>`
    );

    replaceNodeWithHtml(div, blockHtml);
  });
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
  // The Button block's "Button border size/color" controls emit a
  // `border: <n>px solid <color>` shorthand on the anchor.
  const borderMatch = (anchorStyleMap.border || '').match(/^(\d+(?:\.\d+)?)px\s+solid\s+(.+)$/i);
  const borderColor = borderMatch ? borderMatch[2] : null;
  const borderWidth = borderMatch ? Math.round(Number(borderMatch[1])) : 0;
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
  const estimatedTextWidth = Math.max(1, Math.round(text.length * fontSize * (fontWeight.toLowerCase() === 'bold' ? 0.68 : 0.62)));
  // An auto-sized CSS button grows by the border on every edge, while a VML
  // strokeweight straddles the shape edge — half in, half out — so adding one
  // border width per axis is what matches the drawn outer size. Explicit boxes
  // are border-box in CSS and need no adjustment.
  const estimatedWidth = explicitWidth ?? Math.max(40, estimatedTextWidth + paddingValues.left + paddingValues.right) + borderWidth;
  // The 32px floor guards the text-length guess when no line-height is
  // available; a real line-height + padding is the actual CSS box and must not
  // be inflated, or short custom-height buttons render taller in Outlook.
  const measuredHeight = lineHeight + paddingValues.top + paddingValues.bottom;
  // Even with a real line-height, keep 2px of slack over the text line: Word
  // HIDES a text line that does not fit the shape (it does not clip glyphs).
  const estimatedHeight = explicitHeight
    ?? (lineHeightPx !== null ? Math.max(measuredHeight, lineHeight + 2) : Math.max(measuredHeight, 32)) + borderWidth;
  const arcsize = Math.max(0, Math.min(50, Math.round((borderRadius / estimatedHeight) * 100)));
  const cleanAnchorStyle = anchor.getAttribute('style') || '';
  const strokeAttrs = borderColor
    ? `strokecolor="${escapeAttribute(borderColor)}" strokeweight="${borderWidth}px"`
    : `strokecolor="${escapeAttribute(buttonColor)}"`;
  const vml = `<v:roundrect xmlns:v="urn:schemas-microsoft-com:vml" xmlns:w="urn:schemas-microsoft-com:office:word" href="${escapeAttribute(href)}" style="height:${estimatedHeight}px;v-text-anchor:middle;width:${estimatedWidth}px;" arcsize="${arcsize}%" ${strokeAttrs} fillcolor="${escapeAttribute(buttonColor)}"><w:anchorlock/><v:textbox inset="0,0,0,0"><center style="color:${escapeAttribute(textColor)};font-family:${escapeAttribute(fontFamily)};font-size:${fontSize}px;font-weight:${escapeAttribute(fontWeight)};mso-line-height-rule:exactly;line-height:${estimatedHeight - borderWidth}px;">${escapeHtml(text)}</center></v:textbox></v:roundrect>`;
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
  const padding = getPaddingValues(anchorStyleMap);
  const border = anchorStyleMap.border;
  const href = anchor.getAttribute('href') || '#';
  const target = anchor.getAttribute('target');
  const targetAttr = target ? ` target="${escapeAttribute(target)}"` : '';
  const text = anchor.textContent?.replace(/\s+/g, ' ').trim() || '';

  // Word ignores display:block, padding, text-align and borders on an inline
  // anchor, which collapses the CSS button to left-aligned text width. In the
  // mso copy the TD is the button — explicit px table width, td-level
  // centering/padding/border — and the anchor keeps only text styling.
  const msoTdStyle = [
    `background-color:${buttonColor}`,
    'text-align:center',
    `padding:${formatPaddingShorthand(padding)}`,
    border ? `border:${border}` : '',
  ].filter(Boolean).join(';');
  const msoAnchorStyle = setStyleValues(anchor.getAttribute('style'), [
    ['display', null],
    ['padding', null],
    ['border', null],
    ['border-radius', null],
    ['background-color', null],
    ['width', null],
    ['text-align', null],
  ]);

  const msoButton = `<table role="presentation" width="${width}" cellpadding="0" cellspacing="0" border="0" style="${PRESENTATION_TABLE_STYLE}"><tbody><tr><td bgcolor="${escapeAttribute(buttonColor)}" align="center" style="${escapeAttribute(msoTdStyle)}"><a href="${escapeAttribute(href)}"${targetAttr} style="${escapeAttribute(msoAnchorStyle)}">${escapeHtml(text)}</a></td></tr></tbody></table>`;

  // The non-mso copy gets the same computed width as a determinate px value.
  // A width="100%" table inside an auto-sized column cell is circular, and the
  // Gmail app resolves it by shrinking to content — the button collapses to a
  // text-width pill (seen 2026-08-07). Where % resolution works the table was
  // already exactly this many px, so nothing changes there; max-width:100%
  // keeps narrow-viewport clients able to shrink it.
  table.setAttribute('width', String(width));
  table.setAttribute('class', `${table.getAttribute('class') || ''} lm-btn-pin`.trim());
  table.setAttribute('style', setStyleValues(table.getAttribute('style'), [
    ['width', `${width}px`],
    ['max-width', '100%'],
    // Centering fallback for reflow clients that widen the cell past the
    // pinned width — td align=center does not center a block table.
    ['margin', '0 auto'],
  ]));

  replaceNodeWithHtml(table, [
    makeSafeTemplate('<!--[if mso]>'),
    msoButton,
    makeSafeTemplate('<![endif]-->'),
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

function addOutlookMobileStyles(doc: Document) {
  if (!doc.querySelector('table.lm-btn-pin') || !doc.head) {
    return;
  }

  // Outlook's mobile apps render the non-mso path, REFLOW the layout, and
  // ignore max-width — a px-pinned button forces the columns wider than the
  // screen. Their preprocessor stamps data-outlook-cycle on the body, so this
  // reverts pinned buttons to fluid only there. Other clients either honor
  // the pin (Gmail) or never see this path (desktop Outlook renders mso).
  const style = doc.createElement('style');
  style.textContent = '[data-outlook-cycle] table.lm-btn-pin{width:100%!important;max-width:100%!important;min-width:0!important}';
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
  addOutlookMobileStyles(doc);
  addTableDefaults(doc);
  hardenImages(doc);

  return `<!doctype html>\n${doc.documentElement.outerHTML}`;
}
