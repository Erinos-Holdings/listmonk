// Fork (integrations EDITOR-DEFAULTS-SPEC I2-I5) -- the campaign editor's defaults:
//   I2 "first load: campaign gets the pair / template and official do not" (firstLoadDocument);
//   I3 "withOfficialFooters: both / neither / brand-only / corporate-only / in a column";
//   I4 "applyLinkDefault" (only an empty Image/Button link is filled; never overwritten);
//   I5 "insert paths apply the link default" -- every Add-menu and Insert above/below insertion
//      builds its block through BlocksMenu's freshBlock (the one menu EditorChildrenIds' Add
//      button and TuneMenu's Insert above/below both render).
// The host wiring (TemplateForm never sets `campaign`) has no unit seam: it is the spec's G5 step
// "a new template opens with no footers".
const path = require('path');
const fs = require('fs');
const { createRequire } = require('module');

const { withOfficialFooters, firstLoadDocument, insertOfficialFooter } = require(path.join(__dirname, '.build', 'official', 'insert.cjs'));
const { applyLinkDefault, freshBlock, RETIRED_BUTTON_PLACEHOLDER } = require(path.join(__dirname, '.build', 'linkDefaults.cjs'));

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }
const J = (v) => JSON.stringify(v);

const base = () => ({
  root: { type: 'EmailLayout', data: { backdropColor: '#F5F5F5', childrenIds: ['a', 'box', 'b'] } },
  a: { type: 'Text', data: { props: { text: 'a' } } },
  b: { type: 'Text', data: { props: { text: 'b' } } },
  box: { type: 'Container', data: { style: {}, props: { childrenIds: ['c'] } } },
  c: { type: 'Text', data: { props: { text: 'c' } } },
  cols: { type: 'ColumnsContainer', data: { props: { columns: [{ childrenIds: [] }, { childrenIds: [] }, { childrenIds: [] }] } } },
});
const kinds = (doc, ids) => ids.map((id) => (doc[id].type === 'OfficialFooter' ? `OF:${doc[id].data.props.kind}` : id));
const rootKinds = (doc) => kinds(doc, doc.root.data.childrenIds);
const OF = (kind) => ({ type: 'OfficialFooter', data: { props: { kind } } });

// ---- I3: withOfficialFooters --------------------------------------------------------------
{
  const neither = base();
  const frozen = J(neither);
  const n = withOfficialFooters(neither);
  check('I3 neither: the pair at the end of root', J(rootKinds(n)) === J(['a', 'box', 'b', 'OF:brand', 'OF:corporate']), J(rootKinds(n)));
  check('I3 neither: input not mutated', J(neither) === frozen);

  const both = insertOfficialFooter(base(), 'root', 1, null);
  check('I3 both: the SAME document', withOfficialFooters(both) === both);
  const bothBox = insertOfficialFooter(base(), 'box', 0, null);
  check('I3 both inside a Container: the SAME document', withOfficialFooters(bothBox) === bothBox);

  const brandOnly = base();
  brandOnly.fb = OF('brand');
  brandOnly.root.data.childrenIds = ['a', 'fb', 'box', 'b'];
  const bo = withOfficialFooters(brandOnly);
  check('I3 brand-only: corporate right after the brand block', J(rootKinds(bo)) === J(['a', 'OF:brand', 'OF:corporate', 'box', 'b']), J(rootKinds(bo)));

  const corpOnly = base();
  corpOnly.fc = OF('corporate');
  corpOnly.root.data.childrenIds = ['a', 'box', 'fc', 'b'];
  const co = withOfficialFooters(corpOnly);
  check('I3 corporate-only: brand right before the corporate block', J(rootKinds(co)) === J(['a', 'box', 'OF:brand', 'OF:corporate', 'b']), J(rootKinds(co)));

  const corpInBox = base();
  corpInBox.fc = OF('corporate');
  corpInBox.box.data.props.childrenIds = ['c', 'fc'];
  const cib = withOfficialFooters(corpInBox);
  check('I3 corporate-only inside a Container: brand before it, in that Container', J(kinds(cib, cib.box.data.props.childrenIds)) === J(['c', 'OF:brand', 'OF:corporate']) && J(rootKinds(cib)) === J(['a', 'box', 'b']), J(kinds(cib, cib.box.data.props.childrenIds)));

  const inColumn = base();
  inColumn.root.data.childrenIds = ['a', 'cols', 'b'];
  inColumn.fb = OF('brand');
  inColumn.cols.data.props.columns[1].childrenIds = ['fb'];
  const ic = withOfficialFooters(inColumn);
  check('I3 in a column: the missing kind at the end of root', J(rootKinds(ic)) === J(['a', 'cols', 'b', 'OF:corporate']) && J(ic.cols.data.props.columns[1].childrenIds) === J(['fb']), J(rootKinds(ic)));

  const corpInColumn = base();
  corpInColumn.root.data.childrenIds = ['a', 'cols', 'b'];
  corpInColumn.fc = OF('corporate');
  corpInColumn.cols.data.props.columns[0].childrenIds = ['fc'];
  const cic = withOfficialFooters(corpInColumn);
  check('I3 corporate in a column: brand at the end of root', J(rootKinds(cic)) === J(['a', 'cols', 'b', 'OF:brand']), J(rootKinds(cic)));

  const orphan = base();
  orphan.old = OF('brand');
  const o = withOfficialFooters(orphan);
  check('I3 an orphaned footer does not count (the pair is added at root end)', J(rootKinds(o)) === J(['a', 'box', 'b', 'OF:brand', 'OF:corporate']), J(rootKinds(o)));

  const notADoc = { a: { type: 'Text', data: {} } };
  check('I3 a document with no EmailLayout root: unchanged', withOfficialFooters(notADoc) === notADoc);
}

