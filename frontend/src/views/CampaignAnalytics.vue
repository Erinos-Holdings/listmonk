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
            <h4>
              {{ v.name }}
              <span v-if="v.type !== 'bar'" class="has-text-grey-light">({{ $utils.niceNumber(counts[k]) }})</span>
            </h4>
            <chart :type="v.type" v-if="!v.loading" :data="v.data" :on-click="v.onClick" />
          </div>
          <div class="column is-2 donut-container">
            <chart type="donut" v-if="!v.loading" :data="v.donutData" />
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
        <b-table-column v-slot="props" field="views" :label="$t('campaigns.views')" numeric sortable>
          {{ $utils.niceNumber(props.row.views) }}
        </b-table-column>
        <b-table-column v-slot="props" field="clicks" :label="$t('campaigns.clicks')" numeric sortable>
          {{ $utils.niceNumber(props.row.clicks) }}
        </b-table-column>
        <template #empty v-if="!countries.loading">
          <p class="has-text-grey">{{ $t('globals.messages.emptyState') }}</p>
        </template>
      </b-table>
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
  },

  data() {
    return {
      isSearchLoading: false,
      queriedCampaigns: [],

      // Data for each view.
      counts: {
        views: 0,
        clicks: 0,
        bounces: 0,
        links: 0,
      },
      urls: [],
      charts: {
        views: {
          name: this.$t('campaigns.views'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignViewCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        clicks: {
          name: this.$t('campaigns.clicks'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignClickCounts,
          chartFn: this.makeCharts,
          loading: false,
        },

        bounces: {
          name: this.$t('globals.terms.bounces'),
          type: 'line',
          data: null,
          fn: this.$api.getCampaignBounceCounts,
          chartFn: this.makeCharts,
          donutColor: chartColorRed,
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
      },

      // Fork (location stats) -- the Location table.
      countries: {
        rows: [],
        loading: false,
        fetched: false,
        sort: { ...DEFAULT_SORT },
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
            backgroundColor: chartColors,
          }],
      };

      return { points: out, donut: null };
    },

    makeCharts(typ, campaigns, data) {
      // Make a campaign id => camp lookup map to group incoming
      // data by campaigns.
      const camps = campaigns.reduce((obj, c) => {
        const out = { ...obj };
        out[c.id] = c;
        return out;
      }, {});
      const campIDs = Object.keys(camps);
      // datasets[] array for line chart.
      const lines = campIDs.map((id, n) => {
        const cId = parseInt(id, 10);
        const points = data.filter((item) => item.campaignId === cId);

        return {
          label: camps[id].name,
          data: points.map((item) => ({ x: this.formatDateTime(item.timestamp), y: item.count })),
          borderColor: chartColors[n % chartColors.length],
          borderWidth: 2,
          pointHoverBorderWidth: 5,
          pointBorderWidth: 0.5,
        };
      });

      // Donut.
      const labels = [];
      const points = campIDs.map((id) => {
        labels.push(camps[id].name);
        const cId = parseInt(id, 10);
        const sum = data.reduce((a, item) => (item.campaignId === cId ? a + item.count : a), 0);
        return sum;
      });

      const donut = {
        labels,
        datasets: [{
          data: points, backgroundColor: chartColors, borderWidth: 6,
        }],
      };
      return { points: { datasets: lines }, donut };
    },

    onSubmit() {
      this.$router.push({ query: { id: this.form.campaigns.map((c) => c.id), from: dayjs(this.form.from).unix(), to: dayjs(this.form.to).unix() } });
    },

    // Fork (brand analytics, BRAND-ANALYTICS-SPEC D3/D4) -- the narrow, list-scoped picker endpoint
    // for every user, so the picker never offers a campaign the analytics endpoint would refuse.
    queryCampaigns(q) {
      this.isSearchLoading = true;
      this.$api.getAnalyticsCampaigns({
        query: q,
      }).then((data) => {
        this.isSearchLoading = false;
        this.queriedCampaigns = data.map((c) => {
          // Change the name to include the ID in the auto-suggest results.
          const camp = c;
          camp.name = `#${c.id}: ${c.name}`;
          return camp;
        });
      });
    },

    getData(typ, camps) {
      this.charts[typ].loading = true;
      // Call the HTTP API.
      this.charts[typ].fn({
        id: camps.map((c) => c.id),
        from: this.form.from,
        to: this.form.to,
      }).then((data) => {
        // Set the total count.
        this.counts[typ] = data.reduce((sum, d) => sum + d.count, 0);

        const { points, donut } = this.charts[typ].chartFn(typ, camps, data);
        this.charts[typ].data = points;
        this.charts[typ].donutData = donut;
        this.charts[typ].loading = false;
      });
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

    onLinkClick(e) {
      const bars = e.chart.getElementsAtEventForMode(e, 'nearest', { intersect: true }, true);
      if (bars.length > 0) {
        window.open(this.urls[bars[0].index], '_blank', 'noopener noreferrer');
      }
    },
  },

  computed: {
    ...mapState(['serverConfig']),

    locale() {
      return (this.$i18n && this.$i18n.locale) || 'en';
    },

    countryTableRows() {
      const { field, order } = this.countries.sort;
      return sortCountryRows(this.countries.rows, field, order, this.locale);
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

          // Fetch count for each analytics type (views, counts, bounces);
          Object.keys(this.charts).forEach((k) => {
            this.charts[k].data = null;
            this.charts[k].donutData = null;

            // Fetch views, clicks, bounces for every campaign.
            this.getData(k, this.form.campaigns);
          });

          // Fork (location stats) -- the Location table.
          this.getCountries(this.form.campaigns);
        });
      });
    }
  },
});
</script>
