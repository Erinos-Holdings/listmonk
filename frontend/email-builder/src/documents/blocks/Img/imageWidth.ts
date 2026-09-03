import { CANVAS_WIDTH } from '../../canvasWidth';

// Image-block Width auto-fill (IMAGE-WIDTH-SPEC D3.2/D3.3/D3.5). Pure DOM-only
// module, no React: the sidebar panel wires these into its URL-set trigger.
//
// Why: an Image block with an empty Width emits no `width` attribute, and
// Word-based Outlook then draws the image at its stored pixel size — usually the
// optimizer's 1200px retina ceiling, twice the canvas. Filling Width from the
// image's natural size in the editor keeps the value visible and overridable and
// keeps body/body_source consistent (the builder regenerates both).

/**
 * Resolve an image URL's natural width. `null` on error, on a zero natural
 * width (an SVG with no intrinsic size), or on an empty URL. Never rejects —
 * failure is silent and leaves the field empty (D3.5). Cross-origin images
 * without CORS headers still expose naturalWidth, so no crossOrigin is set.
 */
export function measureImageWidth(url: string | null | undefined): Promise<number | null> {
  return new Promise((resolve) => {
    if (!url || typeof Image === 'undefined') {
      resolve(null);
      return;
    }
    const img = new Image();
    img.onload = () => resolve(img.naturalWidth > 0 ? img.naturalWidth : null);
    img.onerror = () => resolve(null);
    img.src = url;
  });
}

/**
 * A measurer whose calls supersede one another: when the URL is set again before
 * an earlier measure resolves, the earlier call resolves `null` (so its result is
 * ignored by decideWidth) and only the latest call's result can be applied.
 */
export function createImageWidthMeasurer() {
  let seq = 0;
  return (url: string | null | undefined): Promise<number | null> => {
    seq += 1;
    const mine = seq;
    return measureImageWidth(url).then((width) => (mine === seq ? width : null));
  };
}

type TPadding = { left?: number | null; right?: number | null } | null | undefined;

/**
 * The auto-fill cap: the canvas minus the block's own horizontal padding — what
 * a full-width block actually renders at, so the common case (24px each side)
 * fills as 552 and needs no mso dual emit. A block inside a ColumnsContainer is
 * further reduced to its column's share by outlook.ts::clampImageWidths at save
 * time; the sidebar deliberately does no ancestry walk.
 */
export function autoFillCap(padding: TPadding): number {
  const left = padding?.left ?? 0;
  const right = padding?.right ?? 0;
  return CANVAS_WIDTH - left - right;
}

/**
 * The D3.3 rule. Returns the width to write, or `null` for "leave the document
 * alone" — there is never an auto-fill that writes null.
 *
 * - `measured` null (load failed / superseded / zero) → no write.
 * - `current` empty (null/undefined) → `min(measured, cap)`.
 * - `current` equal to what the previous auto-fill wrote in this panel session
 *   (`autoPrev`) → re-filled; a typed width that happens to equal it is re-filled
 *   too (accepted collision, F12).
 * - any other `current` is authored and survives a URL swap.
 */
export function decideWidth(
  current: number | null | undefined,
  autoPrev: number | null | undefined,
  measured: number | null | undefined,
  cap: number,
): number | null {
  if (measured === null || measured === undefined || measured <= 0) {
    return null;
  }
  const isEmpty = current === null || current === undefined;
  const isPreviousAutoFill = autoPrev !== null && autoPrev !== undefined && current === autoPrev;
  if (!isEmpty && !isPreviousAutoFill) {
    return null;
  }
  const width = Math.min(Math.round(measured), Math.floor(cap));
  return width >= 1 ? width : null;
}
