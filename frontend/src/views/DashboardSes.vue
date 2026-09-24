<template>
  <!-- Fork (system health, integrations SES-HEALTH-SPEC D5/D7). DISPOSABLE Vue (upstream deletes
  this SPA at v7). Renders the newest `ses` document the integrations BrandHealth Lambda writes
  (GET /api/system/health/ses) and computes nothing: every status, threshold, scope string and
  per-brand row is the document's. The only arithmetic is display formatting (percentages and
  bar widths scaled to the document's own thresholds). -->
  <section class="dashboard-ses">
    <b-loading v-if="loading.systemHealth" active :is-full-page="false" />

    <p v-if="!doc && !loading.systemHealth" class="has-text-grey" data-cy="ses-empty">{{ $t('dashboard.ses.empty') }}</p>

    <template v-if="doc">
      <!-- Headline: overall + enforcement -->
      <div class="box">
        <div class="level">
          <div class="level-left">
            <div>
              <h2 class="title is-5 mb-1">
                {{ $t('dashboard.ses.title') }} <health-chip :status="doc.status" />
              </h2>
              <p class="is-size-7 has-text-grey">
                <template v-if="doc.account">{{ doc.account.region }} · {{ doc.account.configurationSet }} · </template>
                {{ $t('dashboard.ses.asOf', { date: doc.day }) }}
                <template v-if="doc.computedAt"> · {{ $t('dashboard.ses.computedAt', { date: doc.computedAt }) }}</template>
              </p>
            </div>
          </div>
          <div class="level-right">
            <div class="has-text-right" data-cy="ses-enforcement">
              <p class="is-size-7 has-text-grey mb-1">{{ $t('dashboard.ses.enforcement') }}</p>
              <health-chip :status="sub('enforcement').status" :note="reasonOf(sub('enforcement'))" />
              <p class="is-size-7 mt-1">
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

        <!-- Quota -->
        <div v-if="doc.quota" class="quota mt-3" data-cy="ses-quota">
          <p class="is-size-7 has-text-grey mb-1">{{ $t('dashboard.ses.quota') }}</p>
          <div class="quota-track"><span class="quota-fill" :style="{ width: quotaWidth }" /></div>
          <p class="is-size-7 mt-1">
            {{ $t('dashboard.ses.quotaSent', { sent: num(doc.quota.sent24h), max: num(doc.quota.max24h) }) }}
            <template v-if="typeof doc.quota.maxSendRate === 'number'">
              · {{ $t('dashboard.ses.maxSendRate', { rate: num(doc.quota.maxSendRate) }) }}
            </template>
          </p>
        </div>
      </div>

      <!-- Two rate cards -->
      <div class="columns">
        <div v-for="k in rateKeys" :key="k" class="column is-6">
          <div class="box" :data-cy="`ses-${k}`">
            <h3 class="title is-6 mb-1">
              {{ $t(`dashboard.ses.${k}`) }} <health-chip :status="sub(k).status" :note="reasonOf(sub(k))" />
              <span class="is-size-7 has-text-grey">{{ sub(k).asOf ? $t('dashboard.ses.asOf', { date: sub(k).asOf }) : '' }}</span>
            </h3>
            <p class="is-size-7 has-text-grey mb-3">{{ detailOf(k).scope }}</p>
            <p v-if="reasonOf(sub(k))" class="is-size-7 mb-2">{{ reasonOf(sub(k)) }}</p>

            <div v-for="r in rates" :key="r.key" class="rate mb-3">
              <p class="is-size-7">
                <strong>{{ $t(`dashboard.ses.${r.key}`) }}</strong>
                {{ pct(detailOf(k)[r.key], r.digits) }}
                <span class="has-text-grey">
                  · {{ $t('dashboard.ses.thresholds', { warn: pct(threshold(r.kind, 'warn'), r.digits), issues: pct(threshold(r.kind, 'issues'), r.digits) }) }}
                </span>
              </p>
              <!-- The bar's full width is the document's pause (issues) line; the mark is its
              review (warn) line. -->
              <div class="rate-track">
                <span :class="['rate-fill', `health-${rateStatus(detailOf(k)[r.key], r.kind)}`]"
                  :style="{ width: barWidth(detailOf(k)[r.key], r.kind) }" />
                <span class="rate-mark" :style="{ left: markLeft(r.kind) }" />
              </div>
            </div>

            <p class="is-size-7 has-text-grey mb-1">{{ $t('dashboard.ses.series') }}</p>
            <p v-if="!(detailOf(k).series || []).length" class="is-size-7 has-text-grey">{{ $t('dashboard.ses.noData') }}</p>
            <template v-else>
              <div v-for="r in rates" :key="`s-${r.key}`" class="series mb-2">
                <b-tooltip v-for="p in detailOf(k).series" :key="p.day" type="is-dark"
                  :label="`${p.day} · ${$t(`dashboard.ses.${r.key}`)} ${pct(p[r.key], r.digits)}`">
                  <span class="series-cell">
                    <span :class="['series-bar', `health-${rateStatus(p[r.key], r.kind)}`]"
                      :style="{ height: barWidth(p[r.key], r.kind) }" />
                  </span>
                </b-tooltip>
              </div>
            </template>
          </div>
        </div>
      </div>

      <!-- Recommended actions -->
      <div v-if="doc.actions && doc.actions.length" class="box">
        <h3 class="title is-6">{{ $t('dashboard.ses.actions') }}</h3>
        <div v-for="(a, i) in doc.actions" :key="i" class="mb-3">
          <p class="has-text-weight-semibold">{{ a.title }}</p>
          <p class="is-size-7">{{ a.detail }}</p>
        </div>
      </div>

      <div v-if="doc.errors && doc.errors.length" class="notification is-light">
        <p class="has-text-weight-semibold">{{ $t('dashboard.ses.errors') }}</p>
        <ul><li v-for="(e, i) in doc.errors" :key="i">{{ e }}</li></ul>
      </div>

      <!-- Per-brand table (a copy of the same run's per-brand SES inputs) -->
      <div class="box">
        <!-- BRANDS-UX-SPEC D4: the hide toggle sits right-aligned on the title's line; it hides
        only unknown rows with no sends (health-sort.mjs). Sticky per browser. -->
        <div class="level mb-3">
          <div class="level-left">
            <h3 class="title is-6">{{ $t('dashboard.ses.brands') }}</h3>
          </div>
          <div class="level-right">
            <b-switch v-model="hideUnlaunched" data-cy="ses-hide-unlaunched" @input="onHideChange">
              {{ $t('brands.hideUnlaunched', { n: unlaunchedCount }) }}
            </b-switch>
          </div>
        </div>
        <!-- BRANDS-UX-SPEC D3: backend-sorting -- the view owns {field, order} and sorts its own
        rows through health-sort.mjs; first click on any non-Brand column is descending (worst /
        largest first); the tiebreak is always Brand A-Z. -->
        <b-table ref="table" :data="visibleRows" hoverable data-cy="ses-brands"
          backend-sorting :default-sort="[sort.field, sort.order]" @sort="onSort">
          <b-table-column v-slot="props" field="name" :label="$t('dashboard.ses.brand')" sortable>
            <router-link v-if="linkable(props.row)" :to="{ name: 'brand', params: { brand: props.row.brand } }">
              {{ props.row.displayName || props.row.brand }}
            </router-link>
            <span v-else>{{ props.row.displayName || props.row.brand }}</span>
          </b-table-column>
          <b-table-column v-slot="props" field="status" :label="$t('dashboard.ses.health')" sortable>
            <health-chip :status="props.row.status" />
          </b-table-column>
          <b-table-column v-slot="props" field="sends" :label="$t('dashboard.ses.sends')" numeric sortable>
            {{ num(props.row.sends) }}
          </b-table-column>
          <b-table-column v-slot="props" field="deliveries" :label="$t('dashboard.ses.deliveries')" numeric sortable>
            {{ num(props.row.deliveries) }}
          </b-table-column>
          <!-- Bounces / Complaints: the RATE is the value (it is what SES judges and what the
          column sorts on); the count rides in grey. Under the volume floor there is no rate, so
          the count stands alone and the row sorts last. Hover: the rule, from the row's own
          thresholds/floors (rows written before they were carried show no tooltip). -->
          <b-table-column v-slot="props" field="bounceRate" :label="$t('dashboard.ses.bounces')" numeric sortable>
            <b-tooltip :label="rateTip(props.row, 'bounce')" :active="!!rateTip(props.row, 'bounce')" type="is-dark" multilined append-to-body>
              <template v-if="typeof props.row.bounceRate === 'number'">
                {{ pct(props.row.bounceRate, 2) }}
                <span class="is-size-7 has-text-grey">({{ num(props.row.bounces) }})</span>
              </template>
              <template v-else>{{ num(props.row.bounces) }}</template>
            </b-tooltip>
          </b-table-column>
          <b-table-column v-slot="props" field="complaintRate" :label="$t('dashboard.ses.complaints')" numeric sortable>
            <b-tooltip :label="rateTip(props.row, 'complaint')" :active="!!rateTip(props.row, 'complaint')" type="is-dark" multilined append-to-body>
              <template v-if="typeof props.row.complaintRate === 'number'">
                {{ pct(props.row.complaintRate, 3) }}
                <span class="is-size-7 has-text-grey">({{ num(props.row.complaints) }})</span>
              </template>
              <template v-else>{{ num(props.row.complaints) }}</template>
            </b-tooltip>
          </b-table-column>
          <b-table-column v-slot="props" field="alarms" :label="$t('dashboard.ses.alarms')" sortable>
            <span v-if="props.row.alarms && (props.row.alarms.complaints || props.row.alarms.bounces)" class="is-size-7">
              {{ props.row.alarms.complaints || '—' }} / {{ props.row.alarms.bounces || '—' }}
            </span>
            <span v-else class="is-size-7 has-text-grey">{{ props.row.alarms ? $t('dashboard.ses.noAlarm') : '—' }}</span>
          </b-table-column>
        </b-table>
      </div>
    </template>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import HealthChip from '../components/HealthChip.vue';
import {
  compareAlarms, compareNumber, compareStatus, isUnlaunchedSesRow, sortRows,
} from '../health-sort';

const RATE_KEYS = ['accountRates', 'listmonkRates'];
const RATES = [
  { key: 'bounceRate', kind: 'bounce', digits: 2 },
  { key: 'complaintRate', kind: 'complaint', digits: 3 },
];
// The per-brand row with no brand document behind it (the `brand=unattributed` SES tag value);
// it has no Brands page to link to. A display rule, not a threshold.
const UNLINKED = ['unattributed'];
// BRANDS-UX-SPEC D3: the Brand column (field 'name') is the only ascending-first column.
const ASC_FIRST_FIELDS = ['name'];
const PREF_HIDE = 'dashboard.ses.hideUnlaunched';
const NUMERIC_FIELDS = ['sends', 'deliveries', 'bounceRate', 'complaintRate'];
const nameOf = (row) => row.displayName || row.brand || '';

export default Vue.extend({
  components: { HealthChip },

  data() {
    return {
      doc: null,
      rateKeys: RATE_KEYS,
      rates: RATES,
      // BRANDS-UX-SPEC D3/M6: the view owns the sort; a re-fetch (page.refresh) never resets it.
      sort: { field: 'name', order: 'asc' },
      hideUnlaunched: this.$utils.getPref(PREF_HIDE) === true,
    };
  },

  computed: {
    ...mapState(['loading']),

    brandRows() {
      return (this.doc && Array.isArray(this.doc.brands)) ? this.doc.brands : [];
    },

    unlaunchedCount() {
      return this.brandRows.filter(isUnlaunchedSesRow).length;
    },

    visibleRows() {
      const rows = this.hideUnlaunched ? this.brandRows.filter((r) => !isUnlaunchedSesRow(r)) : this.brandRows;
      const comparators = {
        status: (a, b, o) => compareStatus(a.status, b.status, o),
        alarms: (a, b, o) => compareAlarms(a.alarms, b.alarms, o),
      };
      NUMERIC_FIELDS.forEach((f) => {
        comparators[f] = (a, b, o) => compareNumber(a[f], b[f], o);
      });
      return sortRows(rows, this.sort.field, this.sort.order, nameOf, comparators);
    },

    quotaWidth() {
      const q = this.doc && this.doc.quota;
      if (!q || typeof q.sent24h !== 'number' || !q.max24h) {
        return '0%';
      }
      return `${Math.min(100, Math.max(0, (q.sent24h / q.max24h) * 100))}%`;
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

    barWidth(v, kind) {
      const full = this.threshold(kind, 'issues');
      if (!full || typeof v !== 'number') {
        return '0%';
      }
      return `${Math.min(100, Math.max(0, (v / full) * 100))}%`;
    },

    markLeft(kind) {
      const warn = this.threshold(kind, 'warn');
      const issues = this.threshold(kind, 'issues');
      if (warn === null || !issues) {
        return '0%';
      }
      return `${Math.min(100, (warn / issues) * 100)}%`;
    },

    pct(v, digits) {
      if (typeof v !== 'number') {
        return '—';
      }
      return `${(v * 100).toFixed(digits)}%`;
    },

    num(v) {
      return typeof v === 'number' ? this.$utils.niceNumber(v) : '—';
    },

    // The rule behind a rate cell, from the ROW's thresholds and floors (the brand lines the
    // Lambda classified against); '' when the row carries none.
    rateTip(row, kind) {
      const t = row && row.thresholds && row.thresholds[kind];
      const f = (row && row.floors) || {};
      if (!t || typeof t.warn !== 'number' || typeof t.issues !== 'number') {
        return '';
      }
      const warn = this.pct(t.warn, 1);
      const issues = this.pct(t.issues, 1);
      return kind === 'bounce'
        ? this.$t('brands.tips.bounceRate', { floor: f.bounceSends || '—', warn, issues })
        : this.$t('brands.tips.complaintRate', {
          floorDeliveries: f.complaintDeliveries || '—', floorCount: f.complaintCount || '—', warn, issues,
        });
    },

    // The Lists.vue onSort pattern (BRANDS-UX-SPEC D3); see Brands.vue onSort.
    onSort(field, direction) {
      let order = direction;
      const { table } = this.$refs;
      if (!ASC_FIRST_FIELDS.includes(field) && field !== this.sort.field && direction === 'asc'
        && table && typeof table.isAsc === 'boolean') {
        table.isAsc = false;
        order = 'desc';
      }
      this.sort = { field, order };
    },

    onHideChange(v) {
      this.$utils.setPref(PREF_HIDE, v === true);
    },

    linkable(row) {
      return !!row.brand && !UNLINKED.includes(row.brand);
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

  .quota-track,
  .rate-track {
    position: relative;
    height: 8px;
    background: #f2f2f2;
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
  .series {
    display: flex;
    align-items: flex-end;
    gap: 2px;
    height: 32px;
  }
  .series-cell {
    display: inline-flex;
    align-items: flex-end;
    width: 8px;
    height: 32px;
    background: #f7f7f7;
  }
  .series-bar {
    display: inline-block;
    width: 100%;
    min-height: 1px;
  }
  .rate-fill,
  .series-bar {
    &.health-ok { background: #3aa65b; }
    &.health-warn { background: #e0a100; }
    &.health-issues { background: #d33b3b; }
    &.health-unknown { background: #bdbdbd; }
  }
}
</style>
