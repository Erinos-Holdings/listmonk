// integrations RENDERING-BIBLE-SPEC I15 -- the compiled document carries `<html lang>` exactly
// when the compile context has a language: `{lang: 'fr'}` -> `<html lang="fr">`; no context, or
// an empty lang, -> a bare `<html>`. Against the BUILT bundle's compileDocument (the headless
// compile the re-save sweep, the canary and the review Lambda all use) and its editor path
// (setOfficialContext + resetDocument -> onChange, what a UI Save stores).
const { loadUmd } = require('./_umd.cjs');

let failed = 0;
function check(name, ok, detail) {
  if (!ok) failed++;
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail ? `  [${String(detail).slice(0, 300)}]` : ''}`);
}

const doc = {
  root: { type: 'EmailLayout', data: { backdropColor: '#F5F5F5', canvasColor: '#FFFFFF', textColor: '#262626', fontFamily: 'MODERN_SANS', childrenIds: ['t'] } },
  t: { type: 'Text', data: { style: { padding: { top: 16, bottom: 16, right: 24, left: 24 } }, props: { text: 'Hello' } } },
};
const htmlTag = (h) => (/<html\b[^>]*>/i.exec(h) || [''])[0];

const { dom, EB } = loadUmd();
try {
  const c = (ctx) => EB.compileDocument(JSON.parse(JSON.stringify(doc)), ctx, []);
  check('lang en -> <html lang="en">', htmlTag(c({ lang: 'en', brand: null })) === '<html lang="en">', htmlTag(c({ lang: 'en', brand: null })));
  check('lang fr -> <html lang="fr">', htmlTag(c({ lang: 'fr', brand: 'ruze' })) === '<html lang="fr">', htmlTag(c({ lang: 'fr', brand: 'ruze' })));
  check('no context -> no lang attribute', htmlTag(c(null)) === '<html>', htmlTag(c(null)));
  check('empty lang -> no lang attribute', htmlTag(c({ lang: '', brand: 'ruze' })) === '<html>', htmlTag(c({ lang: '', brand: 'ruze' })));
  check('whitespace lang -> no lang attribute', htmlTag(c({ lang: '  ' })) === '<html>', htmlTag(c({ lang: '  ' })));
  check('the head contents still land inside <head>', /<html lang="en"><head><meta name="viewport"/.test(c({ lang: 'en' })));

  // The editor path: the host's context (VisualEditor.vue setOfficialContext) reaches onChange.
  let last = null;
  EB.render('visual-editor-container', { data: {}, onChange: (_d, body) => { last = body; } });
  const wait = (ms) => new Promise((r) => setTimeout(r, ms));
  (async () => {
    for (let i = 0; i < 100 && !EB.isRendered('visual-editor-container'); i++) await wait(50);
    EB.setOfficialContext({ lang: 'de', brand: null });
    EB.resetDocument(JSON.parse(JSON.stringify(doc)));
    check('editor save under {lang: de} -> <html lang="de">', htmlTag(last || '') === '<html lang="de">', htmlTag(last || ''));
    EB.setOfficialContext(null);
    EB.resetDocument(JSON.parse(JSON.stringify(doc)));
    check('editor save with no context -> no lang attribute', htmlTag(last || '') === '<html>', htmlTag(last || ''));
    dom.window.close();
    console.log(failed ? `${failed} FAILURE(S)` : 'ALL PASS');
    process.exit(failed ? 1 : 0);
  })().catch((e) => {
    console.error(e);
    process.exit(1);
  });
} catch (e) {
  dom.window.close();
  throw e;
}
