import React, { CSSProperties, useMemo, useState } from 'react';

import { CropSquareOutlined, ViewColumnOutlined, WarningAmberOutlined } from '@mui/icons-material';
import { Box, Tooltip } from '@mui/material';

import { useCurrentBlockId } from '../../../editor/EditorBlock';
import { setSelectedBlockId, useDocument, useSelectedBlockId, useShowStructure } from '../../../editor/EditorContext';
import { containerDepth, wrapperMessage, wrapperState } from '../../../structure';

import TuneMenu from './TuneMenu';

// Fork (container structure) -- CONTAINER-NESTING-SPEC §2.3/§2.5. Editor chrome only: the
// Reader path never renders this wrapper, so none of it reaches compiled output (D10).
const TAB_SIZE = 18;
const TAB_STEP = 20;
// Show structure: a fixed 4-colour cycle by container depth.
const STRUCTURE_COLORS = ['#8e24aa', '#00897b', '#f4511e', '#3949ab'];

type TEditorBlockWrapperProps = {
  children: JSX.Element;
  // Fork (official footer) -- OFFICIAL-FOOTER-SPEC D6: the TuneMenu offers move up/down only.
  moveOnly?: boolean;
};

export default function EditorBlockWrapper({ children, moveOnly }: TEditorBlockWrapperProps) {
  const selectedBlockId = useSelectedBlockId();
  const [mouseInside, setMouseInside] = useState(false);
  const blockId = useCurrentBlockId();
  const document = useDocument();
  const block = document[blockId];
  const showStructure = useShowStructure();

  const isStructural = block?.type === 'Container' || block?.type === 'ColumnsContainer';
  const depth = useMemo(() => (isStructural ? containerDepth(document, blockId) : 0), [isStructural, document, blockId]);
  const flag = isStructural ? wrapperState(block) : null;

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
  let outlineOffset = '-1px';
  if (selectedBlockId === blockId) {
    outline = '2px solid rgba(0,121,204, 1)';
  } else if (showStructure && isStructural) {
    // With Show structure on, a container keeps its depth outline while hovered: every ancestor
    // of the hovered block is "mouseInside", and that is exactly where the author reads nesting.
    // Coincident nested edges separate visibly: each depth insets its outline 3px further.
    outline = `2px dashed ${STRUCTURE_COLORS[depth % STRUCTURE_COLORS.length]}`;
    outlineOffset = `-${1 + 3 * depth}px`;
  } else if (mouseInside) {
    outline = '2px solid rgba(0,121,204, 0.3)';
  }

  // §2.3: the handle tab. Visible when selected, while the pointer is anywhere inside the block,
  // with Show structure on, and ALWAYS when flagged (D3's hint must be findable). Absent from
  // the DOM otherwise.
  const renderTab = () => {
    if (!isStructural) {
      return null;
    }
    const visible = selectedBlockId === blockId || mouseInside || showStructure || flag !== null;
    if (!visible) {
      return null;
    }
    const kind = block.type === 'Container' ? 'container' : 'columns';
    const base = kind === 'container' ? 'Container' : 'Columns';
    const title = flag ? `${base}: ${wrapperMessage(flag)}` : base;
    const Icon = flag ? WarningAmberOutlined : kind === 'container' ? CropSquareOutlined : ViewColumnOutlined;
    return (
      <Tooltip title={title} placement="top-start">
        <Box
          component="span"
          role="button"
          aria-label={title}
          className="lm-structure-tab"
          data-lm-structure-tab={kind}
          data-lm-structure-flag={flag ? flag.kind : undefined}
          onClick={(ev: React.MouseEvent) => {
            setSelectedBlockId(blockId);
            ev.stopPropagation();
            ev.preventDefault();
          }}
          sx={{
            position: 'absolute',
            top: 0,
            // Clamped so a deep tab never runs past a narrow column.
            left: `min(${depth * TAB_STEP}px, calc(100% - ${TAB_SIZE}px))`,
            width: TAB_SIZE,
            height: TAB_SIZE,
            zIndex: 2,
            boxSizing: 'border-box',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            cursor: 'pointer',
            borderRadius: '0 0 4px 0',
            border: '1px solid',
            borderColor: flag ? '#ed6c02' : 'rgba(0,121,204,1)',
            bgcolor: flag ? '#fff3e0' : '#ffffff',
            color: flag ? '#ed6c02' : 'rgba(0,121,204,1)',
          }}
        >
          <Icon sx={{ fontSize: 14 }} />
        </Box>
      </Tooltip>
    );
  };

  const renderMenu = () => {
    if (selectedBlockId !== blockId) {
      return null;
    }
    return <TuneMenu blockId={blockId} moveOnly={moveOnly} />;
  };

  return (
    <Box
      data-lm-text={textMarker}
      style={textSizeStyle}
      sx={{
        position: 'relative',
        maxWidth: '100%',
        outlineOffset,
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
      {renderTab()}
      {children}
    </Box>
  );
}
