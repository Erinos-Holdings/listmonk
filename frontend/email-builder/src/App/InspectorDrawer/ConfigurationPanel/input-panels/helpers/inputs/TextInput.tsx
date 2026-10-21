import React, { useEffect, useRef, useState } from 'react';

import { InputAdornment, InputProps, TextField } from '@mui/material';

import { insertEmoji } from '../../../../../../emojiInsert';

import AttributeMenuButton from './AttributeMenuButton';
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
  // Fork (attribute picker, integrations ATTRIBUTE-PICKER-SPEC D2/D6): an "Insert attribute"
  // button as a second end adornment, offering the roster group for the field's kind.
  attributePicker?: 'text' | 'link';
  onChange: (v: string) => void;
};
export default function TextInput({ helperText, label, placeholder, rows, InputProps, defaultValue, className, emojiPicker, attributePicker, onChange }: Props) {
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

  // The emoji and the attribute buttons share one recorded selection and one insertion rule
  // (ATTRIBUTE-PICKER-SPEC D5): the pick replaces the selection captured when the button opened.
  const recordSelection = () => {
    const el = inputRef.current;
    emojiSelection.current = el ? { start: el.selectionStart, end: el.selectionEnd } : null;
  };
  const insertAtSelection = (str: string) => {
    const sel = emojiSelection.current;
    const res = insertEmoji(value, sel?.start, sel?.end, str);
    if (res.text === value) {
      return;
    }
    pendingSelection.current = { start: res.selectionStart, end: res.selectionEnd };
    setValue(res.text);
    onChange(res.text);
  };
  // PRODUCT-LINK-SPEC D2/D3: the product link replaces the whole field (caret to the end).
  const replaceValue = (str: string) => {
    if (str === value) {
      return;
    }
    pendingSelection.current = { start: str.length, end: str.length };
    setValue(str);
    onChange(str);
  };

  const endAdornment = emojiPicker || attributePicker ? (
    <InputAdornment position="end">
      {emojiPicker && <EmojiPickerButton onOpen={recordSelection} onPick={insertAtSelection} />}
      {attributePicker && (
        <AttributeMenuButton
          kind={attributePicker}
          onOpen={recordSelection}
          onPick={insertAtSelection}
          value={value}
          onReplace={replaceValue}
        />
      )}
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
