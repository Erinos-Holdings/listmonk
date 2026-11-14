// Fork (list grid; dashboard counts, integrations DASHBOARD-COUNTS-SPEC I3). Run: npm run
// test:dashboard-subs. The one lines rule shared by the Lists page and the Dashboard.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  GRID_LANGS, ZERO_ROW, gridLines, langGridLines, lineOf,
} from './gridLines.mjs'; // eslint-disable-line import/extensions

const row = (total) => ({
  active: total, held: 0, unsubscribed: 0, pending: 0, blocked: 0, total,
});
const keys = (lines) => lines.map((l) => l.key);

test('the display order of the send languages', () => {
  assert.deepEqual(GRID_LANGS, ['en', 'fr', 'es', 'de', 'it', 'other']);
});

test('one language: the all line alone', () => {
  assert.deepEqual(keys(gridLines({ all: row(5), en: row(5) }, true)), ['all']);
  assert.deepEqual(keys(gridLines({ all: row(3), fr: row(3) }, true)), ['all']);
});

test('two languages: all then each, in display order', () => {
  const g = { all: row(5), fr: row(2), en: row(3) };
  assert.deepEqual(keys(gridLines(g, true)), ['all', 'en', 'fr']);
});

test('a no-language residue shows the language lines, with none on en only', () => {
  const g = { all: row(5), en: row(5), none: row(2) };
  const lines = gridLines(g, true);
  assert.deepEqual(keys(lines), ['all', 'en']);
  assert.deepEqual(lines[1].none, g.none);
});

test('other shows the language lines even alone', () => {
  assert.deepEqual(keys(gridLines({ all: row(1), other: row(1) }, true)), ['all', 'other']);
  const lines = gridLines({ all: row(3), en: row(2), other: row(1) }, true);
  assert.deepEqual(keys(lines), ['all', 'en', 'other']);
  assert.equal(lines[2].none, null);
});

test('a zero-total language is no line', () => {
  const g = {
    all: row(5), en: row(5), fr: row(0), none: row(1),
  };
  assert.deepEqual(keys(gridLines(g, true)), ['all', 'en']);
});

test('languages disabled: the all line alone', () => {
  const g = {
    all: row(5), en: row(3), fr: row(2), other: row(1),
  };
  assert.deepEqual(keys(gridLines(g, false)), ['all']);
});

// integrations GLOBAL-LANG-SPEC I11 -- the Lists page (D7) and the Dashboard (D5) pick L's line.
test('GLOBAL-LANG I11: lineOf -- L\'s send-language line, EN includes none, a missing key is zero', () => {
  const g = {
    all: row(10), en: row(6), none: row(2), fr: row(3), other: row(1),
  };
  assert.deepEqual(lineOf(g, 'all'), g.all);
  assert.deepEqual(lineOf(g, undefined), g.all);
  assert.deepEqual(lineOf(g, 'en'), g.en); // the send-language en line (en plus none)
  assert.deepEqual(lineOf(g, 'fr'), g.fr);
  assert.deepEqual(lineOf(g, 'de'), ZERO_ROW); // never the all line
  assert.deepEqual(lineOf(undefined, 'es'), ZERO_ROW);
  assert.deepEqual(lineOf(undefined, 'all'), {});
});

test('GLOBAL-LANG I11: langGridLines -- under L the L line alone; under All or languages off, gridLines', () => {
  const g = {
    all: row(10), en: row(6), none: row(2), fr: row(3), other: row(1),
  };
  assert.deepEqual(langGridLines(g, true, 'all'), gridLines(g, true));
  assert.deepEqual(langGridLines(g, false, 'fr'), gridLines(g, false));
  assert.deepEqual(langGridLines(g, true, 'fr'), [{ key: 'fr', row: g.fr, none: null }]);
  assert.deepEqual(langGridLines(g, true, 'en'), [{ key: 'en', row: g.en, none: g.none }]);
  assert.deepEqual(langGridLines(g, true, 'it'), [{ key: 'it', row: ZERO_ROW, none: null }]);
});

test('an empty or missing grid: an all line with an empty row', () => {
  assert.deepEqual(gridLines(undefined, true), [{ key: 'all', row: {} }]);
  assert.deepEqual(gridLines({}, true), [{ key: 'all', row: {} }]);
});
