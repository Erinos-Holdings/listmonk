// Fork (dark-mode readiness) -- DARK-MODE-SPEC D1.
//
// The per-template link color is emitted as `<style>a{color:…}</style>` in both <head>
// and <body>. Six clients drop BOTH copies and fall back to their own link blue: Gmail.com
// web, GMX, Web.de, T-Online, Libero and Outlook for Mac (template 29 full matrix,
// 2026-09-10). Five Gmail.com variants (`a[href]`, `!important`, head-only, combinations)
// all failed; only an inline `style="color:…"` on the anchor survived.
//
// So the color is ALSO inlined onto every anchor at compile time. An anchor that already
// declares its own color keeps it verbatim — Button blocks and hand-colored links are
// untouched, which preserves the "any inline color still wins" property. The <style> copies
// stay as belt-and-braces for anchors created later (Go-side link rewrites).
//
// Dependency-free by construction (like normalizeHex.ts) so it can be transpiled and
// evaluated standalone in the test suite.

type TStyleMap = Record<string, string>;

// A local copy of postProcess.ts's parser. Deliberate: this module imports nothing, and the
// substring test it replaces is the bug it exists to avoid — `background-color:` must not
// count as a `color:` declaration.
function parseStyleMap(style: string | null): TStyleMap {
  return (style || '')
    .split(';')
    .map((entry) => entry.trim())
    .filter(Boolean)
    .reduce<TStyleMap>((acc, entry) => {
      const separator = entry.indexOf(':');
      if (separator === -1) {
        return acc;
      }

      const property = entry.slice(0, separator).trim().toLowerCase();
      const value = entry.slice(separator + 1).trim();
      if (property) {
        acc[property] = value;
      }
      return acc;
    }, {});
}

/**
 * inlineLinkColor appends `color:<color>` to the inline style of every <a> in the compiled
 * document that does not already declare a color of its own.
 *
 * With no color it is a strict no-op on the string — a document without a link color
 * compiles byte-identically to before this pass existed, DOMParser round trip included.
 * Idempotent: re-running it over its own output changes nothing, because every anchor it
 * touched now declares a color.
 */
export function inlineLinkColor(html: string, color?: string | null): string {
  if (!color) {
    return html;
  }
  if (typeof DOMParser === 'undefined') {
    return html;
  }

  const doc = new DOMParser().parseFromString(html, 'text/html');

  doc.querySelectorAll('a').forEach((anchor) => {
    const style = anchor.getAttribute('style');
    if (parseStyleMap(style).color) {
      return;
    }

    const existing = (style || '').trim().replace(/;+$/, '');
    anchor.setAttribute('style', existing ? `${existing};color:${color}` : `color:${color}`);
  });

  return `<!doctype html>\n${doc.documentElement.outerHTML}`;
}
