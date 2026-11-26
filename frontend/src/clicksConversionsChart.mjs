// Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D11 follow-up, 2026-10-29) --
// the Dashboard's Clicks + conversions chart, under Sends + clicks and on the same window. Clicks
// are pale BARS on the left axis; Tracked (UTM) conversions a LINE on the right axis -- sales in
// dollars by default, orders with the $|# toggle. "Show Matched" (on by default) adds Matched as a
// FILL of the line's hue beneath it: Matched is nearly always above Tracked, so the shaded area
// above the line reads as the orders our click data found that the UTM export did not.
// Rendering only -- every value is a stored daily sum (the charts statement).
//
// Days are the purchase day (Mountain), so a send's response is the trailing hump after its click
// bars, and the newest days fill in as orders land inside the 5-day Matched window.
//
// Colors: the bars are a pale tint of the top chart's Clicks orange (so Clicks keeps one hue across
// both charts); the line a deep green (money), its axis text in the same green; the Matched fill
// that green at a moderate alpha. There is no legend: each axis's title and numbers wear its series'
// color. The tooltip always lists all four values for the hovered day, whatever is toggled.

import {
  CLICKS_COLOR, styleToday, todayIndexOf, todayTitle,
} from './sendsClicksChart.mjs'; // eslint-disable-line import/extensions
import { formatUsd } from './conversionCells.mjs'; // eslint-disable-line import/extensions

export const CLICKS_FILL = '#f2c2a6';
export const CONV_COLOR = '#166534';
export const MATCHED_FILL = 'rgba(22, 101, 52, 0.38)';
export const UNITS = Object.freeze(['usd', 'orders']);

const num = (v) => (Number.isFinite(Number(v)) ? Number(v) : 0);
const pick = (day, measure, unit) => num(day && day[measure] && day[measure][unit === 'usd' ? 'sales' : 'orders']);

// clicks: [{date, count}]; conversions: [{date, tracked: {orders, sales}, matched: {orders, sales}}]
// as GET /api/dashboard/charts returns them. Returns {} when there are no conversion days (no
// conversions doc yet) -- the host renders nothing rather than a false zero line. `today` (the
// response's) marks the partial day on the Clicks bar and the Tracked line through the top chart's
// one helper (TODAY-SO-FAR-SPEC D5); the Matched fill is unchanged beneath the dashed line.
export function shapeClicksConversions(clicks, conversions, {
  labelOf, clicksLabel, matchedLabel, trackedLabel, unit = 'usd', showMatched = true, today,
}) {
  const conv = new Map((conversions || []).map((d) => [d.date, d]));
  if (conv.size === 0) {
    return {};
  }
  const c = new Map((clicks || []).map((d) => [d.date, d.count]));
  const dates = [...new Set([...c.keys(), ...conv.keys()])].sort();
  const series = (measure) => dates.map((d) => (conv.has(d) ? pick(conv.get(d), measure, unit) : null));
  const datasets = [
    // chart.js draws the lowest `order` on top: the Tracked line, then the Matched fill, then bars.
    {
      type: 'line',
      label: trackedLabel,
      data: series('tracked'),
      yAxisID: 'y1',
      order: 0,
      borderColor: CONV_COLOR,
      backgroundColor: CONV_COLOR,
      borderWidth: 2,
      pointRadius: 2,
      pointHoverRadius: 5,
      spanGaps: false,
    },
  ];
  if (showMatched) {
    datasets.push({
      type: 'line',
      label: matchedLabel,
      data: series('matched'),
      yAxisID: 'y1',
      order: 1,
      fill: 'origin',
      backgroundColor: MATCHED_FILL,
      borderColor: 'transparent',
      borderWidth: 0,
      pointRadius: 0,
      pointHoverRadius: 0,
      spanGaps: false,
    });
  }
  datasets.push({
    type: 'bar',
    label: clicksLabel,
    data: dates.map((d) => (c.has(d) ? c.get(d) : null)),
    yAxisID: 'y',
    order: 2,
    backgroundColor: CLICKS_FILL,
    borderWidth: 0,
    borderRadius: 2,
  });
  styleToday(datasets[datasets.length - 1], datasets[0], todayIndexOf(dates, today), {
    fill: CLICKS_FILL, outline: CLICKS_COLOR, lineColor: CONV_COLOR,
  });
  return { labels: dates.map(labelOf), dates, datasets };
}

