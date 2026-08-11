// Whether an Image block's alt text was never set, as opposed to explicitly
// emptied. `alt=""` is CORRECT for decorative images (spacers, social icons in
// some designs) and must never be flagged — the sidebar nudge fires only when
// the attribute is absent entirely. The Add-menu Image default deliberately
// carries no alt key (see AddBlockMenu/buttons.tsx) so a fresh block starts in
// the never-set state instead of shipping placeholder alt text into real mail.
export function isAltMissing(props: { alt?: string | null } | null | undefined): boolean {
  const alt = props?.alt;
  return alt === undefined || alt === null;
}
