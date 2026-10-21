// Fork (attribute picker) -- integrations ATTRIBUTE-PICKER-SPEC D3/D5/D6.
//
// The builder's one "Insert attribute" button: an IconButton opening an MUI Menu of the roster
// group for the field's kind (src/attributes.js). Used by the Text block's ribbon
// (MarkdownContentInput) and TextInput's attributePicker prop (Heading, Button Url, Image link).
// The caller owns the field: onOpen captures its selection (the button never takes focus),
// onPick receives exactly the item's `expr` and inserts it through src/emojiInsert.js; an item
// with `replace: true` (the product link, PRODUCT-LINK-SPEC D3) goes to onReplace instead and
// becomes the whole field value ('link') or the selection ('markdown' -- the ribbon).
//
// The shop link's brand is the editing context's (useOfficialContext -- the campaign's derived
// brand or the template's brand column), never the global brand selector (D3). A disabled item
// shows its reason as secondary text inside the item -- never a Tooltip, which MUI's
// pointer-events: none on a disabled MenuItem would never show (D6).
import React, { useState } from 'react';

import { DataObjectOutlined } from '@mui/icons-material';
import { IconButton, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';

import { attributeMenu } from '../../../../../../attributes';
import { useOfficialContext } from '../../../../../../documents/editor/EditorContext';

type Item = { id: string; label: string; expr: string; disabled: boolean; reason: string; replace?: boolean };

type Props = {
  kind: 'text' | 'link' | 'markdown';
  onOpen: () => void;
  onPick: (expr: string) => void;
  // PRODUCT-LINK-SPEC D3: the text the product link transforms (the whole field on 'link', the
  // selection on 'markdown') and the replacement callback for an item with `replace: true` (the
  // caller decides what it replaces). A caller passing no `value` gets the product link
  // disabled with its reason.
  value?: string;
  onReplace?: (v: string) => void;
  title?: string;
};

export default function AttributeMenuButton({ kind, onOpen, onPick, value, onReplace, title = 'Insert attribute' }: Props) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  // null before the host pushes a context -- treated as no brand (the shop link reads disabled).
  const context = useOfficialContext();
  const items: Item[] = attributeMenu(kind, { brand: context?.brand ?? null, value }).items;

  return (
    <>
      <Tooltip title={title}>
        <span>
          <IconButton
            size="small"
            aria-label={title}
            // The field's selection must survive the click (the ribbon's keepSelection).
            onMouseDown={(ev) => ev.preventDefault()}
            onClick={(ev) => {
              onOpen();
              setAnchor(ev.currentTarget);
            }}
          >
            <DataObjectOutlined fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
      <Menu
        anchorEl={anchor}
        open={Boolean(anchor)}
        onClose={() => setAnchor(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
      >
        {items.map((it) => (
          <MenuItem
            key={it.id}
            disabled={it.disabled}
            onClick={() => {
              setAnchor(null);
              if (it.disabled || !it.expr) {
                return;
              }
              if (it.replace) {
                if (onReplace) {
                  onReplace(it.expr);
                }
                return;
              }
              onPick(it.expr);
            }}
          >
            <ListItemText primary={it.label} secondary={it.disabled ? it.reason : undefined} />
          </MenuItem>
        ))}
      </Menu>
    </>
  );
}
