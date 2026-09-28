// Fork (container structure) -- CONTAINER-NESTING-SPEC §2.1. The ONE tree model of a builder
// document. Every structure action the editor offers (Select parent, Insert above/below,
// Duplicate, Unwrap, Delete, the breadcrumb, the handle tabs' depth and ⚠ flag) goes through
// these functions; no component walks the document itself.
//
// Pure functions: none mutates its input. A block's children live in a SLOT:
//   - the EmailLayout's data.childrenIds                      {parentId, column: null}
//   - a Container's data.props.childrenIds                    {parentId, column: null}
//   - column i of a ColumnsContainer's data.props.columns[i]  {parentId, column: i}
// Columns props are a fixed 3-tuple; with columnsCount 2 column index 2 is hidden on the
// canvas but its blocks are still in the tree, so it is treated as a normal slot. Every
// rewrite preserves all other ColumnsContainer props and every other key of a column entry.
//
// Import-free by construction: test/run.cjs compiles this file standalone (I9).

export type TBlock = { type?: string; data?: any };
export type TDocument = Record<string, TBlock>;

export type TSlot = { parentId: string; column: number | null };
export type TPosition = TSlot & { index: number };

export type TWrapperState = { kind: 'empty' | 'single' | 'group'; count: number };

export type TUnwrapRefusal = 'not-container' | 'root' | 'detached' | 'footer-in-column' | 'shared';

const ROOT = 'root';

function isContainerType(type: string | undefined): boolean {
  return type === 'Container' || type === 'ColumnsContainer';
}

// Every slot a block owns, in order. Non-parents own none.
function slotsOfBlock(block: TBlock | undefined): { column: number | null; ids: string[] }[] {
  if (!block || !block.data) {
    return [];
  }
  const data = block.data;
  switch (block.type) {
    case 'EmailLayout':
      return [{ column: null, ids: Array.isArray(data.childrenIds) ? data.childrenIds : [] }];
    case 'Container':
      return [{ column: null, ids: data.props && Array.isArray(data.props.childrenIds) ? data.props.childrenIds : [] }];
    case 'ColumnsContainer': {
      const columns = data.props && Array.isArray(data.props.columns) ? data.props.columns : [];
      return columns.map((c: any, i: number) => ({ column: i, ids: c && Array.isArray(c.childrenIds) ? c.childrenIds : [] }));
    }
    default:
      return [];
  }
}

// The ids in one slot ([] when the slot does not exist).
export function slotIds(doc: TDocument, slot: TSlot): string[] {
  const found = slotsOfBlock(doc[slot.parentId]).find((s) => s.column === slot.column);
  return found ? found.ids : [];
}

// Every child id of a block across all its slots, in slot order.
export function childIdsOf(block: TBlock | undefined): string[] {
  const out: string[] = [];
  for (const s of slotsOfBlock(block)) {
    out.push(...s.ids);
  }
  return out;
}

// Returns a copy of `block` with the slot `column` replaced by `ids`, or null when the block has
// no such slot. Preserves every other prop (and every other key of a column entry).
function withSlot(block: TBlock, column: number | null, ids: string[]): TBlock | null {
  const data = block.data || {};
  switch (block.type) {
    case 'EmailLayout':
      if (column !== null) return null;
      return { ...block, data: { ...data, childrenIds: ids } };
    case 'Container':
      if (column !== null) return null;
      return { ...block, data: { ...data, props: { ...(data.props || {}), childrenIds: ids } } };
    case 'ColumnsContainer': {
      const columns = data.props && Array.isArray(data.props.columns) ? data.props.columns : null;
      if (column === null || !columns || column < 0 || column >= columns.length) return null;
      const nextColumns = columns.map((c: any, i: number) => (i === column ? { ...(c || {}), childrenIds: ids } : c));
      return { ...block, data: { ...data, props: { ...data.props, columns: nextColumns } } };
    }
    default:
      return null;
  }
}

// Block keys in reachability order from root (cycle-safe), then every other key in document
// order. Used so a lookup prefers the parent the canvas actually renders.
function keysReachableFirst(doc: TDocument): string[] {
  const seen = new Set<string>();
  const order: string[] = [];
  const stack = [ROOT];
  while (stack.length) {
    const id = stack.shift() as string;
    if (seen.has(id) || !(id in doc)) continue;
    seen.add(id);
    order.push(id);
    stack.push(...childIdsOf(doc[id]));
  }
  for (const k of Object.keys(doc)) {
    if (!seen.has(k)) order.push(k);
  }
  return order;
}

