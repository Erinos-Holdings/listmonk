<template>
  <!-- Fork (campaign matrix, integrations listmonk/CAMPAIGN-MATRIX-SPEC D3-D10). The Matrix pane of
       Analytics: one row per campaign in scope with activity in the window (the Mountain
       days from 60 days back through TODAY -- HOURLY-HEALTH-SPEC D1; the Brand Matrix still ends
       yesterday), in the Brand Matrix's grouped-header style, with a Status column (D2). The server
       (GET /api/analytics/campaigns/matrix) scopes and counts; every rate, header, sort and
       missing-value rule is campaignMatrix.mjs. The brand selector reaches the server as list_id;
       the navbar launch filter narrows here by each row's brand. Fork (matrix polish, integrations
       MATRIX-POLISH-SPEC D3-D7): one Views column, the Shop Link Attribution group, Tracked /
       Matched headers, no Brand column under a single-brand selection, names truncated. Fork
       (integrations MATRIX-VIEWS-SPEC D2-D4): the Health | Attribution view toggle, the left columns
       Date, Brand, Campaign, Status, Sent, Skipped, the Brand cell as the upper-cased slug and no
       brand chip in the Campaign cell (campaignMatrix.mjs). -->
  <section class="campaign-matrix relative" data-cy="campaign-matrix">
    <b-loading v-if="loading" active :is-full-page="false" />
    <div class="cm-toolbar is-flex is-align-items-center mb-3">
      <!-- D4: the Campaigns page's kinds and labels; own session state, All by default. The pills are
           the Campaigns page's .segmented toggle at its normal size, so the name filter's input
           (the Campaigns page's search height) sits level with them. -->
      <b-field class="segmented mb-0 mr-4">
        <b-radio-button v-model="state.kind" native-value="all" type="is-primary"
          data-cy="cm-kind-all" @input="onKind">
          {{ $t('globals.terms.all') }}
        </b-radio-button>
        <b-radio-button v-model="state.kind" native-value="broadcasts" type="is-primary"
          data-cy="cm-kind-broadcasts" @input="onKind">
          {{ $t('campaigns.typeBroadcasts') }}
        </b-radio-button>
        <b-radio-button v-model="state.kind" native-value="automations" type="is-primary"
          data-cy="cm-kind-automations" @input="onKind">
          {{ $t('campaigns.typeAutomations') }}
        </b-radio-button>
      </b-field>
      <b-field class="segmented mb-0 mr-4">
        <b-radio-button v-model="state.mode" native-value="pct" type="is-primary"
          data-cy="cm-mode-pct" @input="saveState">
          {{ $t('dashboard.matrix.pctUsd') }}
        </b-radio-button>
        <b-radio-button v-model="state.mode" native-value="num" type="is-primary"
          data-cy="cm-mode-num" @input="saveState">
          {{ $t('dashboard.matrix.num') }}
        </b-radio-button>
      </b-field>
      <!-- The name filter: a client-side substring of the stored name, or of the brand's slug or display
           name (NAMING-STANDARD-SPEC D9; campaignMatrix.mjs::filterByName), applied as you type; not
           sticky, like the Campaigns page's search. -->
      <b-field class="mb-0 cm-name-filter">
        <b-input v-model="nameQuery" name="name" icon="magnify" :placeholder="$t('analytics.matrix.nameFilter')"
          data-cy="cm-name-filter" />
      </b-field>
      <!-- MATRIX-VIEWS-SPEC D2: the view toggle, right of the name filter; part of the session state.
           A sort on a column the new view hides falls back to Date desc (campaignMatrix.mjs::sortForView). -->
      <b-field class="segmented mb-0 ml-4">
        <b-radio-button v-model="state.view" native-value="health" type="is-primary"
          data-cy="cm-view-health" @input="onView">
          {{ $t('analytics.matrix.viewHealth') }}
        </b-radio-button>
        <b-radio-button v-model="state.view" native-value="attribution" type="is-primary"
          data-cy="cm-view-attribution" @input="onView">
          {{ $t('analytics.matrix.viewAttribution') }}
        </b-radio-button>
      </b-field>
    </div>
    <div class="table-container">
      <table class="table is-fullwidth is-hoverable is-narrow bm-table" data-cy="cm-table">
        <thead>
          <!-- MATRIX-POLISH-SPEC D4/D5: no Views group (Views is one column); CONVERSIONS after
               Clicks, then the view's groups (campaignMatrix.mjs::groupHeaders, derived from the
               same column list as the header cells -- MATRIX-VIEWS-SPEC D2). -->
          <tr class="bm-groups">
            <th :colspan="leftCols.length" />
            <th v-for="(g, i) in groupCols" :key="i" :colspan="g.span" :class="g.cls">{{ groupLabel(g) }}</th>
          </tr>
          <tr>
            <th v-for="c in leftCols" :key="c.field"
              :class="[bmTh(c.field), { 'has-text-right': c.field === 'sent' || c.field === 'skipped' }]"
              :title="c.help ? $t(c.help) : null"
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
            <!-- MATRIX-VIEWS-SPEC D3: the left cells in leftCols' order (Date, Brand, Campaign, Status,
                 Sent, Skipped); MATRIX-POLISH-SPEC D6: no Brand column under a single-brand selection. -->
            <template v-for="c in leftCols">
              <td v-if="c.field === 'name'" :key="c.field" :class="bmTd('name')" class="cm-name">
                <!-- D3: the campaign's own Campaign pane (the remount the URL change causes is intended).
                     MATRIX-POLISH-SPEC D7 (widened 2026-11-06): the first NAME_MAX characters, then '…';
                     the stored name on hover. -->
                <!-- Fork (naming standard, NAMING-STANDARD-SPEC D1/D10) -- the language chip
                     (campaignMatrix.mjs::matrixChips: chipsFor's language rule, no brand chip --
                     MATRIX-VIEWS-SPEC D4) and the former name on hover. -->
                <b-tag v-if="!row.totals && chipsOf(row).lang" class="is-small lang" :data-cy="`cm-lang-chip-${row.id}`">
                  {{ chipsOf(row).lang }}
                </b-tag>
                <router-link v-if="!row.totals" :to="{ name: 'campaignAnalytics', query: { id: row.id } }"
                  :title="nameTitle(row)" :data-cy="`cm-name-${row.id}`">
{{ nameOf(row).text }}
</router-link>
                <template v-else>{{ row.name }}</template>
              </td>
              <!-- MATRIX-VIEWS-SPEC D4: the upper-cased slug, the display name on hover. -->
              <td v-else-if="c.field === 'brand'" :key="c.field" :class="bmTd('brand')"
                :title="brandOf(row).title" :data-cy="`cm-brand-${row.totals ? 'totals' : row.id}`">
                {{ brandOf(row).text }}
              </td>
              <td v-else-if="c.field === 'date'" :key="c.field" :class="bmTd('date')">
                <span :title="dateTip(row)">{{ dateText(row) }}</span>
              </td>
              <!-- HOURLY-HEALTH-SPEC D2: the Campaigns page's status tag; empty on the totals row. -->
              <td v-else-if="c.field === 'status'" :key="c.field" :class="bmTd('status')"
                :data-cy="`cm-status-${row.totals ? 'totals' : row.id}`">
                <b-tag v-if="statusOf(row)" :class="statusOf(row).status">{{ $t(statusOf(row).label) }}</b-tag>
              </td>
              <td v-else-if="c.field === 'sent'" :key="c.field" class="has-text-right" :class="bmTd('sent')">
                {{ cell(row, 'sent') }}
              </td>
              <!-- SMART-SENDING-SPEC D5: the recorded skips, "off" for a bypass, '—' for a journey. -->
              <td v-else-if="c.field === 'skipped'" :key="c.field" class="has-text-right" :class="bmTd('skipped')"
                :data-cy="`cm-skipped-${row.totals ? 'totals' : row.id}`">
                {{ cell(row, 'skipped') }}
              </td>
            </template>
            <td v-for="c in midCols" :key="c.field" class="has-text-right" :class="[bmTd(c.field), c.group]"
              :data-cy="`cm-${c.field}-${row.totals ? 'totals' : row.id}`">
              <span v-if="c.field === 'gp_rate'" :class="gpClass(row) || null" :title="gpTitle(row)">
                {{ row.totals ? '' : cell(row, 'gp_rate') }}</span>
              <span v-else-if="isConversion(c.field)" :title="conversionTip(c.field)">{{ conversionText(row, c.field) }}</span>
              <!-- GMAIL-GAP-SPEC D3/D4: the fraction or the rule (or why '—') on hover; the Gap
                   colored against the stored line (campaignMatrix.mjs::gapClass). -->
              <span v-else-if="isGap(c.field)" :class="c.field === 'gmail_gap' ? (gapClass(row) || null) : null"
                :title="gapTitle(row, c.field)">{{ cell(row, c.field) }}</span>
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
  CONVERSION_FIELDS, GAP_FAILED, GAP_FIELDS, GAP_PENDING, GAP_READY, brandCell, cellText, clickSort, conversionValue,
  dateCell, filterByName, gapClass, gapRequestIds, gapTip, gpClass, gpTip, groupHeaders, leftColumns, loadState,
  matrixChips, mergeGapCounts, midColumns, nameCell, saveState, shapeRows, showBrandColumn, sortForView, statusCell,
  tableRows,
} from '../campaignMatrix.mjs'; // eslint-disable-line import/extensions
import { conversionTooltip, scopedCell } from '../conversionCells.mjs'; // eslint-disable-line import/extensions
import { brandLabelOf } from '../brandScope.mjs'; // eslint-disable-line import/extensions
import { formerNameOf, langMatches } from '../naming.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';

