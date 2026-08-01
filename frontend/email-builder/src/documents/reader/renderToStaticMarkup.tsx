import React from 'react';
import { renderToStaticMarkup as baseRenderToStaticMarkup } from 'react-dom/server';

import { Reader, TReaderDocument } from './core';

/**
 * A local fork of @usewaypoint/email-builder's renderToStaticMarkup (MIT, (c)
 * 2024 Waypoint (Metaccountant, Inc.)), differing from it only in rendering our
 * Reader — see ./core.tsx. The markup it emits is otherwise identical.
 */
export default function renderToStaticMarkup(
  document: TReaderDocument,
  { rootBlockId }: { rootBlockId: string }
) {
  return (
    '<!DOCTYPE html>' +
    baseRenderToStaticMarkup(
      <html>
        <body>
          <Reader document={document} rootBlockId={rootBlockId} />
        </body>
      </html>
    )
  );
}
