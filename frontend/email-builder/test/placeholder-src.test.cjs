// integrations ALT-COPY-SPEC D2 / I9 — the agreement pin: the review Lambda's alt planning never
// describes the builder's placeholder picture, matched by URL (integrations
// `lib/campaign-review/alt-generate.ts::BUILDER_PLACEHOLDER_SRC`). This test pins the Add menu's
// Image block literal to that same URL, so changing the builder's placeholder fails here instead of
// silently reopening D2. A deliberate change edits this constant AND integrations' constant together.
const fs = require('fs');
const path = require('path');

const BUILDER_PLACEHOLDER_SRC = 'https://upload.wikimedia.org/wikipedia/commons/3/3f/Placeholder_view_vector.svg';

let failed = 0;
function check(name, ok, detail) { if (!ok) failed++; console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  [' + detail + ']' : ''}`); }

const file = path.join(__dirname, '..', 'src', 'documents', 'blocks', 'helpers', 'EditorChildrenIds', 'AddBlockMenu', 'buttons.tsx');
const src = fs.readFileSync(file, 'utf8');
// The Image entry's block: the `url:` literal after `label: 'Image'`, before the next `label:`.
const start = src.indexOf("label: 'Image'");
const end = start === -1 ? -1 : src.indexOf('label:', start + 1);
const entry = start === -1 ? '' : src.slice(start, end === -1 ? undefined : end);
const m = entry.match(/url:\s*'([^']*)'/);
check('the Add menu has an Image entry with a url literal', !!m, m ? '' : 'no url literal found');
check('the Add menu Image url equals the pinned placeholder (ALT-COPY-SPEC I9)', !!m && m[1] === BUILDER_PLACEHOLDER_SRC, m ? m[1] : '');

console.log(failed ? `\n${failed} FAILURES` : '\nALL PASS');
process.exit(failed ? 1 : 0);
