// Fork (dashboard sends, integrations DASHBOARD-SENDS-SPEC follow-up: one Sends-bars + Clicks-line
// chart). Run: node --test src/sendsClicksChart.test.mjs. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  shapeSendsClicks, sendsClicksOptions, clicksAxisMax, SENDS_COLOR, SENDS_FILL, CLICKS_COLOR,
  CLICKS_HEADROOM, TODAY_FILL_ALPHA, TODAY_DASH, TODAY_OUTLINE_WIDTH, TODAY_POINT_RADIUS,
  todayIndexOf, todayTitle, todayOutline, withAlpha, styleToday,
} from './sendsClicksChart.mjs'; // eslint-disable-line import/extensions

const opts = { labelOf: (d) => d.slice(5), sendsLabel: 'Sends', clicksLabel: 'Clicks' };
const byLabel = (out, label) => out.datasets.find((d) => d.label === label);

test('Sends are bars on y (left), Clicks a line on y1 (right), aligned by date, the line drawn on top', () => {
  const out = shapeSendsClicks(
    [{ date: '2026-10-21', count: 8109 }, { date: '2026-10-22', count: 10999 }],
    [{ date: '2026-10-21', count: 207 }, { date: '2026-10-22', count: 395 }],
    opts,
  );
  assert.deepEqual(out.labels, ['10-21', '10-22']);
  const s = byLabel(out, 'Sends');
  const c = byLabel(out, 'Clicks');
  assert.deepEqual([s.type, s.yAxisID, s.backgroundColor, s.borderWidth], ['bar', 'y', SENDS_FILL, 0]);
  assert.deepEqual([c.type, c.yAxisID, c.borderColor], ['line', 'y1', CLICKS_COLOR]);
  assert.deepEqual(s.data, [8109, 10999]);
  assert.deepEqual(c.data, [207, 395]);
  assert.ok(c.order < s.order, 'chart.js draws the lowest order on top');
});

test('a day only one series has is a gap (null) in the other, never 0 -- the pre-read morning', () => {
  const out = shapeSendsClicks(
    [{ date: '2026-10-21', count: 5 }],
    [{ date: '2026-10-21', count: 1 }, { date: '2026-10-22', count: 0 }],
    opts,
  );
  assert.deepEqual(out.labels, ['10-21', '10-22']);
  assert.deepEqual(byLabel(out, 'Sends').data, [5, null]);
  assert.deepEqual(byLabel(out, 'Clicks').data, [1, 0]);
  assert.equal(byLabel(out, 'Clicks').spanGaps, false);
});

test('dates are sorted whatever the input order; both empty -> {}', () => {
  const out = shapeSendsClicks([], [{ date: '2026-10-22', count: 2 }, { date: '2026-10-20', count: 1 }], opts);
  assert.deepEqual(out.labels, ['10-20', '10-22']);
  assert.deepEqual(byLabel(out, 'Sends').data, [null, null]);
  assert.deepEqual(shapeSendsClicks([], [], opts), {});
  assert.deepEqual(shapeSendsClicks(null, undefined, opts), {});
});

test('the Clicks axis maximum is the headroom multiple of the busiest day, rounded up to a whole step; none without clicks', () => {
  assert.equal(CLICKS_HEADROOM, 2);
  assert.equal(clicksAxisMax([{ count: 120 }, { count: 395 }, { count: 0 }]), 800);
  assert.equal(clicksAxisMax([{ count: 425 }]), 900); // the odd "850" label
  assert.equal(clicksAxisMax([{ count: 825 }]), 2000);
  assert.equal(clicksAxisMax([{ count: 32 }]), 70);
  assert.equal(clicksAxisMax([{ count: 3 }]), 6);
  assert.equal(clicksAxisMax([{ count: 500 }]), 1000);
  assert.equal(clicksAxisMax([{ count: 0 }]), undefined);
  assert.equal(clicksAxisMax([]), undefined);
  assert.equal(clicksAxisMax(null), undefined);
});

// Fork (integrations TODAY-SO-FAR-SPEC I6) -- the partial day. The scriptable options are evaluated
// with fake chart.js contexts ({dataIndex} for elements, {p1DataIndex} for a line segment).
const todaySends = [
  { date: '2026-11-05', count: 11534, live: false },
  { date: '2026-11-06', count: 28919, live: true },
];
const todayClicks = [{ date: '2026-11-05', count: 300 }, { date: '2026-11-06', count: 324 }];

test('today: the Sends bar at 45% fill with the dashed outline mark, the Clicks segment into today dashed, today\'s point hollow', () => {
  const out = shapeSendsClicks(todaySends, todayClicks, { ...opts, today: '2026-11-06' });
  assert.deepEqual(out.dates, ['2026-11-05', '2026-11-06']);
  assert.deepEqual(out.live, [false, true]);
  const s = byLabel(out, 'Sends');
  const c = byLabel(out, 'Clicks');
  assert.equal(TODAY_FILL_ALPHA, 0.45);
  assert.equal(s.backgroundColor({ dataIndex: 0 }), SENDS_FILL);
  assert.equal(s.backgroundColor({ dataIndex: 1 }), withAlpha(SENDS_FILL, 0.45));
  assert.equal(withAlpha(SENDS_FILL, 0.45), 'rgba(134, 182, 239, 0.45)');
  assert.deepEqual(s.todayOutline, { index: 1, color: SENDS_COLOR });
  assert.equal(s.borderWidth, 0, 'the plugin draws the outline; the bar keeps no solid border');
  assert.deepEqual(c.segment.borderDash({ p1DataIndex: 1 }), [3, 2]);
  assert.equal(c.segment.borderDash({ p1DataIndex: 0 }), undefined);
  assert.deepEqual([c.pointBackgroundColor({ dataIndex: 1 }), c.pointBorderColor, c.pointRadius({ dataIndex: 1 })], ['#fff', CLICKS_COLOR, TODAY_POINT_RADIUS]);
  assert.deepEqual([c.pointBackgroundColor({ dataIndex: 0 }), c.pointRadius({ dataIndex: 0 })], [CLICKS_COLOR, 2]);
  assert.equal(TODAY_POINT_RADIUS, 3);
});

