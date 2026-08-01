import { z } from 'zod';

import { ButtonPropsSchema as BaseButtonPropsSchema } from '@usewaypoint/block-button';

const BasePropsShape = BaseButtonPropsSchema.shape.props.unwrap().unwrap().shape;

/**
 * Adds opt-in fixed dimensions to the upstream Button props.
 *
 * Both are absent from every previously saved document, and 0 means "unset", so
 * a Button block that predates this schema renders exactly as it did before —
 * see Button.tsx, which only departs from upstream's markup when one of these is
 * greater than 0.
 */
const ButtonPropsSchema = z.object({
  style: BaseButtonPropsSchema.shape.style,
  props: z
    .object({
      ...BasePropsShape,
      customWidth: z.number().min(0).optional().nullable(),
      customHeight: z.number().min(0).optional().nullable(),
    })
    .optional()
    .nullable(),
});

export type ButtonProps = z.infer<typeof ButtonPropsSchema>;
export default ButtonPropsSchema;
