import { z } from 'zod';

import { ImagePropsSchema } from '@usewaypoint/block-image';

// Adds an opt-in `embed` flag to the upstream Image props. The renderer
// (frontend/email-builder/src/utils.tsx) re-tags marked <img>s with
// `data-embed` and the backend resolves the src filename to a media item.
export const ImgPropsSchema = ImagePropsSchema.extend({
  props: z
    .object({
      width: z.number().nullable().optional(),
      height: z.number().nullable().optional(),
      url: z.string().nullable().optional(),
      alt: z.string().nullable().optional(),
      linkHref: z.string().nullable().optional(),
      contentAlignment: z.enum(['top', 'middle', 'bottom']).nullable().optional(),
      embed: z.boolean().nullable().optional(),
    })
    .nullable()
    .optional(),
});

export type ListmonkImageProps = z.infer<typeof ImgPropsSchema>;