test('no today, or a today not among the dates: no styling -- the datasets exactly as before', () => {
  const plain = shapeSendsClicks(todaySends, todayClicks, opts);
  [undefined, null, '', '2026-11-07'].forEach((today) => {
    const out = shapeSendsClicks(todaySends, todayClicks, { ...opts, today });
    assert.deepEqual(out, plain, `today=${today}`);
    const s = byLabel(out, 'Sends');
    const c = byLabel(out, 'Clicks');
    assert.equal(s.backgroundColor, SENDS_FILL);
    assert.equal(s.todayOutline, undefined);
    assert.equal(c.segment, undefined);
    assert.equal(c.pointRadius, 2);
  });
  assert.equal(todayIndexOf(['a', 'b'], 'b'), 1);
  assert.equal(todayIndexOf(['a', 'b'], 'c'), -1);
  assert.equal(todayIndexOf(null, 'a'), -1);
  assert.deepEqual(styleToday(null, null, 0, {}), { bar: null, line: null });
});

test('the dashed outline plugin strokes the marked bar only: dash [3, 2], 1.5 px, the axis color; a hidden dataset or no mark draws nothing', () => {
  const calls = [];
  const ctx = {
    save: () => calls.push(['save']),
    restore: () => calls.push(['restore']),
    setLineDash: (d) => calls.push(['dash', d]),
    strokeRect: (...r) => calls.push(['rect', r, ctx.lineWidth, ctx.strokeStyle]),
  };
  const bar = {
    x: 50, y: 20, base: 120, width: 10,
  };
  const other = {
    x: 0, y: 0, base: 0, width: 0,
  };
  const chart = (datasets, hidden = false) => ({
    ctx,
    data: { datasets },
    getDatasetMeta: () => ({ hidden, data: [other, bar] }),
  });
  todayOutline.afterDatasetsDraw(chart([{ todayOutline: { index: 1, color: SENDS_COLOR } }]));
  assert.deepEqual(calls, [['save'], ['dash', [3, 2]], ['rect', [45, 20, 10, 100], TODAY_OUTLINE_WIDTH, SENDS_COLOR], ['restore']]);
  assert.deepEqual([...TODAY_DASH], [3, 2]);
  assert.equal(TODAY_OUTLINE_WIDTH, 1.5);
  calls.length = 0;
  todayOutline.afterDatasetsDraw(chart([{ todayOutline: { index: 1, color: SENDS_COLOR } }], true));
  todayOutline.afterDatasetsDraw(chart([{}]));
  todayOutline.afterDatasetsDraw(chart([{ todayOutline: { index: 5, color: SENDS_COLOR } }]));
  assert.deepEqual(calls, []);
});

test('the tooltip: today\'s title reads "today so far"; a live Sends point that is not today adds the live footer line', () => {
  const out = shapeSendsClicks(
    [{ date: '2026-11-04', count: 85, live: false }, { date: '2026-11-05', count: 11529, live: true }, { date: '2026-11-06', count: 28919, live: true }],
    [],
    { ...opts, today: '2026-11-06' },
  );
  const o = sendsClicksOptions({
    sendsLabel: 'Sends',
    clicksLabel: 'Clicks',
    dates: out.dates,
    live: out.live,
    today: '2026-11-06',
    todayFormat: (l) => `${l} — today so far`,
    liveNote: 'Sends: listmonk\'s own count',
  });
  const { title, footer } = o.plugins.tooltip.callbacks;
  assert.equal(title([{ label: '06 Nov', dataIndex: 2 }]), '06 Nov — today so far');
  assert.equal(title([{ label: '05 Nov', dataIndex: 1 }]), '05 Nov');
  assert.equal(title([]), '');
  assert.deepEqual(footer([{ dataIndex: 1 }]), ['Sends: listmonk\'s own count']);
  assert.deepEqual(footer([{ dataIndex: 2 }]), [], 'today: the title says it');
  assert.deepEqual(footer([{ dataIndex: 0 }]), []);
  assert.equal(todayTitle([{ label: 'x', dataIndex: 0 }], { dates: ['d'], today: undefined, format: () => 'no' }), 'x');
});

test('options: Sends axis left, Clicks axis right with its max, titled; no legend; each axis in its series color; index tooltip', () => {
  const o = sendsClicksOptions({ sendsLabel: 'Sends', clicksLabel: 'Clicks', clicksMax: 790 });
  assert.equal(o.scales.y.position, 'left');
  assert.equal(o.scales.y1.position, 'right');
  assert.equal(o.scales.y1.max, 790);
  assert.equal(o.scales.y.max, undefined);
  assert.equal(o.scales.y.title.text, 'Sends');
  assert.equal(o.scales.y1.title.text, 'Clicks');
  assert.equal(o.plugins.legend.display, false);
  assert.deepEqual([o.scales.y.title.color, o.scales.y.ticks.color], [SENDS_COLOR, SENDS_COLOR]);
  assert.deepEqual([o.scales.y1.title.color, o.scales.y1.ticks.color], [CLICKS_COLOR, CLICKS_COLOR]);
  assert.equal(o.interaction.mode, 'index');
  assert.equal(o.scales.x.offset, true);
  assert.equal(sendsClicksOptions({ sendsLabel: 'S', clicksLabel: 'C' }).scales.y1.max, undefined);
});
