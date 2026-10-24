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
          <!-- Fork (brand matrix, integrations BRAND-MATRIX-SPEC D1) -- the third pane, between
               Overview and SES, behind the same brands:get gate (its rows carry health data). -->
          <b-radio-button v-model="pane" native-value="matrix" type="is-primary"
            data-cy="pane-matrix" @input="onPaneChange">
            {{ $t('dashboard.panes.matrix') }}
          </b-radio-button>
          <b-radio-button v-model="pane" native-value="ses" type="is-primary"
            data-cy="pane-ses" @input="onPaneChange">
            {{ $t('dashboard.panes.ses') }}
          </b-radio-button>
        </b-field>
      </div>
    </header>

    <dashboard-ses v-if="pane === 'ses'" />

    <!-- Fork (brand matrix, integrations BRAND-MATRIX-SPEC D2-D7). One row per brand of the global
         selection (narrowed by the navbar launch filter), the totals row pinned last; every rate,
         sort and missing-value rule is brandMatrix.mjs. -->
    <section v-else-if="pane === 'matrix'" class="brand-matrix relative" data-cy="brand-matrix">
      <b-loading v-if="matrix.loading" active :is-full-page="false" />
      <div class="is-flex is-align-items-center mb-3">
        <span class="has-text-grey mr-2">{{ $t('dashboard.matrix.engagement') }}</span>
        <b-field class="mb-0">
          <b-radio-button v-model="matrix.mode" native-value="pct" size="is-small" type="is-primary"
            data-cy="matrix-mode-pct" @input="onMatrixMode">
            {{ $t('dashboard.matrix.pct') }}
          </b-radio-button>
          <b-radio-button v-model="matrix.mode" native-value="num" size="is-small" type="is-primary"
            data-cy="matrix-mode-num" @input="onMatrixMode">
            {{ $t('dashboard.matrix.num') }}
          </b-radio-button>
        </b-field>
      </div>
      <b-table ref="matrixTable" :data="matrixTableRows" hoverable narrowed :row-class="matrixRowClass"
        backend-sorting :default-sort="[matrix.sort.field, matrix.sort.order]" @sort="onMatrixSort">
        <b-table-column v-slot="props" field="brand" :label="$t('dashboard.matrix.brand')" sortable>
          <span :data-cy="`matrix-brand-${props.row.brand || 'totals'}`">{{ props.row.label }}</span>
        </b-table-column>
        <b-table-column v-slot="props" field="sent" :label="$t('dashboard.matrix.sent')" numeric sortable
          header-class="bm-section" cell-class="bm-section">
          <span :title="$t('dashboard.matrix.sentHelp')">{{ matrixCell(props.row, 'sent') }}</span>
        </b-table-column>
        <b-table-column v-for="(c, i) in matrixSubColumns" :key="c.field" v-slot="props" :field="c.field"
          :label="$t(c.label)" numeric sortable :header-class="i === 0 ? 'bm-section' : ''"
          :cell-class="i === 0 ? 'bm-section' : ''">
          {{ matrixCell(props.row, c.field) }}
        </b-table-column>
        <b-table-column v-for="(c, i) in matrixEngColumns" :key="c.field" v-slot="props" :field="c.field"
          :label="$t(c.label)" numeric sortable :header-class="i === 0 ? 'bm-section' : ''"
          :cell-class="i === 0 ? 'bm-section' : ''">
          <span :title="c.help ? $t(c.help) : null">{{ matrixCell(props.row, c.field) }}</span>
        </b-table-column>
        <b-table-column v-slot="props" field="spam_rate" :label="$t('dashboard.matrix.spam')" numeric sortable
          header-class="bm-section" cell-class="bm-section">
          <span :title="$t('dashboard.matrix.spamHelp')">{{ props.row.totals ? '' : matrixCell(props.row, 'spam_rate') }}</span>
        </b-table-column>
        <b-table-column v-slot="props" field="status" :label="$t('dashboard.matrix.health')">
          <health-chip v-if="props.row.status" :status="props.row.status"
            :to="{ name: 'brand', params: { brand: props.row.brand } }" />
        </b-table-column>
        <template #empty v-if="!matrix.loading">
          <p class="has-text-grey">{{ $t('globals.messages.emptyState') }}</p>
        </template>
      </b-table>
      <p class="is-size-7 has-text-grey mt-2" data-cy="matrix-note">{{ $t('dashboard.matrix.note') }}</p>
    </section>

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
                        <li v-for="r in subsLineRows" :key="r.key" :data-cy="`subscribers-${r.key}`">
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
                      <th />
                      <th v-for="col in subsColumns" :key="col.key" class="has-text-right"
                        :data-cy="`subscribers-col-${col.key}`" :title="subsColTitle(col.key)">
                        {{ subsColLabel(col.key) }}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <!-- Fork (dashboard counts, R1) -- each rate cell over THAT column's total. -->
                    <tr v-for="r in subsRows" :key="r.key">
                      <td class="has-text-grey">{{ $t(r.label) }}</td>
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
                    </ul>
                  </div>
                </div>
              </article><!-- subscribers -->
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

          <!-- Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC R4): views and clicks per
               country over the client panel's window, scope and RAW row count (the Analytics
               Location table dedupes under individual tracking -- a recorded divergence). Rows
               shaped and sorted by countryRows.mjs, Unknown last; each number with its share of
               the column. -->
          <div class="tile is-parent relative">
            <b-loading v-if="countries.loading" active :is-full-page="false" />
            <article class="tile is-child notification" data-cy="countries">
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
                  {{ $utils.formatNumber(props.row.views) }}
                  <span v-if="countryShare(props.row.views, 'views')" class="is-size-7 has-text-grey">({{ countryShare(props.row.views, 'views') }})</span>
                </b-table-column>
                <b-table-column v-slot="props" field="clicks" :label="$t('campaigns.clicks')" numeric sortable>
                  {{ $utils.formatNumber(props.row.clicks) }}
                  <span v-if="countryShare(props.row.clicks, 'clicks')" class="is-size-7 has-text-grey">({{ countryShare(props.row.clicks, 'clicks') }})</span>
                </b-table-column>
                <template #empty v-if="!countries.loading">
                  <p class="has-text-grey">{{ $t('globals.messages.emptyState') }}</p>
                </template>
              </b-table>
              <!-- Stage 4 F1 -- the Dashboard's own hint: this panel counts raw rows, the Analytics table dedupes. -->
              <p class="is-size-7 has-text-grey mt-2">{{ $t('dashboard.clientsWindow') }} {{ $t('dashboard.locationHint') }}</p>
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
import { mapGetters, mapState } from 'vuex';
import Chart from '../components/Chart.vue';
import { clicksAxisMax, sendsClicksOptions, shapeSendsClicks } from '../sendsClicksChart.mjs'; // eslint-disable-line import/extensions
import DashboardSes from './DashboardSes.vue';
import HealthChip from '../components/HealthChip.vue';
import {
  ENGAGEMENT_FIELDS, cellText, initialPane, loadState, resolveSort, saveState, sessionStore, shapeRows, tableRows,
} from '../brandMatrix.mjs'; // eslint-disable-line import/extensions
import {
  DEFAULT_SORT as CLIENT_DEFAULT_SORT, clientTotals, shapeClientRows, sortClientRows,
} from '../clientRows.mjs'; // eslint-disable-line import/extensions
import { rosterFor } from '../clientRoster.mjs'; // eslint-disable-line import/extensions
import { rateCell } from '../campaignRates.mjs'; // eslint-disable-line import/extensions
import {
  DEFAULT_SORT as COUNTRY_DEFAULT_SORT, shapeCountryRows, sortCountryRows,
} from '../countryRows.mjs'; // eslint-disable-line import/extensions
import {
  engagementLines, pctCount, segmentCell, subscriberColumns, subscriberRows,
} from '../dashboardSubs.mjs'; // eslint-disable-line import/extensions
import { isSendPlus, sendLangCode } from '../langs';
import brandScopeMixin from '../brandScopeMixin';

