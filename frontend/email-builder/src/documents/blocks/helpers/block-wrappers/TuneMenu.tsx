import React, { useState } from 'react';

import {
  ArrowDownwardOutlined,
  ArrowUpwardOutlined,
  ContentCopyOutlined,
  DeleteOutlined,
  LayersClearOutlined,
  NorthWestOutlined,
  VerticalAlignBottomOutlined,
  VerticalAlignTopOutlined,
} from '@mui/icons-material';
import { IconButton, Paper, Stack, SxProps, Tooltip } from '@mui/material';

import { insertOfficialFooter } from '../../../../official/insert';
import { TEditorBlock } from '../../../editor/core';
import { resetDocument, setDocument, setSelectedBlockId, useDocument, useOfficialContext } from '../../../editor/EditorContext';
import { freshId, insertAt, parentOf } from '../../../structure';
import { ColumnsContainerProps } from '../../ColumnsContainer/ColumnsContainerPropsSchema';
import BlocksMenu from '../EditorChildrenIds/AddBlockMenu/BlocksMenu';

import { useStructureActions } from './structureActions';

const sx: SxProps = {
  position: 'absolute',
  top: 0,
  left: -56,
  borderRadius: 64,
  paddingX: 0.5,
  paddingY: 1
};

type Props = {
  blockId: string;
  // Fork (official footer) -- OFFICIAL-FOOTER-SPEC D6: an OfficialFooter block can be moved but
  // never duplicated or deleted from its own menu (deleting a Container that holds it remains
  // possible; the Insert action restores it). CONTAINER-NESTING-SPEC §2.2: Select parent is
  // navigation, not an edit, so it is offered here too.
  moveOnly?: boolean;
};

