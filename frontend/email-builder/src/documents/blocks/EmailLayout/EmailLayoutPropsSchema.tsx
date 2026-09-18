import { z } from 'zod';

const COLOR_SCHEMA = z
  .string()
  .regex(/^#[0-9a-fA-F]{6}$/)
  .nullable()
  .optional();

const FONT_FAMILY_SCHEMA = z
  .enum([
    'MODERN_SANS',
    'BOOK_SANS',
    'ORGANIC_SANS',
    'GEOMETRIC_SANS',
    'HEAVY_SANS',
    'ROUNDED_SANS',
    'MODERN_SERIF',
    'BOOK_SERIF',
    'MONOSPACE',
  ])
  .nullable()
  .optional();

const EmailLayoutPropsSchema = z.object({
  backdropColor: COLOR_SCHEMA,
  // Space above and below the canvas where the backdrop shows. Absent = 0 (flush): the
  // upstream builder hard-coded 32px, which read as a gray bar above the header on phones.
  // No bounds here (same as borderRadius): the sidebar safeParses the WHOLE object, so a
  // stored out-of-range value would silently dead-lock every Global control. The renderers
  // clamp through getBackdropPadding instead.
  backdropPadding: z.number().optional().nullable(),
  borderColor: COLOR_SCHEMA,
  borderRadius: z.number().optional().nullable(),
  canvasColor: COLOR_SCHEMA,
  textColor: COLOR_SCHEMA,
  linkColor: COLOR_SCHEMA,
  fontFamily: FONT_FAMILY_SCHEMA,
  childrenIds: z.array(z.string()).optional().nullable(),
  outlook: z.boolean().optional().nullable(),
});

export default EmailLayoutPropsSchema;

export const BACKDROP_PADDING_MAX = 64;

// The one reading of backdropPadding, shared by the canvas and the compile so they cannot
// disagree: absent/null/garbage = 0, whole pixels, clamped to the slider's range.
export function getBackdropPadding(value: unknown): number {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) {
    return 0;
  }
  return Math.min(BACKDROP_PADDING_MAX, Math.max(0, n));
}

export type EmailLayoutProps = z.infer<typeof EmailLayoutPropsSchema>;
