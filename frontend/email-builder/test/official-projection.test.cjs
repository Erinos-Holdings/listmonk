// OFFICIAL-FOOTER-SPEC I4. The projection ignores block ids, colours, widths, padding; collapses
// whitespace; decodes entities; strips style attributes from Html contents; the hash is FNV-1a
// 64 over UTF-8, 16 hex -- pinned on fixtures copied from templates 30 and 14 (live, read-only,
// 2026-09-24).
const path = require('path');
const { officialProjection, fnv1a64 } = require(path.join(__dirname, '.build', 'official', 'projection.cjs'));
const t30 = require('./fixtures/official-template-30.json');
const t14 = require('./fixtures/official-template-14.json');

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }
const clone = (o) => JSON.parse(JSON.stringify(o));

// FNV-1a 64 reference vectors.
check('fnv1a64("") offset basis', fnv1a64('') === 'cbf29ce484222325');
check('fnv1a64("a")', fnv1a64('a') === 'af63dc4c8601ec8c');
check('fnv1a64("foobar")', fnv1a64('foobar') === '85944171f73967e8');
check('fnv1a64 is 16 lower-case hex', /^[0-9a-f]{16}$/.test(fnv1a64('©2026 — ü')));
// UTF-8, not UTF-16: "é" is two bytes c3 a9.
check('hash runs over UTF-8 bytes', fnv1a64('é') === fnv1a64Bytes([0xc3, 0xa9]));
function fnv1a64Bytes(bytes) {
  let h = BigInt('0xcbf29ce484222325');
  for (const b of bytes) { h ^= BigInt(b); h = (h * BigInt('0x100000001b3')) & BigInt('0xffffffffffffffff'); }
  return h.toString(16).padStart(16, '0');
}

// Pinned fixture hashes (templates 30 and 14 as stored 2026-09-24).
const P30 = officialProjection(t30.body_source);
const P14 = officialProjection(t14.body_source);
check('template 30 pinned hash', P30.hash === '6370a827f7c1262e', P30.hash);
check('template 14 pinned hash', P14.hash === '8485f46f79016bcf', P14.hash);
check('accepts the JSON string as well', officialProjection(JSON.stringify(t30.body_source)).hash === P30.hash);
check('projection lines joined with \\n', P30.projection.split('\n')[0] === '<Text>');
check('entities decoded (&bull; -> •, &amp; -> &)', P30.projection.includes('Unsubscribe • Manage Preferences') && P30.projection.includes('&email={{ .Subscriber.Email }}'));
check('whitespace collapsed (newlines in the markdown)', !/text: [^\n]*  /.test(P30.projection));
check('every href in document order', (P30.projection.match(/^href: /gm) || []).length === 5);
check('style fontFamily/fontSize/fontWeight/textAlign kept', /fontFamily: ARIAL\nfontSize: 11\nfontWeight: normal\ntextAlign: center/.test(P30.projection));
check('colour and padding never appear', !/#555555|color|padding/i.test(P30.projection));
check('Image url/linkHref/alt kept', P14.projection.includes('src: https://email.curatedfor.you/uploads/Asset-2RUZE.png\nhref: https://ruzepouches.com/\nalt: Ruze'));
check('Html aria-label and img src kept', P14.projection.includes('aria-label: Facebook\nsrc: https://email.curatedfor.you/uploads/social-curated-facebook.png'));

// Ignored: block ids.
const renamed = clone(t14.body_source);
renamed['block-X'] = renamed['block-1788298044435'];
delete renamed['block-1788298044435'];
renamed.root.data.childrenIds = ['block-X', 'block-joybelle-social'];
check('block ids ignored', officialProjection(renamed).hash === P14.hash);

// Ignored: colours, widths, padding, lineColor, contentAlignment, root props.
const cosmetic = clone(t14.body_source);
cosmetic.root.data.backdropColor = '#000000';
cosmetic.root.data.fontFamily = 'BOOK_SERIF';
cosmetic['block-1788298139076'].data.style.padding = { top: 1, bottom: 2, left: 3, right: 4 };
cosmetic['block-1788298139076'].data.style.backgroundColor = '#123456';
cosmetic['block-1788298139076'].data.props.width = 321;
cosmetic['block-1788298139076'].data.props.height = 99;
cosmetic['block-1788298139076'].data.props.contentAlignment = 'top';
cosmetic['block-joybelle-social'].data.style.color = '#ff0000';
cosmetic['block-joybelle-social'].data.props.contents = cosmetic['block-joybelle-social'].data.props.contents
  .replace(/style="padding:0 6px;"/g, 'style="padding:0 12px;background:#fff"');
check('colours, widths, padding, contentAlignment, root props ignored', officialProjection(cosmetic).hash === P14.hash, officialProjection(cosmetic).projection);

// Html style attributes stripped BEFORE the text/link pass (a style value is never content).
const styled = clone(t14.body_source);
styled['block-joybelle-social'].data.props.contents = styled['block-joybelle-social'].data.props.contents.replace('style="margin:0 auto;"', 'style="margin:0 auto;content:\'x\'"');
check('Html style attributes stripped', officialProjection(styled).hash === P14.hash);

// Whitespace and entity equivalence in text.
const spaced = clone(t30.body_source);
spaced['block-1788298161221'].data.props.text = spaced['block-1788298161221'].data.props.text.replace(' • ', '   &bull;\n ');
check('extra whitespace / entity spelling of the same text ignored', officialProjection(spaced).hash === P30.hash);

// Changes that MUST move the hash.
const wording = clone(t30.body_source);
wording['block-1788298161221'].data.props.text = wording['block-1788298161221'].data.props.text.replace('84048', '84043');
check('a wording change moves the hash', officialProjection(wording).hash !== P30.hash);
const link = clone(t14.body_source);
link['block-1788298139076'].data.props.linkHref = 'https://example.com/';
check('an href change moves the hash', officialProjection(link).hash !== P14.hash);
const order = clone(t14.body_source);
order.root.data.childrenIds = ['block-joybelle-social', 'block-1788298044435'];
check('block order moves the hash', officialProjection(order).hash !== P14.hash);
const size = clone(t30.body_source);
size['block-1788298161221'].data.style.fontSize = 12;
check('a fontSize change moves the hash', officialProjection(size).hash !== P30.hash);

// Per-block projection (the repair planner's reference rule) and cycle safety.
const one = officialProjection(t14.body_source, 'block-joybelle-social');
check('single-block projection', one.projection.startsWith('<Html>') && !one.projection.includes('<Container>'));
const cyc = { root: { type: 'EmailLayout', data: { childrenIds: ['x'] } }, x: { type: 'Container', data: { props: { childrenIds: ['x'] } } } };
check('a childrenIds cycle terminates', officialProjection(cyc).projection === '<Container>\n</Container>');
check('garbage in, empty projection out', officialProjection('not json').projection === '' && officialProjection(null).projection === '');

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
