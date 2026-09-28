// CONTAINER-NESTING-SPEC I10: the structure UI wiring, mounted from the BUILT UMD under jsdom
// (the official-compile.test.cjs I5 pattern: EB.render + EB.resetDocument), asserting on the
// DOM and on the editor's own onChange output.
//
//   - with the c110-shaped fixture the flagged (⚠) tab is in the DOM without hover;
//   - clicking it shows the breadcrumb `Email › Container` (and the D3 Alert);
//   - Unwrap and Delete (through the confirm) emit a document with no orphaned keys and no
//     slot referencing a missing key -- this pins the §2.1 resetDocument commit rule (a merge
//     via setDocument would keep the removed keys);
//   - Insert above at the top of a flush container lands in the PARENT slot;
//   - storage that throws means Show structure off (and a remembered '1' means on);
//   - the Preview and HTML tabs, and the emitted HTML, carry no chrome markers.
// Pixel overlap, hover, the depth colours and real-browser persistence are the human gate
// (I10b / G5).
const path = require('path');
const { loadUmd, compileInputs } = require('./_umd.cjs');
const S = require(path.join(__dirname, '.build', 'documents', 'structure.cjs'));
const c110 = require('./fixtures/campaign110-shaped.json').body_source;

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${!ok && detail !== undefined ? '  [' + String(typeof detail === 'string' ? detail : JSON.stringify(detail)).slice(0, 400) + ']' : ''}`); }
const clone = (o) => JSON.parse(JSON.stringify(o));
const J = (o) => JSON.stringify(o);
const tick = (ms = 40) => new Promise((r) => setTimeout(r, ms));
const CHROME = /data-lm-structure|lm-structure-tab|data-lm-breadcrumb|data-lm-wrapper-alert|data-lm-confirm|data-lm-unwrap/;

const WRAPPER = c110.root.data.childrenIds[0];
const NINE = c110[WRAPPER].data.props.childrenIds;
const FOOTERS = c110.root.data.childrenIds.slice(1);

function orphanFree(doc) {
  const reach = S.reachableIds(doc);
  const keys = Object.keys(doc);
  const refs = [];
  for (const b of Object.values(doc)) refs.push(...S.childIdsOf(b));
  return { orphans: keys.filter((k) => !reach.has(k)), dangling: refs.filter((id) => !(id in doc)) };
}

async function mount(opts = {}) {
  const errors = [];
  const { dom, EB } = loadUmd(undefined, { ...opts, onError: (m) => errors.push(m) });
  const events = [];
  EB.render('visual-editor-container', { data: {}, onChange: (doc, html) => events.push({ doc: clone(doc), html: String(html) }) });
  const deadline = Date.now() + 10000;
  while (!EB.isRendered('visual-editor-container')) {
    if (Date.now() > deadline) throw new Error('builder never mounted');
    await tick(50);
  }
  await tick(100);
  const { context, refs } = compileInputs();
  EB.setOfficialFooters(refs);
  EB.setOfficialContext(context);
  const doc = dom.window.document;
  const $ = (sel) => doc.querySelector(sel);
  const $$ = (sel) => [...doc.querySelectorAll(sel)];
  const load = async (d) => { EB.resetDocument(clone(d)); await tick(); events.length = 0; };
  const last = () => events[events.length - 1];
  const click = async (el) => { el.click(); await tick(); };
  return { dom, EB, doc, $, $$, events, load, last, click, errors };
}

async function main() {
  // ------------------------------------------------------------ the c110 flow
  const ui = await mount();
  const { $, $$, load, last, click, doc } = ui;
  await load(c110);

  const flagged = $$('[data-lm-structure-flag]');
  check('I10: the flagged tab is in the DOM without hover', flagged.length === 1 && flagged[0].getAttribute('data-lm-structure-flag') === 'group', flagged.length);
  check('I10: its tooltip names D3 ("Groups 9 blocks…")', flagged[0] && /Groups 9 blocks with no styling of its own/.test(flagged[0].getAttribute('aria-label') || ''), flagged[0] && flagged[0].getAttribute('aria-label'));
  check('I10: an unflagged, unhovered, unselected container shows no tab (Show structure off)', $$('[data-lm-structure-tab="columns"]').length === 0);

  await click(flagged[0]);
  const crumbs = $$('[data-lm-breadcrumb-segment]').map((e) => e.textContent);
  check('I10: clicking the tab selects the wrapper -> breadcrumb `Email › Container`', J(crumbs) === J(['Email', 'Container']), crumbs);
  check('I10: the breadcrumb renders the › separator', ($('[data-lm-breadcrumb]') || {}).textContent === 'Email›Container', ($('[data-lm-breadcrumb]') || {}).textContent);
  const alert = $('[data-lm-wrapper-alert="group"]');
  check('I10: the flagged Container panel shows the D3 Alert with Unwrap', alert && /Groups 9 blocks/.test(alert.textContent) && alert.querySelector('[data-lm-unwrap]'));
  check('I10: Select parent is hidden when the parent is root', !$('[aria-label="Select parent"]'));

  // Breadcrumb deeper: select a block inside a Columns row via the store path the canvas uses.
  // (Clicking a nested block: its wrapper's onClick selects it.)
  const colText = doc.querySelector('[data-lm-text]');
  await click(colText);
  const deepCrumbs = $$('[data-lm-breadcrumb-segment]').map((e) => e.textContent);
  check('breadcrumb for a block inside the wrapper starts `Email › Container`', J(deepCrumbs.slice(0, 2)) === J(['Email', 'Container']) && deepCrumbs.length >= 3, deepCrumbs);
  await click($('[aria-label="Select parent"]'));
  check('Select parent climbs one level', $$('[data-lm-breadcrumb-segment]').length === deepCrumbs.length - 1, $$('[data-lm-breadcrumb-segment]').map((e) => e.textContent));
  const emailSeg = $$('[data-lm-breadcrumb-segment]')[0];
  await click(emailSeg);
  check('the `Email` segment clears the selection', $$('[data-lm-breadcrumb-segment]').length === 0);

  // Unwrap from the Alert (unstyled -> no confirm).
  await click($('[data-lm-structure-flag]'));
  await click($('[data-lm-unwrap]'));
  check('I10: Unwrap of an unstyled wrapper needs no confirm', !$('[data-lm-confirm]'));
  let ev = last();
  check('I10: Unwrap emits root = the 9 blocks then the two footers', ev && J(ev.doc.root.data.childrenIds) === J([...NINE, ...FOOTERS]), ev && ev.doc.root.data.childrenIds);
  check('I10: Unwrap emits a document with no orphaned key and no dangling reference', ev && J(orphanFree(ev.doc)) === J({ orphans: [], dangling: [] }), ev && orphanFree(ev.doc));
  check('I10: the wrapper key is gone from the emitted document', ev && !(WRAPPER in ev.doc));
  check('I10: the first child is selected after Unwrap', ($$('[data-lm-breadcrumb-segment]').pop() || {}).textContent === 'Image');
  check('I10: no flagged tab remains', $$('[data-lm-structure-flag]').length === 0);

  // Delete the wrapper, through the confirm.
  await load(c110);
  await click($('[data-lm-structure-flag]'));
  await click($('[aria-label="Delete"]'));
  const confirm = $('[data-lm-confirm="delete"]');
  const n = S.descendantsOf(c110, WRAPPER).length;
  check('I10: Delete on a container with children confirms first', Boolean(confirm));
  check(`I10: the confirm counts the ${n} blocks inside and points at Unwrap`, confirm && confirm.textContent.includes(`Delete this container and the ${n} blocks inside it?`) && confirm.textContent.includes('Use Unwrap to keep them.'), confirm && confirm.textContent);
  check('I10: nothing is emitted before the confirm', ui.events.length === 0, ui.events.length);
  await click($('[data-lm-confirm-ok="delete"]'));
  ev = last();
  check('I10: Delete emits a document with no orphaned key and no dangling reference', ev && J(orphanFree(ev.doc)) === J({ orphans: [], dangling: [] }), ev && orphanFree(ev.doc));
  check('I10: Delete removed the whole subtree', ev && J(Object.keys(ev.doc).sort()) === J(['root', ...FOOTERS].sort()), ev && Object.keys(ev.doc));

  // Cancel leaves the document alone.
  await load(c110);
  await click($('[data-lm-structure-flag]'));
  await click($('[aria-label="Delete"]'));
  const cancel = [...(($('[data-lm-confirm="delete"]') || { querySelectorAll: () => [] }).querySelectorAll('button'))].find((b) => b.textContent === 'Cancel');
  await click(cancel);
  check('Cancel closes the confirm without emitting', !$('[data-lm-confirm]') && ui.events.length === 0);

  // Insert above at the top of a flush container -> the PARENT slot.
  await load(c110);
  await click($('[data-lm-structure-flag]'));
  await click($('[aria-label="Insert above"]'));
  const textBtn = $$('.MuiMenu-paper button').find((b) => b.textContent === 'Text');
  const officialBtn = $$('.MuiMenu-paper button').find((b) => b.textContent === 'Official footer');
  check('Insert above offers the Official-footer entry for a root-slot position', Boolean(officialBtn));
  await click(textBtn);
  ev = last();
  const rootIds = ev ? ev.doc.root.data.childrenIds : [];
  check('I10: Insert above lands in the parent slot, above the container', rootIds.length === 4 && rootIds[1] === WRAPPER && ev.doc[rootIds[0]].type === 'Text', rootIds);
  check('I10: the container keeps its 9 children', ev && J(ev.doc[WRAPPER].data.props.childrenIds) === J(NINE));
  check('I10: the new block is selected', ($$('[data-lm-breadcrumb-segment]').map((e) => e.textContent).join('/')) === 'Email/Text');

  // Insert below from inside a column: the column slot (no Official-footer entry there).
  await load(c110);
  const colBlock = c110[NINE[2]].data.props.columns[0].childrenIds[0];
  const colTargets = $$('img').filter((img) => (img.getAttribute('src') || '').includes('17.45.09'));
  await click(colTargets[0]);
  check('column block selected (breadcrumb names the column)', /Columns \(col 1\)/.test($$('[data-lm-breadcrumb-segment]').map((e) => e.textContent).join('/')), $$('[data-lm-breadcrumb-segment]').map((e) => e.textContent));
  await click($('[aria-label="Insert below"]'));
  check('no Official-footer entry for a column slot', !$$('.MuiMenu-paper button').some((b) => b.textContent === 'Official footer'));
  await click($$('.MuiMenu-paper button').find((b) => b.textContent === 'Text'));
  ev = last();
  const col0 = ev ? ev.doc[NINE[2]].data.props.columns[0].childrenIds : [];
  check('Insert below lands after the block in its column slot', col0.length === 2 && col0[0] === colBlock, col0);
  check('Insert keeps the Columns props', ev && ev.doc[NINE[2]].data.props.columnsGap === c110[NINE[2]].data.props.columnsGap && ev.doc[NINE[2]].data.props.columnsCount === 2);

  // A styled container: Unwrap confirms, listing the styling that will be lost.
  const styled = clone(c110);
  styled[WRAPPER].data.style = { padding: { top: 16, bottom: 16, left: 24, right: 24 }, backgroundColor: '#0b3c49' };
  await load(styled);
  check('a styled container is not flagged', $$('[data-lm-structure-flag]').length === 0);
  await click(doc.querySelector('[data-lm-text]'));
  await click($('[aria-label="Select parent"]'));
  check('Select parent reaches the styled wrapper', J($$('[data-lm-breadcrumb-segment]').map((e) => e.textContent)) === J(['Email', 'Container']));
  await click($('[aria-label="Unwrap"]'));
  const uc = $('[data-lm-confirm="unwrap"]');
  check('D4: Unwrap of a styled container confirms, listing the styling', uc && uc.textContent.includes('padding 16/24/16/24') && uc.textContent.includes('background #0b3c49'), uc && uc.textContent);
  await click($('[data-lm-confirm-ok="unwrap"]'));
  ev = last();
  check('D4: the confirmed Unwrap emits no orphan', ev && J(ev.doc.root.data.childrenIds) === J([...NINE, ...FOOTERS]) && J(orphanFree(ev.doc)) === J({ orphans: [], dangling: [] }));

  // D9: a column container holding a footer -> Unwrap disabled.
  const d9 = clone(c110);
  d9['col-box'] = { type: 'Container', data: { style: null, props: { childrenIds: [FOOTERS[1]] } } };
  d9.root.data.childrenIds = [WRAPPER, FOOTERS[0]];
  d9[NINE[2]].data.props.columns[1].childrenIds = ['col-box'];
  await load(d9);
  const d9tab = $$('[data-lm-structure-flag="single"]')[0];
  await click(d9tab);
  const unwrapBtn = $('[aria-label="Unwrap"]');
  check('D9: Unwrap is disabled for a column container holding the footer', unwrapBtn && unwrapBtn.disabled === true);
  check('D9: the Alert\'s Unwrap is disabled too, with the reason', $('[data-lm-unwrap]') && $('[data-lm-unwrap]').disabled && /column is not a footer position/.test($('[data-lm-unwrap]').getAttribute('title') || ''));

  // D13 through the menu: duplicating the wrapper emits no second footer pair.
  const d13 = clone(c110);
  d13[WRAPPER].data.props.childrenIds = [...NINE.slice(0, 2), ...FOOTERS];
  d13.root.data.childrenIds = [WRAPPER];
  await load(d13);
  await click($('[data-lm-structure-flag]'));
  await click($('[aria-label="Duplicate"]'));
  ev = last();
  check('D13: Duplicate of a Container holding the pair emits no second OfficialFooter', ev && Object.values(ev.doc).filter((b) => b.type === 'OfficialFooter').length === 2 && ev.doc.root.data.childrenIds.length === 2);

  // Show structure: off by default (empty storage), on by the toggle, remembered.
  await load(c110);
  const toggle = $('[aria-label="Show structure"]');
  check('Show structure is off by default', toggle && toggle.getAttribute('aria-pressed') === 'false');
  await click(toggle);
  check('Show structure on: every container shows its tab', $('[aria-label="Show structure"]').getAttribute('aria-pressed') === 'true' && $$('[data-lm-structure-tab="columns"]').length === 2 && $$('[data-lm-structure-tab]').length === 3);
  check('Show structure is remembered under lm-eb-show-structure', ui.dom.window.localStorage.getItem('lm-eb-show-structure') === '1');
  check('positive control: the editor canvas does carry chrome markers', CHROME.test($('#visual-editor-container').innerHTML));

  // Preview and HTML carry no chrome markers (with Show structure on and a block selected).
  await click($('[data-lm-structure-flag]'));
  const tabs = $$('[role="tab"]');
  const mainTab = (i) => tabs.filter((t) => !['Styles', 'Inspect'].includes(t.textContent))[i];
  await click(mainTab(1));
  const canvasArea = () => {
    const drawer = $('.sidebar');
    const html = $('#visual-editor-container').innerHTML;
    return drawer ? html.replace(drawer.outerHTML, '') : html;
  };
  check('I10: Preview carries no chrome markers', !CHROME.test(canvasArea()) && /RUZE/i.test(canvasArea()), (canvasArea().match(CHROME) || [])[0]);
  await click(mainTab(2));
  const htmlText = canvasArea();
  check('I10: the HTML tab carries no chrome markers', !CHROME.test(htmlText.replace(/<[^>]+>/g, '')) && /&lt;|<!DOCTYPE|html/i.test(htmlText), (htmlText.replace(/<[^>]+>/g, '').match(CHROME) || [])[0]);
  check('I10: no emitted HTML ever carried a chrome marker', ui.events.every((e) => !CHROME.test(e.html)));
  await click(mainTab(0));
  ui.dom.window.close();

  // ------------------------------------------------------------ storage that throws -> off
  const denied = await mount({
    beforeScript: (w) => {
      Object.defineProperty(w, 'localStorage', { configurable: true, get() { throw new w.DOMException('denied', 'SecurityError'); } });
    },
  });
  await denied.load(c110);
  const t2 = denied.$('[aria-label="Show structure"]');
  check('I10: storage that throws -> Show structure off', t2 && t2.getAttribute('aria-pressed') === 'false' && denied.$$('[data-lm-structure-tab="columns"]').length === 0);
  check('I10: … and the flagged tab is still shown', denied.$$('[data-lm-structure-flag]').length === 1);
  await denied.click(t2);
  check('the toggle still works for the session when storage throws', denied.$('[aria-label="Show structure"]').getAttribute('aria-pressed') === 'true' && denied.$$('[data-lm-structure-tab="columns"]').length === 2);
  denied.dom.window.close();

  // ------------------------------------------------------------ a remembered '1' -> on
  const remembered = await mount({ beforeScript: (w) => w.localStorage.setItem('lm-eb-show-structure', '1') });
  await remembered.load(c110);
  check('a remembered lm-eb-show-structure=1 starts with Show structure on', remembered.$('[aria-label="Show structure"]').getAttribute('aria-pressed') === 'true' && remembered.$$('[data-lm-structure-tab]').length === 3);
  remembered.dom.window.close();

  console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
  process.exit(failed ? 1 : 0);
}

main().catch((e) => { console.log(`FAIL  threw: ${e.stack}`); process.exit(1); });
