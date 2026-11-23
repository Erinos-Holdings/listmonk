<template>
  <!-- Fork (system health, integrations SES-HEALTH-SPEC D5/D7). DISPOSABLE Vue (upstream deletes
  this SPA at v7). Renders the newest `ses` document the integrations BrandHealth Lambda writes
  (GET /api/system/health/ses) and computes nothing: every status, threshold and scope string is
  the document's; account-aggregate only (DELIVERABILITY-CONSISTENCY-SPEC D3). The only arithmetic is display formatting (percentages and
  bar widths scaled to the document's own thresholds).
  2026-11-22 (user): laid out like the Overview pane -- full-width tiles (tile is-parent >
  notification), body text at the page size, the reputation series as full-width charts stacked
  one per rate (sesRateChart.mjs), and no Recompute button (the navbar health glyph,
  HealthSync.vue, is the one on-demand recompute). -->
  <section class="dashboard-ses wrap">
    <b-loading v-if="loading.systemHealth" active :is-full-page="false" />

    <p v-if="!doc && !loading.systemHealth" class="has-text-grey" data-cy="ses-empty">
      {{ $t('dashboard.ses.empty') }}
    </p>

    <div v-if="doc" class="tile is-ancestor">
      <div class="tile is-vertical is-12">
        <!-- Fork (integrations BRAND-OVERLAP-SPEC D3) -- the multi-brand overlap, the document's
        `overlap` sub-input verbatim: status, counts, shares and thresholds are all the document's
        (sesOverlap.mjs formats them); nothing is computed or held here. First on the pane
        (2026-11-23, user). -->
        <div class="tile is-parent">
          <article class="tile is-child notification" data-cy="ses-overlap">
            <h3 class="title is-size-6 mb-1">
              {{ $t('dashboard.ses.overlap') }} <health-chip :status="sub('overlap').status" :note="reasonOf(sub('overlap'))" />
            </h3>
            <p v-if="reasonOf(sub('overlap'))" class="mb-2">{{ reasonOf(sub('overlap')) }}</p>
            <template v-if="overlap">
              <p>{{ $t('dashboard.ses.overlapLine', overlap) }}</p>
              <p class="has-text-grey mb-1">
                {{ $t('dashboard.ses.overlapThresholds', { warn: overlap.warn, issues: overlap.issues }) }}
              </p>
              <!-- The bar's full width is the document's issues line; the mark is its warn line. -->
              <div class="rate-track">
                <span :class="['rate-fill', `health-${sub('overlap').status}`]" :style="{ width: overlap.fill }" />
                <span class="rate-mark" :style="{ left: overlap.mark }" />
              </div>
            </template>
          </article>
        </div>

        <!-- Headline: overall + enforcement + quota -->
        <div class="tile is-parent">
          <article class="tile is-child notification" data-cy="ses-headline">
            <div class="level mb-0">
              <div class="level-left">
                <div>
                  <h2 class="title is-size-5 mb-1">
                    {{ $t('dashboard.ses.title') }} <health-chip :status="doc.status" />
                  </h2>
                  <!-- 2026-11-07 (user): no as-of / computed stamp here -- the navbar's brand health
                  sync indicator is the one place run times show. -->
                  <p v-if="doc.account" class="has-text-grey">
                    {{ doc.account.region }} · {{ doc.account.configurationSet }}
                  </p>
                </div>
              </div>
              <div class="level-right">
                <div class="has-text-right" data-cy="ses-enforcement">
                  <p class="has-text-grey mb-1">{{ $t('dashboard.ses.enforcement') }}</p>
                  <health-chip :status="sub('enforcement').status" :note="reasonOf(sub('enforcement'))" />
                  <p class="mt-1">
                    <template v-if="detailOf('enforcement').enforcementStatus">
                      {{ $t('dashboard.ses.enforcementStatus', { status: detailOf('enforcement').enforcementStatus }) }}
                    </template>
                    <template v-if="detailOf('enforcement').sendingEnabled === true"> · {{ $t('dashboard.ses.sendingEnabled') }}</template>
                    <template v-else-if="detailOf('enforcement').sendingEnabled === false">
                      · <strong>{{ $t('dashboard.ses.sendingDisabled') }}</strong>
                    </template>
                  </p>
                </div>
              </div>
            </div>

            <div v-if="doc.quota" class="quota mt-3" data-cy="ses-quota">
              <p class="has-text-grey mb-1">{{ $t('dashboard.ses.quota') }}</p>
              <div class="quota-track"><span class="quota-fill" :style="{ width: quotaWidth }" /></div>
              <p class="mt-1">
                {{ $t('dashboard.ses.quotaSent', { sent: num(doc.quota.sent24h), max: num(doc.quota.max24h) }) }}
                <template v-if="typeof doc.quota.maxSendRate === 'number'">
                  · {{ $t('dashboard.ses.maxSendRate', { rate: num(doc.quota.maxSendRate) }) }}
                </template>
              </p>
            </div>
          </article>
        </div>

        <!-- The two reputation cards, stacked: current values, then one full-width chart per rate. -->
        <div v-for="k in rateKeys" :key="k" class="tile is-parent">
          <article class="tile is-child notification" :data-cy="`ses-${k}`">
            <h3 class="title is-size-6 mb-1">
              {{ $t(`dashboard.ses.${k}`) }} <health-chip :status="sub(k).status" :note="reasonOf(sub(k))" />
            </h3>
            <p class="has-text-grey mb-3">{{ detailOf(k).scope }}</p>
            <p v-if="reasonOf(sub(k))" class="mb-2">{{ reasonOf(sub(k)) }}</p>

            <!-- 2026-11-22 (user): the charts and their labelled threshold lines tell the story; the
            live current rate (the document's newest 15-minute value) rides in each chart's heading,
            colored by its status, so it costs no chart height. -->
            <p v-if="!(detailOf(k).series || []).length" class="has-text-grey">{{ $t('dashboard.ses.noData') }}</p>
            <template v-else>
              <div v-for="r in rates" :key="`s-${r.key}`" class="ses-rate" :data-cy="`ses-${k}-${r.key}-chart`">
                <h4 class="is-size-6 mb-1">
                  <span class="has-text-weight-semibold">{{ $t(`dashboard.ses.${r.key}`) }}</span>
                  <span :class="['ses-now', `health-${rateStatus(detailOf(k)[r.key], r.kind)}`]"
                    :data-cy="`ses-${k}-${r.key}-now`">
                    · {{ $t('dashboard.ses.now', { value: pct(detailOf(k)[r.key], r.digits) }) }}
                  </span>
                </h4>
                <chart type="line" :data="charts[`${k}.${r.key}`].data" :options="charts[`${k}.${r.key}`].options"
                  :plugins="chartPlugins" class="ses-rate-chart" />
              </div>
              <p class="is-size-7 has-text-grey mt-2">{{ $t('dashboard.ses.series') }}</p>
            </template>
          </article>
        </div>

        <!-- Fork (integrations BLOCKLIST-COMPOSITION-SPEC D5) -- the account-wide blocklist
        composition, the document's `composition` verbatim; nothing when the field is absent. -->
        <div v-if="doc.composition" class="tile is-parent">
          <div class="tile is-child">
            <blocklist-composition :composition="doc.composition" scope="account" />
          </div>
        </div>

        <!-- Recommended actions -->
        <div v-if="doc.actions && doc.actions.length" class="tile is-parent">
          <article class="tile is-child notification">
            <h3 class="title is-size-6">{{ $t('dashboard.ses.actions') }}</h3>
            <div v-for="(a, i) in doc.actions" :key="i" class="mb-3">
              <p class="has-text-weight-semibold">{{ a.title }}</p>
              <p>{{ a.detail }}</p>
            </div>
          </article>
        </div>

        <div v-if="doc.errors && doc.errors.length" class="tile is-parent">
          <article class="tile is-child notification is-light">
            <p class="has-text-weight-semibold">{{ $t('dashboard.ses.errors') }}</p>
            <ul><li v-for="(e, i) in doc.errors" :key="i">{{ e }}</li></ul>
          </article>
        </div>

        <!-- Fork (integrations DELIVERABILITY-CONSISTENCY-SPEC D3) -- this pane is account-aggregate
        only; the per-brand table it carried is retired, the Brands page being the per-brand home. -->
        <div class="tile is-parent pt-0">
          <p class="has-text-grey" data-cy="ses-brands-link">
            <router-link :to="{ name: 'brands' }">{{ $t('dashboard.ses.brandsLink') }}</router-link>
          </p>
        </div>
      </div>
    </div>
  </section>
</template>

<script>
import Vue from 'vue';
import dayjs from 'dayjs';
import { mapState } from 'vuex';
import Chart from '../components/Chart.vue';
import HealthChip from '../components/HealthChip.vue';
import BlocklistComposition from '../components/BlocklistComposition.vue';
import { overlapView } from '../sesOverlap.mjs'; // eslint-disable-line import/extensions
import {
  shapeSesRate, sesRateOptions, refLines, pctText,
} from '../sesRateChart.mjs'; // eslint-disable-line import/extensions

const RATE_KEYS = ['accountRates', 'listmonkRates'];
// Complaints first (2026-11-23, user): the rate that moves.
const RATES = [
  { key: 'complaintRate', kind: 'complaint', digits: 3 },
  { key: 'bounceRate', kind: 'bounce', digits: 2 },
];
const LINE_KEYS = { review: 'dashboard.ses.lineReview', gmail: 'dashboard.ses.lineGmail', pause: 'dashboard.ses.linePause' };

export default Vue.extend({
  components: { Chart, HealthChip, BlocklistComposition },

  data() {
    return {
      doc: null,
      rateKeys: RATE_KEYS,
      rates: RATES,
      chartPlugins: Object.freeze([refLines]), // frozen: Vue never observes the plugin
    };
  },

  computed: {
    ...mapState(['loading']),

    // BRAND-OVERLAP-SPEC D3: the box's numbers, from the stored overlap detail only.
    overlap() {
      return overlapView(this.detailOf('overlap'));
    },

    quotaWidth() {
      const q = this.doc && this.doc.quota;
      if (!q || typeof q.sent24h !== 'number' || !q.max24h) {
        return '0%';
      }
      return `${Math.min(100, Math.max(0, (q.sent24h / q.max24h) * 100))}%`;
    },

    // Every reputation chart's data and options, keyed `<scope>.<rate>`; one object per document
    // so the Chart components see a new prop only when the document changes.
    charts() {
      const out = {};
      RATE_KEYS.forEach((k) => {
        const series = this.detailOf(k).series || [];
        RATES.forEach((r) => {
          const label = this.$t(`dashboard.ses.${r.key}`);
          const data = shapeSesRate(series, r.key, {
            kind: r.kind, doc: this.doc, labelOf: (d) => dayjs(d).format('DD MMM'), label,
          });
          out[`${k}.${r.key}`] = {
            data,
            options: sesRateOptions({
              max: data.max,
              lines: data.lines || [],
              digits: r.digits,
              valueLabel: label,
              lineLabel: (key, value) => this.$t(LINE_KEYS[key], { value }),
            }),
          };
        });
      });
      return out;
    },
  },

  methods: {
    sub(k) {
      return (this.doc && this.doc[k]) || { status: 'unknown' };
    },

    detailOf(k) {
      const d = this.sub(k).detail;
      return d && typeof d === 'object' ? d : {};
    },

    reasonOf(input) {
      const d = input && input.detail;
      if (typeof d === 'string') {
        return d;
      }
      return (d && typeof d.reason === 'string') ? d.reason : '';
    },

    // The document's own thresholds (SES's review/pause lines) -- never a constant here.
    threshold(kind, level) {
      const t = this.doc && this.doc.thresholds && this.doc.thresholds[kind];
      return t && typeof t[level] === 'number' ? t[level] : null;
    },

    rateStatus(v, kind) {
      const warn = this.threshold(kind, 'warn');
      const issues = this.threshold(kind, 'issues');
      if (typeof v !== 'number' || warn === null || issues === null) {
        return 'unknown';
      }
      if (v >= issues) {
        return 'issues';
      }
      return v >= warn ? 'warn' : 'ok';
    },

    // One percent format with the charts (sesRateChart.mjs: trailing zeros dropped).
    pct(v, digits) {
      return pctText(v, digits);
    },

    num(v) {
      return typeof v === 'number' ? this.$utils.niceNumber(v) : '—';
    },

    fetchData() {
      this.$api.getSystemHealth('ses', 30).then((data) => {
        this.doc = Array.isArray(data) && data.length ? data[0] : null;
      });
    },
  },

  created() {
    this.$root.$on('page.refresh', this.fetchData);
  },

  destroyed() {
    this.$root.$off('page.refresh', this.fetchData);
  },

  mounted() {
    this.fetchData();
  },
});
</script>

<style lang="scss">
.dashboard-ses {
  position: relative;

  // section.dashboard's count-label rule (style.scss: label { display: inline-block; text-align:
  // right; font-weight: bold; min-width }) also hits Buefy's switch, which is a <label> -- the
  // control stacked above its text. Restore the switch's own inline-flex layout here so it reads
  // exactly as the Brands page's: control left, label right.
  .switch {
    display: inline-flex;
    font-weight: normal;
    min-width: 0;
    text-align: left;
  }

  // The reputation charts: the Overview chart's size (the tile's width, the global 400px canvas).
  .ses-rate + .ses-rate {
    margin-top: 1.5rem;
  }
  .chart.ses-rate-chart {
    margin-bottom: 0;
  }
  // The live rate in a chart's heading, in a text-safe shade of its status color.
  .ses-now {
    font-weight: 600;
    &.health-ok { color: #257a3e; }
    &.health-warn { color: #8a5a00; }
    &.health-issues { color: #b02a2a; }
    &.health-unknown { color: #7a7a7a; }
  }

  // The shared blocklist composition box at this page's text size, on the tiles' surface.
  .blocklist-composition {
    box-shadow: none;
    background: #f5f5f5;
    .is-size-7 {
      font-size: 1rem !important;
    }
  }

  .quota-track,
  .rate-track {
    position: relative;
    height: 8px;
    background: #f2f2f2; // the empty part of the bar, visible on the white tile
    border-radius: 2px;
  }
  .quota-fill {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    background: #7f2aff;
    border-radius: 2px;
  }
  .rate-fill {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    border-radius: 2px;
  }
  .rate-mark {
    position: absolute;
    top: -3px;
    bottom: -3px;
    width: 2px;
    background: #8a5a00;
  }
  .rate-fill {
    &.health-ok { background: #3aa65b; }
    &.health-warn { background: #e0a100; }
    &.health-issues { background: #d33b3b; }
    &.health-unknown { background: #bdbdbd; }
  }
}
</style>
