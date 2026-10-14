<template>
  <section class="dashboard content">
    <header class="columns">
      <div class="column is-two-thirds">
        <h1 :class="['title', 'is-5', { 'mb-0': canSes }]">
          {{ $utils.niceDate(new Date()) }}
        </h1>
        <!-- Fork (system health, integrations SES-HEALTH-SPEC D7) -- Overview | SES pill, the
             Campaigns Broadcasts | Automations pattern. Sticky via the shared pref helper; the SES
             option exists only for users who may read Brands (brands:get). Directly beneath the
             date, left-aligned (BRANDS-UX-SPEC D2). -->
        <b-field v-if="canSes" class="is-inline-flex mt-2">
          <b-radio-button v-model="pane" native-value="overview" type="is-primary"
            data-cy="pane-overview" @input="onPaneChange">
            {{ $t('dashboard.panes.overview') }}
          </b-radio-button>
          <b-radio-button v-model="pane" native-value="ses" type="is-primary"
            data-cy="pane-ses" @input="onPaneChange">
            {{ $t('dashboard.panes.ses') }}
          </b-radio-button>
        </b-field>
      </div>
    </header>

    <dashboard-ses v-if="pane === 'ses'" />

    <section v-else class="counts wrap">
      <div class="tile is-ancestor">
        <div class="tile is-vertical is-12">
          <div class="tile">
            <div class="tile is-parent is-vertical relative">
              <b-loading v-if="isCountsLoading" active :is-full-page="false" />
              <article class="tile is-child notification" data-cy="lists">
                <div class="columns is-mobile">
                  <div class="column is-6">
                    <p class="title">
                      <b-icon icon="format-list-bulleted-square" />
                      {{ $utils.niceNumber(counts.lists.total) }}
                    </p>
                    <p class="is-size-6 has-text-grey">
                      {{ $tc('globals.terms.list', counts.lists.total) }}
                    </p>
                  </div>
                  <div class="column is-6">
                    <ul class="no has-text-grey">
                      <li>
                        <label for="#">{{ $utils.niceNumber(counts.lists.public) }}</label>
                        {{ $t('lists.types.public') }}
                      </li>
                      <li>
                        <label for="#">{{ $utils.niceNumber(counts.lists.private) }}</label>
                        {{ $t('lists.types.private') }}
                      </li>
                      <li>
                        <label for="#">{{ $utils.niceNumber(counts.lists.optinSingle) }}</label>
                        {{ $t('lists.optins.single') }}
                      </li>
                      <li>
                        <label for="#">{{ $utils.niceNumber(counts.lists.optinDouble) }}</label>
                        {{ $t('lists.optins.double') }}
                      </li>
                    </ul>
                  </div>
                </div>
              </article><!-- lists -->

              <article class="tile is-child notification" data-cy="campaigns">
                <div class="columns is-mobile">
                  <div class="column is-6">
                    <p class="title">
                      <b-icon icon="rocket-launch-outline" />
                      {{ $utils.niceNumber(counts.campaigns.total) }}
                    </p>
                    <p class="is-size-6 has-text-grey">
                      {{ $tc('globals.terms.campaign', counts.campaigns.total) }}
                    </p>
                  </div>
                  <div class="column is-6">
                    <ul class="no has-text-grey">
                      <li v-for="(num, status) in counts.campaigns.byStatus" :key="status">
                        <label for="#" :data-cy="`campaigns-${status}`">{{ num }}</label>
                        {{ $t(`campaigns.status.${status}`) }}
                        <span v-if="status === 'running'" class="spinner is-tiny">
                          <b-loading :is-full-page="false" active />
                        </span>
                      </li>
                    </ul>
                  </div>
                </div>
              </article><!-- campaigns -->
            </div><!-- block -->

            <div class="tile is-parent relative">
              <b-loading v-if="isCountsLoading" active :is-full-page="false" />
              <article class="tile is-child notification" data-cy="subscribers">
                <!-- Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC D1-D3, D7). The
                subscriptions of the PUBLIC lists in scope (each brand's master opt-in list), summed,
                with the Lists grid's segments. Collapsed = the segment lines; expanded = a segment x
                send-language table whose columns are the Lists grid's lines rule (gridLines.mjs).
                Sticky per browser. Numbers link nowhere (N1). The fontello subset has plus/minus,
                no *-box-outline, so the box is drawn in CSS as on the Lists grid. -->
                <button type="button" class="expand-btn subs-expand" data-cy="subscribers-expand"
                  :title="subsExpanded ? $t('dashboard.subsCollapse') : $t('dashboard.subsExpand')"
                  :aria-label="subsExpanded ? $t('dashboard.subsCollapse') : $t('dashboard.subsExpand')"
                  :aria-expanded="subsExpanded ? 'true' : 'false'" @click="toggleSubs">
                  <b-icon :icon="subsExpanded ? 'minus' : 'plus'" size="is-small" />
                </button>
                <div class="columns is-mobile">
                  <div class="column is-6">
                    <p class="title">
                      <b-icon icon="account-multiple" />
                      {{ $utils.niceNumber(counts.subscribers.total) }}
                    </p>
                    <p class="is-size-6 has-text-grey">
                      {{ $tc('globals.terms.subscriber', counts.subscribers.total) }}
                    </p>
                  </div>

                  <div class="column is-6">
                    <!-- Nothing to list when expanded under a scope (Stage 4 F6): no blank column. -->
                    <ul v-if="!subsExpanded || !counts.scoped" class="no has-text-grey">
                      <template v-if="!subsExpanded">
                        <li v-for="r in subsRows" :key="r.key" :data-cy="`subscribers-${r.key}`">
                          <label for="#">{{ $utils.niceNumber(subsAll[r.key] || 0) }}</label>
                          {{ $t(r.label) }}
                        </li>
                      </template>
                      <!-- Fork (brand analytics, BRAND-ANALYTICS-SPEC D7): a list-scoped user's counts
                      are scoped to their lists, where orphans (no list at all) are 0 by definition.
                      Under a global brand (GLOBAL-BRAND-SPEC D5) the counts are scoped too.
                      Orphans has no language, so it stays a line when expanded. -->
                      <li v-if="!counts.scoped">
                        <label for="#">{{ $utils.niceNumber(counts.subscribers.orphans) }}</label>
                        {{ $t('dashboard.orphanSubs') }}
                      </li>
                    </ul>
                  </div><!-- subscriber breakdown -->
                </div><!-- subscriber columns -->
                <table v-if="subsExpanded" class="table is-narrow subs-grid" data-cy="subscribers-grid">
                  <thead>
                    <tr>
                      <th />
                      <th v-for="col in subsColumns" :key="col.key" class="has-text-right"
                        :data-cy="`subscribers-col-${col.key}`" :title="subsColTitle(col.key)">
                        {{ subsColLabel(col.key) }}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="r in subsRows" :key="r.key">
                      <td class="has-text-grey">{{ $t(r.label) }}</td>
                      <td v-for="col in subsColumns" :key="col.key" class="has-text-right"
                        :data-cy="`subscribers-${r.key}-${col.key}`" :title="subsCellTitle(col, r.key)">
                        {{ $utils.formatNumber(col.row[r.key] || 0) }}
                      </td>
                    </tr>
                  </tbody>
                </table>
                <hr />
                <!-- Fork (dashboard counts, D4) -- Views, Clicks and Bounces over the scope's
                campaigns, each get-campaign-stats' per-campaign number summed (a view has ONE
                definition, CAMPAIGN-RATES-SPEC D1). -->
                <div class="columns is-mobile" data-cy="messages">
                  <div class="column is-6">
                    <p class="title">
                      <b-icon icon="email-outline" />
                      {{ $utils.niceNumber(counts.messages) }}
                    </p>
                    <p class="is-size-6 has-text-grey">
                      {{ $t('dashboard.messagesSent') }}
                    </p>
                  </div>
                  <div class="column is-6">
                    <ul class="no has-text-grey">
                      <li data-cy="engagement-views">
                        <label for="#">{{ $utils.niceNumber(engagement.views || 0) }}</label>
                        {{ $t('campaigns.views') }}
                      </li>
                      <li data-cy="engagement-clicks">
                        <label for="#">{{ $utils.niceNumber(engagement.clicks || 0) }}</label>
                        {{ $t('campaigns.clicks') }}
                      </li>
                      <li data-cy="engagement-bounces">
                        <label for="#">{{ $utils.niceNumber(engagement.bounces || 0) }}</label>
                        {{ $t('globals.terms.bounces') }}
                      </li>
                    </ul>
                  </div>
                </div>
              </article><!-- subscribers -->
            </div>
          </div>
          <div class="tile is-parent relative">
            <b-loading v-if="isChartsLoading" active :is-full-page="false" />
            <article class="tile is-child notification charts">
              <div class="columns">
                <div class="column is-6">
                  <h3 class="title is-size-6">
                    {{ $t('dashboard.campaignViews') }}
                  </h3><br />
                  <chart type="line" v-if="campaignViews" :data="campaignViews" />
                </div>
                <div class="column is-6">
                  <h3 class="title is-size-6 has-text-right">
                    {{ $t('dashboard.linkClicks') }}
                  </h3><br />
                  <chart type="line" v-if="campaignClicks" :data="campaignClicks" />
                </div>
              </div>
            </article>
          </div>

          <!-- Fork (client stats, integrations CLIENT-STATS-SPEC D6/D9): views, clicks and combined
               per email client over the charts' window. Its own brand picker is gone (integrations
               GLOBAL-BRAND-SPEC D5): the global brand selector is the one filter, sent with the
               counts and charts. -->
          <div class="tile is-parent relative">
            <b-loading v-if="clients.loading" active :is-full-page="false" />
            <article class="tile is-child notification" data-cy="clients">
              <div class="columns is-mobile">
                <div class="column">
                  <h3 class="title is-size-6">{{ $t('analytics.clients') }}</h3>
                </div>
              </div>
              <b-table :data="clientTableRows" hoverable narrowed
                backend-sorting :default-sort="[clients.sort.field, clients.sort.order]" @sort="onClientSort">
                <b-table-column v-slot="props" field="name" :label="$t('analytics.clientsClient')" sortable>
                  <span :class="{ 'has-text-grey': props.row.unknown }" :title="clientRosterTip(props.row.client)">{{ props.row.name }}</span>
                </b-table-column>
                <b-table-column v-slot="props" field="views" :label="$t('campaigns.views')" numeric sortable>
                  {{ $utils.formatNumber(props.row.views) }}
                  <span v-if="clientShare(props.row.views, 'views')" class="is-size-7 has-text-grey">({{ clientShare(props.row.views, 'views') }})</span>
                </b-table-column>
                <b-table-column v-slot="props" field="clicks" :label="$t('campaigns.clicks')" numeric sortable>
                  {{ $utils.formatNumber(props.row.clicks) }}
                  <span v-if="clientShare(props.row.clicks, 'clicks')" class="is-size-7 has-text-grey">({{ clientShare(props.row.clicks, 'clicks') }})</span>
                </b-table-column>
                <b-table-column v-slot="props" field="combined" :label="$t('analytics.clientsCombined')" numeric sortable>
                  {{ $utils.formatNumber(props.row.combined) }}
                  <span v-if="clientShare(props.row.combined, 'combined')" class="is-size-7 has-text-grey">({{ clientShare(props.row.combined, 'combined') }})</span>
                </b-table-column>
                <template #empty v-if="!clients.loading">
                  <p class="has-text-grey">{{ $t('globals.messages.emptyState') }}</p>
                </template>
              </b-table>
              <p class="is-size-7 has-text-grey mt-2">{{ $t('dashboard.clientsWindow') }} {{ $t('analytics.clientsHint') }}</p>
            </article>
          </div>
        </div>
      </div><!-- tile block -->
      <p v-if="settings['app.cache_slow_queries']" class="has-text-grey">
        *{{ $t('globals.messages.slowQueriesCached') }}
        <a href="https://listmonk.app/docs/maintenance/performance/" target="_blank" rel="noopener noreferer"
          class="has-text-grey">
          <b-icon icon="link-variant" /> {{ $t('globals.buttons.learnMore') }}
        </a>
      </p>
    </section>
  </section>
