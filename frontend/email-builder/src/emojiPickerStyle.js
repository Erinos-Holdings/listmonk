// Fork (emoji picker) -- integrations EMOJI-PICKER-SPEC D12.
//
// The ONE styling of emoji-picker-element's shadow DOM, shared by the SPA popover
// (frontend/src/components/EmojiPickerPopover.vue) and the builder button
// (App/.../helpers/inputs/EmojiPickerButton.tsx): the category tab row shows single-colour
// outline icons instead of emoji glyphs (so the row reads as navigation, not as emoji to
// insert), and both the tab row and the pinned Favorites row sit on a subtle tint. The library
// documents exactly one hook for this -- a <style> appended to the element's open shadow root
// -- and draws each tab as a <button class="nav-button" data-group-id="N"><div class="nav-emoji">
// so the icons are keyed by the tab's group id, never by position.
//
// Plain ESM, dependency-free (the darkSim.js reason): test/emoji-picker-style.test.cjs
// evaluates it standalone in the builder's CI step, and the SPA imports it by relative path.
//
// Icon paths are Material Icons (Apache 2.0), 24 px viewBox.

// One entry per category tab the picker renders, keyed by the emojibase group id it carries
// (the picker skips group 2, the skin-tone components; src/emojiData.test.mjs pins this list
// against the installed data's groups, I9).
export const EMOJI_PICKER_TAB_ICONS = [
  { group: 0, name: 'smileys-emotion', path: 'M15.5 11c.83 0 1.5-.67 1.5-1.5S16.33 8 15.5 8 14 8.67 14 9.5s.67 1.5 1.5 1.5zm-7 0c.83 0 1.5-.67 1.5-1.5S9.33 8 8.5 8 7 8.67 7 9.5 7.67 11 8.5 11zm3.5 6.5c2.33 0 4.31-1.46 5.11-3.5H6.89c.8 2.04 2.78 3.5 5.11 3.5zM11.99 2C6.47 2 2 6.48 2 12s4.47 10 9.99 10C17.52 22 22 17.52 22 12S17.52 2 11.99 2zM12 20c-4.42 0-8-3.58-8-8s3.58-8 8-8 8 3.58 8 8-3.58 8-8 8z' },
  { group: 1, name: 'people-body', path: 'M12 5.9c1.16 0 2.1.94 2.1 2.1s-.94 2.1-2.1 2.1S9.9 9.16 9.9 8s.94-2.1 2.1-2.1m0 9c2.97 0 6.1 1.46 6.1 2.1v1.1H5.9V17c0-.64 3.13-2.1 6.1-2.1M12 4C9.79 4 8 5.79 8 8s1.79 4 4 4 4-1.79 4-4-1.79-4-4-4zm0 9c-2.67 0-8 1.34-8 4v3h16v-3c0-2.66-5.33-4-8-4z' },
  { group: 3, name: 'animals-nature', path: 'M4.5 7a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zm4.5-4a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zm6 0a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zm4.5 4a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zm-2.16 7.86c-.87-1.02-1.6-1.89-2.48-2.91-.46-.54-1.05-1.08-1.75-1.32-.11-.04-.22-.07-.33-.09-.25-.04-.52-.04-.78-.04s-.53 0-.79.05c-.11.02-.22.05-.33.09-.7.24-1.28.78-1.75 1.32-.87 1.02-1.6 1.89-2.48 2.91-1.31 1.31-2.92 2.76-2.62 4.79.29 1.02 1.02 2.03 2.33 2.32.73.15 3.06-.44 5.54-.44h.18c2.48 0 4.81.58 5.54.44 1.31-.29 2.04-1.31 2.33-2.32.31-2.04-1.3-3.49-2.61-4.8z' },
  { group: 4, name: 'food-drink', path: 'M11 9H9V2H7v7H5V2H3v7c0 2.12 1.66 3.84 3.75 3.97V22h2.5v-9.03C11.34 12.84 13 11.12 13 9V2h-2v7zm5-3v8h2.5v8H21V2c-2.76 0-5 2.24-5 4z' },
  { group: 5, name: 'travel-places', path: 'M21 16v-2l-8-5V3.5c0-.83-.67-1.5-1.5-1.5S10 2.67 10 3.5V9l-8 5v2l8-2.5V19l-2 1.5V22l3.5-1 3.5 1v-1.5L13 19v-5.5l8 2.5z' },
  { group: 6, name: 'activities', path: 'M19 5h-2V3H7v2H5c-1.1 0-2 .9-2 2v1c0 2.55 1.92 4.63 4.39 4.94.63 1.5 1.98 2.63 3.61 2.96V19H7v2h10v-2h-4v-3.1c1.63-.33 2.98-1.46 3.61-2.96C19.08 12.63 21 10.55 21 8V7c0-1.1-.9-2-2-2zM5 8V7h2v3.82C5.84 10.4 5 9.3 5 8zm14 0c0 1.3-.84 2.4-2 2.82V7h2v1z' },
  { group: 7, name: 'objects', path: 'M9 21c0 .55.45 1 1 1h4c.55 0 1-.45 1-1v-1H9v1zm3-19C8.14 2 5 5.14 5 9c0 2.38 1.19 4.47 3 5.74V17c0 .55.45 1 1 1h6c.55 0 1-.45 1-1v-2.26c1.81-1.27 3-3.36 3-5.74 0-3.86-3.14-7-7-7zm2.85 11.1l-.85.6V16h-4v-2.3l-.85-.6C7.8 12.16 7 10.63 7 9c0-2.76 2.24-5 5-5s5 2.24 5 5c0 1.63-.8 3.16-2.15 4.1z' },
  { group: 8, name: 'symbols', path: 'M20 10V8h-4V4h-2v4h-4V4H8v4H4v2h4v4H4v2h4v4h2v-4h4v4h2v-4h4v-2h-4v-4h4zm-6 4h-4v-4h4v4z' },
  { group: 9, name: 'flags', path: 'M14 6l-1-2H5v17h2v-7h5l1 2h7V6h-6zm4 8h-4l-1-2H7V6h5l1 2h5v6z' },
];

