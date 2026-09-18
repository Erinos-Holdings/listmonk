import React from 'react';
import { renderToStaticMarkup as baseRenderToStaticMarkup } from 'react-dom/server';

import { Reader, TReaderDocument } from './core';

/**
 * A local fork of @usewaypoint/email-builder's renderToStaticMarkup (MIT, (c)
 * 2024 Waypoint (Metaccountant, Inc.)), differing from it only in rendering our
 * Reader — see ./core.tsx — and in zeroing the body margin: the backdrop no longer
 * pads the canvas, so a client's default 8px body margin would show as a band.
 */
export default function renderToStaticMarkup(
  document: TReaderDocument,
  { rootBlockId }: { rootBlockId: string }
) {
  return (
    '<!DOCTYPE html>' +
    baseRenderToStaticMarkup(
      <html>
        <body style={{ margin: '0', padding: '0' }}>
          <Reader document={document} rootBlockId={rootBlockId} />
        </body>
      </html>
    )
  );
}
