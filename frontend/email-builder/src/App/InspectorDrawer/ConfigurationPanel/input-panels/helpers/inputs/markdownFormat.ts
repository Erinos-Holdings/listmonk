// Selection-based formatting transforms for the Text block's Content textarea.
// Pure and dependency-free so the test suite can transpile it standalone
// (test/markdown-format.test.cjs), the same isolation approach as outlook.ts.

export type FormatResult = {
  text: string;
  selectionStart: number;
  selectionEnd: number;
};

// Windows double-click selects the word PLUS its trailing space, and CommonMark
// refuses a closing **/_ preceded by whitespace — "**bold **" is a literal
// no-op. Shrink every wrap to the non-whitespace core so markers hug the text.
function trimRange(text: string, start: number, end: number): { start: number; end: number } {
  while (start < end && /\s/.test(text[start])) {
    start += 1;
  }
  while (end > start && /\s/.test(text[end - 1])) {
    end -= 1;
  }
  return { start, end };
}

// Markdown turns a blank line into a paragraph break, so a single HTML tag
// pair crossing one would emit <p><u>…</p><p>…</u></p> — invalid nesting that
// Outlook's Word engine may not recover from. Wrap each paragraph segment of
// the selection separately instead. Separators (newline runs with at most
// whitespace between) are captured by the split and land at odd indices.
// Each segment's own whitespace edges stay outside the wrappers too.
function encloseSegments(selected: string, prefix: string, suffix: string): string {
  return selected
    .split(/(\n(?:[ \t]*\n)+)/)
    .map((part, i) => {
      if (i % 2 === 1 || part.length === 0) {
        return part;
      }
      const lead = (part.match(/^\s*/) as RegExpMatchArray)[0];
      const trail = (part.match(/\s*$/) as RegExpMatchArray)[0];
      const core = part.slice(lead.length, part.length - trail.length);
      if (core.length === 0) {
        return part;
      }
      return lead + prefix + core + suffix + trail;
    })
    .join('');
}

export function applyEnclose(text: string, start: number, end: number, prefix: string, suffix: string): FormatResult {
  ({ start, end } = trimRange(text, start, end));
  const selected = text.slice(start, end);
  const wrapped = start === end ? prefix + suffix : encloseSegments(selected, prefix, suffix);
  const next = text.slice(0, start) + wrapped + text.slice(end);
  if (wrapped === prefix + selected + suffix) {
    // Single segment (or collapsed): select the inner text so wraps chain.
    return { text: next, selectionStart: start + prefix.length, selectionEnd: end + prefix.length };
  }
  return { text: next, selectionStart: start, selectionEnd: start + wrapped.length };
}

export function applyWrap(text: string, start: number, end: number, marker: string): FormatResult {
  return applyEnclose(text, start, end, marker, marker);
}

const LINK_URL_PLACEHOLDER = 'https://';
const LINK_LABEL_PLACEHOLDER = 'link text';

export function applyLink(text: string, start: number, end: number): FormatResult {
  ({ start, end } = trimRange(text, start, end));
  const label = start === end ? LINK_LABEL_PLACEHOLDER : text.slice(start, end);
  const next = `${text.slice(0, start)}[${label}](${LINK_URL_PLACEHOLDER})${text.slice(end)}`;
  // Select the URL placeholder so typing (or pasting) replaces it directly.
  const urlStart = start + label.length + '[]('.length;
  return { text: next, selectionStart: urlStart, selectionEnd: urlStart + LINK_URL_PLACEHOLDER.length };
}

// Expand the selection to whole lines and prefix each with "- ". Lines that
// are already bullets (or blank) are left alone, so the action is idempotent.
export function applyBulletList(text: string, start: number, end: number): FormatResult {
  // A selection ending just past a trailing newline (triple-click, shift+down)
  // must not drag the next line into the block.
  if (end > start && text[end - 1] === '\n') {
    end -= 1;
  }
  const blockStart = text.lastIndexOf('\n', start - 1) + 1;
  let blockEnd = text.indexOf('\n', end);
  if (blockEnd === -1) {
    blockEnd = text.length;
  }
  const bulleted = text
    .slice(blockStart, blockEnd)
    .split('\n')
    .map((line) => (line.trim().length === 0 || line.startsWith('- ') ? line : `- ${line}`))
    .join('\n');
  const next = text.slice(0, blockStart) + bulleted + text.slice(blockEnd);
  return { text: next, selectionStart: blockStart, selectionEnd: blockStart + bulleted.length };
}

const COLOR_LABEL_PLACEHOLDER = 'colored text';

export function applyColor(text: string, start: number, end: number, color: string): FormatResult {
  // Inline HTML, not markdown: markdown has no color syntax. marked passes raw
  // HTML through and block-text's insane sanitizer allowlists span[style].
  ({ start, end } = trimRange(text, start, end));
  const open = `<span style="color: ${color}">`;
  if (start === end) {
    const next = `${text.slice(0, start)}${open}${COLOR_LABEL_PLACEHOLDER}</span>${text.slice(end)}`;
    return {
      text: next,
      selectionStart: start + open.length,
      selectionEnd: start + open.length + COLOR_LABEL_PLACEHOLDER.length,
    };
  }
  return applyEnclose(text, start, end, open, '</span>');
}
