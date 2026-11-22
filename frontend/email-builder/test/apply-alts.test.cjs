// Fork (campaign review) -- integrations ALT-ON-INSERT-SPEC I9: EmailBuilder.applyAlts never
// overwrites. A fix with the wrong `from`, the wrong `src` (compared trimmed), a missing block or a
// non-Image block is skipped; the applied fixes are ONE store update (the host's onChange fires
// once, the selection is kept, the selected block's Alt field shows the new alt); nothing applied is
// no update (no onChange). The pure half (documents/applyAlts.ts, compiled standalone by run.cjs) and
// the BUILT UMD (the structure-ui.test.cjs mount pattern).
const path = require('path');
const { loadUmd, compileInputs } = require('./_umd.cjs');
const A = require(path.join(__dirname, '.build', 'documents', 'applyAlts.cjs'));

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail !== undefined ? '  [' + String(typeof detail === 'string' ? detail : JSON.stringify(detail)).slice(0, 400) + ']' : ''}`); }
const clone = (o) => JSON.parse(JSON.stringify(o));
const tick = (ms = 40) => new Promise((r) => setTimeout(r, ms));

const DOC = {
  root: { type: 'EmailLayout', data: { backdropColor: '#F5F5F5', canvasColor: '#FFFFFF', textColor: '#262626', fontFamily: 'MODERN_SANS', childrenIds: ['img-u', 'img-h', 'img-t', 'txt'] } },
  'img-u': { type: 'Image', data: { style: { padding: { top: 0, bottom: 0, left: 0, right: 0 } }, props: { url: 'https://e/u.jpg ' } } },
  'img-h': { type: 'Image', data: { style: { padding: { top: 0, bottom: 0, left: 0, right: 0 } }, props: { url: 'https://e/h.jpg', alt: 'Human words' } } },
  'img-t': { type: 'Image', data: { style: { padding: { top: 0, bottom: 0, left: 0, right: 0 } }, props: { url: 'https://e/t.jpg', alt: null } } },
  txt: { type: 'Text', data: { style: {}, props: { text: 'Words.' } } },
};
const fix = (block, over = {}) => ({
  kind: 'alt.set', block, from: '', to: `Alt for ${block}`, undecided: true, src: `https://e/${block.slice(4)}.jpg`, ...over,
});

// ---- the pure half -------------------------------------------------------------------------
{
  const input = clone(DOC);
  const r = A.applyAltsToDocument(input, [
    fix('img-u'), // undecided, url compared trimmed: applied
    fix('img-h', { from: 'Not the alt', undecided: undefined }), // wrong from: skipped
    fix('img-t', { src: 'https://e/other.jpg' }), // wrong src: skipped
    fix('img-x'), // missing block: skipped
    fix('txt', { src: '' }), // not an Image: skipped
  ]);
  check('pure: only the matching fix applies', r.changed && r.applied.length === 1 && r.applied[0].block === 'img-u' && r.skipped.length === 4, r);
  check('pure: the applied alt is set; the others are untouched', r.document['img-u'].data.props.alt === 'Alt for img-u' && r.document['img-h'].data.props.alt === 'Human words' && r.document['img-t'].data.props.alt === null, r.document);
  check('pure: the input document is never mutated', JSON.stringify(input) === JSON.stringify(DOC));
  const h = A.applyAltsToDocument(clone(DOC), [fix('img-h', { from: 'Human words', undecided: undefined, to: 'New words' })]);
  check('pure: a matching `from` applies over an existing alt', h.changed && h.document['img-h'].data.props.alt === 'New words');
  const typed = A.applyAltsToDocument(clone(DOC), [fix('img-h')]);
  check('pure: undecided against an alt that is present is skipped (an alt typed while the request was out)', !typed.changed && typed.skipped.length === 1);
  const none = A.applyAltsToDocument(DOC, []);
  check('pure: nothing applied -> unchanged, the same document', !none.changed && none.document === DOC);
  const nul = A.applyAltsToDocument(clone(DOC), [fix('img-t')]);
  check('pure: a null alt counts as absent (undecided)', nul.changed && nul.document['img-t'].data.props.alt === 'Alt for img-t');
}

// ---- the built bundle ----------------------------------------------------------------------
(async () => {
  const errors = [];
  const { dom, EB } = loadUmd(undefined, { onError: (m) => errors.push(m) });
  check('the bundle exports applyAlts', typeof EB.applyAlts === 'function');
  const events = [];
  EB.render('visual-editor-container', { data: {}, onChange: (doc) => events.push(clone(doc)) });
  const deadline = Date.now() + 10000;
  while (!EB.isRendered('visual-editor-container')) {
    if (Date.now() > deadline) throw new Error('builder never mounted');
    await tick(50);
  }
  await tick(100);
  const { context, refs } = compileInputs();
  EB.setOfficialFooters(refs);
  EB.setOfficialContext(context);
  EB.resetDocument(clone(DOC));
  await tick();
  check('img-u can be selected', EB.selectBlock('img-u') === true);
  await tick(120);
  const altInput = () => {
    const label = [...dom.window.document.querySelectorAll('label')].find((l) => l.textContent.trim() === 'Alt text');
    return label ? dom.window.document.getElementById(label.htmlFor) : null;
  };
  check('the selected block shows its (empty) Alt field', altInput() && altInput().value === '');
  events.length = 0;

  // Skipped-only: no store write, no onChange.
  const skipped = EB.applyAlts([fix('img-h', { from: 'Not the alt', undecided: undefined }), fix('img-x'), fix('txt', { src: '' }), fix('img-t', { src: 'https://e/other.jpg' })]);
  await tick();
  check('UMD: nothing applied -> no onChange', events.length === 0 && skipped.applied.length === 0 && skipped.skipped.length === 4, { events: events.length, skipped });

  // Two applied + one skipped: ONE onChange, the selection kept, the Alt field shows the new alt.
  const r = EB.applyAlts([fix('img-u'), fix('img-t'), fix('img-h')]);
  await tick(120);
  check('UMD: applied and skipped are the fix objects', r.applied.map((f) => f.block).join(',') === 'img-u,img-t' && r.skipped.map((f) => f.block).join(',') === 'img-h', r);
  check('UMD: the applied fixes are ONE onChange', events.length === 1, events.length);
  const last = events[events.length - 1] || {};
  check('UMD: the emitted document carries both alts and keeps the human one', last['img-u'] && last['img-u'].data.props.alt === 'Alt for img-u' && last['img-t'].data.props.alt === 'Alt for img-t' && last['img-h'].data.props.alt === 'Human words', last);
  check('UMD: the document is merged, not replaced (no key lost)', Object.keys(last).sort().join(',') === Object.keys(DOC).sort().join(','), Object.keys(last));
  check('UMD: the selected block\'s Alt field shows the new alt at once', altInput() && altInput().value === 'Alt for img-u', altInput() && altInput().value);
  check('UMD: no runtime error', errors.length === 0, errors);

  console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
  process.exit(failed ? 1 : 0);
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
