import React, { CSSProperties, useState } from 'react';

import { Box } from '@mui/material';

import { useCurrentBlockId } from '../../../editor/EditorBlock';
import { setSelectedBlockId, useDocument, useSelectedBlockId } from '../../../editor/EditorContext';

import TuneMenu from './TuneMenu';

type TEditorBlockWrapperProps = {
  children: JSX.Element;
};

export default function EditorBlockWrapper({ children }: TEditorBlockWrapperProps) {
  const selectedBlockId = useSelectedBlockId();
  const [mouseInside, setMouseInside] = useState(false);
  const blockId = useCurrentBlockId();
  const block = useDocument()[blockId];

  // PARAGRAPH-SPACING-SPEC D3: the canvas half of the text-margin rule. A Text block's box
  // carries `data-lm-text` plus its effective font size (its own, else the canvas's 16px —
  // the same two-step resolution the compile's normalizeTextMargins performs), which the
  // `.lm-email-canvas` rule in EmailLayoutEditor reads. A Heading carries the marker with
  // no size: its margins are inline 0 already, and the rule has nothing to reach in it.
  const textMarker = block?.type === 'Text' || block?.type === 'Heading' ? 'true' : undefined;
  const textSizeStyle = block?.type === 'Text'
    ? ({ ['--lm-text-size' as string]: `${block.data.style?.fontSize || 16}px` } as CSSProperties)
    : undefined;

  let outline: CSSProperties['outline'];
  if (selectedBlockId === blockId) {
    outline = '2px solid rgba(0,121,204, 1)';
  } else if (mouseInside) {
    outline = '2px solid rgba(0,121,204, 0.3)';
  }

  const renderMenu = () => {
    if (selectedBlockId !== blockId) {
      return null;
    }
    return <TuneMenu blockId={blockId} />;
  };

  return (
    <Box
      data-lm-text={textMarker}
      style={textSizeStyle}
      sx={{
        position: 'relative',
        maxWidth: '100%',
        outlineOffset: '-1px',
        outline,
      }}
      onMouseEnter={(ev) => {
        setMouseInside(true);
        ev.stopPropagation();
      }}
      onMouseLeave={() => {
        setMouseInside(false);
      }}
      onClick={(ev) => {
        setSelectedBlockId(blockId);
        ev.stopPropagation();
        ev.preventDefault();
      }}
    >
      {renderMenu()}
      {children}
    </Box>
  );
}
