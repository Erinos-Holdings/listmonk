// Fork (integrations EDITOR-DEFAULTS-SPEC D3/I4/I5) -- the default link a freshly inserted Image or
// Button block gets. The value is the campaign's own brand's default link, computed by the host
// (frontend/src/linkDefaults.mjs campaignLinkDefault) and pushed as `linkDefault` on the official
// context; this module only applies it.
//
// applyLinkDefault fills an EMPTY link only and never overwrites a set one:
//   Image  -- `linkHref` null, absent or '' (whitespace included);
//   Button -- `url` null, absent, '' or the Add menu's retired placeholder https://listmonk.app.
// A null/empty default leaves the block as it is; every other block type is untouched. Returns
// a NEW block when it changed anything, else the SAME object.
//
// freshBlock is the ONE path every Add-menu and Insert above/below insertion takes (BlocksMenu,
// shared by EditorChildrenIds and TuneMenu). Paste, Duplicate, template import and existing
// blocks never pass through it (D7).
//
// Import-free by construction: test/run.cjs compiles this file standalone.

type TBlockLike = { type?: string; data?: any };

// The Add menu's former Button url (buttons.tsx), retired by D3 -- still read as empty.
export const RETIRED_BUTTON_PLACEHOLDER = 'https://listmonk.app';

function blank(v: unknown): boolean {
  return v === null || v === undefined || (typeof v === 'string' && v.trim() === '');
}

export function applyLinkDefault<T extends TBlockLike>(block: T, linkDefault: string | null | undefined): T {
  if (typeof linkDefault !== 'string' || linkDefault.trim() === '' || !block || !block.data) {
    return block;
  }
  const props = (block.data && block.data.props) || {};
  let key: string | null = null;
  if (block.type === 'Image' && blank(props.linkHref)) {
    key = 'linkHref';
  } else if (block.type === 'Button' && (blank(props.url) || props.url === RETIRED_BUTTON_PLACEHOLDER)) {
    key = 'url';
  }
  if (key === null) {
    return block;
  }
  return { ...block, data: { ...block.data, props: { ...props, [key]: linkDefault } } };
}

export function freshBlock<T extends TBlockLike>(button: { block: () => T }, linkDefault: string | null | undefined): T {
  return applyLinkDefault(button.block(), linkDefault);
}
