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

// Fork (integrations TODAY-SO-FAR-SPEC D5) -- a partial day never looks like a complete one. Both
// Dashboard charts run through today (Mountain, so far); the response's `today` (never the browser
// clock) names the partial day. Its bar is drawn at TODAY_FILL_ALPHA of the series fill with a
// dashed outline (TODAY_DASH, TODAY_OUTLINE_WIDTH px) in the series' axis color -- the outline
// carries the >= 3:1 contrast the pale fill no longer does (SENDS_COLOR 6.5:1, CLICKS_COLOR 7.3:1 on
// white); its line's segment from the day before is dashed and its point hollow (white fill, the
// line's color as border, TODAY_POINT_RADIUS). ONE implementation: clicksConversionsChart.mjs
// imports styleToday, todayTitle and todayOutline from here.
export const TODAY_FILL_ALPHA = 0.45;
export const TODAY_DASH = Object.freeze([3, 2]);
export const TODAY_OUTLINE_WIDTH = 1.5;
export const TODAY_POINT_RADIUS = 3;

// The index of `today` in the chart's sorted dates; -1 when there is no today or it is not drawn.
export function todayIndexOf(dates, today) {
  return today ? (dates || []).indexOf(today) : -1;
}

// '#rrggbb' at alpha a, as rgba().
export function withAlpha(hex, a) {
  const n = parseInt(String(hex).slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${a})`; // eslint-disable-line no-bitwise
}

// Styles the partial day on a bar dataset and a line dataset (either may be null), in place, and
// returns them. index < 0 leaves both exactly as they were. The bar's outline is drawn by the
// todayOutline plugin (chart.js bars have no dashed border), which reads `todayOutline`.
export function styleToday(bar, line, index, { fill, outline, lineColor }) {
  if (!(index >= 0)) {
    return { bar, line };
  }
  if (bar) {
    const pale = withAlpha(fill, TODAY_FILL_ALPHA);
    Object.assign(bar, {
      backgroundColor: (ctx) => (ctx.dataIndex === index ? pale : fill),
      todayOutline: { index, color: outline },
    });
  }
  if (line) {
    const base = line.pointRadius;
    Object.assign(line, {
      segment: { borderDash: (ctx) => (ctx.p1DataIndex === index ? [...TODAY_DASH] : undefined) },
      pointBackgroundColor: (ctx) => (ctx.dataIndex === index ? '#fff' : lineColor),
      pointBorderColor: lineColor,
      pointRadius: (ctx) => (ctx.dataIndex === index ? TODAY_POINT_RADIUS : base),
    });
  }
  return { bar, line };
}

// The tooltip title: the hovered day's label, or format(label) -- "{date} — today so far" -- on today.
export function todayTitle(items, { dates = [], today, format }) {
  if (!items || !items.length) {
    return '';
  }
  const { label, dataIndex } = items[0];
  return today && dates[dataIndex] === today ? format(label) : label;
}

// The dashed outline of the partial day's bar: a per-instance chart.js plugin (the host passes it
// in the chart's `plugins`). Draws only for a visible dataset carrying `todayOutline`.
export const todayOutline = {
  id: 'todayOutline',
  afterDatasetsDraw(chart) {
    const { ctx } = chart;
    chart.data.datasets.forEach((ds, i) => {
      const mark = ds.todayOutline;
      const meta = chart.getDatasetMeta(i);
      const el = mark && !meta.hidden ? meta.data[mark.index] : null;
      if (!el) {
        return;
      }
      const top = Math.min(el.y, el.base);
      ctx.save();
      ctx.setLineDash([...TODAY_DASH]);
      ctx.lineWidth = TODAY_OUTLINE_WIDTH;
      ctx.strokeStyle = mark.color;
      ctx.strokeRect(el.x - el.width / 2, top, el.width, Math.abs(el.base - el.y));
      ctx.restore();
    });
  },
};

// sends: [{date: 'YYYY-MM-DD', count, live}], clicks: [{date, count}] as GET /api/dashboard/charts
// returns them, and its `today`. labelOf(date) formats an axis label. Returns {} when both are
// empty (the host renders nothing); otherwise also `dates` and `live` (per index: a live Sends
// point, listmonk's own count -- the tooltip's footer reads it).
export function shapeSendsClicks(sends, clicks, {
  labelOf, sendsLabel, clicksLabel, today,
}) {
  const s = new Map((sends || []).map((d) => [d.date, d.count]));
  const live = new Set((sends || []).filter((d) => d.live === true).map((d) => d.date));
  const c = new Map((clicks || []).map((d) => [d.date, d.count]));
  const dates = [...new Set([...s.keys(), ...c.keys()])].sort();
  if (dates.length === 0) {
    return {};
  }
  const series = (m) => dates.map((d) => (m.has(d) ? m.get(d) : null));
  const out = {
    labels: dates.map(labelOf),
    dates,
    live: dates.map((d) => live.has(d)),
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
  styleToday(out.datasets[1], out.datasets[0], todayIndexOf(dates, today), {
    fill: SENDS_FILL, outline: SENDS_COLOR, lineColor: CLICKS_COLOR,
  });
  return out;
}

// The Clicks axis maximum: CLICKS_HEADROOM x the busiest click day, rounded UP to a multiple of
// its leading power of ten (850 -> 900, 790 -> 800, 1,650 -> 2,000), so the top tick lands on the
// axis's own step instead of an odd extra label; the line peaks at or below half height.
// undefined (chart.js picks) when there are no clicks.
//
// The headroom rule itself is headroomAxisMax over plain numbers (CAMPAIGN-SEND-SERIES-SPEC D7
// generalizes it to the Campaign Analytics metric axis); non-numbers count as 0.
export function headroomAxisMax(values) {
  const top = Math.max(0, ...(values || []).map((v) => (Number.isFinite(v) ? v : 0)));
  if (top <= 0) {
    return undefined;
  }
  const raw = top * CLICKS_HEADROOM;
  const unit = 10 ** Math.floor(Math.log10(raw));
  return Math.ceil(raw / unit) * unit;
}

export function clicksAxisMax(clicks) {
  return headroomAxisMax((clicks || []).map((d) => d.count));
}

// The chart.js options merged over the line defaults: no legend (each axis's title and numbers
// wear its series' color), one tooltip listing both values for the hovered day, Sends on the left
// axis and Clicks on the right. The tooltip's title marks today (todayTitle, format = the
// "today so far" string); a live Sends point that is not today adds liveNote as its footer
// (dates and live are shapeSendsClicks's).
export function sendsClicksOptions({
  sendsLabel, clicksLabel, clicksMax, dates = [], live = [], today, todayFormat = (l) => l, liveNote = '',
}) {
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
      tooltip: {
        callbacks: {
          title: (items) => todayTitle(items, { dates, today, format: todayFormat }),
          footer: (items) => {
            const i = items && items.length ? items[0].dataIndex : -1;
            return i >= 0 && live[i] && dates[i] !== today && liveNote ? [liveNote] : [];
          },
        },
      },
    },
    scales: {
      // offset: a line-type chart does not pad the category axis, which would halve the end bars.
      x: { grid: { display: false }, offset: true },
      y: axis('left', sendsLabel, SENDS_COLOR, {}),
      y1: axis('right', clicksLabel, CLICKS_COLOR, clicksMax === undefined ? {} : { max: clicksMax }),
    },
  };
}
