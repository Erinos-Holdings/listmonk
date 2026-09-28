import React from 'react';

import { Alert, Box, Breadcrumbs, Button, Link, Typography } from '@mui/material';

import { useStructureActions } from '../../../documents/blocks/helpers/block-wrappers/structureActions';
import { setSelectedBlockId, useDocument } from '../../../documents/editor/EditorContext';
import { breadcrumb, wrapperMessage, wrapperState } from '../../../documents/structure';

// Fork (container structure) -- CONTAINER-NESTING-SPEC §2.4. `Email › Container › Columns (col 2)
// › Text`, from structure.ts's ancestorsOf; every ancestor segment is clickable (`Email` clears
// the selection, which opens the Styles tab); the selected block's own segment is plain text.
export function StructureBreadcrumb({ blockId }: { blockId: string }) {
  const document = useDocument();
  const segments = breadcrumb(document, blockId);
  return (
    <Box px={2} pt={2} data-lm-breadcrumb="">
      <Breadcrumbs separator="›" aria-label="Block position" sx={{ fontSize: 13 }}>
        {segments.map((seg, i) =>
          i === segments.length - 1 ? (
            <Typography key={seg.id} color="text.primary" sx={{ fontSize: 13 }} data-lm-breadcrumb-segment={seg.id}>
              {seg.label}
            </Typography>
          ) : (
            <Link
              key={seg.id}
              component="button"
              underline="hover"
              color="inherit"
              sx={{ fontSize: 13 }}
              data-lm-breadcrumb-segment={seg.id}
              onClick={() => setSelectedBlockId(seg.id === 'root' ? null : seg.id)}
            >
              {seg.label}
            </Link>
          )
        )}
      </Breadcrumbs>
    </Box>
  );
}

// A flagged (unstyled, D2/D3) Container's panel: the warning and an Unwrap button -- the same
// action as the TuneMenu's (useStructureActions).
export function WrapperAlert({ blockId }: { blockId: string }) {
  const document = useDocument();
  const actions = useStructureActions(blockId);
  const flag = wrapperState(document[blockId]);
  if (!flag) {
    return null;
  }
  return (
    <Box px={2} pt={1} data-lm-wrapper-alert={flag.kind}>
      <Alert
        severity="warning"
        action={
          <Button color="inherit" size="small" disabled={Boolean(actions.unwrapRefusal)} title={actions.unwrapRefusal ?? undefined} onClick={actions.requestUnwrap} data-lm-unwrap="">
            Unwrap
          </Button>
        }
      >
        {wrapperMessage(flag)}
      </Alert>
      {actions.dialog}
    </Box>
  );
}
