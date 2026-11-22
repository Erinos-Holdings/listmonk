// Fork (campaign review) -- integrations ALT-ON-INSERT-SPEC D6: the in-place alt write's pure half.
// Import-free (compiled standalone by test/run.cjs, like structure.ts).
//
// A fix ({kind: "alt.set", block, from, to, undecided?, src}, the review Lambda's answer) is applied
// only when the block exists, is an Image block, its trimmed props.url equals `src`, and its current
// alt equals `from` (`undecided` <=> the alt is absent: undefined or null). Otherwise it is skipped --
// so an alt typed while the request was out, and an image replaced while it was out, are never
// overwritten. The input document is never mutated: the answer is a new document holding a copy of
// each changed block (and `changed: false`, the same document, when nothing applied).

export type TAltFix = {
  kind: string;
  block?: string;
  index?: number;
  from: string;
  to: string;
  undecided?: boolean;
  src: string;
};

export type TApplyAltsResult<D> = {
  document: D;
  changed: boolean;
  applied: TAltFix[];
  skipped: TAltFix[];
};

export function applyAltsToDocument<D extends Record<string, any>>(document: D, fixes: TAltFix[]): TApplyAltsResult<D> {
  const applied: TAltFix[] = [];
  const skipped: TAltFix[] = [];
  const next: Record<string, any> = { ...document };
  for (const fix of Array.isArray(fixes) ? fixes : []) {
    const id = fix && typeof fix.block === 'string' ? fix.block : null;
    const block = id !== null && Object.prototype.hasOwnProperty.call(next, id) ? next[id] : null;
    if (!fix || fix.kind !== 'alt.set' || typeof fix.to !== 'string' || !block || block.type !== 'Image') {
      skipped.push(fix);
      continue;
    }
    const props = (block.data && block.data.props) || {};
    const url = typeof props.url === 'string' ? props.url.trim() : '';
    const alt = props.alt;
    const current = fix.undecided ? alt === undefined || alt === null : alt === fix.from;
    if (url !== fix.src || !current) {
      skipped.push(fix);
      continue;
    }
    next[id as string] = { ...block, data: { ...(block.data || {}), props: { ...props, alt: fix.to } } };
    applied.push(fix);
  }
  if (!applied.length) {
    return { document, changed: false, applied, skipped };
  }
  return { document: next as D, changed: true, applied, skipped };
}
