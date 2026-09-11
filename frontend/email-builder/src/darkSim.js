// Fork (dark-mode readiness) -- DARK-MODE-SPEC D2.
//
// A dark-mode SIMULATION for the admin's preview modal. Three asset defects on template 29
// (a vanishing envelope glyph, an invisible hero, a white logo slab) would each have been
// visible for free in an admin preview that could show the inverting client families; the
// Mailgun matrix was the only way to see them, at ~100 credits a run.
//
// Two families are modelled, both observed in the 2026-09-10 matrix:
//   full     (Windows Outlook desktop, Gmail iOS): everything flips, images untouched.
//   partial  (Gmail apps, Outlook.com/M365 web, Outlook mobile, Yahoo web): light grounds
//            go dark, dark text goes light, mid-tones are kept, images untouched.
// NOT modelled, and the caption in the modal says so: Windows Outlook's text-only inversion
// (runbook hazard 54), Outlook.com's palette, and Gmail Android's recolour of small
// dark-on-transparent glyphs. Those still need a Mailgun run.
//
// Plain ESM, dependency-free, and deliberately here rather than under frontend/src: this
// directory's test harness runs in CI ("Email-builder tests", build-image.yml) and the Vue
// SPA has no unit-test harness at all. CampaignPreview.vue imports it by relative path.

// Rec. 709 luma on the sRGB-ENCODED channel values -- the same gamma-space measure the Go
// classifier uses (internal/media/optimizer), so "dark" means the same thing on both sides.
// WCAG's linear relative luminance reads #777777 as 0.184 ("dark"); gamma-space reads it
// 0.467, a mid-tone, which is what the eye sees.
export function luma({ r, g, b }) {
  return (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;
}

// Compiled mail carries hex and rgb()/rgba() only (plus bgcolor attributes, same forms).
// Anything else -- a keyword, a gradient, `transparent`, `inherit` -- is returned untouched
// by remapSchemeColor rather than guessed at.
export function parseCssColor(value) {
  if (typeof value !== 'string') {
    return null;
  }

  const v = value.trim();

  const short = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(v);
  if (short) {
    return {
      r: parseInt(short[1] + short[1], 16),
      g: parseInt(short[2] + short[2], 16),
      b: parseInt(short[3] + short[3], 16),
    };
  }

  const long = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(v);
  if (long) {
    return { r: parseInt(long[1], 16), g: parseInt(long[2], 16), b: parseInt(long[3], 16) };
  }

  const fn = /^rgba?\(\s*(\d{1,3})\s*[,\s]\s*(\d{1,3})\s*[,\s]\s*(\d{1,3})\s*(?:[,/][^)]*)?\)$/i.exec(v);
  if (fn) {
    const [r, g, b] = [fn[1], fn[2], fn[3]].map(Number);
    if (r <= 255 && g <= 255 && b <= 255) {
      return { r, g, b };
    }
  }

  return null;
}

function toHex({ r, g, b }) {
  const h = (n) => Math.round(Math.min(255, Math.max(0, n))).toString(16).padStart(2, '0');
  return `#${h(r)}${h(g)}${h(b)}`;
}

// The dark palette. PAGE is what a pure-white ground becomes; CARD is what the merely light
// grounds become. The whiter the source, the darker its replacement -- the inverting
// families push a white card further than the light-grey page it sits on.
export const PAGE_DARK = '#1f1f1f';
export const CARD_DARK = '#2a2a2a';
export const LIGHT_TEXT = '#e8e8e8';

// Thresholds, all in the gamma-space luma above.
const LIGHT_GROUND = 0.6; // >= this, a background is "light" and gets remapped
const DARK_INK = 0.3; // <= this, text is "dark" and gets lightened
const ALREADY_DARK = 0.2; // <= this, a background is already dark and is left alone

/**
 * remapSchemeColor maps one CSS colour into the partial (Gmail-style) dark simulation.
 *
 * role is 'background' or 'text'. Mid-tones (0.3-0.6) are unchanged in both roles -- that is
 * the defining property of a partial invert, and it is why a #888888 link renders dimmer in
 * these clients than the footer text around it. A value that does not parse is returned
 * verbatim, so an unknown keyword or a gradient can never be corrupted into a colour.
 */
