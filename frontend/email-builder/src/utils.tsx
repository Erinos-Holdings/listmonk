import renderToStaticMarkup from './documents/reader/renderToStaticMarkup';
import { TEditorConfiguration } from './documents/editor/core';
import { makeSafeTemplate, postProcess } from './postProcess';
import { inlineLinkColor } from './inlineLinkColor';

const VIEWPORT_META = '<meta name="viewport" content="width=device-width, initial-scale=1.0">';
// Fork (dark-mode readiness) -- DARK-MODE-SPEC D1. The document-settings block is a RAW
// HTML comment, and Go's html/template ELIDES raw comments on every compile, so no
// recipient has ever received it -- which is why the 120 dpi Outlook renders scale text
// x1.25 against an unscaled card. Wrapping it in the same `{{ Safe "..." }}` encoder the
// body-side MSO comments use (postProcess.ts makeSafeTemplate) is what makes it survive the
// compile. Emitted only for outlook:true documents, exactly as before.
const MSO_DOCUMENT_SETTINGS = makeSafeTemplate('<!--[if mso]><noscript><xml xmlns:o="urn:schemas-microsoft-com:office:office"><o:OfficeDocumentSettings><o:AllowPNG/><o:PixelsPerInch>96</o:PixelsPerInch></o:OfficeDocumentSettings></xml></noscript><![endif]-->');
const HTML_ATTRIBUTE_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '"': '&quot;',
  "'": '&#x27;',
  '<': '&lt;',
  '>': '&gt;',
};

function injectHeadContents(html: string, contents: string) {
  const headMatch = html.match(/<head\b([^>]*)>/i);
  if (headMatch) {
    return html.replace(/<head\b([^>]*)>/i, `<head$1>${contents}`);
  }

  const htmlMatch = html.match(/<html\b([^>]*)>/i);
  if (htmlMatch) {
    return html.replace(/<html\b([^>]*)>/i, `<html$1><head>${contents}</head>`);
  }

  return `<head>${contents}</head>${html}`;
}

function collectImageEmbedURLs(document: TEditorConfiguration): string[] {
  // The upstream renderer strips the custom `embed` prop before rendering, so
  // collect URLs from blocks marked for embedding and re-tag the matching <img>
  // with a `data-embed` flag after rendering. The backend resolves the src
  // filename to a media item at compile time.
  const embedURLs: string[] = [];

  for (const block of Object.values(document)) {
    if (!block || (block as { type?: string }).type !== 'Image') {
      continue;
    }

    const props = ((block as { data?: { props?: { url?: string; embed?: boolean } } }).data || {}).props || {};
    if (props.embed && props.url) {
      embedURLs.push(props.url);
    }
  }

  return embedURLs;
}

function applyImageEmbeds(html: string, embedURLs: string[]): string {
  let output = html;

  for (const url of embedURLs) {
    const re = new RegExp(`<img\\b[^>]*?\\ssrc="${escapeRegExp(escapeHtmlAttribute(url))}"[^>]*>`, 'g');
    output = output.replace(re, (tag) => (
      /\bdata-embed\b/.test(tag) ? tag : tag.replace(/(\ssrc="[^"]*")/, '$1 data-embed="true"')
    ));
  }

  return output;
}

export function renderHtmlWithMeta(
  document: TEditorConfiguration,
  options: { rootBlockId: string; outlook?: boolean; linkColor?: string | null }
): string {
  const embedURLs = collectImageEmbedURLs(document);
  const html = renderToStaticMarkup(document, options);
  // The link-color inlining runs UNCONDITIONALLY (the Outlook transforms are opt-in; this
  // one must reach every template) and BEFORE postProcess, so the VML button builder reads
  // the same anchor styles it always has. Button anchors already carry a color, so the pass
  // never reaches them. postProcess itself runs for every document: its text-margin pass is
  // unconditional, and the flag gates only the Word idioms (PARAGRAPH-SPACING-SPEC D4/D7).
  const linked = inlineLinkColor(html, options.linkColor);
  const rendered = postProcess(linked, { outlook: Boolean(options.outlook) });
  const output = applyImageEmbeds(rendered, embedURLs);
  const meta = options.outlook ? `${VIEWPORT_META}${MSO_DOCUMENT_SETTINGS}` : VIEWPORT_META;
  // Gmail strips a <style> tag placed in <body> (caniemail: html-style, note 1)
  // but honors one in <head> — duplicated here as belt-and-braces alongside the
  // body-scoped copy the EmailLayout reader renders for clients that instead
  // strip <head> (e.g. older Yahoo Android, caniemail note 3).
  const head = options.linkColor ? `${meta}<style>a{color:${options.linkColor};}</style>` : meta;

  return injectHeadContents(output, head);
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function escapeHtmlAttribute(s: string): string {
  return s.replace(/[&"'<>]/g, (ch) => HTML_ATTRIBUTE_ESCAPES[ch]);
}
