// Attribute picker roster (integrations ATTRIBUTE-PICKER-SPEC D1-D4, invariants I1-I4, I9).
//
// attributes.js is dependency-free ESM shared with the Vue SPA (Campaign.vue,
// RichtextEditor.vue); the evaluate() harness below (emoji-insert.test.cjs's) fails on any
// require at all (I9).
//
// I1: these strings land in live greetings and hrefs -- a change here is the acknowledgment.
// The same literals are pinned in Go (models/attributes_test.go, I1b) and in the integrations
// campaign review (tests/lib/campaign-review/attribute-roster.test.ts, I1c).
const path = require('path');
const fs = require('fs');

const builderRoot = path.join(__dirname, '..');
const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));

const src = fs.readFileSync(path.join(builderRoot, 'src', 'attributes.js'), 'utf8');
const js = ts.transpileModule(src, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;
const mod = { exports: {} };
new Function('module', 'exports', 'require', js)(mod, mod.exports, () => {
  throw new Error('attributes.js must stay dependency-free');
});
const {
  attributeMenu, shopLinkExpr, isTemplateIdent, attributeI18nKey,
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
const show = (r) => JSON.stringify(r);

// ---- I9: the module evaluates standalone and exports the three functions ----------------
check('I9: exports attributeMenu/shopLinkExpr/isTemplateIdent',
  [attributeMenu, shopLinkExpr, isTemplateIdent].every((f) => typeof f === 'function'));

// ---- I1: every expression, by exact string ------------------------------------------------
const TEXT_PINS = {
  first_name: '{{ or .Subscriber.FirstName "there" }}',
  last_name: '{{ .Subscriber.LastName }}',
  name: '{{ .Subscriber.Name }}',
  email: '{{ .Subscriber.Email }}',
};
const LINK_PINS = {
  shop_link: '{{ .Subscriber.Attribs.shop_links.ruze.url }}',
  unsubscribe: '{{ UnsubscribeURL }}',
  message: '{{ MessageURL }}',
};
const text = attributeMenu('text', { brand: 'ruze' }).items;
const link = attributeMenu('link', { brand: 'ruze' }).items;
for (const [id, expr] of Object.entries(TEXT_PINS)) {
  const it = text.find((i) => i.id === id);
  check(`I1: text ${id} is ${expr}`, it && it.expr === expr && it.disabled === false, show(it));
}
for (const [id, expr] of Object.entries(LINK_PINS)) {
  const it = link.find((i) => i.id === id);
  check(`I1: link ${id} is ${expr}`, it && it.expr === expr && it.disabled === false, show(it));
}
check('I1: text items need no brand', show(attributeMenu('text', { brand: null }).items) === show(text));

// ---- I2: the shop link is bare --------------------------------------------------------------
check('I2: shopLinkExpr(ruze) exact',
  shopLinkExpr('ruze') === '{{ .Subscriber.Attribs.shop_links.ruze.url }}', shopLinkExpr('ruze'));
for (const it of link) {
  check(`I2: link ${it.id} carries no or/index`, !/\bor\b|index/.test(it.expr), it.expr);
}
check('I2: shopLinkExpr refuses a non-identifier', shopLinkExpr('thirsty-girl') === null);
check('I2: shopLinkExpr refuses null', shopLinkExpr(null) === null);

// ---- I3: the shop link's brand states -------------------------------------------------------
for (const [name, ctx] of [['null brand', { brand: null }], ['undefined brand', {}], ['no context', undefined]]) {
  const it = attributeMenu('link', ctx).items.find((i) => i.id === 'shop_link');
  check(`I3: ${name} -> shop link shown, disabled, "Choose a list first"`,
    it && it.disabled === true && it.expr === '' && it.reason === 'Choose a list first', show(it));
}
{
  const it = attributeMenu('link', { brand: 'thirsty-girl' }).items.find((i) => i.id === 'shop_link');
  check('I3: a non-identifier slug -> disabled with the identifier reason',
    it && it.disabled === true && it.expr === ''
      && it.reason === 'Brand slug thirsty-girl is not a template identifier', show(it));
}
{
  const it = attributeMenu('link', { brand: 'thirstygirl' }).items.find((i) => i.id === 'shop_link');
  check('I3: a valid slug -> enabled with the brand in the label',
    it && it.disabled === false && it.label.includes('thirstygirl')
      && it.expr === '{{ .Subscriber.Attribs.shop_links.thirstygirl.url }}', show(it));
}
check('isTemplateIdent', isTemplateIdent('ruze') && isTemplateIdent('a_1') && !isTemplateIdent('thirsty-girl')
  && !isTemplateIdent('') && !isTemplateIdent(null) && !isTemplateIdent('a.b'));

// ---- I4: groups, exactly and in order -------------------------------------------------------
check('I4: text ids in order', show(text.map((i) => i.id)) === show(['first_name', 'last_name', 'name', 'email']),
  show(text.map((i) => i.id)));
check('I4: link ids in order', show(link.map((i) => i.id)) === show(['shop_link', 'unsubscribe', 'message']),
  show(link.map((i) => i.id)));
check('I4: an unknown kind returns no items', attributeMenu('html', { brand: 'ruze' }).items.length === 0);
check('I4: a missing kind returns no items', attributeMenu(undefined, undefined).items.length === 0);
for (const it of [...text, ...link]) {
  check(`every item has a label: ${it.id}`, typeof it.label === 'string' && it.label !== '');
}

// ---- D8: the SPA's i18n key per text id (Stage 4 F2 — one mapping, pinned) ------------------
const I18N_PINS = {
  first_name: 'attributes.firstName',
  last_name: 'attributes.lastName',
  name: 'attributes.name',
  email: 'attributes.email',
};
for (const [id, key] of Object.entries(I18N_PINS)) {
  check(`D8: i18n key for ${id} is ${key}`, attributeI18nKey(id) === key, attributeI18nKey(id));
}

console.log(failed === 0 ? 'attributes: all checks pass' : `attributes: ${failed} FAILED`);
process.exit(failed === 0 ? 0 : 1);
