// The email canvas width, in px. One constant for the reader/editor canvas
// (`max-width` on the centered table) and for the Image block's auto-fill cap
// (Img/imageWidth.ts) — a canvas change must move the cap with it. outlook.ts
// keeps its own 320/600 phone ratio because run.cjs compiles that file standalone
// (no imports allowed); keep the two in step by hand.
export const CANVAS_WIDTH = 600;
