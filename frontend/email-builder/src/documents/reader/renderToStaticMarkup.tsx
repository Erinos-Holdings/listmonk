import React from 'react';
import { renderToStaticMarkup as baseRenderToStaticMarkup } from 'react-dom/server';

import { OfficialRenderContext, TOfficialRender } from '../blocks/OfficialFooter/OfficialContext';

import { Reader, TReaderDocument } from './core';

/**
 * A local fork of @usewaypoint/email-builder's renderToStaticMarkup (MIT, (c)
 * 2024 Waypoint (Metaccountant, Inc.)), differing from it only in rendering our
 * Reader — see ./core.tsx — and in zeroing the body margin: the backdrop no longer
 * pads the canvas, so a client's default 8px body margin would show as a band.
 *
 * Fork (official footer) -- OFFICIAL-FOOTER-SPEC D9: `official` (the host's references +
 * context) is provided as a React context so the reader's OfficialFooter block can resolve
 * without reading the editor store. Absent, an OfficialFooter block renders nothing.
 *
 * Fork (rendering bible) -- integrations RENDERING-BIBLE-SPEC §3.11 / I15: the compile context's
 * `lang` is the document's `<html lang>` (screen readers pronounce by it; the review's D1.3b
 * compares it with the campaign language). No context, or an empty lang, emits no attribute.
 */
export function htmlLangOf(official: TOfficialRender | null | undefined): string | undefined {
  const lang = String(official?.context?.lang ?? '').trim();
  return lang === '' ? undefined : lang;
}

export default function renderToStaticMarkup(
  document: TReaderDocument,
  { rootBlockId, official }: { rootBlockId: string; official?: TOfficialRender | null }
) {
  return (
    '<!DOCTYPE html>' +
    baseRenderToStaticMarkup(
      <html lang={htmlLangOf(official)}>
        <body style={{ margin: '0', padding: '0' }}>
          <OfficialRenderContext.Provider value={official ?? null}>
            <Reader document={document} rootBlockId={rootBlockId} />
          </OfficialRenderContext.Provider>
        </body>
      </html>
    )
  );
}