// Every block id reachable from root (root included).
export function reachableIds(doc: TDocument): Set<string> {
  const seen = new Set<string>();
  const stack = [ROOT];
  while (stack.length) {
    const id = stack.pop() as string;
    if (seen.has(id) || !(id in doc)) continue;
    seen.add(id);
    stack.push(...childIdsOf(doc[id]));
  }
  return seen;
}

// The slot and index holding `id`; null for root and for an id in no slot. When an id sits in
// several slots (hand-edited JSON), a parent reachable from root wins.
export function parentOf(doc: TDocument, id: string): TPosition | null {
  if (id === ROOT) return null;
  for (const parentId of keysReachableFirst(doc)) {
    for (const s of slotsOfBlock(doc[parentId])) {
      const index = s.ids.indexOf(id);
      if (index >= 0) return { parentId, column: s.column, index };
    }
  }
  return null;
}

// How many slots reference `id` (a count > 1 means a shared id).
function referenceCount(doc: TDocument, id: string): number {
  let n = 0;
  for (const block of Object.values(doc)) {
    for (const s of slotsOfBlock(block)) {
      for (const c of s.ids) if (c === id) n += 1;
    }
  }
  return n;
}

// root first, the direct parent last. Stops -- returning what it has -- on a cycle or a
// missing parent, so a detached id yields [] and an id under an orphan yields a partial chain.
export function ancestorsOf(doc: TDocument, id: string): string[] {
  const chain: string[] = [];
  const seen = new Set<string>([id]);
  let cur = parentOf(doc, id);
  while (cur) {
    if (seen.has(cur.parentId)) break;
    seen.add(cur.parentId);
    chain.unshift(cur.parentId);
    if (cur.parentId === ROOT) break;
    cur = parentOf(doc, cur.parentId);
  }
  return chain;
}

// The count of Container/ColumnsContainer ancestors.
export function containerDepth(doc: TDocument, id: string): number {
  return ancestorsOf(doc, id).filter((a) => doc[a] && isContainerType(doc[a].type)).length;
}

// Every existing block id reachable below `id`, depth-first (pre-order); each id once, `id`
// itself never included (a cycle back to it is ignored).
export function descendantsOf(doc: TDocument, id: string): string[] {
  const out: string[] = [];
  const seen = new Set<string>([id]);
  const visit = (pid: string) => {
    for (const c of childIdsOf(doc[pid])) {
      if (seen.has(c) || !(c in doc)) continue;
      seen.add(c);
      out.push(c);
      visit(c);
    }
  };
  visit(id);
  return out;
}

// D2: a Container is unstyled when its style is null/absent or every key is empty.
function isEmptyPadding(p: any): boolean {
  if (p === null || p === undefined) return true;
  if (typeof p !== 'object') return false;
  return p.top === 0 && p.right === 0 && p.bottom === 0 && p.left === 0;
}

export function isUnstyledContainer(block: TBlock | undefined): boolean {
  if (!block || block.type !== 'Container') return false;
  const style = block.data ? block.data.style : null;
  if (style === null || style === undefined) return true;
  if (typeof style !== 'object') return false;
  for (const [key, value] of Object.entries(style)) {
    switch (key) {
      case 'backgroundColor':
      case 'borderColor':
        if (value) return false;
        break;
      case 'borderRadius':
        if (value !== null && value !== undefined && value !== 0) return false;
        break;
      case 'padding':
        if (!isEmptyPadding(value)) return false;
        break;
      default:
        // Any other key with a truthy value makes it styled (fail toward not flagging).
        if (value) return false;
    }
  }
  return true;
}

// D3: null unless the block is an unstyled Container; kind by its direct child count.
export function wrapperState(block: TBlock | undefined): TWrapperState | null {
  if (!isUnstyledContainer(block)) return null;
  const count = childIdsOf(block).length;
  if (count === 0) return { kind: 'empty', count };
  if (count === 1) return { kind: 'single', count };
  return { kind: 'group', count };
}

// D3's wording, shared by the tab tooltip and the Container panel's Alert.
export function wrapperMessage(state: TWrapperState): string {
  switch (state.kind) {
    case 'empty':
      return 'Empty container';
    case 'single':
      return 'Redundant wrapper — it adds nothing around its one block';
    default:
      return `Groups ${state.count} blocks with no styling of its own`;
  }
}

