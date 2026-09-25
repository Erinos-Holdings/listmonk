import { createContext } from 'react';

import type { TOfficialContext, TOfficialRef } from '../../../official/resolve';

// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D9. What the READER needs to resolve an
// OfficialFooter block, threaded as a React context so the reader stays store-free:
// renderToStaticMarkup provides it from the value renderHtmlWithMeta/compileDocument read once
// from the store; the canvas provides the same shape from the store's hooks.
//
// `nested` is set while a reference's own blocks are rendering: an OfficialFooter met there
// renders nothing (depth 1 only -- a reference can never recurse, D4).
export type TOfficialRender = {
  refs: TOfficialRef[];
  context: TOfficialContext | null;
  nested?: boolean;
};

export const OfficialRenderContext = createContext<TOfficialRender | null>(null);

// Parsed reference documents, keyed by the body_source string (refs arrive as JSON text).
const parsed = new Map<string, Record<string, any> | null>();

export function parseReference(bodySource: string | null | undefined): Record<string, any> | null {
  const key = String(bodySource ?? '');
  if (parsed.has(key)) {
    return parsed.get(key) ?? null;
  }
  let doc: Record<string, any> | null = null;
  try {
    const v = JSON.parse(key);
    doc = v && typeof v === 'object' && v.root ? v : null;
  } catch (e) {
    doc = null;
  }
  if (parsed.size > 200) {
    parsed.clear();
  }
  parsed.set(key, doc);
  return doc;
}

export const OFFICIAL_LOCK_HINT = 'Official footer — edit the Official_… template, not the campaign.';