// The right axis maximum: exactly twice the highest Matched value, unrounded, so the Matched peak
// sits at half the chart's height in $ and in # alike and under the click bars. Matched whether or
// not it is drawn, so the toggle never rescales the axis (a rare Tracked day above Matched still
// fits: Tracked <= 2 x Matched). undefined (chart.js picks) when every Matched value is zero.
export function conversionsAxisMax(conversions, { unit = 'usd' } = {}) {
  const top = Math.max(0, ...(conversions || []).map((d) => pick(d, 'matched', unit)));
  return top > 0 ? top * 2 : undefined;
}

// tooltipLines is the four values for one day, for the tooltip footer -- always all four, so the
// overlay and the toggle only ever change what is drawn, never what can be read.
export function tooltipLines(day, { matchedLabel, trackedLabel, ordersLabel }) {
  const line = (label, measure) => `${label}: ${formatUsd(pick(day, measure, 'usd'))} · ${num(day && day[measure] && day[measure].orders).toLocaleString('en-US')} ${ordersLabel}`;
  return day ? [line(matchedLabel, 'matched'), line(trackedLabel, 'tracked')] : [];
}

// The chart.js options merged over the line defaults: no legend, Clicks on the left axis, the
// conversions axis on the right in dollars or orders, one index tooltip whose body lists Clicks and
// whose footer lists all four conversion values for the day (byDate: date -> conversions row), and
// whose title marks today (todayTitle, todayFormat = the "today so far" string).
export function clicksConversionsOptions({
  clicksLabel, axisLabel, unit = 'usd', convMax, dates = [], byDate = new Map(), matchedLabel, trackedLabel, ordersLabel,
  today, todayFormat = (l) => l,
}) {
  const axis = (position, title, color, extra) => ({
    position,
    beginAtZero: true,
    title: { display: true, text: title, color },
    grid: { display: false },
    ticks: { precision: 0, color },
    ...extra,
  });
  const convTicks = unit === 'usd' ? { callback: (v) => formatUsd(v) } : {};
  return {
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: { display: false },
      tooltip: {
        // The conversion lines are in the footer; the body keeps Clicks only.
        filter: (item) => item.dataset && item.dataset.type === 'bar',
        callbacks: {
          title: (items) => todayTitle(items, { dates, today, format: todayFormat }),
          footer: (items) => (items.length ? tooltipLines(byDate.get(dates[items[0].dataIndex]), { matchedLabel, trackedLabel, ordersLabel }) : []),
        },
      },
    },
    scales: {
      x: { grid: { display: false }, offset: true },
      y: axis('left', clicksLabel, CLICKS_COLOR, {}),
      y1: axis('right', axisLabel, CONV_COLOR, {
        ...(convMax === undefined ? {} : { max: convMax }),
        ticks: { precision: 0, color: CONV_COLOR, ...convTicks },
      }),
    },
  };
}

// The session state of the two toggles ({ unit, showMatched }); bad input is the default ($, Matched
// shown). Only an explicit false hides Matched; a pre-swap stored showTracked is ignored.
export const DEFAULT_CONV_STATE = Object.freeze({ unit: 'usd', showMatched: true });
export const CONV_STORAGE_KEY = 'dashboard.conversionsChart';

export function normalizeConvState(s) {
  return {
    unit: s && UNITS.includes(s.unit) ? s.unit : DEFAULT_CONV_STATE.unit,
    showMatched: !(s && s.showMatched === false),
  };
}

export function loadConvState(store) {
  try {
    const raw = store ? store.getItem(CONV_STORAGE_KEY) : null;
    return normalizeConvState(raw ? JSON.parse(raw) : null);
  } catch {
    return normalizeConvState(null);
  }
}

export function saveConvState(store, state) {
  try {
    if (!store) {
      return false;
    }
    store.setItem(CONV_STORAGE_KEY, JSON.stringify(normalizeConvState(state)));
    return true;
  } catch {
    return false;
  }
}
