<template>
  <!-- Fork (campaign matrix, integrations listmonk/CAMPAIGN-MATRIX-SPEC D3-D10). The Matrix pane of
       Campaigns > Analytics: one row per campaign in scope with activity in the Brand Matrix's window
       (the 60 Mountain days ending yesterday), in the Brand Matrix's grouped-header style. The server
       (GET /api/analytics/campaigns/matrix) scopes and counts; every rate, tier fold, sort and
       missing-value rule is campaignMatrix.mjs. The brand selector reaches the server as list_id;
       the navbar launch filter narrows here by each row's brand. -->
  <section class="campaign-matrix relative" data-cy="campaign-matrix">
    <b-loading v-if="loading" active :is-full-page="false" />
    <div class="is-flex is-align-items-center mb-3">
      <!-- D4: the Campaigns page's kinds and labels; own session state, All by default. -->
      <b-field class="mb-0 mr-5">
        <b-radio-button v-model="state.kind" native-value="all" size="is-small" type="is-primary"
          data-cy="cm-kind-all" @input="onKind">
          {{ $t('globals.terms.all') }}
        </b-radio-button>
        <b-radio-button v-model="state.kind" native-value="broadcasts" size="is-small" type="is-primary"
          data-cy="cm-kind-broadcasts" @input="onKind">
          {{ $t('campaigns.typeBroadcasts') }}
        </b-radio-button>
        <b-radio-button v-model="state.kind" native-value="automations" size="is-small" type="is-primary"
          data-cy="cm-kind-automations" @input="onKind">
          {{ $t('campaigns.typeAutomations') }}
        </b-radio-button>
      </b-field>
      <b-field class="mb-0">
        <b-radio-button v-model="state.mode" native-value="pct" size="is-small" type="is-primary"
          data-cy="cm-mode-pct" @input="saveState">
          {{ $t('dashboard.matrix.pctUsd') }}
        </b-radio-button>
        <b-radio-button v-model="state.mode" native-value="num" size="is-small" type="is-primary"
          data-cy="cm-mode-num" @input="saveState">
          {{ $t('dashboard.matrix.num') }}
        </b-radio-button>
      </b-field>
    </div>
    <div class="table-container">
      <table class="table is-fullwidth is-hoverable is-narrow bm-table" data-cy="cm-table">
        <thead>
          <tr class="bm-groups">
            <th colspan="4" />
            <th :colspan="4" class="bm-eng bm-group">{{ $t('analytics.matrix.views') }}</th>
            <th />
            <th :colspan="2" class="bm-eng bm-group">{{ $t('analytics.conversions.title') }}</th>
            <th :colspan="2" class="bm-subs bm-group">{{ $t('analytics.matrix.bounces') }}</th>
            <th :colspan="showGp ? 2 : 1" class="bm-subs bm-group">{{ $t('analytics.matrix.complaints') }}</th>
          </tr>
          <tr>
            <th v-for="c in leftCols" :key="c.field" :class="[bmTh(c.field), { 'has-text-right': c.field === 'sent' }]"
              :aria-sort="bmAria(c.field)" @click="onSort(c.field)" :data-cy="`cm-head-${c.field}`">
              {{ $t(c.label) }}<span class="bm-arrow">{{ bmArrow(c.field) }}</span>
            </th>
            <th v-for="c in midCols" :key="c.field" class="has-text-right" :class="[bmTh(c.field), c.group]"
              :title="c.help ? $t(c.help) : null" :aria-sort="bmAria(c.field)" @click="onSort(c.field)"
              :data-cy="`cm-head-${c.field}`">
              {{ $t(c.label) }}<span class="bm-arrow">{{ bmArrow(c.field) }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in tableRows" :key="row.totals ? 'totals' : row.id" :class="row.totals ? 'bm-totals' : ''">
            <td :class="bmTd('name')">
              <!-- D3: the campaign's own Campaign pane (the remount the URL change causes is intended). -->
              <router-link v-if="!row.totals" :to="{ name: 'campaignAnalytics', query: { id: row.id } }"
                :data-cy="`cm-name-${row.id}`">
{{ row.name }}
</router-link>
              <template v-else>{{ row.name }}</template>
            </td>
            <td :class="bmTd('brand')">{{ row.totals ? '' : (row.label || '—') }}</td>
            <td :class="bmTd('date')">
              <span :title="dateTip(row)">{{ dateText(row) }}</span>
            </td>
            <td class="has-text-right" :class="bmTd('sent')">{{ cell(row, 'sent') }}</td>
            <td v-for="c in midCols" :key="c.field" class="has-text-right" :class="[bmTd(c.field), c.group]"
              :data-cy="`cm-${c.field}-${row.totals ? 'totals' : row.id}`">
              <span v-if="c.field === 'gp_rate'" :class="gpClass(row) || null" :title="gpTitle(row)">
                {{ row.totals ? '' : cell(row, 'gp_rate') }}</span>
              <span v-else-if="isConversion(c.field)" :title="conversionTip(c.field)">{{ conversionText(row, c.field) }}</span>
              <span v-else-if="isTier(c.field) && row.clientFloor" :title="$t('analytics.matrix.clientFloor')">{{ cell(row, c.field) }}</span>
              <span v-else>{{ cell(row, c.field) }}</span>
            </td>
          </tr>
          <tr v-if="!loading && !tableRows.length">
            <td :colspan="leftCols.length + midCols.length">
              <p class="has-text-grey">{{ $t('globals.messages.emptyState') }}</p>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="is-size-7 has-text-grey mt-2" data-cy="cm-note">
      {{ $t('analytics.matrix.note', { from: period.from || '—', to: period.to || '—' }) }}
    </p>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapGetters, mapState } from 'vuex';
