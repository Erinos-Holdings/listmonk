import { z } from 'zod';

import { ButtonPropsSchema as BaseButtonPropsSchema } from '@usewaypoint/block-button';

const BasePropsShape = BaseButtonPropsSchema.shape.props.unwrap().unwrap().shape;

/**
 * Adds opt-in fixed dimensions and an opt-in border to the upstream Button
 * props.
 *
 * All of these are absent from every previously saved document, and 0 means
 * "unset", so a Button block that predates this schema renders exactly as it
 * did before — see Button.tsx, which only departs from upstream's markup when
 * one of these is greater than 0.
 */
const ButtonPropsSchema = z.object({
  style: BaseButtonPropsSchema.shape.style,
  props: z
    .object({
      ...BasePropsShape,
      customWidth: z.number().min(0).optional().nullable(),
      customHeight: z.number().min(0).optional().nullable(),
      borderSize: z.number().min(0).optional().nullable(),
      borderColor: z.string().optional().nullable(),
    })
    .optional()
    .nullable(),
});

// borderColor has no upstream counterpart in ButtonPropsDefaults; the renderer
// and the sidebar picker must agree on the fallback, so it lives here.
export const BUTTON_BORDER_COLOR_DEFAULT = '#000000';

export type ButtonProps = z.infer<typeof ButtonPropsSchema>;
export default ButtonPropsSchema;
