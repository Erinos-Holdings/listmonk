// INSPECT-SCOPE-SPEC I9 (builder half) -- the render canary.
//
//   - deterministic: two builds with the bundle produce byte-identical files;
//   - the key grammar: every block type the corpus uses has a key, plus Body; every corpus
//     document has an EmailLayout root; the §2.1 dimensions are all present;
//   - a compile change to type T changes EXACTLY the keys of the types that share a document with
//     T (T's own key included) and no other (the per-type invalidation integrations' coverage.ts
//     reads);
//   - Body hashes the raw file.
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const { loadUmd } = require('./_umd.cjs');
const { CANARY_DIR, buildCanary, corpus, itemsOf, typesOf } = require('./build-canary.cjs');

let failed = 0;
function check(name, ok, detail) {
  if (!ok) failed++;
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? `  [${String(detail).slice(0, 400)}]` : ''}`);
}

const TYPES = ['EmailLayout', 'Text', 'Heading', 'Image', 'Button', 'Divider', 'Spacer', 'Avatar', 'Html', 'Container', 'ColumnsContainer', 'OfficialFooter'];

const { dom, EB } = loadUmd();
try {
  const a = buildCanary(EB);
  const b = buildCanary(EB);
  check('two builds are byte-identical', JSON.stringify(a) === JSON.stringify(b));
  check('version 1', a.version === 1);

  const keys = Object.keys(a.items).sort();
  check('every block type has a key, plus Body', [...TYPES, 'Body'].sort().join(',') === keys.join(','), keys.join(','));
  check('every key is 64 hex', keys.every((k) => /^[0-9a-f]{64}$/.test(a.items[k])));

  const docs = corpus();
  check('every corpus document has an EmailLayout root', docs.every(({ doc }) => doc.root && doc.root.type === 'EmailLayout'), docs.filter(({ doc }) => !(doc.root && doc.root.type === 'EmailLayout')).map((d) => d.stem).join(','));
  check('file-stem order', JSON.stringify(Object.keys(a.documents)) === JSON.stringify(docs.map((d) => d.stem)));

  // The §2.1 dimensions.
  const blocks = docs.flatMap(({ doc }) => Object.values(doc));
  const has = (pred) => blocks.some(pred);
  const p = (x) => (x.data && x.data.props) || {};
  const s = (x) => (x.data && x.data.style) || {};
  check('EmailLayout outlook on and off', has((x) => x.type === 'EmailLayout' && x.data.outlook === true) && has((x) => x.type === 'EmailLayout' && !x.data.outlook));
  check('Text default, bold, large', has((x) => x.type === 'Text' && !s(x).fontWeight) && has((x) => x.type === 'Text' && s(x).fontWeight === 'bold') && has((x) => x.type === 'Text' && s(x).fontSize > 24));
  check('Heading default, bold, large', has((x) => x.type === 'Heading' && !s(x).fontWeight) && has((x) => x.type === 'Heading' && s(x).fontWeight === 'bold') && has((x) => x.type === 'Heading' && s(x).fontSize > 24));
  check('Image sized and unsized', has((x) => x.type === 'Image' && typeof p(x).width === 'number') && has((x) => x.type === 'Image' && typeof p(x).width !== 'number' && typeof p(x).height !== 'number'));
  const btn = (st, full) => has((x) => x.type === 'Button' && p(x).buttonStyle === st && !!p(x).fullWidth === full);
  check('Button every buttonStyle x inline/full', ['rectangle', 'rounded', 'pill'].every((st) => btn(st, false) && btn(st, true)));
  check('Html with a <style> and a link', has((x) => x.type === 'Html' && /<style/i.test(p(x).contents) && /href=/i.test(p(x).contents)));
  check('ColumnsContainer 2- and 3-column', has((x) => x.type === 'ColumnsContainer' && p(x).columnsCount === 2) && has((x) => x.type === 'ColumnsContainer' && p(x).columnsCount === 3));
  check('OfficialFooter brand and corporate', has((x) => x.type === 'OfficialFooter' && p(x).kind === 'brand') && has((x) => x.type === 'OfficialFooter' && p(x).kind === 'corporate'));
  check('Divider, Spacer, Avatar, Container', ['Divider', 'Spacer', 'Avatar', 'Container'].every((t) => has((x) => x.type === t)));

  // A compile change to T: every document containing T compiles differently.
  for (const T of ['Avatar', 'Html', 'Text', 'OfficialFooter']) {
    const changed = {};
    for (const [stem, d] of Object.entries(a.documents)) {
      changed[stem] = typesOf(d.doc).has(T) ? { doc: d.doc, html: `${d.html}<!-- ${T} changed -->` } : d;
    }
    const after = itemsOf(changed, a.body);
    const sharing = new Set();
    Object.values(a.documents).forEach((d) => {
      if (typesOf(d.doc).has(T)) typesOf(d.doc).forEach((t) => sharing.add(t));
    });
    const moved = keys.filter((k) => after[k] !== a.items[k]).sort();
    check(`a compile change to ${T} moves exactly the keys sharing a document with it`, moved.join(',') === [...sharing].sort().join(','), `${moved.join(',')} vs ${[...sharing].sort().join(',')}`);
  }

  check('Body hashes the raw file', a.items.Body === crypto.createHash('sha256').update(fs.readFileSync(path.join(CANARY_DIR, 'body.html'), 'utf8'), 'utf8').digest('hex'));
  check('the file carries the raw body for the server half', a.body === fs.readFileSync(path.join(CANARY_DIR, 'body.html'), 'utf8'));
} finally {
  dom.window.close();
}

process.exit(failed ? 1 : 0);