// ---- I2: firstLoadDocument ------------------------------------------------------------------
{
  const start = base();
  const camp = firstLoadDocument(start, { lang: 'en', brand: 'ruze', campaign: true });
  check('I2 first load: campaign gets the pair', J(rootKinds(camp)) === J(['a', 'box', 'b', 'OF:brand', 'OF:corporate']), J(rootKinds(camp)));
  check('I2 first load: campaign with no brand yet still gets the pair', J(rootKinds(firstLoadDocument(base(), { lang: 'en', brand: null, campaign: true }))) === J(['a', 'box', 'b', 'OF:brand', 'OF:corporate']));
  check('I2 first load: template (no campaign flag) does not', firstLoadDocument(start, { lang: 'en', brand: 'curated', official: false }) === start);
  check('I2 first load: official does not', firstLoadDocument(start, { lang: 'en', brand: 'curated', official: true, campaign: true }) === start);
  check('I2 first load: no context does not', firstLoadDocument(start, null) === start && firstLoadDocument(start, undefined) === start);
  check('I2 first load: campaign flag must be exactly true', firstLoadDocument(start, { campaign: 'yes' }) === start);
  // Stage 4 S4-4: a non-editable campaign (the host sends campaign: canEdit = false) and a stored
  // body with no source are left alone; an empty or blank stored body still gets the pair.
  check('S4-4 first load: a campaign that cannot be edited (campaign: false) does not', firstLoadDocument(start, { lang: 'en', brand: 'ruze', campaign: false }) === start);
  check('S4-4 first load: a stored body with no source is kept', firstLoadDocument(start, { campaign: true }, '<html><body>stored</body></html>') === start);
  check('S4-4 first load: an empty or blank stored body still gets the pair',
    J(rootKinds(firstLoadDocument(base(), { campaign: true }, ''))) === J(['a', 'box', 'b', 'OF:brand', 'OF:corporate'])
    && J(rootKinds(firstLoadDocument(base(), { campaign: true }, '  \n'))) === J(['a', 'box', 'b', 'OF:brand', 'OF:corporate'])
    && J(rootKinds(firstLoadDocument(base(), { campaign: true }, null))) === J(['a', 'box', 'b', 'OF:brand', 'OF:corporate']));
  // The host passes the stored body on the first load (VisualEditor.vue), and Editor.vue binds it.
  const hostSrc = (rel) => fs.readFileSync(path.join(__dirname, '..', '..', 'src', rel), 'utf8');
  check('S4-4 host: VisualEditor passes storedBody to firstLoadDocument', /firstLoadDocument\(em\.DEFAULT_SOURCE, this\.officialContext, this\.storedBody\)/.test(hostSrc(path.join('components', 'VisualEditor.vue'))));
  check('S4-4 host: Editor.vue binds the stored body', /:stored-body="self\.body \|\| ''"/.test(hostSrc(path.join('components', 'Editor.vue'))));
  check('S4-4 / S4b-1 host: Campaign.vue sends campaign: this.canEdit && this.canManage', /campaign: this\.canEdit && this\.canManage,/.test(hostSrc(path.join('views', 'Campaign.vue'))));
}

