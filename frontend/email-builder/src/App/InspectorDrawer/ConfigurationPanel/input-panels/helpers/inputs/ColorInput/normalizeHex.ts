// react-colorful's HexColorInput accepts 3-digit hex, but every color prop schema
// (COLOR_SCHEMA in the block schemas) is /^#[0-9a-fA-F]{6}$/ — a short form passes
// the picker, paints the swatch, and is then silently dropped by the panel's safeParse.
// Dependency-free: pinned by test/hex-normalize.test.cjs.
export function normalizeHex(value: string): string {
  const m = /^#([0-9a-fA-F])([0-9a-fA-F])([0-9a-fA-F])$/.exec(value.trim());
  if (!m) return value;
  return `#${m[1]}${m[1]}${m[2]}${m[2]}${m[3]}${m[3]}`;
}
