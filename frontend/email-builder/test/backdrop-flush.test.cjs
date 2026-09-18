// The EmailLayout backdrop padding is a layout setting (backdropPadding) that defaults to 0 —
// upstream hard-coded 32px, which showed as a gray bar above the header and below the footer
// on phones — and the compiled <body> zeroes its margin so a client's default 8px does not
// put a band back.
//
// run.cjs compiles postProcess.ts alone, so the reader/editor call sites are pinned as text
// (same approach as font-family-parity), the one reading of the value (getBackdropPadding) is
// transpiled and run for real (same approach as new-document-default), and the post-processor is run on the shape the
// reader now emits. campaign10.test.cjs keeps pinning the legacy stored-body path (32px on
// the wrapper td) — stored bodies carry it until they are re-saved.
const fs = require('fs');
const path = require('path');
const { pp, makeChecker, decodeSafe, modernText } = require('./_fixtures-hardening.cjs');
const { check, done } = makeChecker();

const root = path.join(__dirname, '..');
const read = (...p) => fs.readFileSync(path.join(root, ...p), 'utf8');

// ---- source pins ------------------------------------------------------------------------
const reader = read('src', 'documents', 'reader', 'core.tsx');
const editor = read('src', 'documents', 'blocks', 'EmailLayout', 'EmailLayoutEditor.tsx');
const schema = read('src', 'documents', 'blocks', 'EmailLayout', 'EmailLayoutPropsSchema.tsx');
const sidebar = read('src', 'App', 'InspectorDrawer', 'ConfigurationPanel', 'input-panels', 'EmailLayoutSidebarPanel.tsx');
const render = read('src', 'documents', 'reader', 'renderToStaticMarkup.tsx');
const tpl = read('..', '..', 'static', 'email-templates', 'default-visual.tpl');

// One shared call in BOTH renderers, so the canvas and the sent mail cannot disagree.
const PADDING_EXPR = 'padding: `${getBackdropPadding(props.backdropPadding)}px 0`,';
const BACKDROP_LITERAL = /padding: '\d+px 0'/; // the upstream hard-coded form
check('reader backdrop reads the setting, no hard-coded padding', reader.includes(PADDING_EXPR) && !BACKDROP_LITERAL.test(reader));
check('editor backdrop reads the setting, no hard-coded padding', editor.includes(PADDING_EXPR) && !BACKDROP_LITERAL.test(editor));
check('schema carries backdropPadding UNBOUNDED (bounds would dead-lock the sidebar safeParse)',
  /backdropPadding: z\.number\(\)\.optional\(\)\.nullable\(\)/.test(schema));
check('sidebar exposes the control through the same helper',
  /defaultValue=\{getBackdropPadding\(data\.backdropPadding\)\}/.test(sidebar) && /updateData\(\{ \.\.\.data, backdropPadding \}\)/.test(sidebar));

// ---- the helper, for real ---------------------------------------------------------------
const ts = require(path.join(root, 'node_modules', 'typescript'));
const { createRequire } = require('module');
const builderRequire = createRequire(path.join(root, 'package.json'));
const js = ts.transpileModule(schema, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, esModuleInterop: true } }).outputText;
const mod = { exports: {} };
new Function('module', 'exports', 'require', js)(mod, mod.exports, (n) => { if (n === 'zod') return builderRequire('zod'); throw new Error(`unexpected import ${n}`); });
const { getBackdropPadding, default: Schema } = mod.exports;
for (const [input, want] of [[undefined, 0], [null, 0], [0, 0], [32, 32], [12, 12], [-5, 0], [100, 64], [4.5, 5], ['abc', 0], [NaN, 0], ['32', 32]]) {
  check(`getBackdropPadding(${JSON.stringify(input) ?? 'undefined'}) === ${want}`, getBackdropPadding(input) === want, `got ${getBackdropPadding(input)}`);
}
check('schema accepts absent, null, 0, 32 and an out-of-range stored value (sidebar stays live)',
  [{}, { backdropPadding: null }, { backdropPadding: 0 }, { backdropPadding: 32 }, { backdropPadding: 500 }].every((d) => Schema.safeParse(d).success));
check('schema keeps the key through a parse (not stripped on sidebar updates)', Schema.safeParse({ backdropPadding: 32 }).data.backdropPadding === 32);

check('compiled <body> zeroes margin and padding', /<body style=\{\{ margin: '0', padding: '0' \}\}>/.test(render));
check('default-visual.tpl: backdrop padding 0, body margin 0',
  /margin:0;padding:0px 0;min-height:100%/.test(tpl) && !/padding:32px/.test(tpl) && /<body style="margin:0;padding:0">/.test(tpl));

// ---- post-processor on the zero-padding shape -------------------------------------------
const docWith = (padding) => `<!doctype html><html><head></head><body style="margin:0;padding:0">
<div style="background-color:#F5F5F5;color:#262626;font-size:16px;margin:0;padding:${padding};min-height:100%;width:100%">
<table align="center" width="100%" style="margin:0 auto;max-width:600px;background-color:#FFFFFF"><tbody><tr><td>
${modernText}
</td></tr></tbody></table>
</div>
</body></html>`;
const doc = docWith('0px 0');

const on = decodeSafe(pp.postProcess(doc, { outlook: true }));
check('outlook:true — wrapper td carries the backdrop colour and NO padding',
  /<td align="center" bgcolor="#F5F5F5" style="background-color:#F5F5F5">/.test(on), on.slice(0, 600));
check('outlook:true — no empty style attribute, and the backdrop div gave its padding up',
  !/style=""/.test(on) && !/<div style="background-color:#F5F5F5;[^"]*padding/.test(on));
check('outlook:true — ghost table still pins the canvas width', /<!--\[if mso\]><table[^>]*width="600"/.test(on));

const off = pp.postProcess(doc, { outlook: false });
check('outlook:false — backdrop div kept, padding 0', /<div style="background-color:#F5F5F5;[^"]*padding:0px 0;[^"]*min-height:100%/.test(off), off.slice(0, 400));

for (const [tag, out] of [['outlook:true', on], ['outlook:false', off]]) {
  check(`${tag} — body margin reset survives the post-processor`, /<body[^>]*style="[^"]*margin:\s*0/.test(out), (out.match(/<body[^>]*>/) || [''])[0]);
}

const mid = decodeSafe(pp.postProcess(docWith('12px 0'), { outlook: true }));
check('backdropPadding 12, outlook:true — wrapper td carries 12px top and bottom',
  /style="background-color:#F5F5F5;padding:12px 0px 12px 0px"/.test(mid), mid.slice(0, 600));

// ---- opt-in card look: backdropPadding 32 compiles as the legacy shape --------------------
const card = decodeSafe(pp.postProcess(docWith('32px 0'), { outlook: true }));
check('backdropPadding 32, outlook:true — padding moves onto the wrapper td (Word drops div padding)',
  /<td align="center" bgcolor="#F5F5F5" style="background-color:#F5F5F5;padding:32px 0px 32px 0px">/.test(card), card.slice(0, 600));
check('backdropPadding 32, outlook:false — backdrop div keeps it',
  /padding:32px 0;/.test(pp.postProcess(docWith('32px 0'), { outlook: false })));

done();
