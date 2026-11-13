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

// Fork (integrations EDITOR-DEFAULTS-SPEC D2/I3) -- where the first reachable footer of each kind
// sits: its parent, the index in that parent's slot, and whether the slot is a Columns column (not
// a footer position). The same root walk as presentKinds, recording positions. (The spec names
// documents/structure.ts::parentOf as the locator; this module must stay import-free -- test/run.cjs
// compiles it standalone and asserts that -- so the walk is inlined here.)
type TFooterPos = { parentId: string; index: number; column: boolean };

function footerPositions(doc: TDocument): Record<string, TFooterPos> {
  const out: Record<string, TFooterPos> = {};
  const seen = new Set<string>();
  const visit = (id: string, pos: TFooterPos | null) => {
    if (seen.has(id)) return;
    seen.add(id);
    const b = doc[id];
    if (!b) return;
    const data = b.data || {};
    if (b.type === 'OfficialFooter' && data.props && typeof data.props.kind === 'string') {
      if (pos && !(data.props.kind in out)) out[data.props.kind] = pos;
      return;
    }
    if (b.type === 'EmailLayout' && Array.isArray(data.childrenIds)) {
      data.childrenIds.forEach((c: string, i: number) => visit(c, { parentId: id, index: i, column: false }));
    } else if (b.type === 'Container' && data.props && Array.isArray(data.props.childrenIds)) {
      data.props.childrenIds.forEach((c: string, i: number) => visit(c, { parentId: id, index: i, column: false }));
    } else if (b.type === 'ColumnsContainer' && data.props && Array.isArray(data.props.columns)) {
      for (const col of data.props.columns) {
        if (col && Array.isArray(col.childrenIds)) col.childrenIds.forEach((c: string, i: number) => visit(c, { parentId: id, index: i, column: true }));
      }
    }
  };
  visit('root', null);
  return out;
}

function rootLength(doc: TDocument): number {
  const r = doc.root;
  return r && r.data && Array.isArray(r.data.childrenIds) ? r.data.childrenIds.length : 0;
}

// Fork (integrations EDITOR-DEFAULTS-SPEC D2/I3) -- the ONE helper the campaign editor's first load
// and the template import share, over insertOfficialFooter: both kinds present -> the SAME
// document; neither -> the pair at the end of root; one kind only -> the missing kind next to it in
// brand -> corporate order (the brand block before an existing corporate block, the corporate block
// after an existing brand block); the existing one in a Columns column (not a footer position) ->
// the missing kind at the end of root. Returns a NEW document when anything was inserted.
export function withOfficialFooters(document: TDocument): TDocument {
  if (!document || !document.root || document.root.type !== 'EmailLayout') {
    return document;
  }
  const pos = footerPositions(document);
  const brand = pos.brand;
  const corporate = pos.corporate;
  if (brand && corporate) {
    return document;
  }
  if (!brand && !corporate) {
    return insertOfficialFooter(document, 'root', rootLength(document), null);
  }
  const existing = (brand || corporate) as TFooterPos;
  if (existing.column) {
    return insertOfficialFooter(document, 'root', rootLength(document), null);
  }
  // brand present -> corporate right after it; corporate present -> brand right before it.
  return insertOfficialFooter(document, existing.parentId, brand ? existing.index + 1 : existing.index, null);
}

// Fork (integrations EDITOR-DEFAULTS-SPEC D2/I2) -- the document the builder opens a NULL
// body_source with: in the campaign editor (the host's context carries `campaign: true`) the
// footer pair is added; in the template editor (no `campaign`) or while an Official_ template is
// edited (`official`) the document is returned unchanged (the SAME object). Stage 4 S4-4: so is a
// campaign whose STORED body is not empty (`storedBody` -- a visual row written with a body but no
// source keeps its body); the template-import path passes no body.
export function firstLoadDocument(
  document: TDocument,
  context: { campaign?: boolean; official?: boolean } | null | undefined,
  storedBody?: string | null
): TDocument {
  if (!context || context.campaign !== true || context.official) {
    return document;
  }
  if (typeof storedBody === 'string' && storedBody.trim() !== '') {
    return document;
  }
  return withOfficialFooters(document);
}
