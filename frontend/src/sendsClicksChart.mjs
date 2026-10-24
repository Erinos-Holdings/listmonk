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
// Colors (dataviz validator, light surface; chosen by the user from a six-way comparison,
// 2026-10-24, "E"): solid bars, no border, in a mid step of the Dashboard blue; the line a deep
// burnt orange -- blue's complement, dark enough to read over a bar (3.5:1 against the fill; 3:1
// is the floor for chart graphics) and as axis text (7.3:1 on white; the blue is 6.5:1). The
// blue/orange pair passes CVD and normal-vision separation. There is no legend: each axis's title
// and numbers wear its series' color, so the axis IS the key. Darkening the fill needs a darker
// line to keep 3:1 -- re-measure both together.

export const SENDS_COLOR = '#0055d4';
export const SENDS_FILL = '#86b6ef';
export const CLICKS_COLOR = '#9a3412';
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
        borderWidth: 0,
        borderRadius: 2,
      },
    ],
  };
}

// The Clicks axis maximum: CLICKS_HEADROOM x the busiest click day, rounded UP to a multiple of
// its leading power of ten (850 -> 900, 790 -> 800, 1,650 -> 2,000), so the top tick lands on the
// axis's own step instead of an odd extra label; the line peaks at or below half height.
// undefined (chart.js picks) when there are no clicks.
export function clicksAxisMax(clicks) {
  const top = Math.max(0, ...(clicks || []).map((d) => d.count || 0));
  if (top <= 0) {
    return undefined;
  }
  const raw = top * CLICKS_HEADROOM;
  const unit = 10 ** Math.floor(Math.log10(raw));
  return Math.ceil(raw / unit) * unit;
}

// The chart.js options merged over the line defaults: no legend (each axis's title and numbers
// wear its series' color), one tooltip listing both values for the hovered day, Sends on the left
// axis and Clicks on the right.
export function sendsClicksOptions({ sendsLabel, clicksLabel, clicksMax }) {
  const axis = (position, title, color, extra) => ({
    position,
    beginAtZero: true,
    title: { display: true, text: title, color },
    grid: { display: false },
    ticks: { precision: 0, color },
    ...extra,
  });
  return {
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: { display: false },
    },
    scales: {
      // offset: a line-type chart does not pad the category axis, which would halve the end bars.
      x: { grid: { display: false }, offset: true },
      y: axis('left', sendsLabel, SENDS_COLOR, {}),
      y1: axis('right', clicksLabel, CLICKS_COLOR, clicksMax === undefined ? {} : { max: clicksMax }),
    },
  };
}
