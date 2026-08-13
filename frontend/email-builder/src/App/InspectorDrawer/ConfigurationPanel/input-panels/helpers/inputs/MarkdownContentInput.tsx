import React, { useEffect, useRef, useState } from 'react';

import {
  FormatBoldOutlined,
  FormatColorTextOutlined,
  FormatItalicOutlined,
  FormatListBulletedOutlined,
  FormatUnderlinedOutlined,
  InsertLinkOutlined,
} from '@mui/icons-material';
import { Box, IconButton, Menu, Stack, TextField, Tooltip } from '@mui/material';
import { HexColorInput, HexColorPicker } from 'react-colorful';

import { useBrandPalettes } from '../../../../../../documents/editor/EditorContext';

import BrandSwatches from './ColorInput/BrandSwatches';
import { DEFAULT_PRESET_COLORS } from './ColorInput/Picker';
import Swatch from './ColorInput/Swatch';
import { applyBulletList, applyColor, applyEnclose, applyLink, applyWrap, FormatResult } from './markdownFormat';

const DEFAULT_PICK_COLOR = '#E11D48';

const PICKER_SX = {
  p: 1,
  width: 220,
  '.react-colorful': { width: '100%' },
  '.react-colorful__pointer': { width: 16, height: 16 },
  '.react-colorful__saturation': { mb: 1, borderRadius: '4px' },
  '.react-colorful__last-control': { borderRadius: '4px' },
  '.react-colorful__hue-pointer': { width: '4px', borderRadius: '4px', height: 24, cursor: 'col-resize' },
  '.react-colorful__saturation-pointer': { cursor: 'all-scroll' },
  input: {
    padding: 1,
    border: '1px solid',
    borderColor: 'grey.300',
    borderRadius: '4px',
    width: '100%',
  },
};

type Props = {
  label: string;
  rows: number;
  defaultValue: string;
  markdownEnabled: boolean;
  onChange: (v: string) => void;
};

export default function MarkdownContentInput({ label, rows, defaultValue, markdownEnabled, onChange }: Props) {
  const [value, setValue] = useState(defaultValue);
  const brandPalettes = useBrandPalettes();
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const pendingSelection = useRef<{ start: number; end: number } | null>(null);

  const [colorAnchor, setColorAnchor] = useState<null | HTMLElement>(null);
  const [pickColor, setPickColor] = useState(DEFAULT_PICK_COLOR);
  // Captured when the color menu opens — the menu steals focus, so the
  // textarea's live selection is gone by the time a color is chosen.
  const colorSelection = useRef({ start: 0, end: 0 });
  // True once any picker interaction staged a color this open. Unlike the style-chip picker
  // (which writes the style live and where closing merely dismisses), this popover performs
  // a ONE-SHOT text edit, so it stages everything and commits exactly once on close-by-
  // click-off. Esc cancels; opening and clicking off untouched inserts nothing.
  const colorDirty = useRef(false);

  useEffect(() => {
    const sel = pendingSelection.current;
    const el = inputRef.current;
    if (sel && el) {
      pendingSelection.current = null;
      el.focus();
      el.setSelectionRange(sel.start, sel.end);
    }
  }, [value]);

  const currentSelection = () => {
    const el = inputRef.current;
    if (!el) {
      return { start: value.length, end: value.length };
    }
    return { start: el.selectionStart ?? value.length, end: el.selectionEnd ?? value.length };
  };

  const applyResult = (result: FormatResult) => {
    // No-op transforms (e.g. bulleting an already-bulleted line) never
    // re-render, so the [value] effect would leave a stale pendingSelection
    // armed to fire on the next keystroke — apply the selection directly.
    if (result.text === value) {
      const el = inputRef.current;
      if (el) {
        el.focus();
        el.setSelectionRange(result.selectionStart, result.selectionEnd);
      }
      return;
    }
    pendingSelection.current = { start: result.selectionStart, end: result.selectionEnd };
    setValue(result.text);
    onChange(result.text);
  };

  const stageColor = (color: string) => {
    setPickColor(color);
    colorDirty.current = true;
  };

  const closeColorMenu = (reason?: string) => {
    setColorAnchor(null);
    if (reason === 'escapeKeyDown' || !colorDirty.current) {
      return;
    }
    const { start, end } = colorSelection.current;
    applyResult(applyColor(value, start, end, pickColor));
  };

  // Toolbar buttons must not take focus, or the textarea selection collapses
  // before the click handler can read it.
  const keepSelection = (ev: React.MouseEvent) => ev.preventDefault();

  const toolbarButton = (title: string, icon: JSX.Element, onClick: (ev: React.MouseEvent<HTMLButtonElement>) => void) => (
    <Tooltip key={title} title={markdownEnabled ? title : 'Enable Markdown to use formatting'}>
      <span>
        <IconButton size="small" disabled={!markdownEnabled} onMouseDown={keepSelection} onClick={onClick}>
          {icon}
        </IconButton>
      </span>
    </Tooltip>
  );

  return (
    <Box>
      <Stack
        direction="row"
        spacing={0.5}
        sx={{ bgcolor: 'grey.100', borderRadius: 1, px: 0.5, py: 0.25, mb: 1 }}
      >
        {toolbarButton('Bold', <FormatBoldOutlined fontSize="small" />, () => {
          const { start, end } = currentSelection();
          applyResult(applyWrap(value, start, end, '**'));
        })}
        {toolbarButton('Italic', <FormatItalicOutlined fontSize="small" />, () => {
          const { start, end } = currentSelection();
          applyResult(applyWrap(value, start, end, '_'));
        })}
        {toolbarButton('Underline', <FormatUnderlinedOutlined fontSize="small" />, () => {
          const { start, end } = currentSelection();
          applyResult(applyEnclose(value, start, end, '<u>', '</u>'));
        })}
        {toolbarButton('Link', <InsertLinkOutlined fontSize="small" />, () => {
          const { start, end } = currentSelection();
          applyResult(applyLink(value, start, end));
        })}
        {toolbarButton('Text color', <FormatColorTextOutlined fontSize="small" />, (ev) => {
          colorSelection.current = currentSelection();
          colorDirty.current = false;
          setColorAnchor(ev.currentTarget);
        })}
        {toolbarButton('Bullet list', <FormatListBulletedOutlined fontSize="small" />, () => {
          const { start, end } = currentSelection();
          applyResult(applyBulletList(value, start, end));
        })}
      </Stack>
      <TextField
        fullWidth
        multiline
        minRows={rows}
        variant="outlined"
        label={label}
        inputRef={inputRef}
        value={value}
        onChange={(ev) => {
          const v = ev.target.value;
          setValue(v);
          onChange(v);
        }}
      />
      <Menu
        anchorEl={colorAnchor}
        open={Boolean(colorAnchor)}
        onClose={(ev, reason) => closeColorMenu(reason)}
        MenuListProps={{
          sx: { height: 'auto', padding: 0 },
        }}
      >
        {/* Section order mirrors ColorInput/Picker.tsx — keep the two in sync. */}
        <Stack spacing={1} sx={PICKER_SX}>
          <BrandSwatches palettes={brandPalettes} value={pickColor} onChange={stageColor} />
          <Box>
            <HexColorInput prefixed color={pickColor} onChange={stageColor} />
          </Box>
          <HexColorPicker color={pickColor} onChange={stageColor} />
          <Swatch paletteColors={DEFAULT_PRESET_COLORS} value={pickColor} onChange={stageColor} />
        </Stack>
      </Menu>
    </Box>
  );
}