</template>

<script>
import dayjs from 'dayjs';
import Vue from 'vue';
import { mapState } from 'vuex';
import { colors } from '../constants';
import Chart from '../components/Chart.vue';
import DashboardSes from './DashboardSes.vue';
import {
  DEFAULT_SORT as CLIENT_DEFAULT_SORT, clientTotals, shapeClientRows, sortClientRows,
} from '../clientRows.mjs'; // eslint-disable-line import/extensions
import { rosterFor } from '../clientRoster.mjs'; // eslint-disable-line import/extensions
import { rateCell } from '../campaignRates.mjs'; // eslint-disable-line import/extensions
import { subscriberColumns, subscriberRows } from '../dashboardSubs.mjs'; // eslint-disable-line import/extensions
import { isSendPlus, sendLangCode } from '../langs';
import brandScopeMixin from '../brandScopeMixin';

export default Vue.extend({
  components: {
    Chart,
    DashboardSes,
  },

  // Fork (global brand, integrations GLOBAL-BRAND-SPEC D4/D5).
  mixins: [brandScopeMixin],

  data() {
    const canSes = this.$can('brands:get');
    return {
      canSes,
      // Overview unless the user may see SES and last chose it.
      pane: canSes && this.$utils.getPref('dashboard.sesPane') === true ? 'ses' : 'overview',
      // Fork (dashboard counts, DASHBOARD-COUNTS-SPEC D3) -- the Subscribers container's
      // language table, sticky per browser (the dashboard.sesPane pattern).
      subsExpanded: this.$utils.getPref('dashboard.subsExpanded') === true,
      isChartsLoading: true,
      isCountsLoading: true,
      campaignViews: null,
      campaignClicks: null,
      counts: {
        lists: {},
        subscribers: {},
        campaigns: {},
        messages: 0,
      },
      // Fork (client stats) -- the client panel.
      clients: {
        rows: [],
        loading: false,
        sort: { ...CLIENT_DEFAULT_SORT },
      },
    };
  },

  methods: {
    onPaneChange() {
      this.$utils.setPref('dashboard.sesPane', this.pane === 'ses');
    },

    // Fork (dashboard counts, D3).
    toggleSubs() {
      this.subsExpanded = !this.subsExpanded;
      this.$utils.setPref('dashboard.subsExpanded', this.subsExpanded);
    },

    // The expanded table's headers: All, then the send-language codes (EN+ for en) and Other --
    // the Lists grid's labels and help.
    subsColLabel(key) {
      if (key === 'all') {
        return this.$t('globals.terms.all');
      }
      if (key === 'other') {
        return this.$t('lists.grid.other');
      }
      return sendLangCode(key);
    },

    subsColTitle(key) {
      if (key === 'other') {
        return this.$t('lists.grid.otherHelp');
      }
      return isSendPlus(key) ? this.$t('langs.sendPlusHelp') : null;
    },

    // The Lists grid's enSplit rule: an en number that includes no-language subscribers says so.
    subsCellTitle(col, seg) {
      if (col.key !== 'en' || !col.none || !(col.none[seg] > 0)) {
        return null;
      }
      return this.$t('lists.grid.enSplit', {
        en: this.$utils.formatNumber(col.row[seg] - col.none[seg]),
        none: this.$utils.formatNumber(col.none[seg]),
      });
    },

    // Fork (global brand, D4/D5) -- the three reads together, each with the global brand's
    // effective list set (no list_id under All brands: today's Dashboard exactly).
    fetchData() {
      this.isCountsLoading = true;
      this.isChartsLoading = true;
      const params = this.scopeListIds ? { list_id: this.scopeListIds } : {};

      this.$api.getDashboardCounts(params).then((data) => {
        this.counts = data;
        this.isCountsLoading = false;
      });

      this.$api.getDashboardCharts(params).then((data) => {
        this.isChartsLoading = false;
        this.campaignViews = this.makeChart(data.campaignViews);
        this.campaignClicks = this.makeChart(data.linkClicks);
      });

      this.fetchClients(params);
    },

    // Fork (client stats) -- the client panel over the same scope as the counts.
    fetchClients(params) {
      this.clients.loading = true;
      this.$api.getDashboardClients(params).then((data) => {
        this.clients.rows = shapeClientRows(data.clients, {
          locale: (this.$i18n && this.$i18n.locale) || 'en',
          unknownLabel: this.$t('analytics.clientsUnknown'),
        });
      }).finally(() => {
        this.clients.loading = false;
      });
    },

    onClientSort(field, order) {
      this.clients.sort = { field, order };
    },

    clientShare(n, field) {
      return rateCell(n, this.clientTotalsSum[field], 1).pct;
    },

    clientRosterTip(token) {
      const ids = rosterFor(token);
      return ids.length ? this.$t('analytics.clientsRoster', { ids: ids.join(', ') }) : '';
    },

    makeChart(data) {
      if (data.length === 0) {
        return {};
      }
      return {
        labels: data.map((d) => dayjs(d.date).format('DD MMM')),
        datasets: [
          {
            data: [...data.map((d) => d.count)],
            borderColor: colors.primary,
            borderWidth: 2,
            pointHoverBorderWidth: 5,
            pointBorderWidth: 0.5,
          },
        ],
      };
    },
  },

  computed: {
    ...mapState(['settings', 'serverConfig']),

    // Fork (dashboard counts, D2/D3/D6) -- the API's subscribers.grid; the module decides the lines.
    subsGrid() {
      return (this.counts.subscribers && this.counts.subscribers.grid) || {};
    },

    subsAll() {
      return this.subsGrid.all || {};
    },

    subsRows() {
      return subscriberRows(this.subsGrid);
    },

    subsColumns() {
      return subscriberColumns(this.subsGrid, !!(this.serverConfig && this.serverConfig.lang_enabled));
    },

    engagement() {
      return this.counts.engagement || {};
    },
    dayjs() {
      return dayjs;
    },

    clientTotalsSum() {
      return clientTotals(this.clients.rows);
    },

    clientTableRows() {
      const { field, order } = this.clients.sort;
      return sortClientRows(this.clients.rows, field, order, (this.$i18n && this.$i18n.locale) || 'en');
    },
  },

  watch: {
    // Fork (global brand, D4) -- a selection change re-issues the three reads together.
    brandKey(now, before) {
      if (before !== 'pending' && now !== 'pending') {
        this.fetchData();
      }
    },
  },

  created() {
    this.$root.$on('page.refresh', this.fetchData);
  },

  destroyed() {
    this.$root.$off('page.refresh', this.fetchData);
  },

  mounted() {
    // Fork (global brand, D3) -- the first scoped fetch waits for the lists (I13).
    this.afterListsLoaded(() => this.fetchData());
  },
});
</script>
