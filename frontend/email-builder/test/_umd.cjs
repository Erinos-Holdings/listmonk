// Shared jsdom loader for the BUILT email-builder UMD bundle (the pattern
// official-compile.test.cjs established), plus the compile inputs the CONTAINER-NESTING-SPEC
// I8 snapshot is taken with. Not a suite (no .test.cjs suffix): run.cjs does not execute it.
const path = require('path');
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');

const DEFAULT_UMD = path.join(__dirname, '..', '..', 'public', 'static', 'email-builder', 'email-builder.umd.js');
const FIXTURES = path.join(__dirname, 'fixtures');
const SNAPSHOT = path.join(FIXTURES, 'compile-snapshot.json');

// Loads the bundle into a fresh jsdom window. `onWarn` receives console.warn lines;
// `beforeScript(window)` runs before the bundle is evaluated (e.g. to stub localStorage).
function loadUmd(umdPath = DEFAULT_UMD, { onWarn, onError, beforeScript } = {}) {
  const virtualConsole = new VirtualConsole();
  virtualConsole.on('warn', (m) => onWarn && onWarn(String(m)));
  virtualConsole.on('error', (m) => onError && onError(String(m)));
  virtualConsole.on('jsdomError', (e) => onError && onError(e.message.split('\n')[0]));
  const dom = new JSDOM('<!doctype html><html><head></head><body><div id="visual-editor-container"></div></body></html>', {
    runScripts: 'dangerously',
    pretendToBeVisual: true,
    url: 'https://builder.test/',
    virtualConsole,
  });
  // jsdom 24 has no TextEncoder; react-dom/server's browser build constructs one at load.
  if (typeof dom.window.TextEncoder !== 'function') {
    dom.window.TextEncoder = require('util').TextEncoder;
    dom.window.TextDecoder = require('util').TextDecoder;
  }
  if (beforeScript) beforeScript(dom.window);
  const script = dom.window.document.createElement('script');
  script.textContent = fs.readFileSync(umdPath, 'utf8');
  dom.window.document.head.appendChild(script);
  return { dom, EB: dom.window.EmailBuilder };
}

// The builder-document fixtures: every .json under test/fixtures except the snapshot itself.
// Each is {id, name, body_source, ...}; the document is body_source.
function documentFixtures() {
  return fs.readdirSync(FIXTURES)
    .filter((f) => f.endsWith('.json') && path.join(FIXTURES, f) !== SNAPSHOT)
    .sort()
    .map((f) => ({ file: f, document: JSON.parse(fs.readFileSync(path.join(FIXTURES, f), 'utf8')).body_source }));
}

// The context and references official-compile.test.cjs compiles with.
function compileInputs() {
  const t30 = JSON.parse(fs.readFileSync(path.join(FIXTURES, 'official-template-30.json'), 'utf8'));
  const t14 = JSON.parse(fs.readFileSync(path.join(FIXTURES, 'official-template-14.json'), 'utf8'));
  const refOf = (t) => ({ id: t.id, name: t.name, body_source: JSON.stringify(t.body_source) });
  return { context: { lang: 'en', brand: 'ruze' }, refs: [refOf(t30), refOf(t14)] };
}

module.exports = { DEFAULT_UMD, FIXTURES, SNAPSHOT, loadUmd, documentFixtures, compileInputs };
