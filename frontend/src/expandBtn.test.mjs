// Fork: the +/- expand/collapse box has ONE definition (assets/style.scss .expand-btn). The
// button is empty (the rule draws frame and sign), carries aria-expanded (which picks the sign),
// and no page restyles its size or puts a glyph in it.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));
const vueFiles = readdirSync(root, { recursive: true }).filter((f) => f.endsWith('.vue'));

test('every .expand-btn is an empty button with aria-expanded', () => {
  let seen = 0;
  vueFiles.forEach((f) => {
    const src = readFileSync(join(root, f), 'utf8');
    [...src.matchAll(/class="expand-btn[ "]/g)].forEach((m) => {
      const tag = /^(?:[^>"]|"[^"]*")*>/.exec(src.slice(src.lastIndexOf('<', m.index)))[0];
      assert.match(tag, /^<button\s/, f);
      assert.ok(tag.endsWith('/>'), `${f}: the box takes no children`);
      assert.match(tag, /:aria-expanded=/, f);
      seen += 1;
    });
    assert.doesNotMatch(src, /^\.expand-btn/m, `${f}: style it in style.scss only`);
  });
  assert.ok(seen >= 12, `found ${seen}`);
});
