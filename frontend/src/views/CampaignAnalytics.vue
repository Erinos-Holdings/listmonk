<template>
  <section class="analytics content relative">
    <h1 class="title is-4">
      {{ $t('analytics.title') }}
    </h1>
    <div v-if="serverConfig.privacy.disable_tracking || !serverConfig.privacy.individual_tracking"
      class="notification is-info">
      <template v-if="serverConfig.privacy.disable_tracking">
        {{ $t('analytics.trackingDisabled') }}
      </template>
      <template v-else-if="!serverConfig.privacy.individual_tracking">
        {{ $t('analytics.nonIndividualTracking') }}
      </template>
    </div>
    <hr />

    <!-- Fork (global brand, integrations GLOBAL-BRAND-SPEC D9/D11, S4) -- a prefilled campaign
         outside the selected brand stays selected, with the notice (one line per such brand). -->
    <brand-context-notice v-for="b in noticeBrands" :key="`notice-${b}`" :record-brand="b"
      :kind="$tc('globals.terms.campaign', 1).toLowerCase()" />

    <form @submit.prevent="onSubmit">
      <div class="columns">
        <div class="column is-6">
          <b-field :label="$t('globals.terms.campaigns')" label-position="on-border">
            <b-taginput v-model="form.campaigns" :data="queriedCampaigns" name="campaigns" ellipsis icon="tag-outline"
              :placeholder="$t('globals.terms.campaigns')" autocomplete :allow-new="false" :open-on-focus="true"
              :before-adding="isCampaignSelected" @typing="queryCampaigns" @focus="queryCampaigns" field="name"
              :loading="isSearchLoading" @add="applyDefaultDates" @remove="applyDefaultDates" />
          </b-field>
        </div>

        <div class="column is-5">
          <div class="columns">
            <div class="column is-6">
              <b-field data-cy="from" :label="$t('analytics.fromDate')" label-position="on-border">
                <b-datetimepicker v-model="form.from" icon="calendar-clock" :timepicker="{ hourFormat: '24' }"
                  :datetime-formatter="formatDateTime" @input="onFromDateChange" />
              </b-field>
            </div>
            <div class="column is-6">
              <b-field data-cy="to" :label="$t('analytics.toDate')" label-position="on-border">
                <b-datetimepicker v-model="form.to" icon="calendar-clock" :timepicker="{ hourFormat: '24' }"
                  :datetime-formatter="formatDateTime" @input="onToDateChange" />
              </b-field>
            </div>
          </div><!-- columns -->
          <!-- Fork (brand analytics, BRAND-ANALYTICS-SPEC D11) -->
          <p class="is-size-7 has-text-grey date-hint" data-cy="date-hint">{{ $t('analytics.dateRangeHint') }}</p>
          <!-- Fork (campaign rates, CAMPAIGN-RATES-SPEC follow-up) -->
          <p class="is-size-7 has-text-grey date-hint" data-cy="rate-hint">{{ $t('analytics.rateHint') }}</p>
        </div><!-- columns -->

        <div class="column is-1">
          <b-button native-type="submit" type="is-primary" icon-left="magnify" :disabled="form.campaigns.length === 0"
            data-cy="btn-search" />
        </div>
      </div><!-- columns -->
    </form>

    <section class="charts mt-5">
      <div class="chart" v-for="(v, k) in charts" :key="k">
        <div class="columns">
          <div class="column is-9">
            <b-loading v-if="v.loading" :active="v.loading" :is-full-page="false" />
            <!-- Fork (campaign rates) -- the total as a rate over the selection's Sent, count in grey,
                 the campaigns list's shape; the count alone when nothing was sent. -->
            <!-- Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D7) -- the combined
                 Bounces chart and its split are replaced by the Hard Bounces, Soft Bounces and
                 Complaints charts; each header is its own series summed over the window, over Sent. -->
            <h4 v-if="v.type !== 'bar'" :data-cy="`${k}-total`">
              {{ v.name }}
              <template v-if="!v.loading && totalCells[k].pct">{{ totalCells[k].pct }}</template>
              <span class="has-text-grey-light">({{ totalCells[k].count }})</span>
            </h4>
            <!-- Fork (shop-link header total, 2026-10-19) -- Links too: its clicks over Sent, count in grey. -->
            <h4 v-else-if="k !== 'shopLinks'">
              {{ v.name }}
              <template v-if="!v.loading && totalCells[k].pct">{{ totalCells[k].pct }}</template>
              <span class="has-text-grey-light">({{ totalCells[k].count }})</span>
            </h4>
            <!-- Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D6) -- the Counts | Rate toggle
                 in the chart's header row (the .segmented LaunchFilter shape). -->
            <div v-else class="shop-links-head" data-cy="shop-links-head">
              <!-- Fork (shop-link header total, 2026-10-19) -- the Clicks header's shape: the
                   shop-link clicks as a rate over Sent, the count in grey. -->
              <h4>
                {{ v.name }}
                <template v-if="!v.loading && shopLinksHeaderCell.pct">{{ shopLinksHeaderCell.pct }}</template>
                <span class="has-text-grey-light">({{ shopLinksHeaderCell.count }})</span>
              </h4>
              <div class="shop-links-controls">
                <!-- Fork (hide No link, 2026-10-19) -- drops the No link bar from either view. -->
                <b-checkbox v-model="shopLinksHideNoneModel" size="is-small" data-cy="shop-links-hide-none">
                  {{ $t('analytics.shopLinksHideNone') }}
                </b-checkbox>
                <b-field class="segmented" role="group" :aria-label="$t('analytics.shopLinksView')">
                  <b-radio-button v-model="shopLinksViewModel" native-value="counts" size="is-small" type="is-primary"
                    data-cy="shop-links-counts">
                    {{ $t('analytics.shopLinksCounts') }}
                  </b-radio-button>
                  <b-radio-button v-model="shopLinksViewModel" native-value="rate" size="is-small" type="is-primary"
                    data-cy="shop-links-rate">
                    {{ $t('analytics.shopLinksRate') }}
                  </b-radio-button>
                </b-field>
              </div>
            </div>
            <!-- Fork (shop-link header total, 2026-10-19) -- the Clicks and Links hints: unique clickers
                 versus per-link clickers. -->
            <p v-if="k === 'clicks'" class="is-size-7 has-text-grey" data-cy="clicks-hint">{{ $t('analytics.clicksHint') }}</p>
            <!-- Fork (bounce types, integrations BOUNCE-TYPES-SPEC D4) -- the Bounces hint. -->
            <p v-if="k === 'hardBounces'" class="is-size-7 has-text-grey" data-cy="bounces-hint">{{ $t('analytics.bouncesHint') }}</p>
            <p v-if="k === 'links'" class="is-size-7 has-text-grey" data-cy="links-hint">{{ $t('analytics.linksHint') }}</p>
            <!-- Fork (shop-link attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC D5) -- the hint under the title. -->
            <p v-if="k === 'shopLinks'" class="is-size-7 has-text-grey" data-cy="shop-links-hint">{{ $t('analytics.shopLinksHint') }}</p>
            <!-- Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D6/D7) -- the rate view's notes. -->
            <template v-if="k === 'shopLinks' && !v.loading && shopLinksView === 'rate'">
              <p v-if="shopLinksNoSnapshot" class="is-size-7 has-text-grey mt-2" data-cy="shop-links-no-snapshot">
                {{ $t('analytics.shopLinksNoSnapshot') }}
              </p>
              <p v-else-if="shopLinksPartial" class="is-size-7 has-text-grey" data-cy="shop-links-partial">
                {{ $t('analytics.shopLinksPartial') }}
              </p>
            </template>
            <!-- Fork (campaign send series, D7) -- the line charts carry their send-overlay options
                 (sends on the left axis, the metric on the right); the bar charts none. -->
            <chart :type="v.type" v-if="!v.loading && !(k === 'shopLinks' && shopLinksView === 'rate' && shopLinksNoSnapshot)"
              :data="v.data" :options="v.options || null" :on-click="v.onClick" />
          </div>
          <!-- Fork (campaign send series, D7) -- Views and Clicks keep their ring; the three bounce-type
               charts have none. -->
          <div class="column is-2 donut-container">
            <chart type="donut" v-if="!v.loading && !v.noDonut" :data="v.donutData" />
          </div>
        </div>
      </div>
    </section>

    <!-- Fork (location stats, integrations LOCATION-STATS-SPEC D8): views and clicks per country for
    the same campaigns and date range. The view owns the sort (countryRows.mjs) so Unknown stays
    last in every direction. -->
    <section v-if="countries.fetched" class="location mt-5">
      <h4>{{ $t('analytics.location') }}</h4>
      <p class="is-size-7 has-text-grey mb-3">{{ $t('analytics.locationHint') }}</p>
      <b-table :data="countryTableRows" :loading="countries.loading" hoverable narrowed
        backend-sorting :default-sort="[countries.sort.field, countries.sort.order]" @sort="onCountrySort">
        <b-table-column v-slot="props" field="name" :label="$t('analytics.locationCountry')" sortable>
          <span :class="{ 'has-text-grey': props.row.unknown }" :title="props.row.country">{{ props.row.name }}</span>
        </b-table-column>
        <!-- Fork (campaign rates) -- per country the COUNT first, then its share of the column's
             own total (Unknown included) in grey: a share of the table, not a rate over Sent, hence
             the reversed order. The count alone when the column is empty. -->
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
    </section>

    <!-- Fork (client stats, integrations CLIENT-STATS-SPEC D5/D8/D9): views, clicks and combined per
    email client for the same campaigns and date range, the Location table's shape (clientRows.mjs
    owns the sort: Unknown last always, Other next-to-last by default). A row's tooltip lists the
    Inspect clients that render-verify it (clientRoster.mjs, advisory). -->
    <section v-if="clients.fetched" class="clients mt-5" data-cy="clients">
      <h4>{{ $t('analytics.clients') }}</h4>
      <b-table :data="clientTableRows" :loading="clients.loading" hoverable narrowed
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
      <!-- Fork (client vocabulary, integrations CLIENT-VOCABULARY-SPEC D8): the three trust-tier
           subtotals plus the untiered remainder, as footer rows beneath the table, over Views and
           Clicks ONLY (never Combined -- a tier-combined figure is close to the very number D8
           refuses). Rendered separately and NEVER summed into one figure. -->
      <table v-if="clients.rows.length" class="table is-narrow is-fullwidth client-tiers">
        <thead>
          <tr>
            <th>{{ $t('analytics.clientsTier') }}</th>
            <th class="has-text-right">{{ $t('campaigns.views') }}</th>
            <th class="has-text-right">{{ $t('campaigns.clicks') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in clientTierRows" :key="t.key">
            <td>{{ t.name }}</td>
            <td class="has-text-right">{{ $utils.formatNumber(t.views) }}</td>
            <td class="has-text-right">{{ $utils.formatNumber(t.clicks) }}</td>
          </tr>
        </tbody>
      </table>
      <p class="is-size-7 has-text-grey mt-2">{{ $t('analytics.clientsHint') }}</p>
      <p v-if="clients.rows.length" class="is-size-7 has-text-grey mt-2">{{ $t('analytics.clientsTierHint') }}</p>
    </section>
  </section>
</template>

<script>
import dayjs from 'dayjs';
import Vue from 'vue';
import { mapState } from 'vuex';
import { colors } from '../constants';
import Chart from '../components/Chart.vue';
import { DEFAULT_SORT, shapeCountryRows, sortCountryRows } from '../countryRows.mjs'; // eslint-disable-line import/extensions
import { defaultFromDate } from '../accessPolicy.mjs'; // eslint-disable-line import/extensions
import { rateCell } from '../campaignRates.mjs'; // eslint-disable-line import/extensions
import {
  DEFAULT_SORT as CLIENT_DEFAULT_SORT, TIER_ROWS, clientTierSubtotals, clientTotals,
  shapeClientRows, sortClientRows,
} from '../clientRows.mjs'; // eslint-disable-line import/extensions
import { rosterFor } from '../clientRoster.mjs'; // eslint-disable-line import/extensions
import { metricAxisMax, sendOverlayOptions, shapeSendOverlay } from '../sendOverlay.mjs'; // eslint-disable-line import/extensions
import { aggregateShopLinkRate, shapeShopLinkBars, totalShopLinkClicks } from '../shopLinkBars.mjs'; // eslint-disable-line import/extensions
import { campaignBrand, contextNotice } from '../brandScope.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';
import BrandContextNotice from '../components/BrandContextNotice.vue';

// Fork (campaign rates) -- decimals per metric, the campaigns list's (views/clicks 1, bounces 2).
// Fork (shop-link header total, 2026-10-19) -- shopLinks joins the rate headers: the shop-link
// clicks (every kind, the sum getData keeps in counts.shopLinks) over the selection's Sent.
// EVERY chart in `charts` needs an entry here AND in `counts` -- the header template reads
// totalCells[k] for each one, and a missing key is an undefined cell at render.
// Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D7) -- the three bounce-type
// charts replace bounces, two decimals each.
const RATE_DIGITS = {
  views: 1, clicks: 1, hardBounces: 2, softBounces: 2, complaints: 2, links: 1, shopLinks: 1,
};

// The view's end-of-day convention for To: today 23:59.
const endOfToday = () => dayjs().set('hour', 23).set('minute', 59).set('seconds', 0);

const chartColorRed = '#ee7d5b';
const chartColors = [
  colors.primary,
  '#FFB50D',
  '#41AC9C',
  chartColorRed,
  '#7FC7BC',
  '#3a82d6',
  '#688ED9',
  '#FFC43D',
];

export default Vue.extend({
  components: {
    Chart,
    BrandContextNotice,
  },

  // Fork (global brand, integrations GLOBAL-BRAND-SPEC D4/D9/D11).
  mixins: [brandScopeMixin],

  data() {
    return {
      isSearchLoading: false,
      queriedCampaigns: [],
      // Fork (global brand, D9) -- the picker's last search (re-run on a selection change; null =
      // the picker has not been opened yet) and each prefilled campaign's lists ({ id: lists }),
      // read where the user may read the campaign; absent = brand underivable.
      lastQuery: null,
      prefillLists: {},

      // Data for each view.
      counts: {
        views: 0,
        clicks: 0,
        hardBounces: 0,
        softBounces: 0,
        complaints: 0,
        links: 0,
        // Fork (shop-link attribution, SHOP-LINK-ATTRIBUTION-SPEC D5).
        shopLinks: 0,
      },
      urls: [],
      // Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D6) -- the Counts | Rate view (read
      // from the pref at mount; this view is the pref's one writer), and the raw rows and selection
      // size of the last fetch, kept so a toggle re-shapes the chart without a refetch.
      shopLinksView: 'counts',
      // Fork (hide No link, 2026-10-19) -- the "Hide No link" checkbox's state (pref
      // analytics.shopLinksHideNone, read at mount; the shopLinksHideNoneModel setter is its one writer).
      shopLinksHideNone: false,
      shopLinks: { rows: [], selection: 0 },
      // Fork (campaign rates) -- the Sent of the campaigns the charts were fetched for: the
      // total (headers, Location) and per id (the ring). Set at fetch time, so editing the picker
      // before the next search does not move the denominators under the numbers.
      sent: { total: 0, byId: {} },
      // Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D7) -- the five line
      // charts carry the selection's sends as translucent bars (overlay: true; one sends fetch per
      // search, shared). Views and Clicks keep their ring; the bounce-type charts have none.
      charts: {
        views: {
          name: this.$t('campaigns.views'),
          type: 'line',
          data: null,
          options: null,
          overlay: true,
          fn: this.$api.getCampaignViewCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        clicks: {
          name: this.$t('campaigns.clicks'),
          type: 'line',
          data: null,
          options: null,
          overlay: true,
          fn: this.$api.getCampaignClickCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        hardBounces: {
          name: this.$t('analytics.hardBounces'),
          type: 'line',
          data: null,
          options: null,
          overlay: true,
          noDonut: true,
          fn: this.$api.getCampaignHardBounceCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        softBounces: {
          name: this.$t('analytics.softBounces'),
          type: 'line',
          data: null,
          options: null,
          overlay: true,
          noDonut: true,
          fn: this.$api.getCampaignSoftBounceCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        complaints: {
          name: this.$t('analytics.complaints'),
          type: 'line',
          data: null,
          options: null,
          overlay: true,
          noDonut: true,
          fn: this.$api.getCampaignComplaintCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        links: {
          name: this.$t('analytics.links'),
          type: 'bar',
          data: null,
          loading: false,
          fn: this.$api.getCampaignLinkCounts,
          chartFn: this.makeLinksChart,
          onClick: this.onLinkClick,
        },

        // Fork (shop-link attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC D5) -- directly after
        // links so it renders under it; a bar per stored kind, no ring, no click-through.
        shopLinks: {
          name: this.$t('analytics.shopLinks'),
          type: 'bar',
          data: null,
          loading: false,
          fn: this.$api.getCampaignShopLinkCounts,
          chartFn: this.makeShopLinksChart,
        },
      },

      // Fork (location stats) -- the Location table.
      countries: {
        rows: [],
        loading: false,
        fetched: false,
        sort: { ...DEFAULT_SORT },
      },

      // Fork (client stats) -- the Email clients table.
      clients: {
        rows: [],
        loading: false,
        fetched: false,
        sort: { ...CLIENT_DEFAULT_SORT },
      },

      form: {
        campaigns: [],
        from: null,
        to: null,
      },
    };
  },

  methods: {
    onFromDateChange() {
      if (this.form.from > this.form.to) {
        this.form.to = dayjs(this.form.from).add(7, 'day').toDate();
      }
    },

    onToDateChange() {
      if (this.form.from > this.form.to) {
        this.form.from = dayjs(this.form.to).add(-7, 'day').toDate();
      }
    },

    formatDateTime(s) {
      return dayjs(s).format('YYYY-MM-DD HH:mm');
    },

    // Fork (brand analytics, BRAND-ANALYTICS-SPEC D11) -- From = start of the day of the earliest
    // send start of the selection (created_at for a campaign that never started), To = today 23:59.
    // Called from the picker's @add/@remove and after an ?id= prefill without from/to -- never a
    // watcher on form.campaigns, which the prefill also mutates. An empty selection leaves both.
    applyDefaultDates() {
      const from = defaultFromDate(this.form.campaigns);
      if (!from) {
        return;
      }
      this.form.from = from;
      this.form.to = endOfToday().toDate();
    },

    isCampaignSelected(camp) {
      return !this.form.campaigns.find(({ id }) => id === camp.id);
    },

    makeLinksChart(typ, camps, data) {
      const labels = data.map((l) => {
        try {
          this.urls.push(l.url);
          const u = new URL(l.url);
          if (l.url.length > 80) {
            return `${u.hostname}${u.pathname.substr(0, 50)}..`;
          }
          return u.hostname + u.pathname;
        } catch {
          return l.url;
        }
      });

      const out = {
        labels,
        datasets: [
          {
            data: data.map((l) => l.count),
            // Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D8) -- the counts at the bar ends.
            texts: data.map((l) => this.$utils.formatNumber(l.count)),
            backgroundColor: chartColors,
          }],
      };

      return { points: out, donut: null };
    },

    // Fork (shop-link attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC D5; shop-link rate,
    // SHOP-LINK-RATE-SPEC D6) -- keeps the fetch's raw rows and selection size, then shapes them for
    // the current view.
    makeShopLinksChart(typ, camps, data) {
      this.shopLinks = { rows: Array.isArray(data) ? data : [], selection: camps.length };
      return { points: this.shopLinksPoints(), donut: null };
    },

    // The bars' order, labels and texts are shopLinkBars.mjs's; the Links bar styling.
    shopLinksPoints() {
      const { labels, data: points, texts } = shapeShopLinkBars(this.shopLinks.rows, {
        self: this.$t('analytics.shopLinkSelf'),
        creator: this.$t('analytics.shopLinkCreator'),
        founder: this.$t('analytics.shopLinkFounder'),
        corporate: this.$t('analytics.shopLinkCorporate'),
        none: this.$t('analytics.shopLinkNone'),
        unknown: this.$t('analytics.shopLinkUnknown'),
      }, this.shopLinksView, (n) => this.$utils.formatNumber(n), { hideNone: this.shopLinksHideNone });

      return {
        labels,
        datasets: [
          {
            data: points,
            texts,
            backgroundColor: chartColors,
          }],
      };
    },

    // Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D7) -- the metric as one
    // line per campaign on the right axis and that campaign's sends as translucent bars on the left,
    // in the same color (sendOverlay.mjs owns the shaping; buckets are Mountain, shown verbatim).
    makeCharts(typ, campaigns, data, sends) {
      // Make a campaign id => camp lookup map to group incoming
      // data by campaigns.
      const camps = campaigns.reduce((obj, c) => {
        const out = { ...obj };
        out[c.id] = c;
        return out;
      }, {});
      // Stage 4 F2 -- the ring iterates the campaigns in the picker's order, the order the lines and
      // bars take chartColors[n] in (Object.keys would sort the ids and mismatch the colors).
      const campIDs = campaigns.map((c) => String(c.id));
      const overlay = shapeSendOverlay({
        campaigns: campaigns.map((c) => ({ id: c.id, name: c.name })),
        metric: data,
        sends,
        palette: chartColors,
        sendsLabel: (name) => this.$t('analytics.sendsOf', { name }),
      });
      const options = sendOverlayOptions({
        sendsTitle: this.$t('campaigns.sent'),
        metricTitle: this.charts[typ].name,
        metricMax: metricAxisMax(data),
      });

      if (this.charts[typ].noDonut) {
        return { points: overlay, donut: null, options };
      }

      // Donut.
      const labels = [];
      const points = campIDs.map((id) => {
        labels.push(camps[id].name);
        const cId = parseInt(id, 10);
        const sum = data.reduce((a, item) => (item.campaignId === cId ? a + item.count : a), 0);
        return sum;
      });

      // Fork (campaign rates) -- each slice's rate over its own campaign's Sent (Chart.vue's donut
      // tooltip shows it rate-first when present).
      const rates = campIDs.map((id, i) => rateCell(points[i], this.sent.byId[id], RATE_DIGITS[typ]).pct);
      const donut = {
        labels,
        datasets: [{
          data: points, backgroundColor: chartColors, borderWidth: 6, rates,
        }],
      };
      return { points: overlay, donut, options };
    },

    onSubmit() {
      this.$router.push({ query: { id: this.form.campaigns.map((c) => c.id), from: dayjs(this.form.from).unix(), to: dayjs(this.form.to).unix() } });
    },

    // Fork (brand analytics, BRAND-ANALYTICS-SPEC D3/D4) -- the narrow, list-scoped picker endpoint
    // for every user, so the picker never offers a campaign the analytics endpoint would refuse.
    // Fork (global brand, D9) -- with the global brand's list ids (none under All brands); the
    // first scoped fetch waits for the lists (I13).
    queryCampaigns(q) {
      const query = typeof q === 'string' ? q : '';
      if (!this.listsLoaded) {
        // Queue ONE fetch for the lists' arrival; later calls before then only update the query
        // it will run (Stage 4 review F2 -- every focus/typing event used to queue another).
        if (this.lastQuery === null) {
          this.afterListsLoaded(() => this.queryCampaigns(this.lastQuery));
        }
        this.lastQuery = query;
        return;
      }
      this.lastQuery = query;
      this.isSearchLoading = true;
      const params = { query };
      if (this.scopeListIds) {
        params.list_id = this.scopeListIds;
      }
      this.$api.getAnalyticsCampaigns(params).then((data) => {
        this.isSearchLoading = false;
        this.queriedCampaigns = data.map((c) => {
          // Change the name to include the ID in the auto-suggest results.
          const camp = c;
          camp.name = `#${c.id}: ${c.name}`;
          return camp;
        });
      });
    },

    // Fork (campaign rates) -- the Sent denominators for one fetch of the charts and the table.
    setSent(camps) {
      const byId = {};
      let total = 0;
      camps.forEach((c) => {
        const n = Number.isFinite(c.sent) ? c.sent : 0;
        byId[c.id] = n;
        total += n;
      });
      this.sent = { total, byId };
    },

    // A country's share of the column's own total (the rows' sum, Unknown included), so every
    // table sums to 100% even though the column total legitimately differs from the header.
    countryShare(n, field) {
      return rateCell(n, this.countryTotals[field], 1).pct;
    },

    // Fork (campaign send series, D7) -- an overlay chart waits for the search's one sends fetch
    // (sends, a promise of the rows; a failed sends fetch is no bars, never a failed chart).
    getData(typ, camps, sends) {
      this.charts[typ].loading = true;
      // Call the HTTP API.
      const metric = this.charts[typ].fn({
        id: camps.map((c) => c.id),
        from: this.form.from,
        to: this.form.to,
      });
      const overlay = this.charts[typ].overlay ? sends : Promise.resolve([]);
      Promise.all([metric, overlay]).then(([data, sendRows]) => {
        // Set the total count.
        this.counts[typ] = data.reduce((sum, d) => sum + d.count, 0);

        const { points, donut, options } = this.charts[typ].chartFn(typ, camps, data, sendRows);
        this.charts[typ].data = points;
        this.charts[typ].donutData = donut;
        if (options) {
          this.charts[typ].options = options;
        }
        this.charts[typ].loading = false;
      });
    },

    // The selection's sends per bucket for the overlay charts, fetched once per search.
    getSends(camps) {
      return this.$api.getCampaignSendCounts({
        id: camps.map((c) => c.id),
        from: this.form.from,
        to: this.form.to,
      }).then((rows) => (Array.isArray(rows) ? rows : [])).catch(() => []);
    },

    // Fork (location stats) -- fetched alongside the charts, for the same campaigns and range.
    getCountries(camps) {
      this.countries.loading = true;
      this.countries.fetched = true;
      this.$api.getCampaignCountryCounts({
        id: camps.map((c) => c.id),
        from: this.form.from,
        to: this.form.to,
      }).then((data) => {
        this.countries.rows = shapeCountryRows(data, {
          locale: this.locale,
          unknownLabel: this.$t('analytics.locationUnknown'),
        });
      }).finally(() => {
        this.countries.loading = false;
      });
    },

    onCountrySort(field, order) {
      this.countries.sort = { field, order };
    },

    // Fork (client stats) -- fetched alongside the Location table, same campaigns and range.
    getClients(camps) {
      this.clients.loading = true;
      this.clients.fetched = true;
      this.$api.getCampaignClientCounts({
        id: camps.map((c) => c.id),
        from: this.form.from,
        to: this.form.to,
      }).then((data) => {
        this.clients.rows = shapeClientRows(data, {
          locale: this.locale,
          unknownLabel: this.$t('analytics.clientsUnknown'),
        });
      }).finally(() => {
        this.clients.loading = false;
      });
    },

    onClientSort(field, order) {
      this.clients.sort = { field, order };
    },

    // A client's share of the column's own total (Unknown included), as countryShare.
    clientShare(n, field) {
      return rateCell(n, this.clientTotalsSum[field], 1).pct;
    },

    // The row tooltip: the Inspect clients that render-verify the token (advisory, D8).
    clientRosterTip(token) {
      const ids = rosterFor(token);
      return ids.length ? this.$t('analytics.clientsRoster', { ids: ids.join(', ') }) : '';
    },

    onLinkClick(e) {
      const bars = e.chart.getElementsAtEventForMode(e, 'nearest', { intersect: true }, true);
      if (bars.length > 0) {
        window.open(this.urls[bars[0].index], '_blank', 'noopener noreferrer');
      }
    },
  },

  computed: {
    ...mapState(['serverConfig']),
    ...mapState({ storeLists: 'lists' }),

    locale() {
      return (this.$i18n && this.$i18n.locale) || 'en';
    },

    // Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D6/D7) -- the toggle's model: the only
    // writer of the analytics.shopLinksView pref; a change re-shapes the kept rows (no refetch).
    shopLinksViewModel: {
      get() {
        return this.shopLinksView;
      },
      set(v) {
        this.shopLinksView = v === 'rate' ? 'rate' : 'counts';
        this.$utils.setPref('analytics.shopLinksView', this.shopLinksView);
        if (this.charts.shopLinks.data) {
          this.charts.shopLinks.data = this.shopLinksPoints();
        }
      },
    },

    // Fork (hide No link, 2026-10-19) -- the checkbox's model: the only writer of the
    // analytics.shopLinksHideNone pref; a change re-shapes the kept rows (no refetch).
    shopLinksHideNoneModel: {
      get() {
        return this.shopLinksHideNone;
      },
      set(v) {
        this.shopLinksHideNone = !!v;
        this.$utils.setPref('analytics.shopLinksHideNone', this.shopLinksHideNone ? 'true' : 'false');
        if (this.charts.shopLinks.data) {
          this.charts.shopLinks.data = this.shopLinksPoints();
        }
      },
    },

    // No selected campaign has an audience snapshot (every audience null).
    shopLinksNoSnapshot() {
      return !this.shopLinks.rows.some((r) => r && r.audience !== null && r.audience !== undefined);
    },

    // Some, not all, of the selected campaigns have a snapshot.
    shopLinksPartial() {
      const r = this.shopLinks.rows[0];
      const n = r && Number.isFinite(r.snapshots) ? r.snapshots : 0;
      return n > 0 && n < this.shopLinks.selection;
    },

    // The chart headers' { pct, count } per metric (review tidy: a computed, not a :set attribute).
    totalCells() {
      const fmt = this.$utils.formatNumber.bind(this.$utils);
      return Object.fromEntries(Object.keys(RATE_DIGITS).map((k) => [k, rateCell(this.counts[k], this.sent.total, RATE_DIGITS[k], fmt)]));
    },

    // Fork (shop-link header total, 2026-10-19) -- the Shop Link Attribution header's cell: in
    // the Counts view the clicks over Sent (totalCells, the Clicks header's shape); in the Rate
    // view the aggregate rate, clicks over the snapshot audience, with "n of N" in grey. With no
    // audience the Rate view shows the Counts cell.
    shopLinksHeaderCell() {
      const fmt = this.$utils.formatNumber.bind(this.$utils);
      const opts = { hideNone: this.shopLinksHideNone };
      if (this.shopLinksView === 'rate') {
        const agg = aggregateShopLinkRate(this.shopLinks.rows, fmt, opts);
        if (agg.pct) {
          return { pct: agg.pct, count: `${agg.count} of ${agg.audience}` };
        }
        // No snapshot: the count alone -- a rate over Sent here would contradict the view's
        // "No audience snapshot" note (sanity check 2026-10-19).
        return { pct: '', count: fmt(totalShopLinkClicks(this.shopLinks.rows, opts)) };
      }
      // The header describes the bars on screen: Hide No link drops that row from the total too.
      return rateCell(totalShopLinkClicks(this.shopLinks.rows, opts), this.sent.total, RATE_DIGITS.shopLinks, fmt);
    },

    countryTotals() {
      return this.countries.rows.reduce((t, r) => ({ views: t.views + (r.views || 0), clicks: t.clicks + (r.clicks || 0) }), { views: 0, clicks: 0 });
    },

    countryTableRows() {
      const { field, order } = this.countries.sort;
      return sortCountryRows(this.countries.rows, field, order, this.locale);
    },

    clientTotalsSum() {
      return clientTotals(this.clients.rows);
    },

    clientTableRows() {
      const { field, order } = this.clients.sort;
      return sortClientRows(this.clients.rows, field, order, this.locale);
    },

    // Fork (client vocabulary, D8) -- the tier footer rows. The tier map and this row order are
    // clientRows.mjs's (the tier's single home); this only labels and formats them.
    clientTierRows() {
      const sub = clientTierSubtotals(this.clients.rows);
      return TIER_ROWS.map(({ key, i18n }) => ({
        key, name: this.$t(i18n), views: sub[key].views, clicks: sub[key].clicks,
      }));
    },

    // Fork (global brand, D11) -- the distinct brands of the selected campaigns that carry a
    // notice under the current selection (contextNotice non-null).
    noticeBrands() {
      if (this.brandScope.pending) {
        return [];
      }
      const out = [];
      this.form.campaigns.forEach((c) => {
        const b = this.prefillLists[c.id] ? campaignBrand(this.prefillLists[c.id], this.storeLists) : undefined;
        if (b !== undefined && !out.includes(b) && contextNotice(b, this.brandSelection, this.brandRows)) {
          out.push(b);
        }
      });
      return out;
    },
  },

  watch: {
    // Fork (global brand, D4) -- re-fetch the picker under the new selection; the picked
    // campaigns stay (D9).
    brandKey(now, before) {
      if (before === 'pending' || now === 'pending' || this.lastQuery === null) {
        return;
      }
      this.queryCampaigns(this.lastQuery);
    },
  },

  created() {
    const now = endOfToday();
    const weekAgo = now.subtract(7, 'day').set('hour', 0).set('minute', 0);
    const from = this.$route.query.from ? dayjs.unix(this.$route.query.from) : weekAgo;
    const to = this.$route.query.to ? dayjs.unix(this.$route.query.to) : now;
    this.form.from = from.toDate();
    this.form.to = to.toDate();
  },

  mounted() {
    // Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D6) -- the remembered view, read at
    // mount; default counts.
    this.shopLinksView = this.$utils.getPref('analytics.shopLinksView') === 'rate' ? 'rate' : 'counts';
    // Fork (hide No link, 2026-10-19) -- the remembered checkbox; default shown.
    this.shopLinksHideNone = this.$utils.getPref('analytics.shopLinksHideNone') === 'true';

    // Fetch one or more campaigns if there are ?id params, wait for the fetches
    // to finish, add them to the campaign selector and submit the form.
    const ids = this.$utils.parseQueryIDs(this.$route.query.id);
    if (ids.length > 0) {
      // Fork (brand analytics, D4) -- one read through the picker endpoint; ids the user may not
      // see (or that do not exist) are simply absent, so a shared link degrades to the permitted
      // subset. Kept in the URL's order.
      this.isSearchLoading = true;
      this.$api.getAnalyticsCampaigns({ id: ids }).catch(() => []).then((data) => {
        [...data].sort((a, b) => ids.indexOf(a.id) - ids.indexOf(b.id)).forEach((c) => {
          const camp = c;
          camp.name = `#${camp.id}: ${camp.name}`;
          this.form.campaigns.push(camp);
        });

        // Fork (global brand, D9/D11) -- the prefill is never filtered by the selection (no
        // list_id above). A prefilled campaign's brand comes from its lists, readable only to a
        // user who may read the campaign itself; an analytics-only user's is underivable, so no
        // notice (D11).
        if (this.$can('campaigns:get_all', 'campaigns:get')) {
          this.form.campaigns.forEach((c) => {
            this.$api.getCampaignQuiet(c.id).then((full) => {
              this.$set(this.prefillLists, c.id, (full && full.lists) || []);
            }).catch(() => {});
          });
        }

        // D11 (b): the Campaigns-page link carries only id -- default the range to the selection.
        // A URL with from/to is honoured as-is.
        if (!this.$route.query.from && !this.$route.query.to) {
          this.applyDefaultDates();
        }

        this.$nextTick(() => {
          this.isSearchLoading = false;

          // Nothing permitted to show: no analytics requests (they would 400 on an empty id set).
          if (this.form.campaigns.length === 0) {
            return;
          }

          this.setSent(this.form.campaigns);

          // Fork (campaign send series, D7) -- the sends, once, shared by the overlay charts.
          const sends = this.getSends(this.form.campaigns);

          // Fetch count for each analytics type (views, clicks, the bounce types, links);
          Object.keys(this.charts).forEach((k) => {
            this.charts[k].data = null;
            this.charts[k].donutData = null;

            // Fetch views, clicks, bounces for every campaign.
            this.getData(k, this.form.campaigns, sends);
          });

          // Fork (location stats) -- the Location table.
          this.getCountries(this.form.campaigns);
          // Fork (client stats) -- the Email clients table.
          this.getClients(this.form.campaigns);
        });
      });
    }
  },
});
</script>
