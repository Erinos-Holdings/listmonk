import React, { useEffect, useRef, useState } from 'react';

import { InputAdornment, InputProps, TextField } from '@mui/material';

import { insertEmoji } from '../../../../../../emojiInsert';

import EmojiPickerButton from './EmojiPickerButton';

type Props = {
  label: string;
  rows?: number;
  placeholder?: string;
  helperText?: string | JSX.Element;
  InputProps?: InputProps;
  defaultValue: string;
  className?: string;
  // Fork (emoji picker, integrations EMOJI-PICKER-SPEC D2): an emoji button as the field's end
  // adornment; the pick goes through onChange like typed text.
  emojiPicker?: boolean;
  onChange: (v: string) => void;
};
export default function TextInput({ helperText, label, placeholder, rows, InputProps, defaultValue, className, emojiPicker, onChange }: Props) {
  const [value, setValue] = useState(defaultValue);
  const isMultiline = typeof rows === 'number' && rows > 1;

  const inputRef = useRef<HTMLInputElement | HTMLTextAreaElement | null>(null);
  const emojiSelection = useRef<{ start: number | null; end: number | null } | null>(null);
  const pendingSelection = useRef<{ start: number; end: number } | null>(null);

  // D6: after an insert, focus returns to the field with the caret after the emoji.
  useEffect(() => {
    const sel = pendingSelection.current;
    const el = inputRef.current;
    if (sel && el) {
      pendingSelection.current = null;
      el.focus();
      el.setSelectionRange(sel.start, sel.end);
    }
  }, [value]);

  const endAdornment = emojiPicker ? (
    <InputAdornment position="end">
      <EmojiPickerButton
        onOpen={() => {
          const el = inputRef.current;
          emojiSelection.current = el ? { start: el.selectionStart, end: el.selectionEnd } : null;
        }}
        onPick={(emoji) => {
          const sel = emojiSelection.current;
          const res = insertEmoji(value, sel?.start, sel?.end, emoji);
          if (res.text === value) {
            return;
          }
          pendingSelection.current = { start: res.selectionStart, end: res.selectionEnd };
          setValue(res.text);
          onChange(res.text);
        }}
      />
    </InputAdornment>
  ) : null;

  return (
    <TextField
      fullWidth
      multiline={isMultiline}
      minRows={rows}
      variant={isMultiline ? 'outlined' : 'standard'}
      label={label}
      placeholder={placeholder}
      helperText={helperText}
      InputProps={endAdornment ? { ...InputProps, endAdornment } : InputProps}
      inputRef={inputRef}
      className={className}
      value={value}
      onChange={(ev) => {
        const v = ev.target.value;
        setValue(v);
        onChange(v);
      }}
    />
  );
}
