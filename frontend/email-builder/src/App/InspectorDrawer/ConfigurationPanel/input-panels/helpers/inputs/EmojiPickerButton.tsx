// Fork (emoji picker) -- integrations EMOJI-PICKER-SPEC D2/D4/D6/D8.
//
// The builder's one emoji button: an IconButton that opens an MUI Popover holding
// emoji-picker-element's <emoji-picker>. Used by the Text block's ribbon
// (MarkdownContentInput) and the Heading content field (TextInput's emojiPicker prop). The
// caller owns the field: onOpen captures its selection (the button never takes focus), onPick
// receives exactly the picker's event.detail.unicode and inserts it through
// src/emojiInsert.js. Esc or click-off closes without inserting.
import 'emoji-picker-element';

import React, { useEffect, useRef, useState } from 'react';

import { EmojiEmotionsOutlined } from '@mui/icons-material';
import { IconButton, Popover, Tooltip } from '@mui/material';

import { EMOJI_DATA_PATH } from '../../../../../../emojiInsert';

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'emoji-picker': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        'data-source'?: string;
      };
    }
  }
}

type Props = {
  onOpen: () => void;
  onPick: (emoji: string) => void;
  title?: string;
};

export default function EmojiPickerButton({ onOpen, onPick, title = 'Insert emoji' }: Props) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  // D8: the element is created on the first open, then kept mounted (no data fetch on a page
  // where nobody picks, no re-initialisation on every open).
  const [everOpened, setEverOpened] = useState(false);
  const [pickerEl, setPickerEl] = useState<HTMLElement | null>(null);
  const onPickRef = useRef(onPick);
  onPickRef.current = onPick;

  useEffect(() => {
    if (!pickerEl) {
      return undefined;
    }
    // D8: the light theme, explicitly (the OS dark preference never flips the picker). Set on
    // the element directly -- React's className handling on custom elements varies by version.
    pickerEl.classList.add('light');
    const handler = (ev: Event) => {
      const unicode = (ev as CustomEvent<{ unicode?: string }>).detail?.unicode;
      setAnchor(null);
      if (unicode) {
        onPickRef.current(unicode);
      }
    };
    pickerEl.addEventListener('emoji-click', handler);
    return () => pickerEl.removeEventListener('emoji-click', handler);
  }, [pickerEl]);

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
              setEverOpened(true);
              setAnchor(ev.currentTarget);
            }}
          >
            <EmojiEmotionsOutlined fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
      <Popover
        anchorEl={anchor}
        open={Boolean(anchor)}
        keepMounted={everOpened}
        onClose={() => setAnchor(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
      >
        {everOpened && <emoji-picker ref={setPickerEl} data-source={EMOJI_DATA_PATH} />}
      </Popover>
    </>
  );
}