// The styling a D4 confirm lists (what Unwrap loses), human-readable.
export function styleSummary(block: TBlock | undefined): string[] {
  const style = block && block.data ? block.data.style : null;
  if (!style || typeof style !== 'object') return [];
  const out: string[] = [];
  for (const [key, value] of Object.entries(style) as [string, any][]) {
    switch (key) {
      case 'backgroundColor':
        if (value) out.push(`background ${value}`);
        break;
      case 'borderColor':
        if (value) out.push(`border ${value}`);
        break;
      case 'borderRadius':
        if (value !== null && value !== undefined && value !== 0) out.push(`radius ${value}`);
        break;
      case 'padding':
        if (!isEmptyPadding(value)) {
          const p = value && typeof value === 'object' ? value : {};
          out.push(`padding ${[p.top, p.right, p.bottom, p.left].map((v) => (v === undefined || v === null ? '?' : v)).join('/')}`);
        }
        break;
      default:
        if (value) out.push(`${key} ${typeof value === 'object' ? JSON.stringify(value) : String(value)}`);
    }
  }
  return out;
}

// D4/D9: replace the container, in its parent's slot and at its index, with its direct
// children in order, and remove the container key.
export function unwrap(
  doc: TDocument,
  id: string
): { doc: TDocument; firstChildId: string | null } | { refused: TUnwrapRefusal } {
  if (id === ROOT) return { refused: 'root' };
  const block = doc[id];
  if (!block || block.type !== 'Container') return { refused: 'not-container' };
  const pos = parentOf(doc, id);
  if (!pos) return { refused: 'detached' };
  if (referenceCount(doc, id) > 1) return { refused: 'shared' };
  const children = childIdsOf(block);
  if (pos.column !== null && children.some((c) => doc[c] && doc[c].type === 'OfficialFooter')) {
    return { refused: 'footer-in-column' };
  }
  const ids = [...slotIds(doc, pos)];
  ids.splice(pos.index, 1, ...children);
  const parent = withSlot(doc[pos.parentId], pos.column, ids);
  if (!parent) return { refused: 'detached' };
  const next: TDocument = { ...doc, [pos.parentId]: parent };
  delete next[id];
  return { doc: next, firstChildId: children.length ? children[0] : null };
}

// What Unwrap's refusal means, for the disabled button's tooltip.
export function unwrapRefusalMessage(reason: TUnwrapRefusal): string {
  switch (reason) {
    case 'footer-in-column':
      return 'Cannot unwrap: it holds the official footer and sits in a column — a column is not a footer position';
    case 'shared':
      return 'Cannot unwrap: this container is referenced from more than one place';
    case 'detached':
      return 'Cannot unwrap: this container is not in the email';
    case 'root':
      return 'Cannot unwrap the email itself';
    default:
      return 'Only a Container can be unwrapped';
  }
}

// D5: remove a block and every descendant. Removes the keys {id} ∪ descendantsOf(id) except any
// descendant still referenced from a slot outside the removed set (shared ids), and strips every
// removed id from every slot, so no slot is left referencing a removed key. root is refused.
// `removed` counts the removed descendants (the confirm's n).
export function deleteSubtree(doc: TDocument, id: string): { doc: TDocument; removed: number } {
  if (id === ROOT || !(id in doc)) return { doc, removed: 0 };
  const removing = new Set<string>([id, ...descendantsOf(doc, id)]);
  // Spare any descendant referenced from outside the removed set; its own subtree follows it
  // (it is then referenced by a kept block), hence the fixpoint.
  let changed = true;
  while (changed) {
    changed = false;
    for (const [k, block] of Object.entries(doc)) {
      if (removing.has(k)) continue;
      for (const c of childIdsOf(block)) {
        if (c !== id && removing.has(c)) {
          removing.delete(c);
          changed = true;
        }
      }
    }
  }
  const next: TDocument = {};
  for (const [k, block] of Object.entries(doc)) {
    if (removing.has(k)) continue;
    let out = block;
    for (const s of slotsOfBlock(block)) {
      if (s.ids.some((c) => removing.has(c))) {
        out = withSlot(out, s.column, s.ids.filter((c) => !removing.has(c))) || out;
      }
    }
    next[k] = out;
  }
  return { doc: next, removed: removing.size - 1 };
}

