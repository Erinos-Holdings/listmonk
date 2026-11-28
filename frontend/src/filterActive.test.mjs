// Fork (campaign mode G5, 2026-11-27): a list page's filter field holding text has a red border
// (assets/style.scss .filter-active, the one rule). Every filter field -- a b-input with the magnify
// icon, or the Subscribers advanced query (ref="queryExp") -- binds 'filter-active' to its own
// v-model value. Run: yarn test.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));
const vueFiles = readdirSync(root, { recursive: true }).filter((f) => f.endsWith('.vue'));

test('every filter field binds filter-active to its own value', () => {
  let seen = 0;
  vueFiles.forEach((f) => {
    const src = readFileSync(join(root, f), 'utf8');
    [...src.matchAll(/<b-input\b(?:[^>"]|"[^"]*")*>/g)].forEach(([tag]) => {
      if (!/icon="magnify"|ref="queryExp"/.test(tag)) {
        return;
      }
      const model = /v-model="([^"]+)"/.exec(tag);
      assert.ok(model, `${f}: a filter field without v-model`);
      assert.ok(tag.includes(`:class="{ 'filter-active': !!${model[1]} }"`), `${f}: ${model[1]} lacks filter-active`);
      seen += 1;
    });
  });
  assert.ok(seen >= 7, `found ${seen}`);
  const css = readFileSync(join(root, 'assets/style.scss'), 'utf8');
  assert.match(css, /\.filter-active \.input,[\s\S]*?border-color: \$danger;/);
});
