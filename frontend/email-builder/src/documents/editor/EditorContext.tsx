import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';

import getConfiguration from '../../getConfiguration';
import type { TOfficialContext, TOfficialRef } from '../../official/resolve';

import { TEditorConfiguration } from './core';

// A labeled row of brand colors for the color picker's swatch block. The host page (the admin
// SPA) pushes these in via setBrandPalettes() — the same module-level-store channel
// setDocument()/resetDocument() use to cross the iframe boundary. Rows are N×M by design: a
// brand row is 3 roles (bg/fg/accent) today, a future creator-site row can be 6, and the
// picker renders whatever it is given.
export type TBrandPalette = {
  label: string;
  colors: Array<{ role: string; value: string }>;
};

type TValue = {
  document: TEditorConfiguration;
  // Bumped by resetDocument(). The host mounts the builder BEFORE it loads the stored document
  // (VisualEditor.vue: render, then resetDocument), and the sidebar inputs seed their local
  // state from props once at mount — so the Styles panel is keyed on this to remount with the
  // loaded values instead of the starter document's.
  documentGeneration: number;

  selectedBlockId: string | null;
  selectedSidebarTab: 'block-configuration' | 'styles';
  selectedMainTab: 'editor' | 'preview' | 'json' | 'html';
  selectedScreenSize: 'desktop' | 'mobile';

  inspectorDrawerOpen: boolean;
  samplesDrawerOpen: boolean;

  brandPalettes: TBrandPalette[];

  // Fork (official footer) -- OFFICIAL-FOOTER-SPEC D2/D8. Every `Official_` campaign_visual
  // template ({id, name, body_source}) and the resolution context ({lang, brand, official}),
  // pushed by the host through setOfficialFooters/setOfficialContext.
  officialFooters: TOfficialRef[];
  officialContext: TOfficialContext | null;
};

const editorStateStore = create(subscribeWithSelector<TValue>(() => ({
  document: getConfiguration(window.location.hash),
  documentGeneration: 0,
  selectedBlockId: null,
  selectedSidebarTab: 'styles',
  selectedMainTab: 'editor',
  selectedScreenSize: 'desktop',

  inspectorDrawerOpen: true,
  samplesDrawerOpen: true,

  brandPalettes: [],

  officialFooters: [],
  officialContext: null,
})));

export function useDocument() {
  return editorStateStore((s) => s.document);
}

export function useDocumentGeneration() {
  return editorStateStore((s) => s.documentGeneration);
}

export function subscribeDocument (listener: (selectedState: TEditorConfiguration, previousSelectedState: TEditorConfiguration) => void) {
  editorStateStore.subscribe((state) => state.document, listener)
}

export function useSelectedBlockId() {
  return editorStateStore((s) => s.selectedBlockId);
}

export function useSelectedScreenSize() {
  return editorStateStore((s) => s.selectedScreenSize);
}

export function useSelectedMainTab() {
  return editorStateStore((s) => s.selectedMainTab);
}

export function setSelectedMainTab(selectedMainTab: TValue['selectedMainTab']) {
  return editorStateStore.setState({ selectedMainTab });
}

export function useSelectedSidebarTab() {
  return editorStateStore((s) => s.selectedSidebarTab);
}

export function useInspectorDrawerOpen() {
  return editorStateStore((s) => s.inspectorDrawerOpen);
}

export function useSamplesDrawerOpen() {
  return editorStateStore((s) => s.samplesDrawerOpen);
}

export function useBrandPalettes() {
  return editorStateStore((s) => s.brandPalettes);
}

// Store setter, NOT a re-render: delivering palette changes via render(..., force) would call
// ReactDOM.createRoot() on an already-rooted container (orphaning the old root), and App
// subscribes onChange via subscribeDocument() in its render body with no unsubscribe — each
// forced remount stacks another subscription and every edit thereafter fires onChange N times.
// The store update re-renders only the pickers that read it.
export function setBrandPalettes(brandPalettes: TBrandPalette[]) {
  return editorStateStore.setState({ brandPalettes });
}

export function useOfficialFooters() {
  return editorStateStore((s) => s.officialFooters);
}

export function useOfficialContext() {
  return editorStateStore((s) => s.officialContext);
}

// One read of the official state, for the compile (renderHtmlWithMeta) outside React.
export function getOfficialState(): { refs: TOfficialRef[]; context: TOfficialContext | null } {
  const s = editorStateStore.getState();
  return { refs: s.officialFooters, context: s.officialContext };
}

// OFFICIAL-FOOTER-SPEC D8: a context or references change must regenerate the host's compiled
// body, so the setters re-set the document (new object identity -> the document subscription
// fires onChange) -- WITHOUT resetDocument, which would drop the selection and remount the
// Styles panel. Only once documentGeneration > 0: the host mounts, then loads the stored
// document on a timer, and a re-emit before that would push the empty starter document into
// the host's form. Unchanged values are ignored, so a host watcher re-pushing the same context
// never marks a document dirty.
function reemitDocument() {
  const s = editorStateStore.getState();
  if (s.documentGeneration > 0) {
    editorStateStore.setState({ document: { ...s.document } });
  }
}

function sameJSON(a: unknown, b: unknown): boolean {
  try {
    return JSON.stringify(a) === JSON.stringify(b);
  } catch (e) {
    return false;
  }
}

export function setOfficialFooters(refs: TOfficialRef[] | null | undefined) {
  const next = Array.isArray(refs) ? refs : [];
  if (sameJSON(editorStateStore.getState().officialFooters, next)) {
    return;
  }
  editorStateStore.setState({ officialFooters: next });
  reemitDocument();
}

export function setOfficialContext(context: TOfficialContext | null | undefined) {
  const next = context ?? null;
  if (sameJSON(editorStateStore.getState().officialContext, next)) {
    return;
  }
  editorStateStore.setState({ officialContext: next });
  reemitDocument();
}

export function setSelectedBlockId(selectedBlockId: TValue['selectedBlockId']) {
  const selectedSidebarTab = selectedBlockId === null ? 'styles' : 'block-configuration';
  const options: Partial<TValue> = {};
  if (selectedBlockId !== null) {
    options.inspectorDrawerOpen = true;
  }
  return editorStateStore.setState({
    selectedBlockId,
    selectedSidebarTab,
    ...options,
  });
}

export function setSidebarTab(selectedSidebarTab: TValue['selectedSidebarTab']) {
  return editorStateStore.setState({ selectedSidebarTab });
}

export function resetDocument(document: TValue['document']) {
  return editorStateStore.setState({
    document,
    documentGeneration: editorStateStore.getState().documentGeneration + 1,
    selectedSidebarTab: 'styles',
    selectedBlockId: null,
  });
}

export function setDocument(document: TValue['document']) {
  const originalDocument = editorStateStore.getState().document;
  return editorStateStore.setState({
    document: {
      ...originalDocument,
      ...document,
    },
  });
}

export function toggleInspectorDrawerOpen() {
  const inspectorDrawerOpen = !editorStateStore.getState().inspectorDrawerOpen;
  return editorStateStore.setState({ inspectorDrawerOpen });
}

export function toggleSamplesDrawerOpen() {
  const samplesDrawerOpen = !editorStateStore.getState().samplesDrawerOpen;
  return editorStateStore.setState({ samplesDrawerOpen });
}

export function setSelectedScreenSize(selectedScreenSize: TValue['selectedScreenSize']) {
  return editorStateStore.setState({ selectedScreenSize });
}
