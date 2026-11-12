import React from 'react';

import { LockOutlined } from '@mui/icons-material';
import { Box, Menu } from '@mui/material';

import { TEditorBlock } from '../../../../editor/core';
import { useOfficialContext } from '../../../../editor/EditorContext';
import { freshBlock } from '../../../../../linkDefaults';

import BlockButton from './BlockButton';
import { BUTTONS } from './buttons';

type BlocksMenuProps = {
  anchorEl: HTMLElement | null;
  setAnchorEl: (v: HTMLElement | null) => void;
  onSelect: (block: TEditorBlock) => void;
  onSelectOfficial?: () => void;
};
export default function BlocksMenu({ anchorEl, setAnchorEl, onSelect, onSelectOfficial }: BlocksMenuProps) {
  // Fork (integrations EDITOR-DEFAULTS-SPEC D3/I5) -- every Add-menu and Insert above/below block
  // (this menu serves both EditorChildrenIds and TuneMenu) is built through freshBlock, which
  // fills an empty Image/Button link with the campaign's default link.
  const officialContext = useOfficialContext();
  const linkDefault = officialContext ? officialContext.linkDefault : null;
  const onClose = () => {
    setAnchorEl(null);
  };

  const onClick = (block: TEditorBlock) => {
    onSelect(block);
    setAnchorEl(null);
  };

  if (anchorEl === null) {
    return null;
  }

  return (
    <Menu
      open
      anchorEl={anchorEl}
      onClose={onClose}
      anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      transformOrigin={{ vertical: 'top', horizontal: 'center' }}
    >
      <Box sx={{ p: 1, display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr' }}>
        {BUTTONS.map((k, i) => (
          <BlockButton key={i} label={k.label} icon={k.icon} onClick={() => onClick(freshBlock(k, linkDefault))} />
        ))}
        {onSelectOfficial && (
          <BlockButton
            label="Official footer"
            icon={<LockOutlined />}
            onClick={() => {
              onSelectOfficial();
              setAnchorEl(null);
            }}
          />
        )}
      </Box>
    </Menu>
  );
}
