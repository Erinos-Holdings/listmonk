// Rebrand sweep: re-map a document's brand colors from one palette to another.
//
// Pure function on the editor document JSON, exported on the EmailBuilder global so the
// admin SPA calls it through the iframe (em.remapColors(doc, old, new) then
// em.resetDocument(...)). It lives here, next to the document schema it traverses, so the
// existing test/*.test.cjs harness covers it — the Vue frontend has no unit-test harness.
//
// Contract (BRAND-SWATCHES-SPEC / runbook):
// - Only exact old-palette role-hex matches are touched (case-insensitive); ad-hoc colors
//   have no role identity and are left alone.
// - An old-palette hex duplicated across roles is ambiguous and skipped entirely.
// - Only color-bearing props are rewritten: keys ending in "color" (case-insensitive),
//   which covers every block prop in the schema (backgroundColor, canvasColor, borderColor,
//   buttonTextColor, lineColor, color, ...) while a text/markdown/html prop whose CONTENT
//   happens to be a bare role hex is never rewritten.
// - Returns the replacement count so the caller can surface a zero-match Apply instead of
//   closing silently (and only move document provenance when replaced > 0).

export type TRemapPalette = {
  label: string;
  colors: Array<{ role: string; value: string }>;
};

const COLOR_KEY = /color$/i;

export function remapColors(
  document: unknown,
  oldPalette: TRemapPalette,
  newPalette: TRemapPalette
): { document: unknown; replaced: number } {
  // Old-palette hex occurrence counts, for the ambiguity skip.
  const counts = new Map<string, number>();
  for (const c of oldPalette.colors) {
    const hex = c.value.toLowerCase();
    counts.set(hex, (counts.get(hex) ?? 0) + 1);
  }

  const newByRole = new Map(newPalette.colors.map((c) => [c.role, c.value]));

  // old hex (lowercase) -> new hex, same role only.
  const mapping = new Map<string, string>();
  for (const c of oldPalette.colors) {
    const hex = c.value.toLowerCase();
    if (counts.get(hex) !== 1) continue; // ambiguous across roles
    const target = newByRole.get(c.role);
    if (target === undefined || target.toLowerCase() === hex) continue; // no target / identical
    mapping.set(hex, target);
  }

  let replaced = 0;

  const walk = (node: unknown): unknown => {
    if (Array.isArray(node)) {
      return node.map(walk);
    }
    if (node !== null && typeof node === 'object') {
      const out: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(node as Record<string, unknown>)) {
        if (COLOR_KEY.test(k) && typeof v === 'string' && mapping.has(v.toLowerCase())) {
          out[k] = mapping.get(v.toLowerCase());
          replaced += 1;
        } else {
          out[k] = walk(v);
        }
      }
      return out;
    }
    return node;
  };

  return { document: walk(document), replaced };
}