// The tints (the picker is always the light theme, D8): the tab row and the Favorites row on
// the same shade, the tab icons in the library's own text colour, the active tab in its
// indicator colour.
export const EMOJI_PICKER_ROW_TINT = '#f3f3f0';
export const EMOJI_PICKER_TAB_ICON_COLOR = '#4a4a46';

function svgMask(path) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="${path}"/></svg>`;
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
}

// The style sheet appended to the picker's shadow root.
export const EMOJI_PICKER_SHADOW_CSS = [
  `.nav{background:${EMOJI_PICKER_ROW_TINT}}`,
  `.favorites{background:${EMOJI_PICKER_ROW_TINT}}`,
  // The emoji glyph is kept in the DOM (the library reads nothing from it, but the tab's
  // aria-label/title name the category) and hidden; the icon is a masked pseudo-element in
  // the same box, so hover/active backgrounds and the indicator are untouched.
  '.nav-emoji{position:relative;color:transparent;font-size:0}',
  `.nav-emoji::after{content:"";position:absolute;inset:0;margin:auto;width:1.25rem;height:1.25rem;background:${EMOJI_PICKER_TAB_ICON_COLOR};-webkit-mask:var(--erinos-tab-icon) center/contain no-repeat;mask:var(--erinos-tab-icon) center/contain no-repeat}`,
  '.nav-button[aria-selected="true"] .nav-emoji::after{background:var(--indicator-color)}',
  ...EMOJI_PICKER_TAB_ICONS.map((t) => `.nav-button[data-group-id="${t.group}"] .nav-emoji{--erinos-tab-icon:${svgMask(t.path)}}`),
].join('\n');

export const EMOJI_PICKER_STYLE_ATTR = 'data-erinos-picker-style';

// styleEmojiPicker appends the sheet to the element's open shadow root once (the library
// attaches the root in its constructor). Returns true when it appended, false when the element
// has no shadow root or is already styled.
export function styleEmojiPicker(el) {
  const root = el && el.shadowRoot;
  if (!root || root.querySelector(`style[${EMOJI_PICKER_STYLE_ATTR}]`)) {
    return false;
  }
  const style = root.ownerDocument.createElement('style');
  style.setAttribute(EMOJI_PICKER_STYLE_ATTR, '');
  style.textContent = EMOJI_PICKER_SHADOW_CSS;
  root.appendChild(style);
  return true;
}