export default Vue.extend({
  components: {
    Chart,
    DashboardSes,
    HealthChip,
  },

  // Fork (global brand, integrations GLOBAL-BRAND-SPEC D4/D5).
  mixins: [brandScopeMixin],

  data() {
    const canSes = this.$can('brands:get');
    return {
      canSes,
      // Fork (brand matrix, BRAND-MATRIX-SPEC D1) -- the sticky dashboard.pane pref (the legacy
      // dashboard.sesPane boolean read only while it is unset); Overview without brands:get.
      pane: initialPane(canSes, this.$utils.getPref('dashboard.pane'), this.$utils.getPref('dashboard.sesPane')),
      // Fork (brand matrix, D6/D7) -- the matrix's API rows, and its sort and %|# mode, per
      // browser session (sessionStorage, never the localStorage pref helper).
      matrix: {
        apiRows: [],
        loading: false,
        ...loadState(sessionStore(typeof window !== 'undefined' ? window : null)),
      },
      // Fork (dashboard counts, DASHBOARD-COUNTS-SPEC D3) -- the Subscribers container's
      // language table, sticky per browser (the dashboard.sesPane pattern).
      subsExpanded: this.$utils.getPref('dashboard.subsExpanded') === true,
      isChartsLoading: true,
      isCountsLoading: true,
      sendsClicksChart: null,
      clicksMax: undefined,
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
    // Fork (brand matrix, D1/D7) -- the string pref (dashboard.sesPane is no longer written); the
    // matrix is fetched on every activation, so a scope change made while it was hidden is read.
    onPaneChange() {
      this.$utils.setPref('dashboard.pane', this.pane);
      if (this.pane === 'matrix') {
        this.fetchMatrix(this.scopeParams());
      }
    },

    scopeParams() {
      return this.scopeListIds ? { list_id: this.scopeListIds } : {};
    },

    // Fork (brand matrix, D7) -- the matrix over the same scope as the counts.
    fetchMatrix(params) {
      this.matrix.loading = true;
      this.$api.getDashboardBrandMatrix(params).then((data) => {
        this.matrix.apiRows = (data && Array.isArray(data.rows)) ? data.rows : [];
      }).finally(() => {
        this.matrix.loading = false;
      });
    },

    saveMatrixState() {
      saveState(sessionStore(window), { sort: this.matrix.sort, mode: this.matrix.mode });
    },

    onMatrixMode() {
      this.saveMatrixState();
    },

    // D6 -- the Brands.vue onSort pattern: a click on a NEW column takes its default direction
    // (brandMatrix.mjs::defaultOrder); Buefy's own first click is ascending, so its arrow is flipped
    // when the default is desc. A repeat click is Buefy's own toggle.
    onMatrixSort(field, direction) {
      const next = resolveSort(this.matrix.sort, field, direction);
      const { matrixTable } = this.$refs;
      if (next.order !== direction && matrixTable && typeof matrixTable.isAsc === 'boolean') {
        matrixTable.isAsc = next.order === 'asc';
      }
      this.matrix.sort = next;
      this.saveMatrixState();
    },

    matrixCell(row, field) {
      return cellText(row, field, this.matrix.mode, (n) => this.$utils.formatNumber(n));
    },

    matrixRowClass(row) {
      return row.totals ? 'bm-totals' : '';
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

    // Fork (dashboard counts, R1) -- a rate-first line: "39.5% (751)", or the count alone when
    // the denominator is 0. A null denominator (Active, Pending) is a plain count. One number
    // format per list (Stage 4 F3): fmt is niceNumber on the collapsed lines, as the other
    // containers' lines, and formatNumber in the expanded table.
    pctText(count, denominator, fmt) {
      if (denominator === null) {
        return fmt(count || 0);
      }
      const c = pctCount(count, denominator, fmt);
      return c.pct ? `${c.pct} (${c.count})` : c.count;
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

    // An expanded-table cell, over THAT column's total.
    subsCellText(col, seg) {
      const c = segmentCell(col, seg);
      return this.pctText(c.count, c.denominator, (n) => this.$utils.formatNumber(n));
    },

    // Fork (global brand, D4/D5) -- the reads together (counts, charts, clients and -- DASHBOARD-
    // COUNTS-SPEC R4 -- countries), each with the global brand's effective list set (no list_id
    // under All brands: today's Dashboard exactly).
    fetchData() {
      this.isCountsLoading = true;
      this.isChartsLoading = true;
      const params = this.scopeParams();

      // Fork (brand matrix, D7) -- refetched with the others only while its pane is active.
      if (this.pane === 'matrix') {
        this.fetchMatrix(params);
      }

      this.$api.getDashboardCounts(params).then((data) => {
        this.counts = data;
        this.isCountsLoading = false;
      });

      this.$api.getDashboardCharts(params).then((data) => {
        this.isChartsLoading = false;
        this.sendsClicksChart = this.makeSendsClicks(data);
        this.clicksMax = clicksAxisMax(data.clicks);
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

    clientRosterTip(token) {
      const ids = rosterFor(token);
      return ids.length ? this.$t('analytics.clientsRoster', { ids: ids.join(', ') }) : '';
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

    // Fork (dashboard counts, R1/R2) -- Views, Clicks, Bounces, Complaints over Messages sent.
    // The engagement lines with their aligned-column parts.
    engagementParts() {
      return this.engagementLines.map((l) => ({
        ...l, parts: this.pctParts(l.count, l.denominator, (n) => this.$utils.formatNumber(n)),
      }));
    },

    // Fork (brand matrix, D2/D4/D6) -- the launch value and the brands rows (display names); the
    // module narrows, shapes, sorts and appends the totals row.
    ...mapGetters(['launch']),

    matrixTableRows() {
      const rows = shapeRows(this.matrix.apiRows, { brands: this.brandRows, launch: this.launch });
      return tableRows(rows, this.matrix.sort, this.matrix.mode, {
        locale: (this.$i18n && this.$i18n.locale) || 'en',
        totalsLabel: this.$t('dashboard.matrix.total'),
      });
    },

    matrixSubColumns() {
      return [
        { field: 'total', label: 'dashboard.matrix.subscr' },
        { field: 'active', label: 'dashboard.matrix.active' },
        { field: 'held', label: 'dashboard.matrix.held' },
        { field: 'unsubscribed', label: 'dashboard.matrix.unsub' },
        { field: 'blocked', label: 'dashboard.matrix.blocked' },
      ];
    },

    // D4 -- Blocks is a subset of H. Bounces; its header says so.
    matrixEngColumns() {
      const labels = {
        clicks: 'dashboard.matrix.clicks',
        hard_bounces: 'dashboard.matrix.hardBounces',
        blocks: 'dashboard.matrix.blocks',
        complaints: 'dashboard.matrix.complaints',
      };
      return ENGAGEMENT_FIELDS.map((f) => ({
        field: f, label: labels[f], help: f === 'blocks' ? 'dashboard.matrix.blocksHelp' : null,
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
  },

  watch: {
    // Fork (global brand, D4) -- a selection change re-issues the reads together (fetchData).
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