export default Vue.extend({
  // D6: the brand selector reaches the server as list_id (scopeListIds); the first fetch waits for
  // the scope to resolve and a selection change refetches.
  mixins: [brandScopeMixin],

  data() {
    return {
      apiRows: [],
      conversions: null,
      // GMAIL-GAP-SPEC D2: the gmail_gap response's gap_thresholds (the ses document's stored line),
      // or null; that second request's state (pending | ready | failed); the matrix fetch counter
      // that drops a stale response.
      gapThresholds: null,
      gapState: GAP_READY,
      fetchSeq: 0,
      // The server's window ({ from, to } Mountain dates) for the note.
      period: {},
      loading: false,
      // The name filter's text (never persisted).
      nameQuery: '',
      // Sort, %/$ | # mode, kind and view (MATRIX-VIEWS-SPEC D2), per browser session
      // (sessionStorage key analytics.matrix).
      state: loadState(sessionStore(typeof window !== 'undefined' ? window : null)),
    };
  },

  computed: {
    ...mapGetters(['launch', 'langSelection']),
    ...mapState({ healthDocs: 'brandHealth' }),

    // D7: the GP S% column only for a brands:get holder (the server returns no GP otherwise).
    showGp() {
      return this.$can('brands:get');
    },

    // MATRIX-POLISH-SPEC D6: the Brand column unless the navbar selection is one brand.
    showBrand() {
      return showBrandColumn(this.brandSelection);
    },

    // MATRIX-VIEWS-SPEC D3: Date, Brand, Campaign, Status, Sent, Skipped (campaignMatrix.mjs::leftColumns).
    leftCols() {
      return leftColumns(this.showBrand);
    },

    // MATRIX-VIEWS-SPEC D2: Views, Clicks, Conversions (Tracked, Matched), then Health's Bounces
    // (Hard, Soft), Complaints (FBL, GP S%), Gmail gap (Gmail, Yahoo, Gap) or Attribution's Shop Link
    // Attribution (Founder, Self, Creator, Corp) -- campaignMatrix.mjs::midColumns.
    midCols() {
      return midColumns(this.showGp, this.state.view);
    },

    groupCols() {
      return groupHeaders(this.showGp, this.state.view);
    },

    tableRows() {
      const brands = this.brandRows;
      const rows = shapeRows(this.apiRows, {
        launch: this.launch,
        docs: this.healthDocs,
        brandLabel: (s) => brandLabelOf(s, brands),
        gapThresholds: this.gapThresholds,
        gapState: this.gapState,
      });
      // NAMING-STANDARD-SPEC D2: the language toggle, client-side (naming.mjs::langMatches, the
      // server's lang predicate -- a row with no language is always kept).
      const langRows = rows.filter((r) => langMatches(r.lang, this.langSelection));
      return tableRows(filterByName(langRows, this.nameQuery), this.state.sort, this.state.mode, {
        locale: (this.$i18n && this.$i18n.locale) || 'en',
        totalsLabel: this.$t('dashboard.matrix.total'),
      });
    },
  },

  methods: {
    // Group headers carry the plain title only (2026-11-07, user): the navbar's brand health sync
    // indicator is the one place run times show, so no header names a computed or refresh time.
    // The conversions tooltips still say when the document was computed.
    groupLabel(g) {
      return g.label ? this.$t(g.label) : '';
    },

    params() {
      const p = { kind: this.state.kind };
      if (this.scopeListIds) {
        p.list_id = this.scopeListIds;
      }
      return p;
    },

    fetchRows() {
      this.loading = true;
      this.fetchSeq += 1;
      const seq = this.fetchSeq;
      this.$api.getCampaignMatrix(this.params()).then((data) => {
        if (seq !== this.fetchSeq) return;
        this.apiRows = (data && Array.isArray(data.rows)) ? data.rows : [];
        this.conversions = (data && data.conversions) || null;
        this.period = (data && data.window) || {};
        this.fetchGap(seq);
      }).finally(() => {
        this.loading = false;
      });
    },

    // GMAIL-GAP-SPEC D2 (2026-11-06) -- the GMAIL GAP counts are a SECOND request, made after the
    // matrix renders: gmail_gap over the broadcast rows (journeys are never sent and keep null
    // counts), with an open-ended from (a broadcast's matrix numbers are everything recorded for it)
    // and to = now. The gap cells read '…' meanwhile and '—' (unavailable) on failure; the stored
    // line comes from this response only. A response for an older matrix fetch is dropped.
    fetchGap(seq) {
      this.gapThresholds = null;
      const ids = gapRequestIds(this.apiRows);
      if (!ids.length) {
        this.gapState = GAP_READY;
        return;
      }
      this.gapState = GAP_PENDING;
      // Quiet: no loading overlay over the table already on screen, no toast on failure.
      this.$api.getCampaignGmailGapQuiet({ id: ids, from: '1970-01-01T00:00:00Z', to: new Date().toISOString() }).then((resp) => {
        if (seq !== this.fetchSeq) return;
        const rows = this.apiRows.map((r) => ({ ...r }));
        this.gapThresholds = mergeGapCounts(rows, resp);
        this.apiRows = rows;
        this.gapState = GAP_READY;
      }).catch(() => {
        if (seq !== this.fetchSeq) return;
        this.gapState = GAP_FAILED;
      });
    },

    saveState() {
      saveState(sessionStore(window), this.state);
    },

    onKind() {
      this.saveState();
      this.fetchRows();
    },

    // MATRIX-VIEWS-SPEC D2: a sort on a column the view hides falls back to Date desc.
    onView() {
      this.state.sort = sortForView(this.state.sort, this.state.view, this.showGp);
      this.saveState();
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
      return cellText(row, field, this.state.mode, (n) => this.$utils.formatNumber(n), undefined, (k) => this.$t(k));
    },

    isConversion(field) {
      return CONVERSION_FIELDS.includes(field);
    },

    nameOf(row) {
      return nameCell(row.name);
    },

    // Fork (naming standard, NAMING-STANDARD-SPEC D1/D10; no brand chip since MATRIX-VIEWS-SPEC D4).
    chipsOf(row) {
      return matrixChips(row, { brandSelection: this.brandSelection, langSelection: this.langSelection });
    },

    // MATRIX-VIEWS-SPEC D4.
    brandOf(row) {
      return brandCell(row);
    },

    nameTitle(row) {
      const f = formerNameOf(row);
      return f ? this.$t('naming.formerName', { name: f }) : this.nameOf(row).title;
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

    isGap(field) {
      return GAP_FIELDS.includes(field);
    },

    gapClass(row) {
      return gapClass(row);
    },

    gapTitle(row, field) {
      const t = gapTip(row, field, (n) => this.$utils.formatNumber(n));
      return t ? this.$t(t.key, t.params) : null;
    },

    dateText(row) {
      return dateCell(row).text;
    },

    statusOf(row) {
      return statusCell(row);
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
    // MATRIX-VIEWS-SPEC D2: a stored sort on a column the stored view hides falls back to Date desc.
    this.state.sort = sortForView(this.state.sort, this.state.view, this.showGp);
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
