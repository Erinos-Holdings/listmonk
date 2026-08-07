const path = require('path');
const fs = require('fs');
const { createRequire } = require('module');

// New-document defaults live in EMPTY_EMAIL_MESSAGE: it is the store's initial
// document (VisualEditor.vue renders with `data: {}`, a no-op merge), while
// stored campaigns arrive via resetDocument() and replace it wholesale. Both
// legs are asserted here: the constant itself, and the EditorContext merge /
// replace semantics the guarantee rests on. Sources are transpiled standalone
// and evaluated — same isolation approach as run.cjs takes for outlook.ts.
const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));
const builderRequire = createRequire(path.join(builderRoot, 'package.json'));

function transpile(relSourcePath) {
  const src = fs.readFileSync(path.join(builderRoot, 'src', relSourcePath), 'utf8');
  return ts.transpileModule(src, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      esModuleInterop: true,
    },
  }).outputText;
}

// requireMap maps import specifiers to module values; anything unmapped throws
// so a future value import fails loudly instead of evaluating half-formed.
function evaluate(js, requireMap = {}) {
  const mod = { exports: {} };
  const req = (name) => {
    if (name in requireMap) return requireMap[name];
    throw new Error(`unexpected import "${name}" — add it to the test's requireMap`);
  };
  new Function('module', 'exports', 'require', 'window', js)(
    mod, mod.exports, req, { location: { hash: '' } },
  );
  return mod.exports;
}

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

// Leg 1: the constant seeds the default.
const EMPTY = evaluate(transpile(path.join('getConfiguration', 'sample', 'empty-email-message.ts'))).default;

check('EMPTY_EMAIL_MESSAGE root is an EmailLayout', EMPTY && EMPTY.root && EMPTY.root.type === 'EmailLayout');
check('new documents default Outlook compatibility ON', EMPTY.root.data.outlook === true, `outlook=${JSON.stringify(EMPTY.root.data.outlook)}`);
check('empty document has no children (right object evaluated)', Array.isArray(EMPTY.root.data.childrenIds) && EMPTY.root.data.childrenIds.length === 0);

// Leg 2: the EditorContext wiring. The store's initial document comes from
// getConfiguration (stubbed to the EMPTY evaluated above); zustand is the real
// dependency. VisualEditor.vue always renders with `data: {}`, so App calls
// setDocument({}) — that merge must preserve the default. Stored campaigns
// arrive via resetDocument(), which must replace the document wholesale.
const ctx = evaluate(transpile(path.join('documents', 'editor', 'EditorContext.tsx')), {
  zustand: builderRequire('zustand'),
  'zustand/middleware': builderRequire('zustand/middleware'),
  '../../getConfiguration': () => EMPTY,
});

let current = null;
ctx.subscribeDocument((doc) => { current = doc; });

ctx.setDocument({});
check('setDocument({}) (new campaign: VisualEditor data:{}) keeps the default ON',
  current && current.root.data.outlook === true,
  current ? `outlook=${JSON.stringify(current.root.data.outlook)}` : 'subscriber never fired');

const storedWithoutOutlook = {
  root: { type: 'EmailLayout', data: { backdropColor: '#FFFFFF', childrenIds: [] } },
};
ctx.resetDocument(storedWithoutOutlook);
check('resetDocument(stored doc) replaces wholesale — no outlook key leaks in',
  current === storedWithoutOutlook && !('outlook' in current.root.data),
  current && `outlook in data: ${'outlook' in current.root.data}`);

// Leg 3: the opt-out round trip. The sidebar toggle writes through
// EmailLayoutPropsSchema.safeParse then setDocument({ root }) (see
// EmailLayoutSidebarPanel updateData / StylesPanel setData), and the
// subscribed document is exactly what VisualEditor.vue's onChange
// JSON.stringifies into body_source — an explicit false must survive all of it.
const EmailLayoutPropsSchema = evaluate(
  transpile(path.join('documents', 'blocks', 'EmailLayout', 'EmailLayoutPropsSchema.tsx')),
  { zod: builderRequire('zod') },
).default;

const optOut = EmailLayoutPropsSchema.safeParse({ ...EMPTY.root.data, outlook: false });
check('schema accepts an explicit outlook:false (opt-out parses)',
  optOut.success && optOut.data.outlook === false,
  optOut.success ? `outlook=${JSON.stringify(optOut.data.outlook)}` : 'parse failed');

ctx.setDocument({ root: { type: 'EmailLayout', data: optOut.success ? optOut.data : {} } });
check('opt-out reaches the emitted save document',
  current && current.root.data.outlook === false,
  current && `outlook=${JSON.stringify(current.root.data.outlook)}`);
check('opt-out survives the body_source JSON round trip',
  JSON.parse(JSON.stringify(current)).root.data.outlook === false);

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