import { sessionStore } from '../brandMatrix.mjs'; // eslint-disable-line import/extensions
import {
  CONVERSION_FIELDS, TIER_FIELDS, cellText, clickSort, conversionValue, dateCell, gpClass, gpTip, loadState,
  saveState, shapeRows, tableRows,
} from '../campaignMatrix.mjs'; // eslint-disable-line import/extensions
import { conversionTooltip, scopedCell } from '../conversionCells.mjs'; // eslint-disable-line import/extensions
import { brandLabelOf } from '../brandScope.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';

export default Vue.extend({
  // D6: the brand selector reaches the server as list_id (scopeListIds); the first fetch waits for
  // the scope to resolve and a selection change refetches.
  mixins: [brandScopeMixin],

  data() {
    return {
      apiRows: [],
      conversions: null,
      // The server's window ({ from, to } Mountain dates) for the note.
      period: {},
      loading: false,
      // Sort, %/$ | # mode and kind, per browser session (sessionStorage key analytics.matrix).
      state: loadState(sessionStore(typeof window !== 'undefined' ? window : null)),
    };
  },

  computed: {
    ...mapGetters(['launch']),
    ...mapState({ healthDocs: 'brandHealth' }),

    // D7: the GP S% column only for a brands:get holder (the server returns no GP otherwise).
    showGp() {
      return this.$can('brands:get');
    },

    leftCols() {
      return [
        { field: 'name', label: 'analytics.matrix.campaign' },
        { field: 'brand', label: 'dashboard.matrix.brand' },
        { field: 'date', label: 'analytics.matrix.date' },
        { field: 'sent', label: 'dashboard.matrix.sent' },
      ];
    },

    // D5: Views (Total, Client, Proxy, Machine), Clicks, Conversions, Bounces (Hard, Soft),
    // Complaints (FBL, GP S%).
    midCols() {
      const cols = [
        {
          field: 'views', label: 'analytics.matrix.total', help: 'analytics.matrix.totalHelp', group: 'bm-eng',
        },
        {
          field: 'client', label: 'analytics.matrix.client', help: 'analytics.matrix.clientHelp', group: 'bm-eng',
        },
        {
          field: 'proxy', label: 'analytics.matrix.proxy', help: 'analytics.matrix.proxyHelp', group: 'bm-eng',
        },
        {
          field: 'machine', label: 'analytics.matrix.machine', help: 'analytics.matrix.machineHelp', group: 'bm-eng',
        },
        {
          field: 'clicks', label: 'dashboard.matrix.clicks', help: 'analytics.matrix.clicksHelp', group: '',
        },
        { field: 'tracked', label: 'analytics.conversions.trackedConv', group: 'bm-eng' },
        { field: 'matched', label: 'analytics.conversions.matchedConv', group: 'bm-eng' },
        { field: 'hard_bounces', label: 'analytics.matrix.hard', group: 'bm-subs' },
        { field: 'soft_bounces', label: 'analytics.matrix.soft', group: 'bm-subs' },
        {
          field: 'complaints', label: 'analytics.matrix.fbl', help: 'analytics.matrix.fblHelp', group: 'bm-subs',
        },
      ];
      if (this.showGp) {
        cols.push({
          field: 'gp_rate', label: 'dashboard.matrix.spam', help: 'analytics.matrix.gpHelp', group: 'bm-subs',
        });
      }
      return cols;
    },

    tableRows() {
      const brands = this.brandRows;
      const rows = shapeRows(this.apiRows, {
        launch: this.launch, docs: this.healthDocs, brandLabel: (s) => brandLabelOf(s, brands),
      });
      return tableRows(rows, this.state.sort, this.state.mode, {
        locale: (this.$i18n && this.$i18n.locale) || 'en',
        totalsLabel: this.$t('dashboard.matrix.total'),
      });
    },
  },

  methods: {
    params() {
      const p = { kind: this.state.kind };
      if (this.scopeListIds) {
        p.list_id = this.scopeListIds;
      }
      return p;
    },

    fetchRows() {
      this.loading = true;
      this.$api.getCampaignMatrix(this.params()).then((data) => {
        this.apiRows = (data && Array.isArray(data.rows)) ? data.rows : [];
        this.conversions = (data && data.conversions) || null;
        this.period = (data && data.window) || {};
      }).finally(() => {
        this.loading = false;
      });
    },

    saveState() {
      saveState(sessionStore(window), this.state);
    },

    onKind() {
      this.saveState();
      this.fetchRows();
    },

    onSort(field) {
      this.state.sort = clickSort(this.state.sort, field);
      this.saveState();
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

    isConversion(field) {
      return CONVERSION_FIELDS.includes(field);
    },

    isTier(field) {
      return TIER_FIELDS.includes(field);
    },

    // D9: the cell reuses conversionCells.mjs::scopedCell -- whole-dollar sales under %/$, orders
    // under #, '—' when not covered (or no conversions doc).
    conversionText(row, field) {
      const c = scopedCell(conversionValue(row, field), this.conversions, (n) => this.$utils.formatNumber(n));
      return this.state.mode === 'num' ? c.orders : (c.sales || c.orders);
    },

    conversionTip(field) {
      return conversionTooltip(field, this.conversions, (k, p) => this.$t(k, p));
    },

    gpClass(row) {
      return gpClass(row);
    },

    gpTitle(row) {
      const t = gpTip(row);
      return t ? this.$t(t.key, t.params) : null;
    },

    dateText(row) {
      return dateCell(row).text;
    },

    dateTip(row) {
      const t = dateCell(row).tip;
      return t ? this.$t(t.key, t.params) : null;
    },
  },

  watch: {
    brandKey(now) {
      if (this.brandChanged(now)) {
        this.fetchRows();
      }
    },
  },

  created() {
    this.$root.$on('page.refresh', this.fetchRows);
  },

  destroyed() {
    this.$root.$off('page.refresh', this.fetchRows);
  },

  mounted() {
    this.afterListsLoaded(() => this.fetchRows());
  },
});
</script>
