// Emoji picker shadow styling (integrations EMOJI-PICKER-SPEC D12, invariant I9's builder half).
//
// emojiPickerStyle.js is dependency-free ESM shared with the Vue SPA; the evaluate() harness
// (dark-sim.test.cjs's) fails on any require at all.
const path = require('path');
const fs = require('fs');
const { JSDOM } = require('jsdom');

const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));

const src = fs.readFileSync(path.join(builderRoot, 'src', 'emojiPickerStyle.js'), 'utf8');
const js = ts.transpileModule(src, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;
const mod = { exports: {} };
new Function('module', 'exports', 'require', js)(mod, mod.exports, () => {
  throw new Error('emojiPickerStyle must stay dependency-free');
});
const {
  EMOJI_PICKER_TAB_ICONS, EMOJI_PICKER_SHADOW_CSS, EMOJI_PICKER_STYLE_ATTR, styleEmojiPicker,
} = mod.exports;

let failed = 0;
function check(name, ok, detail) {
  if (!ok) {
    failed++;
    console.log(`FAIL  ${name}${detail ? `  [${detail}]` : ''}`);
  } else {
    console.log(`PASS  ${name}`);
  }
}

// ---- the icon table: the nine tabs the picker renders, each with a drawable path ----------
// (picker.js allGroups minus 'custom': 0, 1, 3, 4, 5, 6, 7, 8, 9 -- group 2 is the skin tones)
const groups = EMOJI_PICKER_TAB_ICONS.map((t) => t.group);
check('nine tabs, in the picker\'s order', JSON.stringify(groups) === '[0,1,3,4,5,6,7,8,9]', JSON.stringify(groups));
check('every icon has a path and a name', EMOJI_PICKER_TAB_ICONS.every((t) => /^M[\d.\s,a-zA-Z-]+z$/.test(t.path) && t.name));
check('names are unique', new Set(EMOJI_PICKER_TAB_ICONS.map((t) => t.name)).size === EMOJI_PICKER_TAB_ICONS.length);

// ---- the sheet: both rows tinted, every tab keyed by its group id, never by position --------
check('tab row tinted', /\.nav\{background:#[0-9a-f]{6}\}/.test(EMOJI_PICKER_SHADOW_CSS));
check('favorites row tinted, same shade', (() => {
  const nav = EMOJI_PICKER_SHADOW_CSS.match(/\.nav\{background:(#[0-9a-f]{6})\}/);
  const fav = EMOJI_PICKER_SHADOW_CSS.match(/\.favorites\{background:(#[0-9a-f]{6})\}/);
  return nav && fav && nav[1] === fav[1];
})());
check('glyph hidden, icon drawn as a mask', /\.nav-emoji\{[^}]*font-size:0/.test(EMOJI_PICKER_SHADOW_CSS) && /\.nav-emoji::after\{[^}]*mask:var\(--erinos-tab-icon\)/.test(EMOJI_PICKER_SHADOW_CSS));
check('active tab takes the indicator colour', EMOJI_PICKER_SHADOW_CSS.includes('.nav-button[aria-selected="true"] .nav-emoji::after{background:var(--indicator-color)}'));
const keyed = (EMOJI_PICKER_SHADOW_CSS.match(/\.nav-button\[data-group-id="\d+"\] \.nav-emoji\{--erinos-tab-icon:url\("data:image\/svg\+xml,/g) || []).length;
check('one data-URI icon rule per tab, keyed by data-group-id', keyed === EMOJI_PICKER_TAB_ICONS.length, `${keyed}`);
check('no positional (nth-child) rule', !EMOJI_PICKER_SHADOW_CSS.includes('nth-child'));
check('no external URL in the sheet', !/url\("?https?:/.test(EMOJI_PICKER_SHADOW_CSS));

// ---- styleEmojiPicker: appends once, into the open shadow root --------------------------------
const dom = new JSDOM('<!doctype html><body></body>');
const { document } = dom.window;
const host = document.createElement('div');
host.attachShadow({ mode: 'open' });
check('appends the sheet on the first call', styleEmojiPicker(host) === true
  && host.shadowRoot.querySelectorAll(`style[${EMOJI_PICKER_STYLE_ATTR}]`).length === 1
  && host.shadowRoot.querySelector('style').textContent === EMOJI_PICKER_SHADOW_CSS);
check('idempotent on the second call', styleEmojiPicker(host) === false
  && host.shadowRoot.querySelectorAll('style').length === 1);
check('no shadow root: a no-op', styleEmojiPicker(document.createElement('div')) === false && styleEmojiPicker(null) === false);

if (failed) {
  console.log(`\n${failed} check(s) failed`);
  process.exit(1);
}
console.log('\nemoji-picker-style: all checks pass');
