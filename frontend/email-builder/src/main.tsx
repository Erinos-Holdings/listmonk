import React from 'react';
import ReactDOM from 'react-dom/client';
import App, { AppProps, DEFAULT_SOURCE } from './App';
import { setDocument, resetDocument, setBrandPalettes, setOfficialFooters, setOfficialContext } from './documents/editor/EditorContext';
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
};
