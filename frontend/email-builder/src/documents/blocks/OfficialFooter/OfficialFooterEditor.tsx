import React from 'react';

import { LockOutlined } from '@mui/icons-material';
import { Box, Tooltip, Typography } from '@mui/material';

import { useOfficialContext, useOfficialFooters } from '../../editor/EditorContext';
import { OfficialReferenceChildren, TReaderDocument } from '../../reader/core';
import { resolveOfficial } from '../../../official/resolve';

import { OFFICIAL_LOCK_HINT, OfficialRenderContext, parseReference } from './OfficialContext';
import { OfficialFooterProps } from './OfficialFooterPropsSchema';

// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D6. Read-only on the canvas: the resolved
// reference is rendered through the READER components (the same ones the compile uses), with
// pointer-events off so none of its blocks can be selected; the surrounding EditorBlockWrapper
// (move-only TuneMenu) makes the block itself selectable. The lock glyph sits top-right.
function Placeholder({ text }: { text: string }) {
  return (
    <Box sx={{ m: 2, p: 2, border: '1px dashed', borderColor: 'divider', textAlign: 'center' }}>
      <Typography variant="body2" color="text.secondary">
        {text}
      </Typography>
    </Box>
  );
}

export default function OfficialFooterEditor({ props }: OfficialFooterProps) {
  const refs = useOfficialFooters();
  const context = useOfficialContext();
  const kind = props?.kind;
  const res = kind === 'corporate' || kind === 'brand' ? resolveOfficial(kind, context, refs) : null;

  let body: JSX.Element;
  if (!res) {
    body = <Placeholder text="Official footer: unknown kind" />;
  } else if (res.status === 'none') {
    body = <Placeholder text="Corporate campaign — no brand footer" />;
  } else if (res.status === 'no-context') {
    body = <Placeholder text={kind === 'brand' ? 'Choose a list to resolve the brand footer' : 'Official footer: no context'} />;
  } else if (res.status === 'missing') {
    body = <Placeholder text={`Official footer missing: no template named ${res.name}`} />;
  } else {
    const doc = res.ref ? parseReference(res.ref.body_source) : null;
    body = doc ? (
      <Box sx={{ pointerEvents: 'none' }}>
        <OfficialRenderContext.Provider value={{ refs, context }}>
          <OfficialReferenceChildren document={doc as TReaderDocument} />
        </OfficialRenderContext.Provider>
      </Box>
    ) : (
      <Placeholder text={`Official footer: ${res.name} could not be read`} />
    );
  }

  return (
    <Box sx={{ position: 'relative' }} data-lm-official={kind}>
      {body}
      <Tooltip title={OFFICIAL_LOCK_HINT} placement="left">
        <LockOutlined
          fontSize="small"
          aria-label={OFFICIAL_LOCK_HINT}
          sx={{ position: 'absolute', top: 4, right: 4, color: 'text.secondary', bgcolor: 'background.paper', borderRadius: '50%', p: '2px' }}
        />
      </Tooltip>
    </Box>
  );
}
