import React from 'react';

import { LockOutlined } from '@mui/icons-material';
import { Alert, Stack, Tooltip, Typography } from '@mui/material';

import { OFFICIAL_LOCK_HINT } from '../../../../documents/blocks/OfficialFooter/OfficialContext';
import { OfficialFooterProps } from '../../../../documents/blocks/OfficialFooter/OfficialFooterPropsSchema';
import { useOfficialContext, useOfficialFooters } from '../../../../documents/editor/EditorContext';
import { resolveOfficial } from '../../../../official/resolve';

import BaseSidebarPanel from './helpers/BaseSidebarPanel';

// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D6. Information only, no inputs: the block's
// kind, what it resolves to (template name + id), and the lock hint.
const STATUS_TEXT: Record<string, string> = {
  ok: 'Resolved',
  duplicate: 'Two templates share this name — using the lowest id. Rename or delete the other.',
  missing: 'Missing — no template of this name exists; the footer compiles empty.',
  none: 'Corporate campaign — no brand footer.',
  'no-context': 'Choose a list to resolve the brand footer.',
};

export default function OfficialFooterSidebarPanel({ data }: { data: OfficialFooterProps }) {
  const refs = useOfficialFooters();
  const context = useOfficialContext();
  const kind = data?.props?.kind;
  const res = kind === 'corporate' || kind === 'brand' ? resolveOfficial(kind, context, refs) : null;

  return (
    <BaseSidebarPanel title="Official footer block">
      <Stack spacing={1}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Tooltip title={OFFICIAL_LOCK_HINT}>
            <LockOutlined fontSize="small" aria-label={OFFICIAL_LOCK_HINT} />
          </Tooltip>
          <Typography variant="body2">{kind === 'brand' ? 'Brand footer' : 'Corporate footer'}</Typography>
        </Stack>
        {res && res.name && (
          <Typography variant="body2" color="text.secondary">
            {res.name}
            {res.ref ? ` (id ${res.ref.id})` : ''}
          </Typography>
        )}
        {res && (
          <Alert severity={res.status === 'ok' || res.status === 'none' ? 'info' : 'warning'}>{STATUS_TEXT[res.status]}</Alert>
        )}
        <Typography variant="caption" color="text.secondary">
          {OFFICIAL_LOCK_HINT}
        </Typography>
      </Stack>
    </BaseSidebarPanel>
  );
}
