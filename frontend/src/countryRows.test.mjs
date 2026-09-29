// Fork (location stats, integrations LOCATION-STATS-SPEC I7). Run: npm run test:countries
// (node --test src/countryRows.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  DEFAULT_SORT,
  regionNames,
  countryName,
  shapeCountryRows,
  sortCountryRows,
} from './countryRows.mjs'; // eslint-disable-line import/extensions

const api = [
  { country: '', views: 900, clicks: 400 },
  { country: 'GB', views: 10, clicks: 1 },
  { country: 'US', views: 50, clicks: 3 },
  { country: 'FR', views: 0, clicks: 7 },
  { country: 'DE', views: 10, clicks: 5 },
  { country: 'CA', views: 10, clicks: 5 },
];
const codes = (rows) => rows.map((r) => r.country);

test('I7 localised name via Intl.DisplayNames, code as the fallback', () => {
  const en = regionNames('en');
  assert.equal(countryName('US', en), 'United States');
  assert.equal(countryName('DE', regionNames('de')), 'Deutschland');
  assert.equal(countryName('FR', null), 'FR');
  // A syntactically invalid region code throws in DisplayNames -- the code is shown.
  assert.equal(countryName('U1', en), 'U1');
  // An unusable locale falls back to English names rather than failing.
  assert.equal(countryName('GB', regionNames('not a locale!')), 'United Kingdom');
});

test('I7 the Unknown row is labelled and flagged; counts are coerced to numbers', () => {
  const rows = shapeCountryRows([{ country: '', views: 2, clicks: null }, { country: 'us', views: '3' }], { unknownLabel: 'Unknown' });
  assert.deepEqual(rows, [
    {
      country: 'US', name: 'United States', views: 0, clicks: 0, unknown: false,
    },
    {
      country: '', name: 'Unknown', views: 2, clicks: 0, unknown: true,
    },
  ]);
  assert.deepEqual(shapeCountryRows(null), []);
});

test('I7 default sort: Views desc, then Clicks desc, then name; Unknown last despite the largest counts', () => {
  assert.deepEqual(DEFAULT_SORT, { field: 'views', order: 'desc' });
  const rows = shapeCountryRows(api);
  // US 50; then the 10-view tie: CA/DE (5 clicks, name A-Z: Canada, Germany) before GB (1 click);
  // FR 0 views; Unknown last.
  assert.deepEqual(codes(rows), ['US', 'CA', 'DE', 'GB', 'FR', '']);
});

test('I7 column sorts keep Unknown last in both directions and tie-break by the default order', () => {
  const rows = shapeCountryRows(api);
  assert.deepEqual(codes(sortCountryRows(rows, 'views', 'asc')), ['FR', 'CA', 'DE', 'GB', 'US', '']);
  assert.deepEqual(codes(sortCountryRows(rows, 'clicks', 'desc')), ['FR', 'CA', 'DE', 'US', 'GB', '']);
  assert.deepEqual(codes(sortCountryRows(rows, 'clicks', 'asc')), ['GB', 'US', 'CA', 'DE', 'FR', '']);
  // Names: Canada, France, Germany, United Kingdom, United States.
  assert.deepEqual(codes(sortCountryRows(rows, 'name', 'asc')), ['CA', 'FR', 'DE', 'GB', 'US', '']);
  assert.deepEqual(codes(sortCountryRows(rows, 'name', 'desc')), ['US', 'GB', 'DE', 'FR', 'CA', '']);
  // Sorting never mutates its input.
  assert.deepEqual(codes(rows), ['US', 'CA', 'DE', 'GB', 'FR', '']);
});
