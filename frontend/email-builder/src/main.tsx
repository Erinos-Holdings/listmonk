import React from 'react';
import ReactDOM from 'react-dom/client';
import App, { AppProps, DEFAULT_SOURCE } from './App';
import {
  setDocument,
  resetDocument,
  setBrandPalettes,
  setOfficialFooters,
  setOfficialContext,
  getDocument,
  setSelectedBlockId,
  setSelectedMainTab,
} from './documents/editor/EditorContext';
import { blockNumbers } from './documents/structure';
import { remapColors } from './remapColors';
import { officialProjection } from './official/projection';
import { insertOfficialFooter } from './official/insert';
import { compileDocument } from './utils';

import { CssBaseline, ThemeProvider } from '@mui/material';
import theme from './theme';

function isRendered(containerId: string): boolean {
  const container = document.getElementById(containerId);
  if (!container) {
    console.error(`Container with id ${containerId} not found`);
    return false;
  }
  return container.hasChildNodes();
}

function render(containerId: string, props: AppProps, force: boolean = false) {
  if (!isRendered(containerId) || force) {
    const container = document.getElementById(containerId);
    if (!container) return;

    ReactDOM.createRoot(container).render(
      <React.StrictMode>
        <ThemeProvider theme={theme}>
          <CssBaseline />
          <App {...props} />
        </ThemeProvider>
      </React.StrictMode>
    );
  }
}

// Fork (review navigation) -- integrations REVIEW-NAVIGATION-SPEC §5.3: select a block from the
// host (the Inspect window's block reference). Only a block that has a number (rendered: not an
// orphan, not in a hidden column) is selectable. Selects it on the Edit tab and scrolls its
// wrapper into view; writes nothing to the document (I15). Returns whether it selected.
function selectBlock(id: string): boolean {
  if (typeof id !== 'string' || !blockNumbers(getDocument() as any).has(id)) {
    return false;
  }
  setSelectedMainTab('editor');
  setSelectedBlockId(id);
  // The wrapper renders on the next frame after the tab switch; scroll once it is there.
  const scroll = () => {
    const sel = `[data-lm-block-id="${typeof CSS !== 'undefined' && typeof CSS.escape === 'function' ? CSS.escape(id) : id.replace(/["\\]/g, '\\$&')}"]`;
    const el = document.querySelector(sel) as HTMLElement | null;
    // jsdom (the structure tests) implements no scrollIntoView.
    if (el && typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ block: 'center' });
    }
  };
  scroll();
  setTimeout(scroll, 50);
  return true;
}

// Fork (official footer) -- OFFICIAL-FOOTER-SPEC §3.1: setOfficialFooters/setOfficialContext
// (the host's references + resolution context, D2/D8), officialProjection (the ONE projection
// and hash, D5), compileDocument (the headless compile the re-save sweep uses, D11) and
// insertOfficialFooter (the Insert action's pure transform, D7).
export {
  App,
  setDocument,
  resetDocument,
  setBrandPalettes,
  remapColors,
  render,
  isRendered,
  DEFAULT_SOURCE,
  setOfficialFooters,
  setOfficialContext,
  officialProjection,
  compileDocument,
  insertOfficialFooter,
  selectBlock,
};
