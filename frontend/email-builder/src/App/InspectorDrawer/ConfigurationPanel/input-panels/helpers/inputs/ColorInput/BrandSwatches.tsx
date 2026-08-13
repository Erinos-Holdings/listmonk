import React from 'react';

import { Box, Button, Stack, Tooltip, Typography } from '@mui/material';

import { TBrandPalette } from '../../../../../../../documents/editor/EditorContext';

type Props = {
  palettes: TBrandPalette[];
  value: string;
  onChange: (value: string) => void;
};

// Brand color rows above the default preset grid: one labeled row per palette, one cell per
// role. Renders nothing when no palettes are provided — a brand with no published catalog
// page gets no row, by design (no fallbacks, no overrides).
export default function BrandSwatches({ palettes, value, onChange }: Props) {
  if (palettes.length === 0) {
    return null;
  }

  const renderCell = (role: string, colorValue: string) => {
    // The picker emits lowercase hex while catalog themes are typically uppercase, so the
    // selected-cell highlight has to compare case-insensitively.
    const selected = typeof value === 'string' && value.toLowerCase() === colorValue.toLowerCase();
    return (
      <Tooltip key={role} title={role} placement="top" arrow>
        <Button
          aria-label={role}
          onClick={() => onChange(colorValue)}
          sx={{
            width: 24,
            height: 24,
            backgroundColor: colorValue,
            border: '1px solid',
            borderColor: selected ? 'black' : 'grey.200',
            minWidth: 24,
            display: 'inline-flex',
            '&:hover': {
              backgroundColor: colorValue,
              borderColor: 'grey.500',
            },
          }}
        />
      </Tooltip>
    );
  };

  return (
    <Stack spacing={0.5}>
      {palettes.map((p) => (
        <Box key={p.label}>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.5 }}>
            {p.label}
          </Typography>
          <Box sx={{ display: 'flex', gap: 1 }}>{p.colors.map((c) => renderCell(c.role, c.value))}</Box>
        </Box>
      ))}
    </Stack>
  );
}