export function remapSchemeColor(cssColor, role) {
  const rgb = parseCssColor(cssColor);
  if (!rgb) {
    return cssColor;
  }

  const l = luma(rgb);

  if (role === 'text') {
    return l <= DARK_INK ? LIGHT_TEXT : cssColor;
  }

  // role === 'background'
  if (l <= ALREADY_DARK || l < LIGHT_GROUND) {
    return cssColor;
  }

  // Interpolate CARD_DARK (at the LIGHT_GROUND threshold) -> PAGE_DARK (at pure white).
  const page = parseCssColor(PAGE_DARK);
  const card = parseCssColor(CARD_DARK);
  const t = Math.min(1, (l - LIGHT_GROUND) / (1 - LIGHT_GROUND));
  return toHex({
    r: card.r + (page.r - card.r) * t,
    g: card.g + (page.g - card.g) * t,
    b: card.b + (page.b - card.b) * t,
  });
}

const BACKGROUND_PROPS = ['background-color', 'background'];
const FULL_INVERT_CSS = 'html{filter:invert(1) hue-rotate(180deg);background:#fff}'
  + 'img,video{filter:invert(1) hue-rotate(180deg)}';

function remapInlineStyle(style) {
  if (!style) {
    return null;
  }

  let next = style;
  let changed = false;

  BACKGROUND_PROPS.forEach((prop) => {
    // Only a declaration whose whole value is a bare colour is remapped; a `background`
    // shorthand carrying a url() or a gradient does not parse and is left alone.
    const re = new RegExp(`(^|;)\\s*${prop}\\s*:\\s*([^;]+)`, 'i');
    const m = re.exec(next);
    if (!m) {
      return;
    }
    const mapped = remapSchemeColor(m[2].trim(), 'background');
    if (mapped !== m[2].trim()) {
      next = next.replace(m[0], `${m[1]}${prop}:${mapped}`);
      changed = true;
    }
  });

  const colorRe = /(^|;)\s*color\s*:\s*([^;]+)/i;
  const cm = colorRe.exec(next);
  if (cm) {
    const mapped = remapSchemeColor(cm[2].trim(), 'text');
    if (mapped !== cm[2].trim()) {
      next = next.replace(cm[0], `${cm[1]}color:${mapped}`);
      changed = true;
    }
  }

  return changed ? next : null;
}

/**
 * applyScheme returns the preview HTML rewritten for one scheme.
 *
 *   'light'    the input string, byte for byte. The modal always re-renders from the cached
 *              original, so the toggle can never drift the document.
 *   'full'     one prepended <style> block: invert the page, invert images back. Nothing in
 *              the document itself is touched.
 *   'partial'  walk the detached document and remap every inline background/colour and
 *              bgcolor attribute. Images are never touched.
 *
 * Pure: it parses into a detached document and serialises back. It performs no network call
 * and reaches nothing outside the string it was given, which is why the toggle cannot alter
 * the preview served to Inspect or to a test send.
 */
export function applyScheme(html, scheme) {
  if (scheme !== 'partial' && scheme !== 'full') {
    return html;
  }
  if (typeof DOMParser === 'undefined') {
    return html;
  }

  const doc = new DOMParser().parseFromString(html, 'text/html');

  if (scheme === 'full') {
    const style = doc.createElement('style');
    style.id = 'lm-dark-sim';
    style.textContent = FULL_INVERT_CSS;
    doc.head.prepend(style);
    return `<!doctype html>\n${doc.documentElement.outerHTML}`;
  }

  doc.querySelectorAll('*').forEach((el) => {
    const bg = el.getAttribute('bgcolor');
    if (bg) {
      el.setAttribute('bgcolor', remapSchemeColor(bg, 'background'));
    }

    const next = remapInlineStyle(el.getAttribute('style'));
    if (next !== null) {
      el.setAttribute('style', next);
    }
  });

  return `<!doctype html>\n${doc.documentElement.outerHTML}`;
}
