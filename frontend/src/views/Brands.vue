<template>
  <!-- Fork (brand health, integrations BRAND-HEALTH-SPEC D12). DISPOSABLE Vue (upstream deletes
  this SPA at v7). The page renders the documents the integrations BrandHealth Lambda writes
  (GET /api/brands/health, /api/brands/health/:brand) and computes nothing: every status, note,
  action and threshold is the document's. The only arithmetic is display formatting. -->
  <section class="brands">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4 mb-2">
          <router-link v-if="brandParam" :to="{ name: 'brands' }">{{ $t('brands.title') }}</router-link>
          <template v-else>{{ $t('brands.title') }}</template>
          <span v-if="!brandParam && rows.length">({{ rows.length }})</span>
          <span v-if="current"> / {{ nameOf(current) }}</span>
        </h1>
        <p class="is-size-7 has-text-grey">{{ $t('brands.gmailNote') }}</p>
      </div>
    </header>

    <p v-if="!$can('brands:get')" class="has-text-grey">{{ $t('brands.noPermission') }}</p>

    <!-- Table -->
    <b-table v-else-if="!brandParam" :data="rows" :loading="loading.brands" hoverable default-sort="name">
      <b-table-column v-slot="props" field="name" :label="$t('brands.brand')" sortable :custom-sort="sortName"
        header-class="cy-name">
        <router-link :to="{ name: 'brand', params: { brand: props.row.brand } }">{{ nameOf(props.row) }}</router-link>
        <b-tag v-if="props.row.default" class="is-small ml-2">{{ $t('brands.defaultSender') }}</b-tag>
        <b-tag v-if="props.row.unmapped" class="is-small ml-2">{{ $t('brands.unmapped') }}</b-tag>
      </b-table-column>
      <b-table-column v-slot="props" field="domain" :label="$t('brands.domain')">
        {{ props.row.domain }}
      </b-table-column>
      <b-table-column v-slot="props" field="channel" :label="$t('brands.channel')">
        {{ (props.row.facts && props.row.facts.channel) || '—' }}
      </b-table-column>
      <b-table-column v-slot="props" field="status" :label="$t('brands.health')" sortable :custom-sort="sortStatus"
        header-class="cy-status">
        <health-chip :status="props.row.status" :note="props.row.note || ''" />
      </b-table-column>
      <b-table-column v-for="k in inputKeys" :key="k" v-slot="props" :field="`input-${k}`"
        :label="$t(`brands.inputs.${k}`)">
        <health-chip :status="inputOf(props.row, k).status" :note="inputError(props.row, k)"
          :detail="inputOf(props.row, k).asOf ? $t('brands.asOf', { date: inputOf(props.row, k).asOf }) : ''" />
      </b-table-column>
      <b-table-column v-slot="props" field="day" :label="$t('brands.asOfCol')">
        {{ props.row.day }}
      </b-table-column>

      <template #empty v-if="!loading.brands">
        <p class="has-text-grey">{{ $t('brands.empty') }}</p>
      </template>
    </b-table>

    <!-- Detail -->
    <div v-else>
      <p v-if="!current && !loading.brands" class="has-text-grey">{{ $t('brands.notFound', { brand: brandParam }) }}</p>

      <div v-if="current" class="brand-detail">
        <div class="box">
          <div class="level">
            <div class="level-left">
              <div>
                <h2 class="title is-5 mb-1">{{ nameOf(current) }}</h2>
                <p class="is-size-7 has-text-grey">
                  {{ current.domain }}
                  <template v-if="current.default"> · {{ $t('brands.defaultSender') }}</template>
                  <template v-if="current.unmapped"> · {{ $t('brands.unmapped') }}</template>
                  · {{ $t('brands.asOf', { date: current.day }) }}
                </p>
              </div>
            </div>
            <div class="level-right">
              <health-chip :status="current.status" :note="current.note || ''" />
            </div>
          </div>
          <p v-if="current.note" class="is-size-7">{{ current.note }}</p>
          <div v-if="current.errors && current.errors.length" class="notification is-light mt-3">
            <p class="has-text-weight-semibold">{{ $t('brands.errors') }}</p>
            <ul><li v-for="(e, i) in current.errors" :key="i">{{ e }}</li></ul>
          </div>
        </div>

        <!-- Recommended actions -->
        <div v-if="current.actions && current.actions.length" class="box">
          <h3 class="title is-6">{{ $t('brands.actions') }}</h3>
          <div v-for="(a, i) in current.actions" :key="i" class="mb-3">
            <p class="has-text-weight-semibold">
              {{ a.title }} <span class="is-size-7 has-text-grey">({{ $t(`brands.inputs.${a.input}`) }})</span>
            </p>
            <p class="is-size-7">{{ a.detail }}</p>
          </div>
        </div>

        <div class="columns is-multiline">
          <!-- Configuration -->
          <div class="column is-6">
            <div class="box">
              <h3 class="title is-6">
                {{ $t('brands.inputs.config') }} <health-chip :status="inputOf(current, 'config').status" />
                <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'config')) }}</span>
              </h3>
              <p v-if="detailOf('config').rolledUpTo" class="is-size-7 mb-2">
                {{ $t('brands.rolledUpTo', { parent: detailOf('config').rolledUpTo }) }}
              </p>
              <p v-if="typeof inputOf(current, 'config').detail === 'string'" class="is-size-7">
                {{ inputOf(current, 'config').detail }}
              </p>
              <table v-else class="table is-narrow is-fullwidth is-size-7">
                <tbody>
                  <tr v-for="r in (detailOf('config').rows || [])" :key="r.requirement">
                    <td>{{ r.requirement }}</td><td>{{ r.status }}</td>
                  </tr>
                  <tr v-if="detailOf('config').oneClickUnsubscribe">
                    <td>{{ $t('brands.oneClick') }}</td><td>{{ detailOf('config').oneClickUnsubscribe.status }}</td>
                  </tr>
                  <tr v-if="detailOf('config').honorUnsubscribe">
                    <td>{{ $t('brands.honorUnsub') }}</td><td>{{ detailOf('config').honorUnsubscribe.status }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Verdict -->
          <div class="column is-6">
            <div class="box">
              <h3 class="title is-6">
                {{ $t('brands.inputs.verdict') }} <health-chip :status="inputOf(current, 'verdict').status" />
                <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'verdict')) }}</span>
              </h3>
              <p v-if="typeof inputOf(current, 'verdict').detail === 'string'" class="is-size-7">
                {{ inputOf(current, 'verdict').detail }}
              </p>
              <p v-else class="is-size-7">
                {{ detailOf('verdict').state || '—' }} / {{ detailOf('verdict').reason || '—' }}
              </p>
            </div>
          </div>

          <!-- Status strip -->
          <div class="column is-12">
            <div class="box">
              <h3 class="title is-6">{{ $t('brands.strip') }}</h3>
              <div class="status-strip">
                <b-tooltip v-for="d in strip" :key="d.day" :label="`${d.day} · ${d.status ? $t(`brands.status.${d.status}`) : $t('brands.noRow')}`"
                  type="is-dark">
                  <span :class="['strip-cell', d.status ? `health-${d.status}` : 'strip-empty']" />
                </b-tooltip>
              </div>
            </div>
          </div>

          <!-- Spam rate -->
          <div class="column is-6">
            <div class="box">
              <h3 class="title is-6">
                {{ $t('brands.inputs.spamRate') }} <health-chip :status="inputOf(current, 'spamRate').status" />
                <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'spamRate')) }}</span>
              </h3>
              <p v-if="!spamSeries.length" class="is-size-7 has-text-grey">{{ $t('brands.noGmailDays') }}</p>
              <table v-else class="table is-narrow is-fullwidth is-size-7">
                <thead><tr><th>{{ $t('brands.day') }}</th><th>{{ $t('brands.inputs.spamRate') }}</th><th /></tr></thead>
                <tbody>
                  <tr v-for="p in spamSeries" :key="p.day">
                    <td>{{ p.day }}</td>
                    <td>{{ pct(p.spamRate, 2) }}</td>
                    <td class="bar-cell"><span class="bar" :style="{ width: barWidth(p.spamRate) }" /></td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Engagement -->
          <div class="column is-6">
            <div class="box">
              <h3 class="title is-6">
                {{ $t('brands.inputs.engagement') }} <health-chip :status="inputOf(current, 'engagement').status" />
                <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'engagement')) }}</span>
              </h3>
              <p v-if="detailOf('engagement').viewRate !== undefined && detailOf('engagement').viewRate !== null"
                class="is-size-7 mb-2">
                {{ $t('brands.viewRate') }}: {{ pct(detailOf('engagement').viewRate, 1) }}
              </p>
              <table v-if="(detailOf('engagement').sends || []).length" class="table is-narrow is-fullwidth is-size-7">
                <thead>
                  <tr>
                    <th>{{ $t('brands.campaign') }}</th><th>{{ $t('brands.sent') }}</th>
                    <th>{{ $t('brands.views') }}</th><th>{{ $t('brands.rate') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="s in detailOf('engagement').sends" :key="s.id">
                    <td>{{ s.name }}</td><td>{{ s.sent }}</td><td>{{ s.views }}</td>
                    <td>{{ s.sent ? pct(s.views / s.sent, 1) : '—' }}</td>
                  </tr>
                </tbody>
              </table>
              <p v-else class="is-size-7 has-text-grey">{{ $t('brands.noSends') }}</p>
            </div>
          </div>

          <!-- Blocklists -->
          <div class="column is-6">
            <div class="box">
              <h3 class="title is-6">
                {{ $t('brands.inputs.dnsbl') }} <health-chip :status="inputOf(current, 'dnsbl').status" />
                <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'dnsbl')) }}</span>
              </h3>
              <table class="table is-narrow is-fullwidth is-size-7">
                <tbody>
                  <tr v-for="l in (detailOf('dnsbl').lists || [])" :key="l.zone">
                    <td>{{ l.zone }}</td><td>{{ l.result }}</td><td>{{ l.code || '' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Registry facts + lists -->
          <div class="column is-6">
            <div class="box">
              <h3 class="title is-6">{{ $t('brands.facts') }}</h3>
              <table class="table is-narrow is-fullwidth is-size-7">
                <tbody>
                  <tr v-for="(v, k) in (current.facts || {})" :key="k"><td>{{ k }}</td><td>{{ v }}</td></tr>
                </tbody>
              </table>
              <h4 class="title is-6 mt-4">{{ $t('globals.terms.lists') }}</h4>
              <ul v-if="(current.lists || []).length" class="is-size-7">
                <li v-for="l in current.lists" :key="l.id">
                  <router-link :to="{ name: 'list', params: { id: l.id } }">{{ l.name }}</router-link>
                </li>
              </ul>
              <p v-else class="is-size-7 has-text-grey">{{ $t('brands.noLists') }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import HealthChip from '../components/HealthChip.vue';

const INPUT_KEYS = ['config', 'verdict', 'spamRate', 'engagement', 'dnsbl'];
// Display order only (worst first) for the status sort -- not a rule.
const STATUS_ORDER = {
  issues: 0, warn: 1, unknown: 2, ok: 3,
};
const STRIP_DAYS = 30;

export default Vue.extend({
  components: { HealthChip },

  data() {
    return {
      rows: [],
      history: [],
      inputKeys: INPUT_KEYS,
    };
  },

  computed: {
    ...mapState(['loading']),

    brandParam() {
      return this.$route.params.brand || '';
    },

    current() {
      if (!this.brandParam) {
        return null;
      }
      return this.rows.find((r) => r.brand === this.brandParam) || null;
    },

    // The last 30 UTC days, newest last; a day with no row is an empty cell.
    strip() {
      const byDay = {};
      this.history.forEach((d) => { byDay[d.day] = d.status; });
      const out = [];
      const now = new Date();
      for (let i = STRIP_DAYS - 1; i >= 0; i -= 1) {
        const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() - i));
        const key = d.toISOString().slice(0, 10);
        out.push({ day: key, status: byDay[key] || '' });
      }
      return out;
    },

    spamBarFull() {
      const t = this.current && this.current.inputs && this.current.inputs.spamRate
        && this.current.inputs.spamRate.detail && this.current.inputs.spamRate.detail.thresholds;
      if (t && typeof t.issues === 'number' && t.issues > 0) {
        return t.issues;
      }
      return this.spamSeries.reduce((m, p) => Math.max(m, p.spamRate), 0);
    },

    // The spam-rate series from every stored row (newest row wins a day), newest first.
    spamSeries() {
      const byDay = {};
      [...this.history].reverse().forEach((doc) => {
        const s = doc.inputs && doc.inputs.spamRate && doc.inputs.spamRate.detail && doc.inputs.spamRate.detail.series;
        (Array.isArray(s) ? s : []).forEach((p) => {
          if (p && p.day && typeof p.spamRate === 'number') {
            byDay[p.day] = p;
          }
        });
      });
      return Object.keys(byDay).sort().reverse().map((k) => byDay[k]);
    },
  },

  methods: {
    nameOf(row) {
      if (row.unmapped) {
        return row.domain;
      }
      return (row.facts && row.facts.displayName) || row.brand;
    },

    inputOf(row, k) {
      return (row && row.inputs && row.inputs[k]) || { status: 'unknown' };
    },

    detailOf(k) {
      const d = this.inputOf(this.current, k).detail;
      return d && typeof d === 'object' ? d : {};
    },

    inputError(row, k) {
      const d = this.inputOf(row, k).detail;
      return typeof d === 'string' ? d : '';
    },

    asOfText(input) {
      return input && input.asOf ? this.$t('brands.asOf', { date: input.asOf }) : '';
    },

    pct(v, digits) {
      if (typeof v !== 'number') {
        return '—';
      }
      return `${(v * 100).toFixed(digits)}%`;
    },

    // The bar's full width is the document's own issues threshold (spamRate.detail.thresholds,
    // written by the Lambda); a document without it falls back to a neutral scale -- the largest
    // value shown is full width. No threshold lives in the page.
    barWidth(v) {
      const full = this.spamBarFull;
      if (!full || typeof v !== 'number') {
        return '0%';
      }
      return `${Math.min(100, Math.max(0, (v / full) * 100))}%`;
    },

    sortName(a, b, isAsc) {
      const x = this.nameOf(a).toLowerCase();
      const y = this.nameOf(b).toLowerCase();
      return isAsc ? x.localeCompare(y) : y.localeCompare(x);
    },

    sortStatus(a, b, isAsc) {
      const x = a.status in STATUS_ORDER ? STATUS_ORDER[a.status] : 9;
      const y = b.status in STATUS_ORDER ? STATUS_ORDER[b.status] : 9;
      return isAsc ? x - y : y - x;
    },

    fetchRows() {
      if (!this.$can('brands:get')) {
        return;
      }
      this.$api.getBrandsHealth().then((data) => {
        this.rows = Array.isArray(data) ? data : [];
      });
    },

    fetchHistory() {
      this.history = [];
      if (!this.brandParam || !this.$can('brands:get')) {
        return;
      }
      const brand = this.brandParam;
      this.$api.getBrandHealthHistory(brand, 90).then((data) => {
        if (brand === this.brandParam) {
          this.history = Array.isArray(data) ? data : [];
        }
      });
    },
  },

  watch: {
    brandParam() {
      this.fetchHistory();
    },
  },

  mounted() {
    this.fetchRows();
    this.fetchHistory();
  },
});
</script>

<style lang="scss">
.brands {
  .status-strip {
    display: flex;
    flex-wrap: wrap;
    gap: 3px;
  }
  .strip-cell {
    display: inline-block;
    width: 14px;
    height: 22px;
    border-radius: 2px;
  }
  .strip-cell.health-ok { background: #3aa65b; }
  .strip-cell.health-warn { background: #e0a100; }
  .strip-cell.health-issues { background: #d33b3b; }
  .strip-cell.health-unknown { background: #bdbdbd; }
  .strip-cell.strip-empty { background: #f2f2f2; border: 1px dashed #d0d0d0; }
  .bar-cell {
    width: 40%;
  }
  .bar {
    display: inline-block;
    height: 8px;
    background: #d33b3b;
    border-radius: 2px;
  }
}
</style>
