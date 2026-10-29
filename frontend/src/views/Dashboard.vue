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
          <!-- Fork (campaign matrix, integrations CAMPAIGN-MATRIX-SPEC D1/D2) -- the Brand Matrix and
               Brand Usage panes moved to the Brands page (List | Matrix | Usage). -->
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
          <!-- Fork (BRAND-USAGE-V2-SPEC D6/D7b) -- Subscribers + Messages sent on the LEFT, in a
               container of fixed width (its widest state, so expanding Subscribers never moves
               it); Lists and Campaigns merged into ONE container on the right, a separator
               between them. The four blocks' content is unchanged. -->
          <div class="tile">
            <div class="tile is-parent relative dash-subs-col">
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
                    <!-- Fork (brand matrix, BRAND-MATRIX-SPEC D9) -- comma-separated, never k/m. -->
                    <p class="title">
                      <b-icon icon="account-multiple" />
                      {{ $utils.formatNumber(counts.subscribers.total || 0) }}
                    </p>
                    <p class="is-size-6 has-text-grey">
                      {{ $tc('globals.terms.subscriber', counts.subscribers.total) }}
                    </p>
                  </div>

                  <div class="column is-6">
                    <!-- Aligned columns (2026-10-24): rate, count, label -- see .pct-lines. -->
                    <ul class="no has-text-grey pct-lines">
                      <!-- Fork (dashboard counts, DASHBOARD-COUNTS-SPEC R1) -- Held, Unsubscribed and
                      Blocklisted read rate-first over the public lists' total; Active and Pending
                      are plain counts (dashboardSubs.mjs::segmentCell decides). -->
                      <template v-if="!subsExpanded">
                        <li v-for="r in subsLineRows" :key="r.key" :data-cy="`subscribers-${r.key}`"
                          :title="subsSegTitle(r.key)">
                          <span class="pl-pct">{{ r.parts.pct }}</span>
                          <span class="pl-count">{{ r.parts.count }}</span>
                          <span class="pl-label">{{ $t(r.label) }}</span>
                        </li>
                      </template>
                      <!-- Fork (dashboard counts, R3) -- Orphans in every scope (0 under a scope by
                      definition: a list-scoped user's or a global brand's counts cover lists, and an
                      orphan is on none) and when expanded (it has no language), so the column is
                      never empty. -->
                      <li data-cy="subscribers-orphans">
                        <span class="pl-pct" />
                        <span class="pl-count">{{ $utils.formatNumber(counts.subscribers.orphans || 0) }}</span>
                        <span class="pl-label">{{ $t('dashboard.orphanSubs') }}</span>
                      </li>
                    </ul>
                  </div><!-- subscriber breakdown -->
                </div><!-- subscriber columns -->
                <table v-if="subsExpanded" class="table is-narrow subs-grid" data-cy="subscribers-grid">
                  <thead>
                    <tr>
                      <!-- Fork (BRAND-USAGE-V2-SPEC D7) -- the matrix's %|# toggle, per session; it
                           governs only the rate-bearing cells (dashboardSubs.mjs::subsCellText). -->
                      <th class="subs-mode">
                        <b-field class="mb-0">
                          <b-radio-button v-model="overview.mode" native-value="pct" size="is-small" type="is-primary"
                            data-cy="subs-mode-pct" @input="onOverviewMode">
                            {{ $t('dashboard.matrix.pct') }}
                          </b-radio-button>
                          <b-radio-button v-model="overview.mode" native-value="num" size="is-small" type="is-primary"
                            data-cy="subs-mode-num" @input="onOverviewMode">
                            {{ $t('dashboard.matrix.num') }}
                          </b-radio-button>
                        </b-field>
                      </th>
                      <th v-for="col in subsColumns" :key="col.key" class="has-text-right"
                        :data-cy="`subscribers-col-${col.key}`" :title="subsColTitle(col.key)">
                        {{ subsColLabel(col.key) }}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <!-- Fork (dashboard counts, R1) -- each rate cell over THAT column's total; V2 D7:
                         % the rate alone, # the comma-separated count, no parentheses. -->
                    <tr v-for="r in subsRows" :key="r.key">
                      <td class="has-text-grey" :title="subsSegTitle(r.key)">{{ $t(r.label) }}</td>
                      <td v-for="col in subsColumns" :key="col.key" class="has-text-right"
                        :data-cy="`subscribers-${r.key}-${col.key}`" :title="subsCellTitle(col, r.key)">
                        {{ subsCellText(col, r) }}
                      </td>
                    </tr>
                    <!-- The Total row: each column's denominator, a plain count. -->
                    <tr class="subs-total">
                      <td class="has-text-grey">{{ $t('dashboard.subsTotal') }}</td>
                      <td v-for="col in subsColumns" :key="col.key" class="has-text-right"
                        :data-cy="`subscribers-total-${col.key}`">
                        {{ $utils.formatNumber(col.row.total || 0) }}
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
                      {{ $utils.formatNumber(counts.messages || 0) }}
                    </p>
                    <p class="is-size-6 has-text-grey">
                      {{ $t('dashboard.messagesSent') }}
                    </p>
                  </div>
                  <div class="column is-6">
                    <!-- Fork (dashboard counts, R1/R2; brand matrix, BRAND-MATRIX-SPEC D8) --
                    rate-first over Messages sent: Views, Clicks, H. Bounces, Complaints. -->
                    <ul class="no has-text-grey pct-lines">
                      <li v-for="l in engagementParts" :key="l.key" :data-cy="`engagement-${l.key}`">
                        <span class="pl-pct">{{ l.parts.pct }}</span>
                        <span class="pl-count">{{ l.parts.count }}</span>
                        <span class="pl-label">{{ $t(l.label) }}</span>
                      </li>
                      <!-- Fork (campaign conversions, CAMPAIGN-CONVERSIONS-SPEC D11) -- Tracked and
                           Matched over the pane's C, lifetime from each measure's floor; sales in
                           the wide rate column, orders beside the label. Labelled "Tracked Conv." /
                           "Matched Conv." (integrations CAMPAIGN-MATRIX-SPEC D11). -->
                      <li v-for="c in overviewConversions" :key="c.key" :title="c.tip"
                        :data-cy="`conversions-${c.key}`">
                        <span class="pl-pct">{{ c.cell.sales }}</span>
                        <span class="pl-count">{{ c.cell.orders }}</span>
                        <span class="pl-label">{{ $t(`analytics.conversions.${c.key}Conv`) }}</span>
                      </li>
                    </ul>
                  </div>
                </div>
              </article><!-- subscribers -->
            </div>

            <div class="tile is-parent relative">
              <b-loading v-if="isCountsLoading" active :is-full-page="false" />
              <article class="tile is-child notification" data-cy="lists-campaigns">
                <!-- DASHBOARDS-POLISH2-SPEC D3 -- the right container's split is 5/7 (not 6/6):
                     its data column moves left and gains the room its lines need; .dash-lc in
                     style.scss keeps every line on one row. -->
                <div class="columns is-mobile dash-lc" data-cy="lists">
                  <div class="column is-5">
                    <p class="title">
                      <b-icon icon="format-list-bulleted-square" />
                      {{ $utils.niceNumber(counts.lists.total) }}
                    </p>
                    <p class="is-size-6 has-text-grey">
                      {{ $tc('globals.terms.list', counts.lists.total) }}
                    </p>
                  </div>
                  <div class="column is-7">
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
                </div><!-- lists -->
                <hr />
                <div class="columns is-mobile dash-lc" data-cy="campaigns">
                  <div class="column is-5">
                    <p class="title">
                      <b-icon icon="rocket-launch-outline" />
                      {{ $utils.niceNumber(counts.campaigns.total) }}
                    </p>
                    <p class="is-size-6 has-text-grey">
                      {{ $tc('globals.terms.campaign', counts.campaigns.total) }}
                    </p>
                  </div>
                  <div class="column is-7">
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
                </div><!-- campaigns -->
              </article><!-- lists + campaigns -->
            </div>
          </div>
          <div class="tile is-parent relative">
            <b-loading v-if="isChartsLoading" active :is-full-page="false" />
            <!-- Fork (dashboard sends, integrations DASHBOARD-SENDS-SPEC D6, revision 2 R4, and the
                 2026-10-24 follow-up): SES sends per day (BrandHealth's ses document) and clicks
                 counted as people under individual tracking, both over the 60 Mountain days ending
                 yesterday -- ONE full-width chart, Sends on the left axis, Clicks on the right in a
                 second color (sendsClicksChart.mjs), one caption under it. -->
            <article class="tile is-child notification charts">
              <div class="columns">
                <div class="column is-12">
                  <h3 class="title is-size-6">
                    {{ $t('dashboard.sendsAndClicks') }}
                  </h3>
                  <chart type="line" v-if="sendsClicksChart" :data="sendsClicksChart" :options="sendsClicksOptions"
                    data-cy="sends-clicks-chart" />
                </div>
              </div>
              <p class="is-size-7 has-text-grey" data-cy="charts-note">{{ $t('dashboard.chartsNote') }}</p>
            </article>
          </div>

          <!-- Fork (campaign conversions, CAMPAIGN-CONVERSIONS-SPEC D11 follow-up): Clicks bars and
               the Matched conversions line by Mountain purchase day on the same window, $ by default
               (the $|# toggle switches to orders) and an optional light Tracked fill beneath the
               line (clicksConversionsChart.mjs). Nothing renders until a conversions doc exists. -->
          <div v-if="clicksConversionsChart.datasets" class="tile is-parent relative">
            <b-loading v-if="isChartsLoading" active :is-full-page="false" />
            <article class="tile is-child notification charts" data-cy="clicks-conversions">
              <div class="is-flex is-align-items-center is-justify-content-space-between mb-2">
                <h3 class="title is-size-6 mb-0">
                  {{ $t('dashboard.conversionsChart.title') }}
                </h3>
                <div class="is-flex is-align-items-center">
                  <b-field class="mb-0 mr-4">
                    <b-radio-button v-model="conv.unit" native-value="usd" size="is-small" type="is-primary"
                      data-cy="conversions-unit-usd" @input="onConvToggle">
                      $
                    </b-radio-button>
                    <b-radio-button v-model="conv.unit" native-value="orders" size="is-small" type="is-primary"
                      data-cy="conversions-unit-orders" @input="onConvToggle">
                      #
                    </b-radio-button>
                  </b-field>
                  <b-checkbox v-model="conv.showTracked" size="is-small" data-cy="conversions-show-tracked"
                    @input="onConvToggle">
                    {{ $t('dashboard.conversionsChart.showTracked') }}
                  </b-checkbox>
                </div>
              </div>
              <chart type="line" :data="clicksConversionsChart" :options="clicksConversionsOptions"
                data-cy="clicks-conversions-chart" />
              <p class="is-size-7 has-text-grey" data-cy="conversions-chart-note">{{ $t('dashboard.conversionsChart.note') }}</p>
            </article>
          </div>

          <!-- Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC R4): views and clicks per
               country over the client panel's window, scope and RAW row count (the Analytics
               Location table dedupes under individual tracking -- a recorded divergence). Rows
               shaped and sorted by countryRows.mjs, Unknown last; each number with its share of
               the column. -->
          <div class="tile is-parent relative">
            <b-loading v-if="countries.loading" active :is-full-page="false" />
            <article class="tile is-child notification share-grid" data-cy="countries">
              <div class="columns is-mobile">
                <div class="column">
                  <h3 class="title is-size-6">{{ $t('analytics.location') }}</h3>
                </div>
              </div>
              <b-table :data="countryTableRows" hoverable narrowed
                backend-sorting :default-sort="[countries.sort.field, countries.sort.order]" @sort="onCountrySort">
                <b-table-column v-slot="props" field="name" :label="$t('analytics.locationCountry')" sortable>
                  <span :class="{ 'has-text-grey': props.row.unknown }" :title="props.row.country">{{ props.row.name }}</span>
                </b-table-column>
                <b-table-column v-slot="props" field="views" :label="$t('campaigns.views')" numeric sortable>
                  {{ $utils.formatNumber(props.row.views) }}<span class="cl-share is-size-7 has-text-grey">{{ parenShare(countryShare(props.row.views, 'views')) }}</span>
                </b-table-column>
                <b-table-column v-slot="props" field="clicks" :label="$t('campaigns.clicks')" numeric sortable>
                  {{ $utils.formatNumber(props.row.clicks) }}<span class="cl-share is-size-7 has-text-grey">{{ parenShare(countryShare(props.row.clicks, 'clicks')) }}</span>
                </b-table-column>
                <template #empty v-if="!countries.loading">
                  <p class="has-text-grey">{{ $t('globals.messages.emptyState') }}</p>
                </template>
              </b-table>
              <!-- Stage 4 F1 -- the Dashboard's own hint: this panel counts raw rows, the Analytics table dedupes. -->
              <p class="is-size-7 has-text-grey mt-2">{{ $t('dashboard.clientsWindow') }} {{ $t('dashboard.locationHint') }}</p>
            </article>
          </div>

          <!-- Fork (client stats, integrations CLIENT-STATS-SPEC D6/D9): views and clicks (Combined
               dropped here by BRAND-USAGE-V2-SPEC D8) per email client over the charts' window. Its own brand picker is gone (integrations
               GLOBAL-BRAND-SPEC D5): the global brand selector is the one filter, sent with the
               counts and charts. -->
          <div class="tile is-parent relative">
            <b-loading v-if="clients.loading" active :is-full-page="false" />
            <article class="tile is-child notification share-grid" data-cy="clients">
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
                <b-table-column v-slot="props" field="views" :label="$t('dashboard.clientsViewEvents')" numeric sortable>
                  {{ $utils.formatNumber(props.row.views) }}<span class="cl-share is-size-7 has-text-grey">{{ parenShare(clientShare(props.row.views, 'views')) }}</span>
                </b-table-column>
                <b-table-column v-slot="props" field="clicks" :label="$t('dashboard.clientsClickEvents')" numeric sortable>
                  {{ $utils.formatNumber(props.row.clicks) }}<span class="cl-share is-size-7 has-text-grey">{{ parenShare(clientShare(props.row.clicks, 'clicks')) }}</span>
                </b-table-column>
                <!-- BRAND-USAGE-V2-SPEC D8: no Combined column on the Dashboard (Campaign Analytics
                     keeps its own). -->
                <template #empty v-if="!clients.loading">
                  <p class="has-text-grey">{{ $t('globals.messages.emptyState') }}</p>
                </template>
              </b-table>
              <!-- Fork (client vocabulary, integrations CLIENT-VOCABULARY-SPEC D8): the three
                   trust-tier subtotals plus the untiered remainder, as footer rows beneath the
                   table, over Views and Clicks (this panel has no Combined column at all).
                   Rendered separately and NEVER summed into one figure. -->
              <table v-if="clients.rows.length" class="table is-narrow is-fullwidth client-tiers">
                <thead>
                  <tr>
                    <th>{{ $t('analytics.clientsTier') }}</th>
                    <th class="has-text-right">{{ $t('dashboard.clientsViewEvents') }}<span class="cl-share is-size-7" /></th>
                    <th class="has-text-right">{{ $t('dashboard.clientsClickEvents') }}<span class="cl-share is-size-7" /></th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="t in clientTierRows" :key="t.key">
                    <td>{{ t.name }}</td>
                    <td class="has-text-right">{{ $utils.formatNumber(t.views) }}<span class="cl-share is-size-7" /></td>
                    <td class="has-text-right">{{ $utils.formatNumber(t.clicks) }}<span class="cl-share is-size-7" /></td>
                  </tr>
                </tbody>
              </table>
              <p class="is-size-7 has-text-grey mt-2">{{ $t('dashboard.clientsWindow') }} {{ $t('analytics.clientsHint') }}</p>
              <p v-if="clients.rows.length" class="is-size-7 has-text-grey mt-2">{{ $t('analytics.clientsTierHint') }}</p>
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
import Chart from '../components/Chart.vue';
import { clicksAxisMax, sendsClicksOptions, shapeSendsClicks } from '../sendsClicksChart.mjs'; // eslint-disable-line import/extensions
import {
  clicksConversionsOptions, conversionsAxisMax, loadConvState, saveConvState, shapeClicksConversions,
} from '../clicksConversionsChart.mjs'; // eslint-disable-line import/extensions
import DashboardSes from './DashboardSes.vue';
// Fork (campaign matrix, integrations CAMPAIGN-MATRIX-SPEC D2) -- the Brand Matrix and Brand Usage
// panes moved to the Brands page (components/BrandMatrix.vue, BrandUsage.vue); the Overview keeps
// what it shares with them (the pane rule, the conversion fields and cells, the session store).
import { CONVERSION_FIELDS, initialPane, sessionStore } from '../brandMatrix.mjs'; // eslint-disable-line import/extensions
import { conversionTooltip, scopedCell } from '../conversionCells.mjs'; // eslint-disable-line import/extensions
import {
  DEFAULT_SORT as CLIENT_DEFAULT_SORT, TIER_ROWS, clientTierSubtotals, clientTotals,
  shapeClientRows, sortClientRows,
} from '../clientRows.mjs'; // eslint-disable-line import/extensions
import { rosterFor } from '../clientRoster.mjs'; // eslint-disable-line import/extensions
import { rateCell } from '../campaignRates.mjs'; // eslint-disable-line import/extensions
import {
  DEFAULT_SORT as COUNTRY_DEFAULT_SORT, shapeCountryRows, sortCountryRows,
} from '../countryRows.mjs'; // eslint-disable-line import/extensions
import {
  engagementLines, loadOverviewState, pctCount, saveOverviewState, segmentCell, subsCellText, subscriberColumns,
  subscriberRows,
} from '../dashboardSubs.mjs'; // eslint-disable-line import/extensions
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
      // Fork (brand matrix, BRAND-MATRIX-SPEC D1) -- the sticky dashboard.pane pref (the legacy
      // dashboard.sesPane boolean read only while it is unset); Overview without brands:get.
      // CAMPAIGN-MATRIX-SPEC D2: Overview | SES -- a stored matrix or usage reads as overview.
      pane: initialPane(canSes, this.$utils.getPref('dashboard.pane'), this.$utils.getPref('dashboard.sesPane')),
      // Fork (BRAND-USAGE-V2-SPEC D7) -- the expanded Subscribers table's %|# mode, per browser
      // session (sessionStorage key dashboard.overview).
      overview: loadOverviewState(sessionStore(typeof window !== 'undefined' ? window : null)),
      // Fork (dashboard counts, DASHBOARD-COUNTS-SPEC D3) -- the Subscribers container's
      // language table, sticky per browser (the dashboard.sesPane pattern).
      subsExpanded: this.$utils.getPref('dashboard.subsExpanded') === true,
      isChartsLoading: true,
      isCountsLoading: true,
      sendsClicksChart: null,
      clicksMax: undefined,
      // Fork (campaign conversions, the Clicks + conversions chart) -- the fetched daily series and
      // the chart's $|# unit and Show Tracked overlay, per browser session
      // (sessionStorage key dashboard.conversionsChart; a fresh session is $ with Tracked off).
      chartClicks: [],
      chartConversions: [],
      conv: loadConvState(sessionStore(typeof window !== 'undefined' ? window : null)),
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
      // Fork (dashboard counts, DASHBOARD-COUNTS-SPEC R4) -- the Location panel.
      countries: {
        rows: [],
        loading: false,
        sort: { ...COUNTRY_DEFAULT_SORT },
      },
    };
  },

  methods: {
    // Fork (brand matrix, D1) -- the string pref (dashboard.sesPane is no longer written).
    onPaneChange() {
      this.$utils.setPref('dashboard.pane', this.pane);
    },

    // V2 D7 -- the expanded Subscribers table's %|# mode, per session.
    onOverviewMode() {
      saveOverviewState(sessionStore(window), { mode: this.overview.mode });
    },

    scopeParams() {
      return this.scopeListIds ? { list_id: this.scopeListIds } : {};
    },

    // The Tracked/Matched tooltip (definition, floor, as-of, stale note) -- conversionCells.mjs.
    conversionTip(measure, meta) {
      return conversionTooltip(measure, meta, (k, p) => this.$t(k, p));
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

    // DELIVERABILITY-CONSISTENCY-SPEC D2 -- the blocklisted segment's hover names its scope (the
    // Brand Matrix and Lists grid count the same status over other scopes).
    subsSegTitle(key) {
      return key === 'blocked' ? this.$t('dashboard.blocklistedHelp') : null;
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

    // A rate-first line as its two columns, for the aligned lists (2026-10-24): {pct, count} --
    // pct '' on a plain count (null denominator, or 0). No parentheses (BRAND-MATRIX-SPEC D9).
    pctParts(count, denominator, fmt) {
      if (denominator === null) {
        return { pct: '', count: fmt(count || 0) };
      }
      const c = pctCount(count, denominator, fmt);
      return { pct: c.pct || '', count: c.count };
    },

    // The collapsed segment line: the all column. Comma-separated, never k/m (D9).
    subsLineParts(seg) {
      const c = segmentCell({ key: 'all', row: this.subsAll }, seg);
      return this.pctParts(c.count, c.denominator, (n) => this.$utils.formatNumber(n));
    },

    // An expanded-table cell, over THAT column's total, in the session's %|# mode (V2 D7:
    // dashboardSubs.mjs::subsCellText -- the mode governs rate-bearing cells only).
    subsCellText(col, seg) {
      return subsCellText(col, seg, this.overview.mode, (n) => this.$utils.formatNumber(n));
    },

    // Fork (global brand, D4/D5) -- the reads together (counts, charts, clients and -- DASHBOARD-
    // COUNTS-SPEC R4 -- countries), each with the global brand's effective list set (no list_id
    // under All brands: today's Dashboard exactly).
    fetchData() {
      this.isCountsLoading = true;
      this.isChartsLoading = true;
      const params = this.scopeParams();

      this.$api.getDashboardCounts(params).then((data) => {
        this.counts = data;
        this.isCountsLoading = false;
      });

      this.$api.getDashboardCharts(params).then((data) => {
        this.isChartsLoading = false;
        this.sendsClicksChart = this.makeSendsClicks(data);
        this.clicksMax = clicksAxisMax(data.clicks);
        this.chartClicks = data.clicks || [];
        this.chartConversions = data.conversions || [];
      });

      this.fetchClients(params);
      this.fetchCountries(params);
    },

    // Fork (dashboard counts, R4) -- the Location panel over the same scope as the counts.
    fetchCountries(params) {
      this.countries.loading = true;
      this.$api.getDashboardCountries(params).then((data) => {
        this.countries.rows = shapeCountryRows(data.countries, {
          locale: (this.$i18n && this.$i18n.locale) || 'en',
          unknownLabel: this.$t('analytics.locationUnknown'),
        });
      }).finally(() => {
        this.countries.loading = false;
      });
    },

    onCountrySort(field, order) {
      this.countries.sort = { field, order };
    },

    // A country's share of the column's own total (Unknown included), as the Analytics table.
    countryShare(n, field) {
      return rateCell(n, this.countryTotals[field], 1).pct;
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

    // A share for its fixed-width slot (Location and Email clients), parenthesized, or '' -- the
    // slot stays either way, so counts align.
    parenShare(pct) {
      return pct ? `(${pct})` : '';
    },

    clientRosterTip(token) {
      const ids = rosterFor(token);
      return ids.length ? this.$t('analytics.clientsRoster', { ids: ids.join(', ') }) : '';
    },

    // The Clicks + conversions chart's toggles, saved for the session.
    onConvToggle() {
      saveConvState(sessionStore(window), this.conv);
    },

    // The one Sends + Clicks chart (sendsClicksChart.mjs aligns the two by date).
    makeSendsClicks(data) {
      return shapeSendsClicks(data.sends, data.clicks, {
        labelOf: (d) => dayjs(d).format('DD MMM'),
        sendsLabel: this.$t('dashboard.sends'),
        clicksLabel: this.$t('dashboard.clicks'),
      });
    },
  },

  computed: {
    ...mapState(['settings', 'serverConfig']),

    // The Sends + Clicks chart's axes, legend and tooltip (sendsClicksChart.mjs); the Clicks axis
    // max follows the fetched data (the chart re-reads options on change).
    sendsClicksOptions() {
      return sendsClicksOptions({
        sendsLabel: this.$t('dashboard.sends'), clicksLabel: this.$t('dashboard.clicks'), clicksMax: this.clicksMax,
      });
    },

    // Fork (campaign conversions) -- the Clicks + conversions chart and its options; both re-shape
    // from the fetched series when a toggle changes (no refetch). {} (nothing drawn) with no doc.
    clicksConversionsChart() {
      return shapeClicksConversions(this.chartClicks, this.chartConversions, {
        labelOf: (d) => dayjs(d).format('DD MMM'),
        clicksLabel: this.$t('dashboard.clicks'),
        matchedLabel: this.$t('analytics.conversions.matched'),
        trackedLabel: this.$t('analytics.conversions.tracked'),
        unit: this.conv.unit,
        showTracked: this.conv.showTracked,
      });
    },

    clicksConversionsOptions() {
      return clicksConversionsOptions({
        clicksLabel: this.$t('dashboard.clicks'),
        axisLabel: this.$t(this.conv.unit === 'usd' ? 'dashboard.conversionsChart.sales' : 'dashboard.conversionsChart.orders'),
        unit: this.conv.unit,
        convMax: conversionsAxisMax(this.chartConversions, this.conv),
        dates: this.clicksConversionsChart.dates || [],
        byDate: new Map(this.chartConversions.map((d) => [d.date, d])),
        matchedLabel: this.$t('analytics.conversions.matched'),
        trackedLabel: this.$t('analytics.conversions.tracked'),
        ordersLabel: this.$t('dashboard.conversionsChart.ordersUnit'),
      });
    },

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

    // The collapsed segment lines with their aligned-column parts.
    subsLineRows() {
      return this.subsRows.map((r) => ({ ...r, parts: this.subsLineParts(r) }));
    },

    subsColumns() {
      return subscriberColumns(this.subsGrid, !!(this.serverConfig && this.serverConfig.lang_enabled));
    },

    // Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D11) -- the Overview's two
    // lines (lifetime over the pane's C, '—' with no doc).
    overviewConversions() {
      const conv = (this.counts && this.counts.conversions) || {};
      const fmt = (n) => this.$utils.formatNumber(n);
      return CONVERSION_FIELDS.map((f) => ({
        key: f, cell: scopedCell(conv[f], conv.meta, fmt), tip: this.conversionTip(f, conv.meta),
      }));
    },

    // Fork (dashboard counts, R1/R2) -- Views, Clicks, Bounces, Complaints over Messages sent.
    // The engagement lines with their aligned-column parts.
    engagementParts() {
      return this.engagementLines.map((l) => ({
        ...l, parts: this.pctParts(l.count, l.denominator, (n) => this.$utils.formatNumber(n)),
      }));
    },

    engagementLines() {
      return engagementLines(this.counts.engagement, this.counts.messages);
    },

    // Fork (dashboard counts, R4) -- the Location panel's column totals and sorted rows.
    countryTotals() {
      return this.countries.rows.reduce((t, r) => ({ views: t.views + (r.views || 0), clicks: t.clicks + (r.clicks || 0) }), { views: 0, clicks: 0 });
    },

    countryTableRows() {
      const { field, order } = this.countries.sort;
      return sortCountryRows(this.countries.rows, field, order, (this.$i18n && this.$i18n.locale) || 'en');
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

    // Fork (client vocabulary, D8) -- the tier footer rows. The tier map and this row order are
    // clientRows.mjs's (the tier's single home); this only labels and formats them.
    clientTierRows() {
      const sub = clientTierSubtotals(this.clients.rows);
      return TIER_ROWS.map(({ key, i18n }) => ({
        key, name: this.$t(i18n), views: sub[key].views, clicks: sub[key].clicks,
      }));
    },
  },

  watch: {
    // Fork (global brand, D4) -- a selection change re-issues the reads together (fetchData).
    brandKey(now) {
      if (this.brandChanged(now)) {
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
