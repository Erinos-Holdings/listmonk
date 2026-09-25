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
 */
export default function renderToStaticMarkup(
  document: TReaderDocument,
  { rootBlockId, official }: { rootBlockId: string; official?: TOfficialRender | null }
) {
  return (
    '<!DOCTYPE html>' +
    baseRenderToStaticMarkup(
      <html>
        <body style={{ margin: '0', padding: '0' }}>
          <OfficialRenderContext.Provider value={official ?? null}>
            <Reader document={document} rootBlockId={rootBlockId} />
          </OfficialRenderContext.Provider>
        </body>
      </html>
    )
  );
}
