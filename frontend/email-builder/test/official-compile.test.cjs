// OFFICIAL-FOOTER-SPEC I2, I3, I5 against the BUILT bundle (frontend/public/static/email-builder/
// email-builder.umd.js; run.cjs rebuilds it when absent or stale), evaluated under jsdom exactly
// as the integrations headless compile loads the served one.
//
//   I2  Fidelity: compiling a document with OfficialFooter blocks equals compiling the same
//       document with the references' root children pasted at that position, except for exactly
//       the two marker comments per block; no wrapper element.
//   I3  The markers survive postProcess and the editor quote hack into the stored body; the
//       corporate marker's brand segment is `-`; a `missing` brand footer compiles to an empty
//       marker pair; a `curated` brand block compiles to nothing; an OfficialFooter inside a
//       reference renders nothing (no recursion).
//   I5  setOfficialContext/setOfficialFooters re-emit onChange with a regenerated body, and not
//       before the first resetDocument.
const path = require('path');
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');

const UMD = path.join(__dirname, '..', '..', 'public', 'static', 'email-builder', 'email-builder.umd.js');
const t30 = require('./fixtures/official-template-30.json');
const t14 = require('./fixtures/official-template-14.json');
const c108 = require('./fixtures/campaign108-source.json');

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + String(detail).slice(0, 400) + ']' : ''}`); }
const clone = (o) => JSON.parse(JSON.stringify(o));
const refOf = (t, over = {}) => ({ id: t.id, name: t.name, body_source: JSON.stringify(t.body_source), ...over });

const warnings = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('warn', (m) => warnings.push(String(m)));
virtualConsole.on('jsdomError', (e) => console.log(`jsdom: ${e.message.split('\n')[0]}`));
const dom = new JSDOM('<!doctype html><html><head></head><body><div id="visual-editor-container"></div></body></html>', {
  runScripts: 'dangerously',
  pretendToBeVisual: true,
  virtualConsole,
});
// jsdom 24 (this package's) has no TextEncoder; react-dom/server's browser build constructs one
// at module load (the integrations host runs jsdom 29, which has it). Only the stream renderer
// ever calls it -- renderToStaticMarkup does not.
if (typeof dom.window.TextEncoder !== 'function') {
  dom.window.TextEncoder = require('util').TextEncoder;
  dom.window.TextDecoder = require('util').TextDecoder;
}
const script = dom.window.document.createElement('script');
script.textContent = fs.readFileSync(UMD, 'utf8');
dom.window.document.head.appendChild(script);
const EB = dom.window.EmailBuilder;

async function main() {
  for (const fn of ['setOfficialFooters', 'setOfficialContext', 'officialProjection', 'compileDocument', 'insertOfficialFooter']) {
    check(`UMD exports ${fn}`, EB && typeof EB[fn] === 'function');
  }

  const REFS = [refOf(t30), refOf(t14)];
  const CTX = { lang: 'en', brand: 'ruze' };
  const H30 = EB.officialProjection(t30.body_source).hash;
  const H14 = EB.officialProjection(t14.body_source).hash;
  check('bundle projection = the pinned standalone hash (one implementation)', H30 === '6370a827f7c1262e' && H14 === '8485f46f79016bcf', `${H30} ${H14}`);

  // Campaign 108 with its copied footer (logo Container, social row, corporate Text -- the last
  // three root children) replaced two ways.
  const FOOTER_IDS = ['block-1788298044435', 'block-joybelle-social', 'block-1788298161221'];
  const body = c108.body_source.root.data.childrenIds.filter((id) => !FOOTER_IDS.includes(id));
  check('fixture: campaign 108 ends with the three copied footer blocks',
    JSON.stringify(c108.body_source.root.data.childrenIds.slice(-3)) === JSON.stringify(FOOTER_IDS));

  const withBlocks = clone(c108.body_source);
  for (const id of FOOTER_IDS) delete withBlocks[id];
  withBlocks['of-brand'] = { type: 'OfficialFooter', data: { props: { kind: 'brand' } } };
  withBlocks['of-corp'] = { type: 'OfficialFooter', data: { props: { kind: 'corporate' } } };
  withBlocks.root.data.childrenIds = [...body, 'of-brand', 'of-corp'];

  const pasted = clone(withBlocks);
  delete pasted['of-brand'];
  delete pasted['of-corp'];
  const pasteIds = [];
  for (const t of [t14, t30]) {
    for (const [id, b] of Object.entries(t.body_source)) if (id !== 'root') pasted[`p${t.id}-${id}`] = remap(b, t.id);
    pasteIds.push(...t.body_source.root.data.childrenIds.map((id) => `p${t.id}-${id}`));
  }
  function remap(b, tid) {
    const c = clone(b);
    if (c.type === 'Container' && c.data.props && c.data.props.childrenIds) c.data.props.childrenIds = c.data.props.childrenIds.map((x) => `p${tid}-${x}`);
    return c;
  }
  pasted.root.data.childrenIds = [...body, ...pasteIds];

  const official = EB.compileDocument(withBlocks, CTX, REFS);
  const reference = EB.compileDocument(pasted, CTX, REFS);
  const MARKER_RE = /<!-- official:[^>]*? -->|<!-- \/official -->/g;
  const markers = official.match(MARKER_RE) || [];
  check('I2: exactly two marker comments per block (4)', markers.length === 4, markers.join(' '));
  check('I2: fidelity -- identical to the pasted compile once the markers are removed', official.replace(MARKER_RE, '') === reference,
    firstDiff(official.replace(MARKER_RE, ''), reference));
  check('I2: no wrapper element / placeholder survives', !/lm-official/.test(official));
  check('I2: the compile really contains the references (unsubscribe + RUZE logo)', official.includes('{{ UnsubscribeURL }}') && official.includes('Asset-2RUZE.png'));

  check(`I3: brand marker carries lang, brand and the reference hash`, official.includes(`<!-- official:brand:en:ruze:${H14} -->`));
  check(`I3: corporate marker brand segment is -`, official.includes(`<!-- official:corporate:en:-:${H30} -->`));
  check('I3: brand pair precedes corporate pair', official.indexOf('official:brand') < official.indexOf('official:corporate'));
  check('I3: the quote hack ran (Go actions carry raw quotes)', !/\{\{[^}]*&quot;[^}]*\}\}/.test(official));

  // missing brand -> empty pair; curated -> nothing.
  const liyora = EB.compileDocument(withBlocks, { lang: 'en', brand: 'liyora' }, REFS);
  check('I3: missing brand footer compiles to an EMPTY marker pair', liyora.includes('<!-- official:brand:en:liyora:missing --><!-- /official -->'), (liyora.match(MARKER_RE) || []).join(' '));
  const curated = EB.compileDocument(withBlocks, { lang: 'en', brand: 'curated' }, REFS);
  const curatedMarkers = curated.match(MARKER_RE) || [];
  check('I3: curated brand block compiles to nothing, marker included', curatedMarkers.length === 2 && !curated.includes('official:brand') && !curated.includes('Asset-2RUZE.png'), curatedMarkers.join(' '));
  const nolist = EB.compileDocument(withBlocks, { lang: 'en', brand: null }, REFS);
  check('no-context brand compiles to an empty pair; corporate still resolves', nolist.includes('<!-- official:brand:en:-:no-context --><!-- /official -->') && nolist.includes(`official:corporate:en:-:${H30}`));
  const es = EB.compileDocument(withBlocks, { lang: '', brand: 'ruze' }, REFS);
  check('empty lang compiles as en', es.includes(`official:corporate:en:-:${H30}`));
  const de = EB.compileDocument(withBlocks, { lang: 'de', brand: 'ruze' }, REFS);
  check('no cross-language fallback: de corporate missing', de.includes('<!-- official:corporate:de:-:missing --><!-- /official -->') && !de.includes('UnsubscribeURL'));

  // Depth 1 only: an OfficialFooter inside a reference renders nothing.
  const recursive = clone(t30.body_source);
  recursive['nested-of'] = { type: 'OfficialFooter', data: { props: { kind: 'corporate' } } };
  recursive.root.data.childrenIds = [...recursive.root.data.childrenIds, 'nested-of'];
  const recRefs = [refOf({ ...t30, body_source: recursive }), refOf(t14)];
  warnings.length = 0;
  const rec = EB.compileDocument(withBlocks, CTX, recRefs);
  check('I3: no recursion -- a nested OfficialFooter renders nothing', (rec.match(MARKER_RE) || []).length === 4 && (rec.match(/UnsubscribeURL/g) || []).length === (official.match(/UnsubscribeURL/g) || []).length);
  check('I3: the nested block logs a console warning', warnings.some((w) => /depth 1/.test(w)), warnings.join(' | '));

  // I5: the re-emit contract, through the editor's own onChange.
  const events = [];
  EB.render('visual-editor-container', { data: {}, onChange: (_d, html) => events.push(html) });
  const deadline = Date.now() + 10000;
  while (!EB.isRendered('visual-editor-container')) {
    if (Date.now() > deadline) throw new Error('builder never mounted');
    await new Promise((r) => setTimeout(r, 50));
  }
  await new Promise((r) => setTimeout(r, 100));
  events.length = 0;
  EB.setOfficialFooters(REFS);
  EB.setOfficialContext(CTX);
  check('I5: no re-emit before the first resetDocument', events.length === 0, `${events.length} events`);
  EB.resetDocument(withBlocks);
  check('I5: resetDocument emits', events.length >= 1 && events[events.length - 1].includes(`official:corporate:en:-:${H30}`));
  // The editor onChange body is pre-quote-hack; compare modulo the hack.
  const hack = (h) => h.replace(/\{\{[^}]*\}\}/g, (m) => m.replace(/&quot;/g, '"'));
  check('I5: the editor body equals compileDocument (one compile)', hack(events[events.length - 1]) === official);

  events.length = 0;
  const ES_REFS = [...REFS, refOf({ ...t30, id: 31, name: 'Official_Footer_ES' })];
  EB.setOfficialFooters(ES_REFS);
  check('I5: a references change re-emits', events.length >= 1);
  events.length = 0;
  EB.setOfficialContext({ lang: 'es', brand: 'ruze' });
  check('I5: a context change re-emits with a regenerated body', events.length >= 1 && events[events.length - 1].includes('official:corporate:es:-:') && events[events.length - 1].includes('official:brand:es:ruze:missing'),
    (events[events.length - 1] || '').match(MARKER_RE));
  events.length = 0;
  EB.setOfficialContext({ lang: 'es', brand: 'ruze' });
  EB.setOfficialFooters(clone(ES_REFS));
  check('I5: an unchanged context/references does not re-emit', events.length === 0, `${events.length} events`);

  dom.window.close();
  console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
  process.exit(failed ? 1 : 0);
}

function firstDiff(a, b) {
  if (a === b) return '';
  let i = 0;
  while (i < a.length && a[i] === b[i]) i++;
  return `at ${i}: ${JSON.stringify(a.slice(Math.max(0, i - 80), i + 80))} vs ${JSON.stringify(b.slice(Math.max(0, i - 80), i + 80))}`;
}

main().catch((e) => { console.log(`FAIL  threw: ${e.stack}`); process.exit(1); });
