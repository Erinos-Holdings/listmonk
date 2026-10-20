// Emoji picker insertion (integrations EMOJI-PICKER-SPEC D5, invariants I1 / I2).
//
// emojiInsert.js is dependency-free ESM shared with the Vue SPA (Campaign.vue); the evaluate()
// harness below (dark-sim.test.cjs's) fails on any require at all.
const path = require('path');
const fs = require('fs');

const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));

const src = fs.readFileSync(path.join(builderRoot, 'src', 'emojiInsert.js'), 'utf8');
const js = ts.transpileModule(src, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;
const mod = { exports: {} };
new Function('module', 'exports', 'require', js)(mod, mod.exports, () => {
  throw new Error('emojiInsert must stay dependency-free');
});
const { insertEmoji, EMOJI_DATA_PATH } = mod.exports;

let failed = 0;
function check(name, ok, detail) {
  if (!ok) {
    failed++;
    console.log(`FAIL  ${name}${detail ? `  [${detail}]` : ''}`);
  } else {
    console.log(`PASS  ${name}`);
  }
}
const show = (r) => JSON.stringify(r);
function same(r, text, caret) {
  return r.text === text && r.selectionStart === caret && r.selectionEnd === caret;
}

// ---- I1: replaces exactly the selection, caret just after the emoji ---------------------
let r = insertEmoji('hello', 0, 0, '😀');
check('I1: caret at start', same(r, '😀hello', 2), show(r));
r = insertEmoji('hello', 2, 2, '😀');
check('I1: caret in the middle', same(r, 'he😀llo', 4), show(r));
r = insertEmoji('hello', 5, 5, '😀');
check('I1: caret at end', same(r, 'hello😀', 7), show(r));
r = insertEmoji('hello world', 6, 11, '🌍');
check('I1: a non-empty selection is replaced', same(r, 'hello 🌍', 8), show(r));
r = insertEmoji('', 0, 0, '😀');
check('I1: empty text', same(r, '😀', 2), show(r));

// Offsets clamp; reversed pairs are put in order; non-numeric means "no selection" -> the end.
r = insertEmoji('hello', 4, 1, 'X');
check('I1: reversed offsets are ordered', same(r, 'hXo', 2), show(r));
r = insertEmoji('hello', -3, 99, 'X');
check('I1: out-of-range offsets clamp to the whole text', same(r, 'X', 1), show(r));
r = insertEmoji('hello', 99, 99, 'X');
check('I1: past-the-end offsets clamp to the end', same(r, 'helloX', 6), show(r));
r = insertEmoji('hello', undefined, undefined, 'X');
check('I1: no recorded selection inserts at the end', same(r, 'helloX', 6), show(r));
r = insertEmoji('hello', NaN, '2', 'X');
check('I1: non-numeric offsets insert at the end', same(r, 'helloX', 6), show(r));

// A multi-code-point emoji: the caret moves by its full UTF-16 length.
const family = '👨‍👩‍👧';
r = insertEmoji('ab', 1, 1, family);
check('I1: a multi-code-point emoji lands whole', same(r, `a${family}b`, 1 + family.length), show(r));
check('I1: the family emoji is 8 UTF-16 units', family.length === 8, String(family.length));

// Empty emoji is the identity on the text.
r = insertEmoji('hello', 1, 3, '');
check('I1: empty emoji leaves the text unchanged',
  r.text === 'hello' && r.selectionStart === 1 && r.selectionEnd === 3, show(r));
r = insertEmoji('hello', 1, 3, undefined);
check('I1: a missing emoji leaves the text unchanged', r.text === 'hello', show(r));

// ATTRIBUTE-PICKER-SPEC I5: the attribute picker inserts through the same rule -- a roster
// expression replaces a mid-text selection whole and the caret lands just after it.
{
  const expr = '{{ or .Subscriber.FirstName "there" }}';
  r = insertEmoji('Hi NAME, welcome', 3, 7, expr);
  check('I5 (attribute picker): a roster expression replaces the mid-text selection',
    same(r, `Hi ${expr}, welcome`, 3 + expr.length), show(r));
}

// ---- I2: nothing but the emoji is inserted ------------------------------------------------
r = insertEmoji('ab', 1, 1, '❤️‍🔥');
check('I2: the ZWJ emoji is inserted verbatim', r.text === 'a❤️‍🔥b', show(r));
for (const [name, emoji] of [['heart on fire', '❤️‍🔥'], ['family', family], ['skin tone', '👍🏽'], ['flag', '🇨🇦']]) {
  const out = insertEmoji('ab', 1, 1, emoji).text;
  check(`I2: ${name} adds no U+200B/U+200C/U+FEFF`, !/[​‌﻿]/.test(out), out);
  check(`I2: ${name} adds no entity`, !out.includes('&'), out);
  check(`I2: ${name} adds exactly the emoji`, out.length === 2 + emoji.length, String(out.length));
}

// The data path is a same-origin admin-surface path (the version pin is src/emojiData.test.mjs's).
check('EMOJI_DATA_PATH is an /admin/static/ path',
  /^\/admin\/static\/emoji-data-\d+\.\d+\.\d+\.json$/.test(EMOJI_DATA_PATH), EMOJI_DATA_PATH);

console.log(failed === 0 ? 'emoji-insert: all checks pass' : `emoji-insert: ${failed} FAILED`);
process.exit(failed === 0 ? 0 : 1);