// `block-<Date.now()>-<n>`, guaranteed absent from `doc` (and from `taken`, when given).
export function freshId(doc: TDocument, taken?: Set<string>): string {
  const base = `block-${Date.now()}`;
  let n = 0;
  let id = `${base}-${n}`;
  while (id in doc || (taken && taken.has(id))) {
    n += 1;
    id = `${base}-${n}`;
  }
  if (taken) taken.add(id);
  return id;
}

// Insert `block` as `newId` at `index` (clamped) of `slot`. Returns the input unchanged when
// `newId` is already a key or the slot does not exist.
export function insertAt(doc: TDocument, slot: TSlot, index: number, newId: string, block: TBlock): TDocument {
  if (newId in doc) return doc;
  const parent = doc[slot.parentId];
  if (!parent) return doc;
  const ids = [...slotIds(doc, slot)];
  const at = Math.max(0, Math.min(Number.isFinite(index) ? Math.trunc(index) : ids.length, ids.length));
  ids.splice(at, 0, newId);
  const nextParent = withSlot(parent, slot.column, ids);
  if (!nextParent) return doc;
  return { ...doc, [slot.parentId]: nextParent, [newId]: block };
}

// D13: clone `id` and its descendants with fresh ids, placed right after `id` in its slot.
// OfficialFooter descendants are skipped (a second footer pair is never emitted). Refuses
// (input returned, newId null) for root, a missing or detached id, and an OfficialFooter itself
// (OFFICIAL-FOOTER-SPEC D6: a footer is never duplicated from its own menu).
export function duplicateSubtree(doc: TDocument, id: string): { doc: TDocument; newId: string | null } {
  const src = doc[id];
  if (id === ROOT || !src || src.type === 'OfficialFooter') return { doc, newId: null };
  const pos = parentOf(doc, id);
  if (!pos) return { doc, newId: null };

  const taken = new Set<string>();
  const clones: TDocument = {};
  const path = new Set<string>();
  const cloneBlock = (srcId: string): string | null => {
    const b = doc[srcId];
    if (!b || path.has(srcId) || b.type === 'OfficialFooter') return null;
    path.add(srcId);
    const newId = freshId(doc, taken);
    const cloned: TBlock = JSON.parse(JSON.stringify(b));
    let out: TBlock = cloned;
    for (const s of slotsOfBlock(cloned)) {
      const ids = s.ids.map(cloneBlock).filter((c): c is string => c !== null);
      out = withSlot(out, s.column, ids) || out;
    }
    clones[newId] = out;
    path.delete(srcId);
    return newId;
  };
  const newId = cloneBlock(id) as string;

  const ids = [...slotIds(doc, pos)];
  ids.splice(pos.index + 1, 0, newId);
  const parent = withSlot(doc[pos.parentId], pos.column, ids);
  if (!parent) return { doc, newId: null };
  return { doc: { ...doc, ...clones, [pos.parentId]: parent }, newId };
}

// A human label for a block type (the breadcrumb and the tab tooltips).
export function blockLabel(type: string | undefined): string {
  switch (type) {
    case 'EmailLayout':
      return 'Email';
    case 'ColumnsContainer':
      return 'Columns';
    case 'OfficialFooter':
      return 'Official footer';
    default:
      return type || 'Block';
  }
}

// §2.4: `Email › Container › Columns (col 2) › Text`. A Columns segment names the column the
// next segment sits in; the hidden third column of a columnsCount-2 row reads `(col 3, hidden)`.
export function breadcrumb(doc: TDocument, id: string): { id: string; label: string }[] {
  const chain = [...ancestorsOf(doc, id), id];
  return chain.map((cid, i) => {
    const block = doc[cid];
    let label = blockLabel(block && block.type);
    if (block && block.type === 'ColumnsContainer' && i + 1 < chain.length) {
      const pos = parentOf(doc, chain[i + 1]);
      if (pos && pos.parentId === cid && pos.column !== null) {
        // The block's own default is 2 columns (@usewaypoint/block-columns-container).
        const raw = block.data && block.data.props ? block.data.props.columnsCount : undefined;
        const count = raw === 3 ? 3 : 2;
        const hidden = pos.column >= count;
        label = `${label} (col ${pos.column + 1}${hidden ? ', hidden' : ''})`;
      }
    }
    return { id: cid, label };
  });
}
