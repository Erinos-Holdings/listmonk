<template>
  <!-- Fork (brand matrix, integrations BRAND-MATRIX-SPEC D2-D7; moved from the Dashboard to the Brands
       page by CAMPAIGN-MATRIX-SPEC D1 -- what it computes and shows is unchanged). One row per brand of
       the global selection (narrowed by the navbar launch filter), the totals row pinned last; every
       rate, sort and missing-value rule is brandMatrix.mjs. -->
  <section class="brand-matrix relative" data-cy="brand-matrix">
    <b-loading v-if="loading" active :is-full-page="false" />
    <div class="is-flex is-align-items-center mb-3">
      <span class="has-text-grey mr-2">{{ $t('dashboard.matrix.engagement') }}</span>
      <b-field class="mb-0">
        <b-radio-button v-model="state.mode" native-value="pct" size="is-small" type="is-primary"
          data-cy="matrix-mode-pct" @input="saveMatrixState">
          {{ $t('dashboard.matrix.pctUsd') }}
        </b-radio-button>
        <b-radio-button v-model="state.mode" native-value="num" size="is-small" type="is-primary"
          data-cy="matrix-mode-num" @input="saveMatrixState">
          {{ $t('dashboard.matrix.num') }}
        </b-radio-button>
      </b-field>
    </div>
    <!-- 2026-10-24 layout pass (user ask): a hand-rolled table so the spreadsheet-style group
         headers exist (Buefy's b-table has no grouped header row) -- Health far left, then
         Brand and Sent, the Engagement section (incl. GP S%), Conversions, then Subscribers,
         each section tinted, the sorted column emphasized. Sorting is clickSort; the spam
         cell's class/style/tooltip stay brandMatrix.mjs::spamCell (GP-SPAM-VISIBILITY D3). -->
    <div class="table-container">
      <table class="table is-fullwidth is-hoverable is-narrow bm-table" data-cy="matrix-table">
        <thead>
          <tr class="bm-groups">
            <th colspan="3" />
            <th :colspan="engCells.length" class="bm-eng bm-group">
              {{ $t('dashboard.matrix.engagement') }}
            </th>
            <!-- Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D11) -- between
                 Engagement and Subscribers (2026-10-29). -->
            <th :colspan="conversionFields.length" class="bm-eng bm-group">
              {{ $t('analytics.conversions.title') }}
            </th>
            <th :colspan="subColumns.length" class="bm-subs bm-group">
              {{ $tc('globals.terms.subscriber', 2) }}
            </th>
          </tr>
          <tr>
            <th>{{ $t('dashboard.matrix.health') }}</th>
            <th :class="bmTh('brand')" :aria-sort="bmAria('brand')" @click="onSort('brand')">
              {{ $t('dashboard.matrix.brand') }}<span class="bm-arrow">{{ bmArrow('brand') }}</span>
            </th>
            <th class="has-text-right" :class="bmTh('sent')" :title="$t('dashboard.matrix.sentHelp')"
              :aria-sort="bmAria('sent')" @click="onSort('sent')">
              {{ $t('dashboard.matrix.sent') }}<span class="bm-arrow">{{ bmArrow('sent') }}</span>
            </th>
            <th v-for="c in engCells" :key="c.field" class="bm-eng has-text-right"
              :class="bmTh(c.field)" :title="c.help ? $t(c.help) : null"
              :aria-sort="bmAria(c.field)" @click="onSort(c.field)">
              {{ $t(c.label) }}<span class="bm-arrow">{{ bmArrow(c.field) }}</span>
            </th>
            <!-- CAMPAIGN-CONVERSIONS-SPEC D11 -- the %/$ mode shows sales, the # mode orders. -->
            <th v-for="f in conversionFields" :key="f" class="bm-eng has-text-right"
              :class="bmTh(f)" :title="conversionTip(f, conversions)"
              :aria-sort="bmAria(f)" @click="onSort(f)" :data-cy="`matrix-head-${f}`">
              {{ $t(`analytics.conversions.${f}`) }}<span class="bm-arrow">{{ bmArrow(f) }}</span>
            </th>
            <th v-for="c in subColumns" :key="c.field" class="bm-subs has-text-right"
              :class="bmTh(c.field)" :title="c.help ? $t(c.help) : null"
              :aria-sort="bmAria(c.field)" @click="onSort(c.field)">
              {{ $t(c.label) }}<span class="bm-arrow">{{ bmArrow(c.field) }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in tableRows" :key="row.brand || 'totals'" :class="row.totals ? 'bm-totals' : ''">
            <td class="bm-health">
              <health-chip v-if="row.status" :status="row.status"
                :to="{ name: 'brand', params: { brand: row.brand } }" />
            </td>
            <td :class="bmTd('brand')">
              <span :data-cy="`matrix-brand-${row.brand || 'totals'}`">{{ row.label }}</span>
            </td>
            <td class="has-text-right" :class="bmTd('sent')">
              <span :title="$t('dashboard.matrix.sentHelp')">{{ cell(row, 'sent') }}</span>
            </td>
            <td v-for="c in engCells" :key="c.field" class="bm-eng has-text-right" :class="bmTd(c.field)">
              <span v-if="c.field === 'spam_rate'" :class="spamCell(row).cls"
                :style="spamCell(row).style || null" :title="$t(spamCell(row).tooltip)"
                :data-cy="`matrix-spam-${row.brand || 'totals'}`">
                {{ row.totals ? '' : cell(row, 'spam_rate') }}</span>
              <!-- DASHBOARDS-POLISH-SPEC D1: red when the displayed rate crosses the row's STORED
                   threshold (brandMatrix.mjs::engagementCell decides; POLISH3 D1: both modes). -->
              <span v-else :class="engagementCell(row, c.field).cls || null" :title="c.help ? $t(c.help) : null"
                :data-cy="`matrix-${c.field}-${row.brand || 'totals'}`">{{ cell(row, c.field) }}</span>
            </td>
            <td v-for="f in conversionFields" :key="f" class="bm-eng has-text-right" :class="bmTd(f)"
              :title="conversionTip(f, conversions)" :data-cy="`matrix-${f}-${row.brand || 'totals'}`">
              {{ conversionText(row, f) }}
            </td>
            <td v-for="c in subColumns" :key="c.field" class="bm-subs has-text-right" :class="bmTd(c.field)">
              {{ cell(row, c.field) }}
            </td>
          </tr>
          <tr v-if="!loading && !tableRows.length">
            <td :colspan="3 + engCells.length + subColumns.length + conversionFields.length">
              <p class="has-text-grey">{{ $t('globals.messages.emptyState') }}</p>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="is-size-7 has-text-grey mt-2" data-cy="matrix-note">{{ $t('dashboard.matrix.note') }}</p>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import HealthChip from './HealthChip.vue';
import {
  CONVERSION_FIELDS, ENGAGEMENT_FIELDS, cellText, clickSort, conversionValue, engagementCell, loadState,
  saveState, sessionStore, shapeRows, spamCell, tableRows,
} from '../brandMatrix.mjs'; // eslint-disable-line import/extensions
import { conversionTooltip, scopedCell } from '../conversionCells.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';

export default Vue.extend({
  components: { HealthChip },

  // Integrations CAMPAIGN-MATRIX-SPEC D1 -- the pane carries the scope mixin itself (Brands.vue cannot:
  // its own brandRows would collide) and owns its fetch, the brandKey watch, the first fetch after the
  // lists load and the page.refresh hook.
  mixins: [brandScopeMixin],

  data() {
    return {
      apiRows: [],
      // CAMPAIGN-CONVERSIONS-SPEC D11 -- the conversions doc's as_of/floors (null = no doc: '—').
      conversions: null,
      loading: false,
      // D6 -- sort and %|# mode per browser session (sessionStorage key dashboard.matrix, kept).
      state: loadState(sessionStore(typeof window !== 'undefined' ? window : null)),
    };
  },

  computed: {
    ...mapGetters(['launch']),

    conversionFields() {
      return CONVERSION_FIELDS;
    },

    // D2/D4/D6 -- the module narrows (launch), shapes, sorts and appends the totals row.
    tableRows() {
      const rows = shapeRows(this.apiRows, { brands: this.brandRows, launch: this.launch });
      return tableRows(rows, this.state.sort, this.state.mode, {
        locale: (this.$i18n && this.$i18n.locale) || 'en',
        totalsLabel: this.$t('dashboard.matrix.total'),
      });
    },

    subColumns() {
      return [
        { field: 'total', label: 'dashboard.matrix.subscr' },
        { field: 'active', label: 'dashboard.matrix.active' },
        { field: 'held', label: 'dashboard.matrix.held' },
        { field: 'unsubscribed', label: 'dashboard.matrix.unsub' },
        // DELIVERABILITY-CONSISTENCY-SPEC D1/D2 -- "Blocklisted" (status), never "Blocked" (the
        // 5.7.x bounce class); the hover names the scope.
        { field: 'blocked', label: 'dashboard.matrix.blocked', help: 'dashboard.matrix.blockedHelp' },
      ];
    },

    // D4 -- Blocks is a subset of H. Bounces; its header says so. The Engagement section's cells
    // in column order, GP Spam % included (2026-10-24 layout pass).
    engCells() {
      const labels = {
        views: 'dashboard.matrix.views',
        clicks: 'dashboard.matrix.clicks',
        hard_bounces: 'dashboard.matrix.hardBounces',
        blocks: 'dashboard.matrix.blocks',
        complaints: 'dashboard.matrix.complaints',
      };
      // DASHBOARDS-POLISH-SPEC D1 -- every Engagement header carries its target hover.
      const helps = {
        views: 'dashboard.matrix.viewsHelp',
        clicks: 'dashboard.matrix.clicksHelp',
        hard_bounces: 'dashboard.matrix.hardBouncesHelp',
        blocks: 'dashboard.matrix.blocksHelp',
        complaints: 'dashboard.matrix.complaintsHelp',
      };
      return [
        ...ENGAGEMENT_FIELDS.map((f) => ({ field: f, label: labels[f], help: helps[f] })),
        { field: 'spam_rate', label: 'dashboard.matrix.spam', help: 'dashboard.matrix.gpSpamHelp' },
      ];
    },
  },

  methods: {
    scopeParams() {
      return this.scopeListIds ? { list_id: this.scopeListIds } : {};
    },

    fetchMatrix() {
      this.loading = true;
      this.$api.getDashboardBrandMatrix(this.scopeParams()).then((data) => {
        this.apiRows = (data && Array.isArray(data.rows)) ? data.rows : [];
        this.conversions = (data && data.conversions) || null;
      }).finally(() => {
        this.loading = false;
      });
    },

    saveMatrixState() {
      saveState(sessionStore(window), { sort: this.state.sort, mode: this.state.mode });
    },

    // D6 -- a header click is brandMatrix.mjs::clickSort, nothing else.
    onSort(field) {
      this.state.sort = clickSort(this.state.sort, field);
      this.saveMatrixState();
    },

    bmSorted(field) {
      return this.state.sort.field === field;
    },

    bmTh(field) {
      return ['bm-sort', { 'bm-sorted': this.bmSorted(field) }];
    },

    bmTd(field) {
      return { 'bm-sorted': this.bmSorted(field) };
    },

    bmArrow(field) {
      if (!this.bmSorted(field)) {
        return '';
      }
      return this.state.sort.order === 'asc' ? '↑' : '↓';
    },

    bmAria(field) {
      if (!this.bmSorted(field)) {
        return null;
      }
      return this.state.sort.order === 'asc' ? 'ascending' : 'descending';
    },

    cell(row, field) {
      return cellText(row, field, this.state.mode, (n) => this.$utils.formatNumber(n));
    },

    // One figure per mode (2026-10-29): whole-dollar sales under %/$, orders under #; '—' with no
    // doc (the sales text is '' then, so fall back to the orders text, which is the dash).
    conversionText(row, field) {
      const c = scopedCell(conversionValue(row, field), this.conversions, (n) => this.$utils.formatNumber(n));
      return this.state.mode === 'num' ? c.orders : (c.sales || c.orders);
    },

    conversionTip(measure, meta) {
      return conversionTooltip(measure, meta, (k, p) => this.$t(k, p));
    },

    spamCell(row) {
      return spamCell(row);
    },

    engagementCell(row, field) {
      return engagementCell(row, field, this.state.mode);
    },
  },

  watch: {
    // Fork (global brand, D4) -- a selection change refetches.
    brandKey(now) {
      if (this.brandChanged(now)) {
        this.fetchMatrix();
      }
    },
  },

  created() {
    this.$root.$on('page.refresh', this.fetchMatrix);
  },

  destroyed() {
    this.$root.$off('page.refresh', this.fetchMatrix);
  },

  mounted() {
    // Fork (global brand, D3) -- the first scoped fetch waits for the lists (I13).
    this.afterListsLoaded(() => this.fetchMatrix());
  },
});
</script>
