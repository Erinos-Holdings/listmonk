import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';

import getConfiguration from '../../getConfiguration';

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

  selectedBlockId: string | null;
  selectedSidebarTab: 'block-configuration' | 'styles';
  selectedMainTab: 'editor' | 'preview' | 'json' | 'html';
  selectedScreenSize: 'desktop' | 'mobile';

  inspectorDrawerOpen: boolean;
  samplesDrawerOpen: boolean;

  brandPalettes: TBrandPalette[];
};

const editorStateStore = create(subscribeWithSelector<TValue>(() => ({
  document: getConfiguration(window.location.hash),
  selectedBlockId: null,
  selectedSidebarTab: 'styles',
  selectedMainTab: 'editor',
  selectedScreenSize: 'desktop',

  inspectorDrawerOpen: true,
  samplesDrawerOpen: true,

  brandPalettes: [],
})));

export function useDocument() {
  return editorStateStore((s) => s.document);
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
