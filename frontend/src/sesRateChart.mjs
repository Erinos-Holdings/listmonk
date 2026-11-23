// Fork (system health, 2026-11-22 user pass on the Dashboard SES pane) -- the account-wide and
// listmonk reputation charts, drawn like the Overview's Sends + Clicks chart: one full-width
// chart.js bar chart per rate, the daily maximum of SES's own rate over the document's series
// (60 UTC days ending yesterday, integrations lib/ses-health.ts SES_RATE_SERIES_DAYS), each bar
// colored by the document's review/pause lines, with horizontal threshold lines labelled at the
// right. Pure, so it is testable (node --test src/sesRateChart.test.mjs).
//
// Computes nothing the document does not carry: every line VALUE is the document's --
// `thresholds.<kind>` (SES's review = warn, pause = issues) and, on the complaint chart only,
// `campaignThresholds.complaint.red` (the Gmail/Yahoo bulk-sender 0.3% line, labelled as theirs
// so nobody reads it as an SES limit). A value missing from the document draws no line.
//
// Scale: every line is always drawn (2026-11-22, user), so the y axis runs to LINE_HEADROOM x the
// HIGHEST line (SES's pause), or RATE_HEADROOM x the busiest day when that is higher.

export const STATUS_FILL = Object.freeze({
  ok: '#3aa65b', warn: '#e0a100', issues: '#d33b3b', unknown: '#bdbdbd',
});
// Line and label colors: dark enough for text on white (>= 4.5:1), one hue per meaning.
export const LINE_COLOR = Object.freeze({ review: '#8a5a00', gmail: '#9a3412', pause: '#b02a2a' });
export const LINE_DASH = Object.freeze({ review: [6, 4], gmail: [2, 3], pause: [] });
export const RATE_HEADROOM = 1.3;
export const LINE_HEADROOM = 1.15;

const num = (v) => (typeof v === 'number' && Number.isFinite(v) ? v : null);

// The bar's status at the document's lines (the pane's own rule): issues at or above pause,
// warn at or above review, else ok; unknown without a value or a line.
export function rateStatus(v, t) {
  const warn = num(t && t.warn);
  const issues = num(t && t.issues);
  if (num(v) === null || warn === null || issues === null) {
    return 'unknown';
  }
  if (v >= issues) {
    return 'issues';
  }
  return v >= warn ? 'warn' : 'ok';
}

// The chart's lines, lowest first: [{ key: 'review'|'gmail'|'pause', value }]. kind is
// 'bounce' | 'complaint'; doc is the stored ses document.
export function rateLines(kind, doc) {
  const t = (doc && doc.thresholds && doc.thresholds[kind]) || {};
  const out = [];
  if (num(t.warn) !== null) out.push({ key: 'review', value: t.warn });
  if (kind === 'complaint') {
    const g = doc && doc.campaignThresholds && doc.campaignThresholds.complaint;
    if (g && num(g.red) !== null) out.push({ key: 'gmail', value: g.red });
  }
  if (num(t.issues) !== null) out.push({ key: 'pause', value: t.issues });
  return out.sort((a, b) => a.value - b.value);
}

// The y-axis top (see the header); undefined (chart.js picks) with neither data nor a line.
export function rateAxisMax(values, lines) {
  const top = Math.max(0, ...(values || []).map((v) => num(v) || 0));
  const ls = lines || [];
  const high = ls.length ? ls[ls.length - 1].value * LINE_HEADROOM : 0;
  const max = Math.max(top * RATE_HEADROOM, high);
  return max > 0 ? max : undefined;
}

// series: the document's [{ day, bounceRate?, complaintRate? }]; key: 'bounceRate'|'complaintRate'.
// Returns {} with no series (the host renders nothing); otherwise chart.js data plus `days`,
// `values`, `max` and every line.
export function shapeSesRate(series, key, {
  kind, doc, labelOf, label,
}) {
  const rows = (series || []).filter((p) => p && typeof p.day === 'string');
  if (!rows.length) {
    return {};
  }
  const values = rows.map((p) => num(p[key]));
  const t = doc && doc.thresholds && doc.thresholds[kind];
  const all = rateLines(kind, doc);
  const max = rateAxisMax(values, all);
  return {
    labels: rows.map((p) => labelOf(p.day)),
    days: rows.map((p) => p.day),
    values,
    max,
    lines: all,
    datasets: [{
      type: 'bar',
      label,
      data: values,
      backgroundColor: values.map((v) => STATUS_FILL[rateStatus(v, t)]),
      borderWidth: 0,
      borderRadius: 2,
    }],
  };
}

// '0.032%' at up to `digits` decimals, trailing zeros dropped ('0.5%', '10%' -- 2026-11-23, user);
// '—' for no value.
export function pctText(v, digits) {
  return num(v) === null ? '—' : `${parseFloat((v * 100).toFixed(digits))}%`;
}

// chart.js options merged over Chart.vue's line defaults: a percent y axis to `max`, no legend,
// the tooltip naming the day's value; the threshold lines ride as plugin options (refLines).
// lineLabel(key, valueText) is the host's i18n for a line's label.
export function sesRateOptions({
  max, lines = [], digits, valueLabel, lineLabel,
}) {
  return {
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: {
          label: (item) => `${valueLabel} ${pctText(item.raw, digits)}`,
        },
      },
      refLines: {
        lines: lines.map((l) => ({
          value: l.value,
          color: LINE_COLOR[l.key],
          dash: [...LINE_DASH[l.key]],
          text: lineLabel(l.key, pctText(l.value, digits)),
        })),
      },
    },
    scales: {
      x: { grid: { display: false }, offset: true },
      y: {
        beginAtZero: true,
        ...(max === undefined ? {} : { max }),
        grid: { display: false },
        ticks: { callback: (v) => pctText(v, digits), maxTicksLimit: 6 },
      },
    },
  };
}

// The threshold lines: a per-instance chart.js plugin (the host passes it in the chart's
// `plugins`). Each line spans the chart area at its value, its label right-aligned just above it
// in the line's color. A line outside the y scale is skipped.
export const refLines = {
  id: 'refLines',
  afterDatasetsDraw(chart, _args, opts) {
    const y = chart.scales && chart.scales.y;
    const area = chart.chartArea;
    if (!y || !area || !opts || !Array.isArray(opts.lines)) {
      return;
    }
    const { ctx } = chart;
    opts.lines.forEach((l) => {
      const py = y.getPixelForValue(l.value);
      if (!(py >= area.top - 0.5 && py <= area.bottom + 0.5)) {
        return;
      }
      ctx.save();
      ctx.strokeStyle = l.color;
      ctx.lineWidth = 1.5;
      ctx.setLineDash(l.dash || []);
      ctx.beginPath();
      ctx.moveTo(area.left, py);
      ctx.lineTo(area.right, py);
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.fillStyle = l.color;
      ctx.font = '600 12px sans-serif';
      ctx.textAlign = 'right';
      ctx.textBaseline = 'bottom';
      ctx.fillText(l.text, area.right - 4, py - 3);
      ctx.restore();
    });
  },
};
