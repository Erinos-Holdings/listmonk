// Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D7). Pure shaping for the
// Campaign Analytics charts (Views, Clicks, Hard Bounces, Soft Bounces, Complaints): each campaign's
// metric is a LINE on the right axis (y1) and that campaign's sends are translucent BARS on the left
// axis (y), in the SAME chartColors[n] -- the bars at SEND_BAR_ALPHA, the line opaque -- so a
// multi-campaign overlay keeps each campaign's bars and line visibly paired. Tested by
// sendOverlay.test.mjs (npm run test:send-overlay). No Vue, no DOM.
//
// Buckets come from the API as NAIVE Mountain wall-clock strings ("2026-10-21T18:00:00", no
// offset -- D5's output contract). They are labelled VERBATIM (bucketLabel slices the digits; no
// Date, no dayjs, no browser-local conversion), so every viewer sees Mountain time with no zone
// annotation. Never parse them into a Date here.

import { headroomAxisMax } from './sendsClicksChart.mjs'; // eslint-disable-line import/extensions

export const SEND_BAR_ALPHA = 0.3;

// "#rrggbb" (or "#rgb") -> "rgba(r, g, b, a)"; anything else is returned unchanged.
export function withAlpha(color, alpha) {
  const m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(String(color || ''));
  if (!m) {
    return color;
  }
  const hex = m[1].length === 3 ? m[1].split('').map((c) => c + c).join('') : m[1];
  const n = parseInt(hex, 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`; // eslint-disable-line no-bitwise
}

// "2026-10-21T18:00:00" -> "2026-10-21 18:00", digits as given (Mountain wall-clock, never shifted).
export function bucketLabel(ts) {
  const s = String(ts || '');
  return s.length >= 16 ? `${s.slice(0, 10)} ${s.slice(11, 16)}` : s;
}

// The right (metric) axis maximum: the Dashboard's headroom rule over every point of every line --
// 2 x the busiest bucket, rounded up to its leading power of ten's step. undefined = chart.js picks.
export function metricAxisMax(metric) {
  return headroomAxisMax((metric || []).map((r) => r.count));
}

// campaigns: [{ id, name }] in the picker's order (the color index n). metric, sends: the API rows
// [{ campaignId, count, timestamp }]. palette: the chartColors array. sendsLabel(name) names a
// campaign's bar dataset. Returns { labels, datasets }: the x-axis is the sorted union of every
// bucket; a bucket a series lacks is null (a gap), never 0. A campaign with no send rows gets no bar
// dataset (nothing to draw, nothing in the tooltip).
export function shapeSendOverlay({
  campaigns, metric, sends, palette, sendsLabel,
}) {
  const camps = campaigns || [];
  const m = metric || [];
  const s = sends || [];
  const keys = [...new Set([...m, ...s].map((r) => String(r.timestamp)))].sort();
  const series = (rows, id) => {
    const by = new Map();
    rows.forEach((r) => {
      if (r.campaignId === id) {
        const k = String(r.timestamp);
        by.set(k, (by.get(k) || 0) + (Number.isFinite(r.count) ? r.count : 0));
      }
    });
    return { has: by.size > 0, data: keys.map((k) => (by.has(k) ? by.get(k) : null)) };
  };

  const lines = [];
  const bars = [];
  camps.forEach((c, n) => {
    const color = palette[n % palette.length];
    const line = series(m, c.id);
    // The line draws on top (lower order); it spans its own missing buckets, as the pre-overlay
    // chart did by plotting only the points it had.
    lines.push({
      type: 'line',
      label: c.name,
      data: line.data,
      yAxisID: 'y1',
      order: 0,
      borderColor: color,
      backgroundColor: color,
      borderWidth: 2,
      pointHoverBorderWidth: 5,
      pointBorderWidth: 0.5,
      spanGaps: true,
    });
    const bar = series(s, c.id);
    if (bar.has) {
      bars.push({
        type: 'bar',
        label: sendsLabel(c.name),
        data: bar.data,
        yAxisID: 'y',
        order: 1,
        backgroundColor: withAlpha(color, SEND_BAR_ALPHA),
        borderWidth: 0,
        borderRadius: 2,
      });
    }
  });

  return { labels: keys.map(bucketLabel), datasets: [...lines, ...bars] };
}

// The chart.js options merged over the line defaults: one tooltip listing every dataset for the
// hovered bucket, sends on the left axis, the metric on the right with the headroom max.
export function sendOverlayOptions({ sendsTitle, metricTitle, metricMax }) {
  const axis = (position, title, extra) => ({
    position,
    beginAtZero: true,
    title: { display: true, text: title },
    grid: { display: false },
    ticks: { precision: 0 },
    ...extra,
  });
  return {
    interaction: { mode: 'index', intersect: false },
    plugins: { legend: { display: false } },
    scales: {
      // offset: a line-type chart does not pad the category axis, which would halve the end bars.
      x: { grid: { display: false }, offset: true },
      y: axis('left', sendsTitle, {}),
      y1: axis('right', metricTitle, metricMax === undefined ? {} : { max: metricMax }),
    },
  };
}
