// Fork (emoji picker) -- integrations EMOJI-PICKER-SPEC D5.
//
// The ONE insertion rule for the emoji picker (emoji-picker-element) on the campaign subject
// and preheader (frontend/src/views/Campaign.vue) and the builder's Text ribbon and Heading
// field (App/.../helpers/inputs/EmojiPickerButton.tsx). The richtext editor inserts through
// TinyMCE's own selection instead (RichtextEditor.vue), with the same string.
//
// The inserted string is exactly the picker's `emoji-click` event.detail.unicode -- no spaces,
// no zero-width characters, no entity (I2).
//
// Plain ESM, dependency-free, here rather than under frontend/src for the darkSim.js reason:
// test/emoji-insert.test.cjs evaluates it standalone in the builder's CI test step, and the SPA
// imports it by relative path.

// The self-hosted emoji data (D3), served by the Go binary on the admin surface. The version is
// in the filename because the admin static route answers HEAD with 405, so the picker's ETag
// revalidation never sees a changed file at the same URL. frontend/package.json's postinstall
// writes the file under this name; src/emojiData.test.mjs pins the two together (I3).
export const EMOJI_DATA_PATH = '/admin/static/emoji-data-1.8.0.json';

function offset(value, fallback, max) {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return fallback;
  }
  return Math.min(Math.max(Math.trunc(value), 0), max);
}

// insertEmoji replaces the selection [start, end) of text with emoji and returns the new text
// with the selection collapsed just after the emoji. Offsets clamp to [0, text.length]; a
// non-numeric offset means "no recorded selection" and lands at the end of the text (D6); a
// reversed pair is put in order. An empty emoji returns the input unchanged.
export function insertEmoji(text, start, end, emoji) {
  const value = typeof text === 'string' ? text : '';
  let from = offset(start, value.length, value.length);
  let to = offset(end, from, value.length);
  if (from > to) {
    [from, to] = [to, from];
  }
  if (typeof emoji !== 'string' || emoji === '') {
    return { text: value, selectionStart: from, selectionEnd: to };
  }
  const caret = from + emoji.length;
  return {
    text: value.slice(0, from) + emoji + value.slice(to),
    selectionStart: caret,
    selectionEnd: caret,
  };
}
