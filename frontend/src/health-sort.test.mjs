// Fork (brands UX, integrations BRANDS-UX-SPEC I1-I3). Run: npm run test:sort
// (node --test src/health-sort.test.mjs). No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  STATUS_RANK,
  ALARM_RANK,
  alarmRank,
  compareStatus,
  compareAlarms,
  compareNumber,
  compareDay,
  compareText,
  compareName,
  withBrandTiebreak,
  sortRows,
  isUnlaunchedBrandDoc,
  isUnlaunchedSesRow,
} from './health-sort.mjs'; // eslint-disable-line import/extensions

const sortVals = (vals, cmp, order) => [...vals].sort((a, b) => cmp(a, b, order));

// I1 ------------------------------------------------------------------------------------------
test('I1 compareStatus: descending is issues > warn > unknown > ok, an unrecognised string last', () => {
  assert.deepEqual(STATUS_RANK, {
    issues: 0, warn: 1, unknown: 2, ok: 3,
  });
  const vals = ['ok', 'bogus', 'unknown', 'issues', 'warn'];
  assert.deepEqual(sortVals(vals, compareStatus, 'desc'), ['issues', 'warn', 'unknown', 'ok', 'bogus']);
  assert.deepEqual(sortVals(vals, compareStatus, 'asc'), ['bogus', 'ok', 'unknown', 'warn', 'issues']);
  assert.equal(compareStatus(undefined, 'bogus', 'desc') === 0, true);
});

test('I1 compareAlarms: all five rungs, mixed pairs take the worse state, unrecognised = INSUFFICIENT_DATA', () => {
  const noSes = null;
  const noPair = { complaints: null, bounces: null };
  const ok = { complaints: 'OK', bounces: 'OK' };
  const insuf = { complaints: 'INSUFFICIENT_DATA', bounces: 'OK' };
  const alarm = { complaints: 'OK', bounces: 'ALARM' };
  assert.equal(alarmRank(noSes), ALARM_RANK.NO_SES);
  assert.equal(alarmRank(noPair), ALARM_RANK.NO_PAIR);
  assert.equal(alarmRank(ok), ALARM_RANK.OK);
  assert.equal(alarmRank(insuf), ALARM_RANK.INSUFFICIENT_DATA);
  assert.equal(alarmRank(alarm), ALARM_RANK.ALARM);
  assert.equal(alarmRank({ complaints: 'ALARM', bounces: 'INSUFFICIENT_DATA' }), ALARM_RANK.ALARM);
  assert.equal(alarmRank({ complaints: null, bounces: 'OK' }), ALARM_RANK.OK);
  assert.equal(alarmRank({ complaints: 'WEIRD', bounces: 'OK' }), ALARM_RANK.INSUFFICIENT_DATA);
  assert.equal(alarmRank({ complaints: 'WEIRD', bounces: null }), ALARM_RANK.INSUFFICIENT_DATA);
  const vals = [noPair, ok, noSes, alarm, insuf];
  assert.deepEqual(sortVals(vals, compareAlarms, 'desc'), [alarm, insuf, ok, noPair, noSes]);
  assert.deepEqual(sortVals(vals, compareAlarms, 'asc'), [noSes, noPair, ok, insuf, alarm]);
});

test('I1 compareNumber: numeric, missing (null / non-number / NaN) last in BOTH directions', () => {
  const vals = [5, null, 100, undefined, 0, 'x', 20, NaN];
  const desc = sortVals(vals, compareNumber, 'desc');
  assert.deepEqual(desc.slice(0, 4), [100, 20, 5, 0]);
  assert.equal(desc.slice(4).every((v) => typeof v !== 'number' || Number.isNaN(v)), true);
  const asc = sortVals(vals, compareNumber, 'asc');
  assert.deepEqual(asc.slice(0, 4), [0, 5, 20, 100]);
  assert.equal(asc.slice(4).every((v) => typeof v !== 'number' || Number.isNaN(v)), true);
});

test('I1 compareText: case-insensitive text; missing/empty last both ways', () => {
  const vals = ['zma', null, 'Listmonk', '', 'listmonk'];
  assert.deepEqual(sortVals(vals, compareText, 'asc'), ['Listmonk', 'listmonk', 'zma', null, '']);
  assert.deepEqual(sortVals(vals, compareText, 'desc'), ['zma', 'Listmonk', 'listmonk', null, '']);
});

test('I1 compareDay: the day string; missing last both ways', () => {
  const vals = ['2026-09-21', null, '2026-09-23', '2026-09-22'];
  assert.deepEqual(sortVals(vals, compareDay, 'desc'), ['2026-09-23', '2026-09-22', '2026-09-21', null]);
  assert.deepEqual(sortVals(vals, compareDay, 'asc'), ['2026-09-21', '2026-09-22', '2026-09-23', null]);
});

test('I1 compareName: case-insensitive, direction applied; withBrandTiebreak is always A->Z', () => {
  const nameOf = (r) => r.n;
  const cmp = compareName(nameOf);
  assert.ok(cmp({ n: 'alpha' }, { n: 'Beta' }, 'asc') < 0);
  assert.ok(cmp({ n: 'alpha' }, { n: 'Beta' }, 'desc') > 0);
  const tie = withBrandTiebreak(() => 0, nameOf);
  assert.ok(tie({ n: 'alpha' }, { n: 'Beta' }, 'desc') < 0);
  assert.ok(tie({ n: 'alpha' }, { n: 'Beta' }, 'asc') < 0);
});

