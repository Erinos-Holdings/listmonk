// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D7. The Add-block menu's "Official footer"
// entry as a pure document transform: the menu's onSelect contract inserts ONE block, the
// footer is two. Inserts, at `index` in `parentId`'s children, the brand block (unless the
// context brand is `curated`, which has no brand footer by design) followed by the corporate
// block. A kind already present ANYWHERE in the document is not inserted again (idempotent).
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
  const kinds = new Set<string>();
  for (const b of Object.values(doc)) {
    if (b && b.type === 'OfficialFooter' && b.data && b.data.props && typeof b.data.props.kind === 'string') {
      kinds.add(b.data.props.kind);
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
  context: TInsertContext
): TDocument {
  const parent = document[parentId];
  if (!parent || (parent.type !== 'EmailLayout' && parent.type !== 'Container')) {
    return document;
  }

  const present = presentKinds(document);
  const brand = context && context.brand !== undefined && context.brand !== null ? String(context.brand) : null;
  const isCurated = brand !== null && brand.toLowerCase().replace(/[^a-z0-9]/g, '') === 'curated';
  const wanted: string[] = [];
  if (!isCurated && !present.has('brand')) {
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
