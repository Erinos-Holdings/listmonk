<template>
  <!-- Fork (brand usage, integrations BRAND-USAGE-SPEC D2/D7; BRAND-USAGE-V2-SPEC D1-D5; moved from the
       Dashboard to the Brands page by CAMPAIGN-MATRIX-SPEC D1 -- unchanged otherwise). The Brand
       Matrix's rows (same endpoint, scope and launch filter), each brand's stored usage object
       rendered: the Score, then the Broadcasts / Journeys / Configurations sections. Every state,
       count, threshold and weight was computed by the integrations BrandHealth Lambda; every cell,
       circle, link and sort rule is brandUsage.mjs. V2 D2: the matrix's hand-rolled table pattern
       (group header row, sorted-column emphasis) with neutral-grey section bands -- the stoplight
       circles are the table's only chroma. -->
  <section class="brand-usage relative" data-cy="brand-usage">
    <b-loading v-if="loading" active :is-full-page="false" />
    <div class="table-container">
      <table class="table is-hoverable is-narrow bu-table" data-cy="usage-table">
        <thead>
          <tr class="bu-groups">
            <th colspan="2" />
            <th v-for="s in sections" :key="s.key" :colspan="s.fields.length"
              :class="['bu-group', 'bu-start', `bu-band-${s.band}`]">
              {{ $t(s.label) }}
            </th>
          </tr>
          <!-- DASHBOARDS-POLISH3-SPEC D4/D5 -- brandUsage.mjs::headerLayout: plain leaves span the
               two lower header rows; a sub-group (No purchase, Skipped) puts its label over its
               leaves (3d | 7d | 15d; FR ES DE IT). -->
          <tr>
            <th rowspan="2" class="bu-head" :class="buTh('score')" :title="$t('dashboard.usage.scoreHelp')"
              :aria-sort="buAria('score')" @click="onSort('score')">
              {{ $t('dashboard.usage.score') }}<span class="bm-arrow">{{ buArrow('score') }}</span>
            </th>
            <th rowspan="2" class="bu-head" :class="buTh('brand')" :aria-sort="buAria('brand')" @click="onSort('brand')">
              {{ $t('dashboard.matrix.brand') }}<span class="bm-arrow">{{ buArrow('brand') }}</span>
            </th>
            <template v-for="c in header.row2">
              <th v-if="c.kind === 'group'" :key="`g-${c.key}`" :colspan="c.span"
                class="has-text-centered bu-head bu-subgroup"
                :class="[`bu-band-${c.band}`, { 'bu-start': c.first, 'bu-small': c.small }]"
                :title="$t(c.help)" :data-cy="`usage-group-${c.key}`">
                {{ $t(c.label) }}
              </th>
              <th v-else :key="c.field" rowspan="2" class="has-text-centered bu-head"
                :class="[buTh(c.field), `bu-band-${c.band}`, { 'bu-start': c.first }]" :title="$t(meta(c.field).help)"
                :aria-sort="buAria(c.field)" @click="onSort(c.field)">
                {{ label(c.field) }}<span class="bm-arrow">{{ buArrow(c.field) }}</span>
              </th>
            </template>
          </tr>
          <tr>
            <th v-for="c in header.row3" :key="c.field" class="has-text-centered bu-small"
              :class="[buTh(c.field), `bu-band-${c.band}`, { 'bu-start': c.first, 'bu-substart': c.groupStart }]"
              :title="$t(meta(c.field).help)" :aria-sort="buAria(c.field)" @click="onSort(c.field)"
              :data-cy="`usage-head-${c.field}`">
              {{ label(c.field) }}<span class="bm-arrow">{{ buArrow(c.field) }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in tableRows" :key="row.brand" :class="{ 'bu-zma': row.zma }">
            <td class="has-text-right bu-score" :class="buTd('score')">
              <span :title="cell(row, 'score').tooltip || null" :data-cy="`usage-score-${row.brand}`">
                {{ cell(row, 'score').text }}</span>
            </td>
            <td :class="buTd('brand')">
              <span :data-cy="`usage-brand-${row.brand}`">{{ row.label }}</span>
              <b-tag v-if="row.zma" size="is-small" class="ml-1" :title="$t('dashboard.usage.zmaHelp')">
                {{ $t('dashboard.usage.zma') }}
              </b-tag>
            </td>
            <td v-for="c in columns" :key="c.field" class="has-text-centered"
              :class="[buTd(c.field), `bu-band-${c.band}`, { 'bu-start': c.first }]">
              <!-- V2 D3/D4: one circle per state, the stored count inside; the link is per CELL
                   (Campaigns, the list preselected, the Automations scope; Journeys -- POLISH2 D2 --
                   the Automations scope filtered to the brand's lists). -->
              <component v-if="cell(row, c.field).circles.length" :is="link(row, c.field) ? 'router-link' : 'span'"
                :to="link(row, c.field) ? { name: 'campaigns', query: link(row, c.field) } : null"
                class="bu-circles" :data-cy="`usage-${c.field}-${row.brand}`">
                <span v-for="ci in cell(row, c.field).circles" :key="ci.color"
                  :class="['bu-circle', `bu-${ci.color}`]" :title="cell(row, c.field).tooltip"
                  :aria-label="cell(row, c.field).tooltip">{{ ci.n }}</span>
              </component>
              <!-- DASHBOARDS-POLISH2-SPEC D2: the Cadence / Last broadcast text links the brand's
                   lists in the Broadcasts scope (brandUsage.mjs::cellLink); '—' never links.
                   POLISH3 D2/D5: the broadcast links carry status=finished; a Skipped count > 0
                   is red and links its skipping lists in the All scope. MATRIX-POLISH-SPEC D10.3:
                   Last broadcast links the last send's campaigns (?id=…, All) when the doc carries
                   broadcasts.lastIds. -->
              <component v-else :is="link(row, c.field) ? 'router-link' : 'span'"
                :to="link(row, c.field) ? { name: 'campaigns', query: link(row, c.field) } : null"
                :class="cell(row, c.field).cls" :title="title(row, c.field)"
                :data-cy="`usage-${c.field}-${row.brand}`">
                <span>{{ cell(row, c.field).text }}</span>
              </component>
              <!-- DASHBOARDS-POLISH3-SPEC D4 (stage 2 F5) -- one amber ⚠ per No purchase step
                   with no parseable days, after the sub-group's last leaf; its parser error in
                   the tooltip (brandUsage.mjs::cellBadges). -->
              <span v-for="(b, bi) in badges(row, c.field)" :key="`w-${bi}`"
                class="bu-warn has-text-warning-dark ml-1" :title="b.tooltip" :aria-label="b.tooltip"
                :data-cy="`usage-npwarn-${row.brand}`">{{ b.glyph }}</span>
            </td>
          </tr>
          <tr v-if="!loading && !tableRows.length">
            <td :colspan="2 + columns.length">
              <p class="has-text-grey">{{ $t('globals.messages.emptyState') }}</p>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <!-- V2 D5: the stoplight key, then the ZMA line and the as-of line (Mountain time). -->
    <p class="is-size-7 has-text-grey mt-2 bu-legend" data-cy="usage-legend">
      <span class="bu-circle bu-green">1</span> {{ $t('dashboard.usage.legendRunning') }}
      <span class="bu-circle bu-amber">1</span> {{ $t('dashboard.usage.legendIdle') }}
      <span class="bu-circle bu-red">0</span> {{ $t('dashboard.usage.legendNone') }}
      · {{ $t('dashboard.usage.legendConfig') }}
    </p>
    <p class="is-size-7 has-text-grey" data-cy="usage-zma-note">{{ $t('dashboard.usage.note') }}</p>
    <p class="is-size-7 has-text-grey" data-cy="usage-note">
      {{ runTime ? $t('dashboard.usage.noteAsOf', { date: runTime }) : $t('dashboard.usage.computed') }}
    </p>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { sessionStore } from '../brandMatrix.mjs'; // eslint-disable-line import/extensions
import {
  SECTIONS, cell as cellOf, cellBadges, cellLink, clickSort, columnMeta, headerLayout, latestAsOf, linkTip,
  loadState, mountainRunTime, saveState, shapeRows, sortRows,
} from '../brandUsage.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';

export default Vue.extend({
  // Integrations CAMPAIGN-MATRIX-SPEC D1 -- the pane carries the scope mixin itself (see BrandMatrix.vue)
  // and renders off its own brand-matrix fetch.
  mixins: [brandScopeMixin],

  data() {
    return {
      apiRows: [],
      loading: false,
      // BRAND-USAGE-SPEC D7 -- the sort, per browser session (sessionStorage key dashboard.usage, kept;
      // V2 D2: a fresh session is Score descending).
      usage: loadState(sessionStore(typeof window !== 'undefined' ? window : null)),
    };
  },

  computed: {
    ...mapGetters(['launch']),

    // D2/D7 -- the matrix rows (same launch filter and display names), sorted. DASHBOARDS-POLISH2-SPEC
    // D2 -- the lists store (active, permitted lists) gives each row its brand list-id set.
    tableRows() {
      const rows = shapeRows(this.apiRows, {
        brands: this.brandRows, launch: this.launch, lists: this.$store.state.lists,
      });
      return sortRows(rows, this.usage.sort, (this.$i18n && this.$i18n.locale) || 'en');
    },

    // V2 D5 -- the newest stored asOf's run time in Mountain time, null without a day.
    runTime() {
      return mountainRunTime(latestAsOf(this.tableRows));
    },

    // V2 D2 -- the sections with their alternating band: grey, white, grey.
    sections() {
      return SECTIONS.map((s, i) => ({ ...s, band: i % 2 === 0 ? 'grey' : 'white' }));
    },

    columns() {
      return this.sections.flatMap((s) => s.fields.map((f, i) => ({
        field: f, band: s.band, first: i === 0,
      })));
    },

    header() {
      return headerLayout();
    },
  },

  methods: {
    scopeParams() {
      return this.scopeListIds ? { list_id: this.scopeListIds } : {};
    },

    fetchRows() {
      this.loading = true;
      this.$api.getDashboardBrandMatrix(this.scopeParams()).then((data) => {
        this.apiRows = (data && Array.isArray(data.rows)) ? data.rows : [];
      }).finally(() => {
        this.loading = false;
      });
    },

    onSort(field) {
      this.usage.sort = clickSort(this.usage.sort, field);
      saveState(sessionStore(window), { sort: this.usage.sort });
    },

    cell(row, field) {
      return cellOf(row, field, (k, p) => this.$t(k, p));
    },

    badges(row, field) {
      return cellBadges(row, field, (k, p) => this.$t(k, p));
    },

    meta(field) {
      return columnMeta(field);
    },

    label(field) {
      const m = columnMeta(field);
      return m.text !== undefined ? m.text : this.$t(m.label, m.params);
    },

    link(row, field) {
      return cellLink(row, field);
    },

    title(row, field) {
      const parts = [this.cell(row, field).tooltip, linkTip(row, field, (k, p) => this.$t(k, p))];
      return parts.filter(Boolean).join('\n') || null;
    },

    buSorted(field) {
      return this.usage.sort.field === field;
    },

    buTh(field) {
      return ['bm-sort', { 'bu-sorted': this.buSorted(field) }];
    },

    buTd(field) {
      return { 'bu-sorted': this.buSorted(field) };
    },

    buArrow(field) {
      if (!this.buSorted(field)) {
        return '';
      }
      return this.usage.sort.order === 'asc' ? '↑' : '↓';
    },

    buAria(field) {
      if (!this.buSorted(field)) {
        return null;
      }
      return this.usage.sort.order === 'asc' ? 'ascending' : 'descending';
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