// Fork (container structure) -- CONTAINER-NESTING-SPEC §2.2. Entries, in order, hidden where
// they do not apply: Select parent, Move up/down, Insert above/below, Duplicate, Unwrap,
// Delete. Delete removes the whole subtree (D5), Duplicate never copies an OfficialFooter
// (D13); both, and Unwrap, go through structure.ts via useStructureActions.
export default function TuneMenu({ blockId, moveOnly }: Props) {
  const document = useDocument();
  const officialContext = useOfficialContext();
  const actions = useStructureActions(blockId);
  const [insertMenu, setInsertMenu] = useState<{ anchorEl: HTMLElement; where: 'above' | 'below' } | null>(null);

  const position = parentOf(document, blockId);
  const showSelectParent = position !== null && position.parentId !== 'root';

  // D7: Insert above/below targets the block's PARENT slot, at its index or index+1. The
  // Official-footer entry follows EditorChildrenIds' rule exactly: parent is the EmailLayout or
  // a Container, and no Official_ template is being edited.
  const insertIndex = position ? position.index + (insertMenu?.where === 'below' ? 1 : 0) : 0;
  const parentType = position ? document[position.parentId]?.type : undefined;
  const offerOfficial = !officialContext?.official && (parentType === 'EmailLayout' || parentType === 'Container');
  const handleInsert = (block: TEditorBlock) => {
    if (!position) {
      return;
    }
    const id = freshId(document);
    const next = insertAt(document, position, insertIndex, id, block);
    if (next !== document) {
      setDocument(next as typeof document);
      setSelectedBlockId(id);
    }
  };
  const handleInsertOfficial = () => {
    if (!position) {
      return;
    }
    const next = insertOfficialFooter(document, position.parentId, insertIndex, officialContext);
    if (next !== document) {
      setDocument(next as typeof document);
    }
  };

  const handleMoveClick = (direction: 'up' | 'down') => {
    const moveChildrenIds = (ids: string[] | null | undefined) => {
      if (!ids) {
        return ids;
      }
      const index = ids.indexOf(blockId);
      if (index < 0) {
        return ids;
      }
      const childrenIds = [...ids];
      if (direction === 'up' && index > 0) {
        [childrenIds[index], childrenIds[index - 1]] = [childrenIds[index - 1], childrenIds[index]];
      } else if (direction === 'down' && index < childrenIds.length - 1) {
        [childrenIds[index], childrenIds[index + 1]] = [childrenIds[index + 1], childrenIds[index]];
      }
      return childrenIds;
    };
    const nDocument: typeof document = { ...document };
    for (const [id, b] of Object.entries(nDocument)) {
      const block = b as TEditorBlock;
      if (id === blockId) {
        continue;
      }
      switch (block.type) {
        case 'EmailLayout':
          nDocument[id] = {
            ...block,
            data: {
              ...block.data,
              childrenIds: moveChildrenIds(block.data.childrenIds),
            },
          };
          break;
        case 'Container':
          nDocument[id] = {
            ...block,
            data: {
              ...block.data,
              props: {
                ...block.data.props,
                childrenIds: moveChildrenIds(block.data.props?.childrenIds),
              },
            },
          };
          break;
        case 'ColumnsContainer':
          nDocument[id] = {
            type: 'ColumnsContainer',
            data: {
              style: block.data.style,
              props: {
                ...block.data.props,
                columns: block.data.props?.columns?.map((c) => ({
                  childrenIds: moveChildrenIds(c.childrenIds),
                })),
              },
            } as ColumnsContainerProps,
          };
          break;
        default:
          nDocument[id] = block;
      }
    }

    resetDocument(nDocument);
    setSelectedBlockId(blockId);
  };

  return (
    <Paper sx={sx} onClick={(ev) => ev.stopPropagation()}>
      <Stack>
        {showSelectParent && (
          <Tooltip title="Select parent" placement="left-start">
            <IconButton aria-label="Select parent" onClick={() => setSelectedBlockId(position!.parentId)} sx={{ color: 'text.primary' }}>
              <NorthWestOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
        )}
        <Tooltip title="Move up" placement="left-start">
          <IconButton aria-label="Move up" onClick={() => handleMoveClick('up')} sx={{ color: 'text.primary' }}>
            <ArrowUpwardOutlined fontSize="small" />
          </IconButton>
        </Tooltip>
        <Tooltip title="Move down" placement="left-start">
          <IconButton aria-label="Move down" onClick={() => handleMoveClick('down')} sx={{ color: 'text.primary' }}>
            <ArrowDownwardOutlined fontSize="small" />
          </IconButton>
        </Tooltip>
        {!moveOnly && position && (
          <Tooltip title="Insert above" placement="left-start">
            <IconButton aria-label="Insert above" onClick={(ev) => setInsertMenu({ anchorEl: ev.currentTarget, where: 'above' })} sx={{ color: 'text.primary' }}>
              <VerticalAlignTopOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
        )}
        {!moveOnly && position && (
          <Tooltip title="Insert below" placement="left-start">
            <IconButton aria-label="Insert below" onClick={(ev) => setInsertMenu({ anchorEl: ev.currentTarget, where: 'below' })} sx={{ color: 'text.primary' }}>
              <VerticalAlignBottomOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
        )}
        {!moveOnly && (
          <Tooltip title="Duplicate" placement="left-start">
            <IconButton aria-label="Duplicate" onClick={actions.requestDuplicate} sx={{ color: 'text.primary' }}>
              <ContentCopyOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
        )}
        {!moveOnly && actions.isContainer && (
          <Tooltip title={actions.unwrapRefusal ?? 'Unwrap (keep its blocks)'} placement="left-start">
            <span>
              <IconButton aria-label="Unwrap" disabled={Boolean(actions.unwrapRefusal)} onClick={actions.requestUnwrap} sx={{ color: 'text.primary' }}>
                <LayersClearOutlined fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
        )}
        {!moveOnly && (
          <Tooltip title="Delete" placement="left-start">
            <IconButton aria-label="Delete" onClick={actions.requestDelete} sx={{ color: 'text.primary' }}>
              <DeleteOutlined fontSize="small" />
            </IconButton>
          </Tooltip>
        )}
      </Stack>
      <BlocksMenu
        anchorEl={insertMenu ? insertMenu.anchorEl : null}
        setAnchorEl={(el) => {
          if (el === null) setInsertMenu(null);
        }}
        onSelect={handleInsert}
        onSelectOfficial={offerOfficial ? handleInsertOfficial : undefined}
      />
      {actions.dialog}
    </Paper>
  );
}
