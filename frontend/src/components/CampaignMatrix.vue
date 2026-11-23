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
        <!-- Polish 2026-11-14: the filter caption -- the selected brand and language (filterBar.mjs). -->
        <caption v-if="$filterCaption()" class="filter-caption" data-cy="filter-caption">{{ $filterCaption() }}</caption>
        <thead>
          <!-- MATRIX-POLISH-SPEC D4/D5: no Views group (Views is one column); CONVERSIONS after
               Clicks, then the view's groups (campaignMatrix.mjs::groupHeaders, derived from the
               same column list as the header cells -- MATRIX-VIEWS-SPEC D2). -->
          <tr class="bm-groups">
            <!-- The user, 2026-11-21: SKIPPED is a group in this row beside CONVERSIONS (light red,
                 campaignMatrix.mjs::leftGroupHeaders -- spans derived from the columns). Its
                 expand/collapse box (the standard .expand-btn plus/minus, as on the Lists grid) sits
                 LEFT of the label, away from the sub-header's sort arrow. SIXTY-DAY-OPENS-SPEC D5:
                 component state, collapsed on every load; collapsed, the table is as wide as before. -->
            <th v-for="(g, i) in leftGroupCols" :key="`l${i}`" :colspan="g.span" :class="g.cls"
              :data-cy="g.expand ? 'cm-group-skipped' : null">
              <template v-if="g.expand">
                <button type="button" class="expand-btn cm-skip-box" data-cy="cm-skipped-expand"
                  :title="$t(skippedExpanded ? 'analytics.matrix.skippedCollapse' : 'analytics.matrix.skippedExpand')"
                  :aria-label="$t(skippedExpanded ? 'analytics.matrix.skippedCollapse' : 'analytics.matrix.skippedExpand')"
                  :aria-expanded="skippedExpanded ? 'true' : 'false'" @click.stop="toggleSkipped">
                  <b-icon :icon="skippedExpanded ? 'minus' : 'plus'" size="is-small" />
                </button>{{ $t(g.label) }}
              </template>
            </th>
            <th v-for="(g, i) in groupCols" :key="i" :colspan="g.span" :class="g.cls">{{ groupLabel(g) }}</th>
          </tr>
          <tr>
            <th v-for="c in leftCols" :key="c.field"
              :class="[bmTh(c.field), c.group || null, {
                'has-text-right': c.field === 'sent' || c.field === 'skipped' || isReason(c.field), 'has-text-centered': c.field === 'lang',
              }]"
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
          <!-- Fork (campaign mode, integrations CAMPAIGN-MODE-SPEC D7) -- in Campaign mode the rows are
               campaignGroups.mjs::groupedTableRows: group rows (a series, a set) with their numbers from
               the totals row's rule, children following when expanded (in-page state), indented. -->
          <tr v-for="row in tableRows" :key="row.totals ? 'totals' : (row.rowKey || row.id)"
            :class="[row.totals ? 'bm-totals' : '', { 'cm-group': row.group, 'cm-child': row.depth > 0 }]">
            <!-- MATRIX-VIEWS-SPEC D3: the left cells in leftCols' order (Date, Brand, Campaign, Status,
                 Sent, Skipped); MATRIX-POLISH-SPEC D6: no Brand column under a single-brand selection. -->
            <template v-for="c in leftCols">
              <td v-if="c.field === 'name'" :key="c.field" :class="bmTd('name')" class="cm-name"
                :style="row.depth > 0 ? { paddingLeft: `${0.75 + row.depth * 1.25}rem` } : null">
                <!-- Campaign mode (D7): a group row's expand box and its name (the newest member's). -->
                <template v-if="row.group">
                  <button type="button" class="expand-btn cm-group-box" :data-cy="`cm-group-toggle-${row.rowKey}`"
                    :aria-expanded="row.expanded ? 'true' : 'false'"
                    :aria-label="$t(row.expanded ? 'campaigns.group.collapse' : 'campaigns.group.expand')"
                    @click.stop="toggleGroup(row.rowKey)">
                    <b-icon :icon="row.expanded ? 'minus' : 'plus'" size="is-small" />
                  </button>
                  <span :title="nameOf(row).title">{{ nameOf(row).text }}</span>
                </template>
                <!-- D3: the campaign's own Campaign pane (the remount the URL change causes is intended).
                     MATRIX-POLISH-SPEC D7 (widened 2026-11-06): the first NAME_MAX characters, then '…';
                     the stored name on hover. -->
                <!-- Fork (naming standard, NAMING-STANDARD-SPEC D10) -- the former name on hover. No
                     chips (polish 2026-11-14: the language is the Lang column). -->
                <router-link v-else-if="!row.totals" :to="{ name: 'campaignAnalytics', query: { id: row.id } }"
                  :title="nameTitle(row)" :data-cy="`cm-name-${row.id}`">
{{ nameOf(row).text }}
</router-link>
              </td>
              <!-- PROMO-SERIES-SPEC D7: the promo Code and Step (blank for a journey and the totals row). -->
              <td v-else-if="c.field === 'promo_code' || c.field === 'promo_step'" :key="c.field" :class="bmTd(c.field)"
                :data-cy="`cm-${c.field}-${row.totals ? 'totals' : row.id}`">
                {{ promoOf(row, c.field) }}
              </td>
              <!-- Polish 2026-11-14: the language code in plain text, under All languages only. -->
              <td v-else-if="c.field === 'lang'" :key="c.field" class="has-text-centered" :class="bmTd('lang')"
                :data-cy="`cm-lang-${row.totals ? 'totals' : row.id}`">
                {{ langOf(row) }}
              </td>
              <!-- MATRIX-VIEWS-SPEC D4 (lowercase since polish 2026-11-14): the slug, the display name on hover. -->
              <td v-else-if="c.field === 'brand'" :key="c.field" :class="bmTd('brand')"
                :title="brandOf(row).title" :data-cy="`cm-brand-${row.totals ? 'totals' : row.id}`">
                {{ brandOf(row).text }}
              </td>
              <!-- The Started column (polish 2026-11-14): every campaign's start day. -->
              <td v-else-if="c.field === 'date'" :key="c.field" :class="bmTd('date')" :data-cy="`cm-date-${row.totals ? 'totals' : row.id}`">
                <template v-if="row.totals">{{ firstField === 'date' ? row.name : '' }}</template>
                <span v-else :title="dateTip(row)">{{ dateText(row) }}</span>
              </td>
              <!-- HOURLY-HEALTH-SPEC D2: the Campaigns page's status tag. Status is the first column
                   (polish 2026-11-14), so the totals row's label sits here (MATRIX-VIEWS S4-1). -->
              <td v-else-if="c.field === 'status'" :key="c.field" :class="bmTd('status')"
                :data-cy="`cm-status-${row.totals ? 'totals' : row.id}`">
                <template v-if="row.totals">{{ firstField === 'status' ? row.name : '' }}</template>
                <!-- Campaign mode (D8): a group row's status summary, e.g. "4 finished, 1 draft". -->
                <span v-else-if="row.group" class="is-size-7">{{ groupStatusText(row) }}</span>
                <b-tag v-else-if="statusOf(row)" :class="statusOf(row).status">{{ $t(statusOf(row).label) }}</b-tag>
              </td>
              <td v-else-if="c.field === 'sent'" :key="c.field" class="has-text-right" :class="bmTd('sent')">
                {{ cell(row, 'sent') }}
              </td>
              <!-- SMART-SENDING-SPEC D5: the recorded skips, "off" for a bypass, '—' for a journey. -->
              <td v-else-if="c.field === 'skipped'" :key="c.field" class="has-text-right bm-skip" :class="bmTd('skipped')"
                :data-cy="`cm-skipped-${row.totals ? 'totals' : row.id}`">
                {{ cell(row, 'skipped') }}
              </td>
              <!-- SIXTY-DAY-OPENS-SPEC D5: Look-back, Look-ahead, 60-day engaged and (PROMO-SERIES-SPEC
                   D5) Purchased (expanded only). -->
              <td v-else-if="isReason(c.field)" :key="c.field" class="has-text-right bm-skip" :class="bmTd(c.field)"
                :data-cy="`cm-${c.field}-${row.totals ? 'totals' : row.id}`">
                {{ cell(row, c.field) }}
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
              <!-- Campaign mode (D8): a group row's Views and Clicks are counted per campaign (hover). -->
              <span v-else :title="groupTip(row, c.field)">{{ cell(row, c.field) }}</span>
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
  CONVERSION_FIELDS, GAP_FAILED, GAP_FIELDS, GAP_PENDING, GAP_READY, REASON_FIELDS, brandCell, cellText, clickSort, conversionValue,
  dateCell, filterByName, gapClass, gapRequestIds, gapTip, gpClass, gpTip, groupHeaders, langCell, leftColumns, leftGroupHeaders,
  loadState,
  mergeGapCounts, midColumns, nameCell, promoCell, saveState, shapeRows, showBrandColumn, showLangColumn, sortForView,
  statusCell, tableRows,
} from '../campaignMatrix.mjs'; // eslint-disable-line import/extensions
import { conversionTooltip, scopedCell } from '../conversionCells.mjs'; // eslint-disable-line import/extensions
import { brandLabelOf } from '../brandScope.mjs'; // eslint-disable-line import/extensions
import { formerNameOf, langMatches } from '../naming.mjs'; // eslint-disable-line import/extensions
import { groupCellTip, groupedTableRows, statusSummaryText } from '../campaignGroups.mjs'; // eslint-disable-line import/extensions
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
      // SIXTY-DAY-OPENS-SPEC D5: Skipped by reason, expanded -- component state, never persisted.
      skippedExpanded: false,
      // Fork (campaign mode, integrations CAMPAIGN-MODE-SPEC D7) -- the expanded group rows' keys
      // (in-page only).
      expandedGroups: [],
      // Sort, %/$ | # mode, kind and view (MATRIX-VIEWS-SPEC D2), per browser session
      // (sessionStorage key analytics.matrix).
      state: loadState(sessionStore(typeof window !== 'undefined' ? window : null)),
    };
  },

  computed: {
    // GLOBAL-LANG-SPEC D1 -- langSelection is All while app.lang_enable is off.
    ...mapGetters(['launch', 'campaignMode']),
    ...mapGetters({ langSelection: 'langEffective' }),
    ...mapState({ healthDocs: 'brandHealth' }),

    // D7: the GP S% column only for a brands:get holder (the server returns no GP otherwise).
    showGp() {
      return this.$can('brands:get');
    },

    // MATRIX-POLISH-SPEC D6: the Brand column unless the navbar selection is one brand.
    showBrand() {
      return showBrandColumn(this.brandSelection);
    },

    // Polish 2026-11-14: Status, Started, Brand, Campaign, Lang (All languages only), Sent, Skipped
    // (campaignMatrix.mjs::leftColumns).
    leftCols() {
      return leftColumns(this.showBrand, this.showLang, this.skippedExpanded);
    },

    showLang() {
      return showLangColumn(this.langSelection);
    },

    // The totals row's label sits in whichever column is first.
    firstField() {
      return this.leftCols[0].field;
    },

    // MATRIX-VIEWS-SPEC D2: Views, Clicks, Conversions (Tracked, Matched), then Health's Bounces
    // (Hard, Soft), Complaints (FBL, GP S%), Gmail gap (Gmail, Yahoo, Gap) or Attribution's Shop Link
    // Attribution (Founder, Self, Creator, Corp) -- campaignMatrix.mjs::midColumns.
    midCols() {
      return midColumns(this.showGp, this.state.view);
    },

    leftGroupCols() {
      return leftGroupHeaders(this.showBrand, this.showLang, this.skippedExpanded);
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
      const locale = (this.$i18n && this.$i18n.locale) || 'en';
      const totalsLabel = this.$t('dashboard.matrix.total');
      // Fork (campaign mode, integrations CAMPAIGN-MODE-SPEC D7) -- bucketed by the server's keys
      // AFTER the language and launch filters, BEFORE the name filter (campaignGroups.mjs).
      if (this.campaignMode === 'groups') {
        return groupedTableRows(langRows, {
          nameQuery: this.nameQuery,
          sort: this.state.sort,
          mode: this.state.mode,
          locale,
          totalsLabel,
          expanded: new Set(this.expandedGroups),
        });
      }
      return tableRows(filterByName(langRows, this.nameQuery), this.state.sort, this.state.mode, {
        locale,
        totalsLabel,
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
      this.state.sort = sortForView(this.state.sort, this.state.view, this.showGp, this.showLang, this.skippedExpanded);
      this.saveState();
    },

    // SIXTY-DAY-OPENS-SPEC D5: a sort on a reason sub-column falls back to Started desc on collapse.
    toggleSkipped() {
      this.skippedExpanded = !this.skippedExpanded;
      this.state.sort = sortForView(this.state.sort, this.state.view, this.showGp, this.showLang, this.skippedExpanded);
      this.saveState();
    },

    // Fork (campaign mode, CAMPAIGN-MODE-SPEC D7/D8) -- a group row's expand box, status summary and
    // per-campaign hover.
    toggleGroup(key) {
      const i = this.expandedGroups.indexOf(key);
      if (i === -1) {
        this.expandedGroups.push(key);
      } else {
        this.expandedGroups.splice(i, 1);
      }
    },

    groupStatusText(row) {
      return statusSummaryText(row.statusSummary, (s) => this.$t(`campaigns.status.${s}`));
    },

    groupTip(row, field) {
      const t = groupCellTip(row, field);
      return t ? this.$t(t.key) : null;
    },

    // PROMO-SERIES-SPEC D7 -- the Code / Step cell text (campaignMatrix.mjs::promoCell).
    promoOf(row, field) {
      return promoCell(row, field);
    },

    isReason(field) {
      return REASON_FIELDS.includes(field);
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

    // Polish 2026-11-14: the Lang column replaces the language chip.
    langOf(row) {
      return langCell(row);
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

    // Fork (campaign mode, CAMPAIGN-MODE-SPEC D1/D7) -- the toggle re-fetches the view (the response
    // carries the keys in both modes) and collapses every group.
    campaignMode() {
      this.expandedGroups = [];
      this.fetchRows();
    },

    // Polish 2026-11-14 (review R5): a Lang sort falls back to Started desc when a language hides
    // the column.
    showLang() {
      this.state.sort = sortForView(this.state.sort, this.state.view, this.showGp, this.showLang, this.skippedExpanded);
      this.saveState();
    },
  },

  created() {
    // MATRIX-VIEWS-SPEC D2: a stored sort on a column the stored view hides falls back to Date desc.
    this.state.sort = sortForView(this.state.sort, this.state.view, this.showGp, this.showLang);
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
