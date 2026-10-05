// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D7. The Add-block menu's "Official footer"
// entry as a pure document transform: the menu's onSelect contract inserts ONE block, the
// footer is two. Inserts, at `index` in `parentId`'s children, the brand block followed by the
// corporate block, for every brand (`curated` included -- CURATED-FOOTER-SPEC D2). A kind
// already present in the document -- in a block REACHABLE FROM ROOT -- is not inserted again
// (idempotent). CONTAINER-NESTING-SPEC D12: an orphaned footer (a key no slot
// path from root reaches, e.g. left behind by the pre-erinos.N Delete) never compiles -- the
// Reader walks from root -- so it no longer blocks the Insert.
//
// Returns a NEW document object when anything was inserted, else the SAME object.
//
// Parents: the EmailLayout root (data.childrenIds) or a Container (data.props.childrenIds). A
// ColumnsContainer column is not a footer position; any other parent returns the document
// unchanged.
//
// Import-free by construction: test/run.cjs compiles this file standalone.

type TBlock = { type?: string; data?: any };
type TDocument = Record<string, TBlock>;

type TInsertContext = { brand?: string | null } | null | undefined;

function presentKinds(doc: TDocument): Set<string> {
  // Walk from root, inlined (the module stays import-free): the EmailLayout's childrenIds, a
  // Container's props.childrenIds, every ColumnsContainer column. Cycle-safe.
  const kinds = new Set<string>();
  const seen = new Set<string>();
  const stack = ['root'];
  while (stack.length) {
    const id = stack.pop() as string;
    if (seen.has(id)) continue;
    seen.add(id);
    const b = doc[id];
    if (!b) continue;
    const data = b.data || {};
    if (b.type === 'OfficialFooter' && data.props && typeof data.props.kind === 'string') {
      kinds.add(data.props.kind);
    } else if (b.type === 'EmailLayout' && Array.isArray(data.childrenIds)) {
      stack.push(...data.childrenIds);
    } else if (b.type === 'Container' && data.props && Array.isArray(data.props.childrenIds)) {
      stack.push(...data.props.childrenIds);
    } else if (b.type === 'ColumnsContainer' && data.props && Array.isArray(data.props.columns)) {
      for (const c of data.props.columns) {
        if (c && Array.isArray(c.childrenIds)) stack.push(...c.childrenIds);
      }
    }
  }
  return kinds;
}

function freshId(doc: TDocument, kind: string, taken: Set<string>): string {
  const base = `block-official-${kind}-${Date.now()}`;
  let id = base;
  let n = 1;
  while (id in doc || taken.has(id)) {
    id = `${base}-${n}`;
    n += 1;
  }
  taken.add(id);
  return id;
}

export function insertOfficialFooter(
  document: TDocument,
  parentId: string,
  index: number,
  // Kept for the callers' contract; since CURATED-FOOTER-SPEC D2 no brand changes what is
  // inserted (the blocks resolve their reference from the context at render time).
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  context: TInsertContext
): TDocument {
  const parent = document[parentId];
  if (!parent || (parent.type !== 'EmailLayout' && parent.type !== 'Container')) {
    return document;
  }

  const present = presentKinds(document);
  const wanted: string[] = [];
  if (!present.has('brand')) {
    wanted.push('brand');
  }
  if (!present.has('corporate')) {
    wanted.push('corporate');
  }
  if (wanted.length === 0) {
    return document;
  }

  const taken = new Set<string>();
  const next: TDocument = { ...document };
  const ids = wanted.map((kind) => {
    const id = freshId(document, kind, taken);
    next[id] = { type: 'OfficialFooter', data: { props: { kind } } };
    return id;
  });

  const current: string[] =
    parent.type === 'EmailLayout'
      ? [...((parent.data && parent.data.childrenIds) || [])]
      : [...((parent.data && parent.data.props && parent.data.props.childrenIds) || [])];
  const at = Math.max(0, Math.min(Number.isFinite(index) ? Math.trunc(index) : current.length, current.length));
  current.splice(at, 0, ...ids);

  if (parent.type === 'EmailLayout') {
    next[parentId] = { ...parent, data: { ...(parent.data || {}), childrenIds: current } };
  } else {
    const data = parent.data || {};
    next[parentId] = { ...parent, data: { ...data, props: { ...(data.props || {}), childrenIds: current } } };
  }
  return next;
}
