<template>
  <section class="chart">
    <canvas class="chart-canvas" />
  </section>
</template>

<script>
import Chart from 'chart.js/auto';
import { barLabelPlacement, INSIDE } from '../barLabels.mjs'; // eslint-disable-line import/extensions

const DEFAULT_DONUT = {
  type: 'doughnut',
  data: {},
  options: {
    responsive: true,
    cutout: '70%',
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: '#fff',
        borderColor: '#ddd',
        borderWidth: 1,
        titleColor: '#666',
        bodyColor: '#666',
        bodyFont: {
          size: 15,
        },
        bodySpacing: 10,
        padding: 10,
        callbacks: {
          label: (item) => {
            const data = item.chart.data.datasets[item.datasetIndex];
            const val = data.data[item.dataIndex];
            // Fork (campaign rates) -- a dataset carrying `rates` (rate over that campaign's Sent,
            // already formatted, null with nothing sent) shows it rate-first like the campaigns
            // list; otherwise the upstream share of the ring.
            if (data.rates) {
              const rate = data.rates[item.dataIndex];
              return rate ? `${rate} (${val})` : `${val}`;
            }
            const total = data.data.reduce((acc, n) => acc + n, 0);
            const percentage = ((val / total) * 100).toFixed(2);
            return `${val} (${percentage}%)`;
          },
        },
      },
    },
  },
};

const DEFAULT_LINE = {
  type: 'line',
  data: {},
  options: {
    responsive: true,
    lineTension: 0.5,
    maintainAspectRatio: false,
    interaction: {
      intersect: false,
      axis: 'index',
    },
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: '#fff',
        borderColor: '#ddd',
        borderWidth: 1,
        bodyColor: '#666',
        displayColors: true,
        bodyFont: {
          size: 15,
        },
        bodySpacing: 10,
        padding: 10,
      },
    },
    scales: {
      x: {
        grid: {
          display: false,
        },
      },
      y: {
        grid: {
          display: false,
        },
        ticks: {
          precision: 0,
        },
      },
    },
  },
};

const DEFAULT_BAR = {
  type: 'bar',
  data: {},
  options: {
    responsive: true,
    indexAxis: 'y',
    barThickness: 40,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: '#fff',
        borderColor: '#ddd',
        borderWidth: 1,
        titleColor: '#666',
        bodyColor: '#666',
        bodyFont: {
          size: 15,
        },
        bodySpacing: 10,
        padding: 10,
        // Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D6) -- a dataset carrying
        // `texts` (the end-of-bar labels) shows its text, as the donut shows `rates`.
        callbacks: {
          label: (item) => {
            const data = item.chart.data.datasets[item.datasetIndex];
            if (Array.isArray(data.texts) && typeof data.texts[item.dataIndex] === 'string') {
              return data.texts[item.dataIndex];
            }
            return `${item.formattedValue}`;
          },
        },
      },
    },
    scales: {
      x: {
        grid: {
          display: false,
        },
      },
      y: {
        grid: {
          display: false,
        },
      },
    },
  },
};

// Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D8) -- end-of-bar labels. For a bar chart
// whose dataset carries `texts` (one string per bar), each text is drawn at the bar's end: inside,
// white, right-aligned BAR_LABEL_PAD px from the end when it fits (barLabels.mjs owns the rule),
// otherwise outside, dark, BAR_LABEL_GAP px past the end. A chart without `texts` is unchanged.
// Registered per instance (the config's plugins), never globally. The bar charts here are
// horizontal (DEFAULT_BAR's indexAxis 'y'); a vertical bar chart gets no labels.
const BAR_LABEL_PAD = 8;
const BAR_LABEL_GAP = 6;
const barLabels = {
  id: 'barLabels',
  afterDatasetsDraw(chart) {
    if (chart.options.indexAxis !== 'y') {
      return;
    }
    const { ctx } = chart;
    chart.data.datasets.forEach((ds, i) => {
      const meta = chart.getDatasetMeta(i);
      if (!Array.isArray(ds.texts) || meta.hidden) {
        return;
      }
      ctx.save();
      ctx.font = `12px ${Chart.defaults.font.family}`;
      ctx.textBaseline = 'middle';
      meta.data.forEach((el, j) => {
        const text = ds.texts[j];
        if (typeof text !== 'string' || text === '') {
          return;
        }
        const barPx = el.x - el.base;
        if (barLabelPlacement(barPx, ctx.measureText(text).width, BAR_LABEL_PAD) === INSIDE) {
          ctx.textAlign = 'right';
          ctx.fillStyle = '#fff';
          ctx.fillText(text, el.x - BAR_LABEL_PAD, el.y);
        } else {
          ctx.textAlign = 'left';
          ctx.fillStyle = '#363636';
          ctx.fillText(text, Math.max(el.x, el.base) + BAR_LABEL_GAP, el.y);
        }
      });
      ctx.restore();
    });
  },
};

export default {
  name: 'Chart',

  props: {
    data: { type: Object, default: () => { } },
    type: { type: String, default: 'line' },
    onClick: { type: Function, default: () => { } },
  },

  mounted() {
    const ctx = this.$el.querySelector('.chart-canvas');

    let def = {};
    switch (this.$props.type) {
      case 'donut':
        def = DEFAULT_DONUT;
        break;
      case 'bar':
        def = DEFAULT_BAR;
        break;
      default:
        def = DEFAULT_LINE;
        break;
    }

    // Fork (shop-link attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC Stage 4 F1) -- the
    // options object is copied per instance: the defaults are module-level and chart.js keeps
    // `options` by reference, so with two bar charts on one page the last to mount used to
    // overwrite the other's onClick (the Links click-through died, or a Shop Link bar opened a
    // Links URL).
    const conf = { ...def, options: { ...def.options }, data: this.$props.data };
    // Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D8) -- the end-of-bar labels, per
    // instance.
    if (def === DEFAULT_BAR) {
      conf.plugins = [barLabels];
    }
    if (this.$props.onClick) {
      conf.options.onClick = this.$props.onClick;
    }
    this.chart = new Chart(ctx, conf);
  },

  // Fork (global brand, integrations GLOBAL-BRAND-SPEC D4; G6 feedback 2026-10-11). The chart
  // was built once at mount and never read a new `data` prop, so a host that re-fetches in
  // place (the Dashboard on a brand change, the navbar Refresh) kept its first render until a
  // reload remounted it. Hand chart.js the new data and let it update.
  watch: {
    data(d) {
      if (this.chart) {
        this.chart.data = d;
        this.chart.update();
      }
    },
  },

  beforeDestroy() {
    if (this.chart) {
      this.chart.destroy();
    }
  },
};
</script>
