// Fork (emoji picker) -- integrations EMOJI-PICKER-SPEC I3 / I4: the picker's emoji data is the
// committed copy of the installed emoji-picker-element-data file, served on the admin surface
// under a filename that carries the data version (D3: the admin static route answers HEAD with
// 405, so a changed file at the same URL never reaches a browser that cached the old one), and
// no source points the picker at a third-party data source.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { EMOJI_DATA_PATH } from '../email-builder/src/emojiInsert.js'; // eslint-disable-line import/extensions, import/no-relative-packages

const frontend = path.join(path.dirname(fileURLToPath(import.meta.url)), '..');
const dataPkg = path.join(frontend, 'node_modules', 'emoji-picker-element-data');
const sha256 = (file) => createHash('sha256').update(fs.readFileSync(file)).digest('hex');

test('I3: EMOJI_DATA_PATH names the installed data version, and the committed file is its copy', () => {
  const { version } = JSON.parse(fs.readFileSync(path.join(dataPkg, 'package.json'), 'utf8'));
  assert.equal(EMOJI_DATA_PATH, `/admin/static/emoji-data-${version}.json`);

  const committed = path.join(frontend, 'public', 'static', `emoji-data-${version}.json`);
  assert.ok(fs.existsSync(committed), `${committed} is missing -- run yarn (postinstall) and commit it`);
  assert.equal(sha256(committed), sha256(path.join(dataPkg, 'en', 'emojibase', 'data.json')));
});

test('I4: no source passes a third-party dataSource (jsdelivr / unpkg)', () => {
  // A quoted string mentioning the CDN -- a comment that names one is not a dataSource.
  const cdnString = /['"`][^'"`\n]*\b(jsdelivr|unpkg)\b/;
  const hits = [];
  const walk = (dir) => {
    fs.readdirSync(dir, { withFileTypes: true }).forEach((e) => {
      const full = path.join(dir, e.name);
      if (e.isDirectory()) {
        if (e.name !== 'node_modules') walk(full);
      } else if (/\.(js|mjs|cjs|vue|ts|tsx)$/.test(e.name) && !/\.test\.mjs$/.test(e.name)) {
        fs.readFileSync(full, 'utf8').split('\n').forEach((line, i) => {
          if (cdnString.test(line)) hits.push(`${path.relative(frontend, full)}:${i + 1}`);
        });
      }
    });
  };
  walk(path.join(frontend, 'src'));
  walk(path.join(frontend, 'email-builder', 'src'));
  assert.deepEqual(hits, []);
});