// A fixture with ties in every column.
const nameOf = (r) => r.name;
const ROWS = [
  {
    name: 'delta', status: 'ok', sends: 10, day: '2026-09-22', alarms: { complaints: 'OK', bounces: 'OK' },
  },
  {
    name: 'Alpha', status: 'warn', sends: null, day: '2026-09-23', alarms: null,
  },
  {
    name: 'charlie', status: 'warn', sends: 10, day: '2026-09-23', alarms: { complaints: 'OK', bounces: 'OK' },
  },
  {
    name: 'Bravo', status: 'ok', sends: null, day: '2026-09-22', alarms: null,
  },
  {
    name: 'echo', status: 'issues', sends: 3, day: '2026-09-22', alarms: { complaints: 'ALARM', bounces: null },
  },
];
const COMPARATORS = {
  status: (a, b, o) => compareStatus(a.status, b.status, o),
  sends: (a, b, o) => compareNumber(a.sends, b.sends, o),
  day: (a, b, o) => compareDay(a.day, b.day, o),
  alarms: (a, b, o) => compareAlarms(a.alarms, b.alarms, o),
};
const names = (rows) => rows.map((r) => r.name);

test('I1 sortRows: primary direction, ties broken Brand A->Z in both directions', () => {
  assert.deepEqual(names(sortRows(ROWS, 'name', 'asc', nameOf, COMPARATORS)), ['Alpha', 'Bravo', 'charlie', 'delta', 'echo']);
  assert.deepEqual(names(sortRows(ROWS, 'name', 'desc', nameOf, COMPARATORS)), ['echo', 'delta', 'charlie', 'Bravo', 'Alpha']);
  assert.deepEqual(names(sortRows(ROWS, 'status', 'desc', nameOf, COMPARATORS)), ['echo', 'Alpha', 'charlie', 'Bravo', 'delta']);
  assert.deepEqual(names(sortRows(ROWS, 'status', 'asc', nameOf, COMPARATORS)), ['Bravo', 'delta', 'Alpha', 'charlie', 'echo']);
  assert.deepEqual(names(sortRows(ROWS, 'sends', 'desc', nameOf, COMPARATORS)), ['charlie', 'delta', 'echo', 'Alpha', 'Bravo']);
  assert.deepEqual(names(sortRows(ROWS, 'sends', 'asc', nameOf, COMPARATORS)), ['echo', 'charlie', 'delta', 'Alpha', 'Bravo']);
  assert.deepEqual(names(sortRows(ROWS, 'day', 'desc', nameOf, COMPARATORS)), ['Alpha', 'charlie', 'Bravo', 'delta', 'echo']);
  assert.deepEqual(names(sortRows(ROWS, 'alarms', 'desc', nameOf, COMPARATORS)), ['echo', 'charlie', 'delta', 'Alpha', 'Bravo']);
  assert.deepEqual(names(sortRows(ROWS, 'alarms', 'asc', nameOf, COMPARATORS)), ['Alpha', 'Bravo', 'charlie', 'delta', 'echo']);
});

test('I1 sortRows: a field with no comparator sorts by name in the given direction; input is not mutated', () => {
  const before = names(ROWS);
  assert.deepEqual(names(sortRows(ROWS, 'nope', 'asc', nameOf, COMPARATORS)), ['Alpha', 'Bravo', 'charlie', 'delta', 'echo']);
  assert.deepEqual(names(ROWS), before);
  assert.deepEqual(sortRows(null, 'name', 'asc', nameOf, COMPARATORS), []);
});

// I2 ------------------------------------------------------------------------------------------
test('I2 isUnlaunchedBrandDoc: status === "unknown" and nothing else', () => {
  assert.equal(isUnlaunchedBrandDoc({ status: 'unknown' }), true);
  assert.equal(isUnlaunchedBrandDoc({ status: 'unknown', note: 'configured — no send data yet' }), true);
  ['ok', 'warn', 'issues'].forEach((s) => assert.equal(isUnlaunchedBrandDoc({ status: s }), false));
  assert.equal(isUnlaunchedBrandDoc({}), false);
  assert.equal(isUnlaunchedBrandDoc({ status: null }), false);
  assert.equal(isUnlaunchedBrandDoc({ status: 2 }), false);
  assert.equal(isUnlaunchedBrandDoc(null), false);
});

// I3 ------------------------------------------------------------------------------------------
test('I3 isUnlaunchedSesRow: only unknown with no sends', () => {
  assert.equal(isUnlaunchedSesRow({ status: 'unknown', sends: 0 }), true);
  assert.equal(isUnlaunchedSesRow({ status: 'unknown', sends: null }), true);
  assert.equal(isUnlaunchedSesRow({ status: 'unknown', sends: 1 }), false);
  assert.equal(isUnlaunchedSesRow({ status: 'ok', sends: 0 }), false);
  assert.equal(isUnlaunchedSesRow({ status: 'warn', sends: 0 }), false);
  assert.equal(isUnlaunchedSesRow({ status: 'issues', sends: null }), false);
  assert.equal(isUnlaunchedSesRow(null), false);
});
