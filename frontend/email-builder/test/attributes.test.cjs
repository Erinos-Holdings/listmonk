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
  attributeMenu, shopLinkExpr, isTemplateIdent, attributeI18nKey, productRefExpr, productRefUrl,
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
// 2026-10-21 (user): UnsubscribeURL / MessageURL removed from the link group (the official
// footers carry both); the Text block's 'markdown' group is text + the two shop items.
check('I4: link ids in order', show(link.map((i) => i.id)) === show(['shop_link', 'product_link']),
  show(link.map((i) => i.id)));
check('I4: no unsubscribe/message anywhere',
  ['link', 'text', 'markdown'].every((k) => attributeMenu(k, { brand: 'ruze' }).items
    .every((i) => i.id !== 'unsubscribe' && i.id !== 'message' && !/UnsubscribeURL|MessageURL/.test(i.expr))));
const markdown = attributeMenu('markdown', { brand: 'ruze', value: 'https://ruzepouches.com/pk/X/' }).items;
check('I4: markdown ids in order',
  show(markdown.map((i) => i.id)) === show(['first_name', 'last_name', 'name', 'email', 'shop_link', 'product_link']),
  show(markdown.map((i) => i.id)));
check('markdown: text items match the text group byte-for-byte',
  show(markdown.slice(0, 4)) === show(text), show(markdown.slice(0, 4)));
check('markdown: shop link enabled with the brand',
  markdown[4].disabled === false && markdown[4].expr === '{{ .Subscriber.Attribs.shop_links.ruze.url }}');
check('markdown: product link transforms the selection',
  markdown[5].disabled === false && markdown[5].replace === true
    && markdown[5].expr === 'https://ruzepouches.com/pk/X/?ref={{ or .Subscriber.Attribs.shop_links.ruze.slug `` }}',
  show(markdown[5]));
{
  const it = attributeMenu('markdown', { brand: 'ruze', value: '' }).items.find((i) => i.id === 'product_link');
  check('markdown: no selection -> disabled with the select reason',
    it && it.disabled === true && it.reason === 'Select the product page URL first', show(it));
}
{
  const it = attributeMenu('link', { brand: 'ruze', value: '' }).items.find((i) => i.id === 'product_link');
  check('link: the no-URL reason is unchanged on link fields',
    it && it.disabled === true && it.reason === 'Type or paste the product page URL first', show(it));
}
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

// ==== PRODUCT-LINK-SPEC (integrations) -- the product-page referral transform ===============
// The literal lands in live hrefs: a change here is the acknowledgment. The same literal is
// pinned in Go (internal/linkresolve TestProductLink, I4) and in the integrations campaign
// review (tests/lib/campaign-review/attribute-roster.test.ts, I4). The fallback is an empty
// BACKQUOTED raw string -- never "" (runbook hazard 102; models/tracking_test.go pins why).
const REF = '{{ or .Subscriber.Attribs.shop_links.ruze.slug `` }}';
check('PL D1: productRefExpr(ruze) exact', productRefExpr('ruze') === REF, productRefExpr('ruze'));
check('PL D1: no double quote in the expression (hazard 102)', !productRefExpr('ruze').includes('"'));
check('PL D1: productRefExpr refuses a non-identifier', productRefExpr('thirsty-girl') === null);
const pr = (v, b = 'ruze') => productRefUrl(v, b);
const prUrl = (v, b) => pr(v, b).url;

// ---- I1: ref stripping (any case, literal or templated) and idempotence ---------------------
const STRIP = [
  ['https://ruzepouches.com/pk/Ruze-Energy/?ref=kara', `https://ruzepouches.com/pk/Ruze-Energy/?ref=${REF}`],
  ['https://ruzepouches.com/pk/Ruze-Energy/?REF=kara', `https://ruzepouches.com/pk/Ruze-Energy/?ref=${REF}`],
  ['https://ruzepouches.com/pk/Ruze-Energy/?Ref=kara', `https://ruzepouches.com/pk/Ruze-Energy/?ref=${REF}`],
  ['https://x.test/p?a=1&ref=kara&b=2', `https://x.test/p?a=1&b=2&ref=${REF}`],
  ['https://x.test/p?ref', `https://x.test/p?ref=${REF}`],
  ['https://x.test/p?ref=kara&ref=lee', `https://x.test/p?ref=${REF}`],
  ['https://x.test/p?ref={{ .Subscriber.Attribs.x }}', `https://x.test/p?ref=${REF}`],
  [`https://x.test/p?ref=${REF}`, `https://x.test/p?ref=${REF}`],
  ['https://x.test/p?referrer=kara&xref=1', `https://x.test/p?referrer=kara&xref=1&ref=${REF}`],
];
for (const [inp, want] of STRIP) {
  check(`PL I1: ${inp}`, prUrl(inp) === want, show(pr(inp)));
}
for (const [inp] of [...STRIP, ['https://x.test/a/b?c=%20d#frag'], ['https://x.test/']]) {
  const once = prUrl(inp);
  check(`PL I1: fixed point ${inp}`, once !== undefined && prUrl(once) === once, show(once));
}
check('PL I1: no creator ref survives', !/kara/.test(prUrl('https://x.test/?a=1&REF=kara#k')));