// ---- I4: applyLinkDefault -------------------------------------------------------------------
{
  const D = '{{ .Subscriber.Attribs.shop_links.thirstygirl.url }}';
  const img = (linkHref) => ({ type: 'Image', data: { style: {}, props: { url: 'u', ...(linkHref === 'ABSENT' ? {} : { linkHref }) } } });
  const btn = (url) => ({ type: 'Button', data: { style: {}, props: { text: 'Shop', ...(url === 'ABSENT' ? {} : { url }) } } });

  for (const v of [null, '', '  ', 'ABSENT']) {
    const out = applyLinkDefault(img(v), D);
    check(`I4 Image linkHref ${J(v)} is filled`, out.data.props.linkHref === D);
  }
  for (const v of ['', null, 'ABSENT', RETIRED_BUTTON_PLACEHOLDER]) {
    const out = applyLinkDefault(btn(v), D);
    check(`I4 Button url ${J(v)} is filled`, out.data.props.url === D);
  }
  const setImg = img('https://example.com/p');
  check('I4 a set Image link is never overwritten (same object)', applyLinkDefault(setImg, D) === setImg);
  const setBtn = btn('https://example.com/p');
  check('I4 a set Button link is never overwritten (same object)', applyLinkDefault(setBtn, D) === setBtn);
  const empty = img(null);
  check('I4 a null default leaves the block as is', applyLinkDefault(empty, null) === empty && applyLinkDefault(empty, undefined) === empty && applyLinkDefault(empty, '') === empty);
  const text = { type: 'Text', data: { props: { text: 'x', url: '' } } };
  check('I4 other block types are untouched', applyLinkDefault(text, D) === text);
  const before = J(img(null));
  const src = img(null);
  applyLinkDefault(src, D);
  check('I4 the input block is not mutated', J(src) === before);
  const kept = applyLinkDefault(img(null), D);
  check('I4 every other prop and the style are kept', kept.data.props.url === 'u' && J(kept.data.style) === '{}');
}

// ---- I5: insert paths apply the link default -----------------------------------------------
{
  const builderRoot = path.join(__dirname, '..');
  const ts = require(path.join(builderRoot, 'node_modules', 'typescript'));
  const builderRequire = createRequire(path.join(builderRoot, 'package.json'));
  const read = (rel) => fs.readFileSync(path.join(builderRoot, 'src', rel), 'utf8');
  const transpile = (rel) => ts.transpileModule(read(rel), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, esModuleInterop: true, jsx: ts.JsxEmit.React } }).outputText;
  const evaluate = (js, requireMap) => {
    const mod = { exports: {} };
    new Function('module', 'exports', 'require', js)(mod, mod.exports, (n) => {
      if (n in requireMap) return requireMap[n];
      throw new Error(`unexpected import "${n}"`);
    });
    return mod.exports;
  };
  const MENU = path.join('documents', 'blocks', 'helpers', 'EditorChildrenIds', 'AddBlockMenu');
  const iconStub = new Proxy({}, { get: () => () => null });
  const { BUTTONS } = evaluate(transpile(path.join(MENU, 'buttons.tsx')), { react: builderRequire('react'), '@mui/icons-material': iconStub, '../../../../editor/core': {} });

  const D = 'https://curatedfor.you/shala';
  for (const b of BUTTONS) {
    const out = freshBlock(b, D);
    if (b.label === 'Image') check('I5 Add menu: a fresh Image carries the default link', out.data.props.linkHref === D);
    else if (b.label === 'Button') check('I5 Add menu: a fresh Button carries the default link', out.data.props.url === D);
    else check(`I5 Add menu: ${b.label} is untouched`, J(out) === J(b.block()));
  }
  const btn = BUTTONS.find((b) => b.label === 'Button');
  check('I5 the listmonk.app placeholder is retired: a fresh Button with no default has url ""', freshBlock(btn, null).data.props.url === '');

  // Every insertion path reaches freshBlock: BlocksMenu is the only consumer of BUTTONS and builds
  // every block through freshBlock with the context's linkDefault; EditorChildrenIds' Add button
  // (AddBlockMenu/index.tsx) and TuneMenu's Insert above/below both render BlocksMenu.
  const blocksMenu = read(path.join(MENU, 'BlocksMenu.tsx'));
  check('I5 BlocksMenu builds every block through freshBlock(k, linkDefault)', /onClick\(freshBlock\(k, linkDefault\)\)/.test(blocksMenu) && !/k\.block\(\)/.test(blocksMenu), 'BlocksMenu.tsx');
  check('I5 BlocksMenu reads linkDefault from the official context', /useOfficialContext\(\)/.test(blocksMenu) && /officialContext\.linkDefault/.test(blocksMenu));
  check('I5 the Add button renders BlocksMenu', /<BlocksMenu\b/.test(read(path.join(MENU, 'index.tsx'))));
  check('I5 EditorChildrenIds uses the Add button', /<AddBlockButton\b/.test(read(path.join('documents', 'blocks', 'helpers', 'EditorChildrenIds', 'index.tsx'))));
  check('I5 TuneMenu (Insert above/below) renders BlocksMenu', /<BlocksMenu\b/.test(read(path.join('documents', 'blocks', 'helpers', 'block-wrappers', 'TuneMenu.tsx'))));
  const users = [];
  (function walk(dir) {
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, e.name);
      if (e.isDirectory()) walk(full);
      else if (/\.(ts|tsx)$/.test(e.name) && /\bBUTTONS\b/.test(fs.readFileSync(full, 'utf8'))) users.push(path.relative(path.join(builderRoot, 'src'), full));
    }
  })(path.join(builderRoot, 'src'));
  check('I5 BUTTONS is read only by buttons.tsx and BlocksMenu.tsx', J(users.sort()) === J([path.join(MENU, 'BlocksMenu.tsx'), path.join(MENU, 'buttons.tsx')].sort()), J(users));
}

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
