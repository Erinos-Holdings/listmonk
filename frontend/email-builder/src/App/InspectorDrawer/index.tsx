import React, { useEffect } from 'react';

import {
  Box, Drawer, Tab, Tabs,
} from '@mui/material';

import { setSidebarTab, useInspectorDrawerOpen, useSelectedSidebarTab } from '../../documents/editor/EditorContext';

import ConfigurationPanel from './ConfigurationPanel';
import StylesPanel from './StylesPanel';

// Widened from upstream's 320: campaigns are authored in this panel (the
// canvas is preview-only, often viewed in mobile width), so give it the room.
// On narrow windows the drawer gives its extra width back first — the canvas
// keeps full 600px desktop-preview visibility until the drawer is back to the
// upstream minimum.
export const INSPECTOR_DRAWER_MAX_WIDTH = 640;
export const INSPECTOR_DRAWER_MIN_WIDTH = 320;
// 600px canvas plus its surrounding gutter.
const CANVAS_RESERVE = 660;

// The computed width travels as a CSS custom property, NOT React state:
// App's render body calls setDocument (new document reference every call) and
// re-registers document subscribers, so a state-driven width would turn every
// resize tick into leaked subscriptions and full-document HTML re-renders.
const INSPECTOR_DRAWER_WIDTH_VAR = '--inspector-drawer-width';
export const INSPECTOR_DRAWER_WIDTH_CSS = `var(${INSPECTOR_DRAWER_WIDTH_VAR}, ${INSPECTOR_DRAWER_MAX_WIDTH}px)`;

function applyInspectorDrawerWidth() {
  // The builder runs inside the visual-editor iframe (VisualEditor.vue), so
  // the available width IS this document's viewport. Do not measure
  // .email-builder-container — that class sits on the iframe element in the
  // PARENT document and matches nothing in here.
  const available = window.innerWidth - CANVAS_RESERVE;
  const width = Math.min(INSPECTOR_DRAWER_MAX_WIDTH, Math.max(INSPECTOR_DRAWER_MIN_WIDTH, available));
  document.documentElement.style.setProperty(INSPECTOR_DRAWER_WIDTH_VAR, `${width}px`);
}

function useInspectorDrawerWidthVar() {
  useEffect(() => {
    applyInspectorDrawerWidth();
    if (typeof ResizeObserver !== 'undefined') {
      const observer = new ResizeObserver(applyInspectorDrawerWidth);
      observer.observe(document.documentElement);
      return () => observer.disconnect();
    }
    window.addEventListener('resize', applyInspectorDrawerWidth);
    return () => window.removeEventListener('resize', applyInspectorDrawerWidth);
  }, []);
}

export default function InspectorDrawer() {
  const selectedSidebarTab = useSelectedSidebarTab();
  const inspectorDrawerOpen = useInspectorDrawerOpen();
  useInspectorDrawerWidthVar();

  const renderCurrentSidebarPanel = () => {
    switch (selectedSidebarTab) {
      case 'block-configuration':
        return <ConfigurationPanel />;
      case 'styles':
        return <StylesPanel />;
    }
  };

  return (
    <Drawer
      variant="persistent"
      anchor="right"
      className="sidebar"
      open={inspectorDrawerOpen}
      sx={{
        width: inspectorDrawerOpen ? INSPECTOR_DRAWER_WIDTH_CSS : 0,
      }}
      // Make the drawer relative to the wrapper instead of body.
      PaperProps={{ style: { position: 'absolute', zIndex: 0 } }}
      ModalProps={{
        container: document.querySelector('.email-builder-container'),
        style: { position: 'absolute', zIndex: 0 },
      }}
    >
      <Box sx={{
        width: INSPECTOR_DRAWER_WIDTH_CSS, height: 49, borderBottom: 1, borderColor: 'divider',
      }}
      >
        <Box px={2}>
          <Tabs value={selectedSidebarTab} onChange={(_, v) => setSidebarTab(v)}>
            <Tab value="styles" label="Styles" />
            <Tab value="block-configuration" label="Inspect" />
          </Tabs>
        </Box>
      </Box>
      <Box sx={{ width: INSPECTOR_DRAWER_WIDTH_CSS, height: 'calc(100% - 49px)', overflow: 'auto' }}>
        {renderCurrentSidebarPanel()}
      </Box>
    </Drawer>
  );
}
