// Fork (dashboard sends, integrations DASHBOARD-SENDS-SPEC, 2026-10-24 follow-up) -- the
// Dashboard's one Sends + Clicks chart. Sends are pale BARS on the left axis, Clicks an orange LINE
// on the right axis: different marks, so a bar's height is never read against the line's (the
// usual dual-axis trap), and the bar-then-trailing-line shape after each send is the relationship
// the chart is for. The Clicks axis has deliberate headroom -- its maximum is twice the busiest
// click day -- so the line stays in the lower half under the send spikes.
//
// The x-axis is the union of both series' dates, sorted; a day one series lacks is null (a gap),
// never 0 -- before the morning SES read the Sends series ends a day before Clicks, and a 0 there
// would read as "nothing was sent".
//
// Colors (dataviz validator, light surface): the bar fill is a pale step of the Dashboard blue
// (below 3:1 on white, so each bar carries a 1px border in the primary blue, which passes) and the
// line is the categorical slot-2 orange; the pair passes CVD and normal-vision separation.

export const SENDS_COLOR = '#0055d4';
export const SENDS_FILL = '#6da7ec';
export const CLICKS_COLOR = '#eb6834';
export const CLICKS_HEADROOM = 2;

// sends, clicks: [{date: 'YYYY-MM-DD', count}] as GET /api/dashboard/charts returns them.
// labelOf(date) formats an axis label. Returns {} when both are empty (the host renders nothing).
export function shapeSendsClicks(sends, clicks, { labelOf, sendsLabel, clicksLabel }) {
  const s = new Map((sends || []).map((d) => [d.date, d.count]));
  const c = new Map((clicks || []).map((d) => [d.date, d.count]));
  const dates = [...new Set([...s.keys(), ...c.keys()])].sort();
  if (dates.length === 0) {
    return {};
  }
  const series = (m) => dates.map((d) => (m.has(d) ? m.get(d) : null));
  return {
    labels: dates.map(labelOf),
    datasets: [
      // The line first with the lower `order`: chart.js draws the lowest order on top.
      {
        type: 'line',
        label: clicksLabel,
        data: series(c),
        yAxisID: 'y1',
        order: 0,
        borderColor: CLICKS_COLOR,
        backgroundColor: CLICKS_COLOR,
        borderWidth: 2,
        pointRadius: 2,
        pointHoverRadius: 5,
        spanGaps: false,
      },
      {
        type: 'bar',
        label: sendsLabel,
        data: series(s),
        yAxisID: 'y',
        order: 1,
        backgroundColor: SENDS_FILL,
        borderColor: SENDS_COLOR,
        borderWidth: 1,
        borderRadius: 2,
      },
    ],
  };
}

// The Clicks axis maximum: CLICKS_HEADROOM x the busiest click day, so the line peaks at half
// height. undefined (chart.js picks) when there are no clicks.
export function clicksAxisMax(clicks) {
  const top = Math.max(0, ...(clicks || []).map((d) => d.count || 0));
  return top > 0 ? top * CLICKS_HEADROOM : undefined;
}

// The chart.js options merged over the line defaults: a legend (two series), one tooltip listing
// both values for the hovered day, Sends on the left axis and Clicks on the right. Axis text stays
// in ink (the titles name the series; the legend swatches carry the color).
export function sendsClicksOptions({ sendsLabel, clicksLabel, clicksMax }) {
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
    plugins: {
      legend: {
        display: true, position: 'top', align: 'start', labels: { boxWidth: 12, boxHeight: 8 },
      },
    },
    scales: {
      // offset: a line-type chart does not pad the category axis, which would halve the end bars.
      x: { grid: { display: false }, offset: true },
      y: axis('left', sendsLabel, {}),
      y1: axis('right', clicksLabel, clicksMax === undefined ? {} : { max: clicksMax }),
    },
  };
}
