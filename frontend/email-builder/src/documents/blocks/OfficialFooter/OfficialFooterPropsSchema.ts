import { z } from 'zod';

// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D1. One prop, `kind`, and no style: the block
// stores no content and no template id. The reference is resolved BY NAME from the context the
// host pushes, on every render (official/resolve.ts) -- a stored id would leave a DE campaign
// with a French footer.
const OfficialFooterPropsSchema = z.object({
  props: z.object({
    kind: z.enum(['corporate', 'brand']),
  }),
});

export default OfficialFooterPropsSchema;

export type OfficialFooterProps = z.infer<typeof OfficialFooterPropsSchema>;
