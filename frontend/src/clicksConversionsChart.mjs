// Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D11 follow-up, 2026-10-29) --
// the Dashboard's Clicks + conversions chart, under Sends + clicks and on the same window. Clicks
// are pale BARS on the left axis; Matched conversions a LINE on the right axis -- sales in dollars
// by default, orders with the $|# toggle. "Show Tracked" adds Tracked as a light FILL of the line's
// hue beneath it: Tracked is nearly always below Matched, so the visible gap between the fill and
// the line reads as tracking lost between click and checkout, one hero series rather than two
// competing lines. Rendering only -- every value is a stored daily sum (the charts statement).
//
// Days are the purchase day (Mountain), so a send's response is the trailing hump after its click
// bars, and the newest days fill in as orders land inside the 5-day Matched window.
//
// Colors: the bars are a pale tint of the top chart's Clicks orange (so Clicks keeps one hue across
// both charts); the line a deep green (money), its axis text in the same green; the Tracked fill
// that green at low alpha. There is no legend: each axis's title and numbers wear its series'
// color. The tooltip always lists all four values for the hovered day, whatever is toggled.

import { CLICKS_COLOR, headroomAxisMax } from './sendsClicksChart.mjs'; // eslint-disable-line import/extensions
import { formatUsd } from './conversionCells.mjs'; // eslint-disable-line import/extensions

export const CLICKS_FILL = '#f2c2a6';
export const CONV_COLOR = '#166534';
export const TRACKED_FILL = 'rgba(22, 101, 52, 0.22)';
export const UNITS = Object.freeze(['usd', 'orders']);

const num = (v) => (Number.isFinite(Number(v)) ? Number(v) : 0);
const pick = (day, measure, unit) => num(day && day[measure] && day[measure][unit === 'usd' ? 'sales' : 'orders']);

// clicks: [{date, count}]; conversions: [{date, tracked: {orders, sales}, matched: {orders, sales}}]
// as GET /api/dashboard/charts returns them. Returns {} when there are no conversion days (no
// conversions doc yet) -- the host renders nothing rather than a false zero line.
export function shapeClicksConversions(clicks, conversions, {
  labelOf, clicksLabel, matchedLabel, trackedLabel, unit = 'usd', showTracked = false,
}) {
  const conv = new Map((conversions || []).map((d) => [d.date, d]));
  if (conv.size === 0) {
    return {};
  }
  const c = new Map((clicks || []).map((d) => [d.date, d.count]));
  const dates = [...new Set([...c.keys(), ...conv.keys()])].sort();
  const series = (measure) => dates.map((d) => (conv.has(d) ? pick(conv.get(d), measure, unit) : null));
  const datasets = [
    // chart.js draws the lowest `order` on top: the Matched line, then the Tracked fill, then bars.
    {
      type: 'line',
      label: matchedLabel,
      data: series('matched'),
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
  if (showTracked) {
    datasets.push({
      type: 'line',
      label: trackedLabel,
      data: series('tracked'),
      yAxisID: 'y1',
      order: 1,
      fill: 'origin',
      backgroundColor: TRACKED_FILL,
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
  return { labels: dates.map(labelOf), dates, datasets };
}

// The right axis maximum: the top chart's headroom rule over the series on that axis (Matched, and
// Tracked when shown -- it can poke above Matched on a day), so the line stays in the lower half
// under the click bars. undefined (chart.js picks) when every value is zero.
export function conversionsAxisMax(conversions, { unit = 'usd', showTracked = false } = {}) {
  const values = [];
  (conversions || []).forEach((d) => {
    values.push(pick(d, 'matched', unit));
    if (showTracked) {
      values.push(pick(d, 'tracked', unit));
    }
  });
  return headroomAxisMax(values);
}

// tooltipLines is the four values for one day, for the tooltip footer -- always all four, so the
// overlay and the toggle only ever change what is drawn, never what can be read.
export function tooltipLines(day, { matchedLabel, trackedLabel, ordersLabel }) {
  const line = (label, measure) => `${label}: ${formatUsd(pick(day, measure, 'usd'))} · ${num(day && day[measure] && day[measure].orders).toLocaleString('en-US')} ${ordersLabel}`;
  return day ? [line(matchedLabel, 'matched'), line(trackedLabel, 'tracked')] : [];
}

// The chart.js options merged over the line defaults: no legend, Clicks on the left axis, the
// conversions axis on the right in dollars or orders, one index tooltip whose body lists Clicks and
// whose footer lists all four conversion values for the day (byDate: date -> conversions row).
export function clicksConversionsOptions({
  clicksLabel, axisLabel, unit = 'usd', convMax, dates = [], byDate = new Map(), matchedLabel, trackedLabel, ordersLabel,
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

// The session state of the two toggles ({ unit, showTracked }); bad input is the default.
export const DEFAULT_CONV_STATE = Object.freeze({ unit: 'usd', showTracked: false });
export const CONV_STORAGE_KEY = 'dashboard.conversionsChart';

export function normalizeConvState(s) {
  return {
    unit: s && UNITS.includes(s.unit) ? s.unit : DEFAULT_CONV_STATE.unit,
    showTracked: !!(s && s.showTracked === true),
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
