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
                    <ul class="no has-text-grey">
                      <li>
                        <label for="#">{{ $utils.niceNumber(counts.subscribers.blocklisted) }}</label>
                        {{ $t('subscribers.status.blocklisted') }}
                      </li>
                      <!-- Fork (brand analytics, BRAND-ANALYTICS-SPEC D7): a list-scoped user's counts
                      are scoped to their lists, where orphans (no list at all) are 0 by definition.
                      Under a global brand (GLOBAL-BRAND-SPEC D5) the counts are scoped too, and
                      blocklisted is that scope's blocklisted members. -->
                      <li v-if="!counts.scoped">
                        <label for="#">{{ $utils.niceNumber(counts.subscribers.orphans) }}</label>
                        {{ $t('dashboard.orphanSubs') }}
                      </li>
                    </ul>
                  </div><!-- subscriber breakdown -->
                </div><!-- subscriber columns -->
                <hr />
                <div class="columns" data-cy="messages">
                  <div class="column is-12">
                    <p class="title">
                      <b-icon icon="email-outline" />
                      {{ $utils.niceNumber(counts.messages) }}
                    </p>
                    <p class="is-size-6 has-text-grey">
                      {{ $t('dashboard.messagesSent') }}
                    </p>
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
    ...mapState(['settings']),
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
