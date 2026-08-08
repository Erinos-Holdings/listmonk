const path = require('path');
const fs = require('fs');
const { createRequire } = require('module');

// Selection math for the Text block's markdown toolbar (markdownFormat.ts).
// The transforms are pure string functions; what matters is that the emitted
// syntax is exactly what marked+insane accept (** / _ / [](): markdown;
// span[style]: raw HTML the sanitizer allowlists) and that the returned
// selection lands where the component will restore it — on the wrapped text,
// or on the URL placeholder so typing replaces it.
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
      jsx: ts.JsxEmit.React,
    },
  }).outputText;
}

function evaluate(js, requireMap = {}) {
  const mod = { exports: {} };
  const req = (name) => {
    if (name in requireMap) return requireMap[name];
    throw new Error(`unexpected import "${name}" — add it to the test's requireMap`);
  };
  new Function('module', 'exports', 'require', js)(mod, mod.exports, req);
  return mod.exports;
}

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

const fmt = evaluate(
  transpile(path.join('App', 'InspectorDrawer', 'ConfigurationPanel', 'input-panels', 'helpers', 'inputs', 'markdownFormat.ts')),
);

function selected(r) { return r.text.slice(r.selectionStart, r.selectionEnd); }

// applyWrap — bold/italic markers around the selection, selection follows the text.
let r = fmt.applyWrap('hello world', 0, 5, '**');
check('bold wraps the selection', r.text === '**hello** world', r.text);
check('bold keeps the wrapped text selected', selected(r) === 'hello', selected(r));

r = fmt.applyWrap('hello world', 6, 11, '_');
check('italic wraps the selection', r.text === 'hello _world_', r.text);
check('italic keeps the wrapped text selected', selected(r) === 'world', selected(r));

r = fmt.applyWrap('ab', 1, 1, '**');
check('collapsed bold inserts markers with the caret between them',
  r.text === 'a****b' && r.selectionStart === 3 && r.selectionEnd === 3,
  `${r.text} @${r.selectionStart},${r.selectionEnd}`);

// applyEnclose — asymmetric wrappers (underline is inline HTML: markdown has
// no underline syntax, and the sanitizer allowlists <u>).
r = fmt.applyEnclose('hello world', 0, 5, '<u>', '</u>');
check('underline wraps the selection', r.text === '<u>hello</u> world', r.text);
check('underline keeps the wrapped text selected', selected(r) === 'hello', selected(r));

// Cross-paragraph selections: each paragraph segment gets its own tag pair so
// no pair ever crosses a markdown paragraph break (invalid HTML in Outlook).
r = fmt.applyEnclose('one\n\ntwo', 0, 8, '<u>', '</u>');
check('underline across a blank line wraps each paragraph separately',
  r.text === '<u>one</u>\n\n<u>two</u>', r.text);
check('cross-paragraph underline selects the whole wrapped run',
  selected(r) === '<u>one</u>\n\n<u>two</u>', selected(r));

r = fmt.applyEnclose('one\n \ntwo', 0, 9, '<u>', '</u>');
check('whitespace-only separator lines still count as paragraph breaks',
  r.text === '<u>one</u>\n \n<u>two</u>', r.text);

r = fmt.applyColor('one\n\ntwo', 0, 8, '#E11D48');
check('color across a blank line wraps each paragraph separately',
  r.text === '<span style="color: #E11D48">one</span>\n\n<span style="color: #E11D48">two</span>', r.text);

// applyBulletList — expands to whole lines, idempotent per line.
r = fmt.applyBulletList('first\nsecond\nthird', 8, 8);
check('collapsed bullet prefixes the caret line only', r.text === 'first\n- second\nthird', r.text);

r = fmt.applyBulletList('intro\nalpha\nbeta\noutro', 8, 14);
check('bullet expands the selection to whole lines', r.text === 'intro\n- alpha\n- beta\noutro', r.text);
check('bullet selects the transformed lines', selected(r) === '- alpha\n- beta', selected(r));

r = fmt.applyBulletList('- already\nfresh', 0, 15);
check('bullet skips lines that already have one', r.text === '- already\n- fresh', r.text);

r = fmt.applyBulletList('alpha\nbeta', 0, 6);
check('selection ending past the trailing newline does not bullet the next line',
  r.text === '- alpha\nbeta', r.text);

// applyLink — selection becomes the label, the URL placeholder is selected.
r = fmt.applyLink('visit google now', 6, 12);
check('link wraps the selection as the label', r.text === 'visit [google](https://) now', r.text);
check('link selects the URL placeholder', selected(r) === 'https://', selected(r));

r = fmt.applyLink('', 0, 0);
check('collapsed link inserts a label placeholder', r.text === '[link text](https://)', r.text);
check('collapsed link still selects the URL placeholder', selected(r) === 'https://', selected(r));

// applyColor — inline HTML span; block-text's sanitizer allowlists span[style].
r = fmt.applyColor('make this pop', 5, 9, '#E11D48');
check('color wraps the selection in a styled span',
  r.text === 'make <span style="color: #E11D48">this</span> pop', r.text);
check('color keeps the wrapped text selected', selected(r) === 'this', selected(r));

r = fmt.applyColor('ab', 1, 1, '#0284C7');
check('collapsed color inserts a placeholder label',
  r.text === 'a<span style="color: #0284C7">colored text</span>b', r.text);
check('collapsed color selects the placeholder', selected(r) === 'colored text', selected(r));

// New-block constructor default: markdown ON (the ribbon writes markdown, and
// the sidebar hides the toggle once it is on). Stored blocks keep their saved
// flag; the conversion constructor (frontend/src/components/editor.js) sets
// markdown: true on its own. Pinned here per the fork's defaults-are-tested
// convention (see new-document-default.test.cjs).
const { BUTTONS } = evaluate(
  transpile(path.join('documents', 'blocks', 'helpers', 'EditorChildrenIds', 'AddBlockMenu', 'buttons.tsx')),
  { react: builderRequire('react'), '@mui/icons-material': builderRequire('@mui/icons-material') },
);
const textButton = BUTTONS.find((b) => b.label === 'Text');
const newText = textButton && textButton.block();
check('new Text blocks default markdown ON', newText && newText.data.props.markdown === true,
  newText ? `markdown=${JSON.stringify(newText.data.props.markdown)}` : 'Text button not found');

process.exit(failed > 0 ? 1 : 0);