// ---- I2: joiner, fragment, untouched bytes --------------------------------------------------
const STRUCT = [
  ['bare path -> ?', 'https://ruzepouches.com/pk/Ruze-Energy/', `https://ruzepouches.com/pk/Ruze-Energy/?ref=${REF}`],
  ['no path -> ?', 'https://ruzepouches.com', `https://ruzepouches.com?ref=${REF}`],
  ['existing query -> &', 'https://x.test/p?utm_source=x', `https://x.test/p?utm_source=x&ref=${REF}`],
  ['fragment after ref', 'https://x.test/p#reviews', `https://x.test/p?ref=${REF}#reviews`],
  ['fragment with query', 'https://x.test/p?a=1&ref=k#r?ref=z', `https://x.test/p?a=1&ref=${REF}#r?ref=z`],
  ['encoding untouched', 'https://x.test/a%2Fb/%C3%A9?q=a+b&s=%26%3D', `https://x.test/a%2Fb/%C3%A9?q=a+b&s=%26%3D&ref=${REF}`],
  ['other expressions untouched', 'https://x.test/p?n={{ urlquery .Subscriber.Name }}', `https://x.test/p?n={{ urlquery .Subscriber.Name }}&ref=${REF}`],
  ['trailing ? dropped into the joiner', 'https://x.test/p?', `https://x.test/p?ref=${REF}`],
  ['surrounding whitespace trimmed', '  https://x.test/p  ', `https://x.test/p?ref=${REF}`],
  ['HTTP scheme any case', 'HTTP://x.test/p', `HTTP://x.test/p?ref=${REF}`],
];
for (const [name, inp, want] of STRUCT) {
  check(`PL I2: ${name}`, prUrl(inp) === want, show(pr(inp)));
}

// ---- I3 (+I5): refusal states, and the menu surfaces them --------------------------------
const NO_URL = 'Type or paste the product page URL first';
for (const [name, v] of [
  ['empty', ''], ['whitespace', '   '], ['undefined', undefined], ['relative', '/pk/Ruze-Energy/'],
  ['schemeless', 'ruzepouches.com/pk/'], ['mailto', 'mailto:a@b.c'], ['javascript', 'javascript:alert(1)'],
  ['ftp', 'ftp://x.test/'], ['expression only', '{{ .Subscriber.Attribs.shop_links.ruze.url }}'],
  ['expression host', 'https://{{ .Subscriber.Attribs.host }}/p'], ['scheme only', 'https://'],
]) {
  const r = pr(v);
  check(`PL I3: ${name} -> reason`, r.url === undefined && r.reason === NO_URL, show(r));
}
check('PL I3: null brand -> "Choose a list first"', show(pr('https://x.test/', null)) === show({ reason: 'Choose a list first' }));
check('PL I3: empty brand -> "Choose a list first"', show(pr('https://x.test/', '')) === show({ reason: 'Choose a list first' }));
check('PL I5: invalid brand -> the isTemplateIdent reason',
  show(pr('https://x.test/', 'thirsty-girl')) === show({ reason: 'Brand slug thirsty-girl is not a template identifier' }));
check('PL I5: brand rule precedes the URL rule',
  pr('', 'thirsty-girl').reason === 'Brand slug thirsty-girl is not a template identifier'
  && pr('', null).reason === 'Choose a list first');

const plItem = (ctx) => attributeMenu('link', ctx).items.find((i) => i.id === 'product_link');
for (const [name, ctx, reason] of [
  ['no brand', { brand: null, value: 'https://x.test/' }, 'Choose a list first'],
  ['no context', undefined, 'Choose a list first'],
  ['invalid brand', { brand: 'thirsty-girl', value: 'https://x.test/' }, 'Brand slug thirsty-girl is not a template identifier'],
  ['no value passed', { brand: 'ruze' }, NO_URL],
  ['relative value', { brand: 'ruze', value: '/pk/' }, NO_URL],
]) {
  const it = plItem(ctx);
  check(`PL I3: menu ${name} -> product link shown, disabled, reason`,
    it && it.disabled === true && it.expr === '' && it.reason === reason && it.replace === true, show(it));
}
{
  const it = plItem({ brand: 'ruze', value: 'https://ruzepouches.com/pk/Ruze-Energy/?ref=kara' });
  check('PL D3: enabled item carries the transformed URL, replace, brand label',
    it && it.disabled === false && it.reason === '' && it.replace === true
      && it.label === 'Personalize product link (ruze)'
      && it.expr === `https://ruzepouches.com/pk/Ruze-Energy/?ref=${REF}`, show(it));
}
{
  // I7: the value never changes the other link items, and the text group ignores it.
  const withValue = attributeMenu('link', { brand: 'ruze', value: 'https://x.test/' }).items.filter((i) => i.id !== 'product_link');
  check('PL I7: other link items unchanged by value', show(withValue) === show(link.filter((i) => i.id !== 'product_link')), show(withValue));
  check('PL I7: existing items carry no replace', withValue.every((i) => i.replace === undefined));
  check('PL I7: text group unchanged by value',
    show(attributeMenu('text', { brand: 'ruze', value: 'https://x.test/' }).items) === show(text));
}

console.log(failed === 0 ? 'attributes: all checks pass' : `attributes: ${failed} FAILED`);
process.exit(failed === 0 ? 0 : 1);
