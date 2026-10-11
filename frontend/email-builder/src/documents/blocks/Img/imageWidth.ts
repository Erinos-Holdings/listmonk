import { CANVAS_WIDTH } from '../../canvasWidth';

// Image-block Width auto-fill (IMAGE-WIDTH-SPEC D3.2/D3.3/D3.5). Pure DOM-only
// module, no React: the sidebar panel wires these into its URL-set trigger.
//
// Why: an Image block with an empty Width emits no `width` attribute, and
// Word-based Outlook then draws the image at its stored pixel size — usually the
// optimizer's 1200px retina ceiling, twice the canvas. Filling Width from the
// image's natural size in the editor keeps the value visible and overridable and
// keeps body/body_source consistent (the builder regenerates both).

export type TImageSize = { width: number; height: number };

/**
 * Resolve an image URL's natural size. `null` on error, on a zero natural
 * width or height (an SVG with no intrinsic size), or on an empty URL. Never
 * rejects — failure is silent and leaves the field empty (D3.5). Cross-origin
 * images without CORS headers still expose naturalWidth, so no crossOrigin is set.
 */
export function measureImageSize(url: string | null | undefined): Promise<TImageSize | null> {
  return new Promise((resolve) => {
    if (!url || typeof Image === 'undefined') {
      resolve(null);
      return;
    }
    const img = new Image();
    img.onload = () =>
      resolve(img.naturalWidth > 0 && img.naturalHeight > 0 ? { width: img.naturalWidth, height: img.naturalHeight } : null);
    img.onerror = () => resolve(null);
    img.src = url;
  });
}

/**
 * A measurer whose calls supersede one another: when the URL is set again before
 * an earlier measure resolves, the earlier call resolves `null` (so its result is
 * ignored by decideWidth) and only the latest call's result can be applied.
 */
export function createImageSizeMeasurer() {
  let seq = 0;
  return (url: string | null | undefined): Promise<TImageSize | null> => {
    seq += 1;
    const mine = seq;
    return measureImageSize(url).then((size) => (mine === seq ? size : null));
  };
}

type TPadding = { left?: number | null; right?: number | null } | null | undefined;

/**
 * The auto-fill cap: the canvas minus the block's own horizontal padding — what
 * a full-width block actually renders at, so the common case (24px each side)
 * fills as 552 and needs no mso dual emit. A block inside a ColumnsContainer is
 * further reduced to its column's share by postProcess.ts::clampImageWidths at save
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

/**
 * A typed Height on an auto-filled Width (2026-10-11). The auto-fill writes the
 * natural width; a Height typed on top of it stretches the image in the editor
 * and in Word (CSS-honoring clients get `height:auto` from hardenImages and
 * silently drop the typed height instead). So when the Width still holds what
 * the auto-fill wrote, a typed Height re-derives it proportionally from the
 * natural size, capped like the auto-fill — the image always carries a width
 * attribute at or under the cap, so no Word overflow is possible (the compiled
 * shape is the ordinary width-sized image). A typed Width is never touched.
 * Returns the width to write, or `null` for "leave the document alone".
 */
export function decideWidthForHeight(
  current: number | null | undefined,
  autoPrev: number | null | undefined,
  natural: TImageSize | null | undefined,
  height: number | null | undefined,
  cap: number,
): number | null {
  if (!natural || natural.width <= 0 || natural.height <= 0) {
    return null;
  }
  if (height === null || height === undefined || height <= 0) {
    return null;
  }
  const isPreviousAutoFill = autoPrev !== null && autoPrev !== undefined && current === autoPrev;
  if (!isPreviousAutoFill) {
    return null;
  }
  const width = Math.min(Math.round((natural.width * height) / natural.height), Math.floor(cap));
  return width >= 1 ? width : null;
}
