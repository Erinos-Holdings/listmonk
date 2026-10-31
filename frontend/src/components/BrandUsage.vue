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
               two lower header rows; a sub-group (No-purchase series, Post-purchase series, Skipped)
               puts its label over its leaves (Day 0 | 2d | 5d | 10d; Day 0 | 5d | 14d | 30d --
               LIFECYCLE-SERIES-SPEC S9, each leaf's hover its D5 description; FR ES DE IT). -->
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
                :class="[`bu-band-${c.band}`, { 'bu-start': c.first, 'bu-small': c.small, 'bu-substart': c.divide }]"
                :title="$t(c.help)" :data-cy="`usage-group-${c.key}`">
                {{ $t(c.label) }}
              </th>
              <th v-else :key="c.field" rowspan="2" class="has-text-centered bu-head bu-leafhead"
                :class="[buTh(c.field), `bu-band-${c.band}`, { 'bu-start': c.first, 'bu-substart': c.divide }]" :title="$t(meta(c.field).help)"
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
            <td class="has-text-centered bu-score" :class="buTd('score')">
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
              :class="[buTd(c.field), `bu-band-${c.band}`, { 'bu-start': c.first, 'bu-substart': c.divide }]">
              <!-- V2 D3: one circle per state, the stored count inside. 2026-10-30: EACH circle links
                   what it counts (brandUsage.mjs::circleLink) -- green the running campaigns / clean
                   steps, amber the rest; red never links; each carries its own accessible name. -->
              <span v-if="cell(row, c.field).circles.length" class="bu-circles" :data-cy="`usage-${c.field}-${row.brand}`">
                <component v-for="ci in cell(row, c.field).circles" :key="ci.color"
                  :is="circleTo(row, c.field, ci.color) ? 'router-link' : 'span'"
                  :to="circleTo(row, c.field, ci.color) ? { name: 'campaigns', query: circleTo(row, c.field, ci.color) } : null"
                  :class="['bu-circle', `bu-${ci.color}`]" :title="cell(row, c.field).tooltip"
                  :aria-label="circleAria(ci)" :data-cy="`usage-${c.field}-${row.brand}-${ci.color}`">{{ ci.n }}</component>
              </span>
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
              <!-- LIFECYCLE-SERIES-SPEC S9 -- after a series group's last leaf: one grey "Nd legacy"
                   chip per off-standard step (does not count toward the score), and one amber ⚠
                   per step with no parseable days (POLISH3 stage 2 F5); the hover carries the
                   legacy description or the parser error (brandUsage.mjs::cellBadges). -->
              <!-- USAGE-DECISIONS-SPEC D7: a legacy chip with stored campaigns opens them
                   (b.link, the Day 0 circle's { id, evergreen: 'all' } shape). -->
              <component v-for="(b, bi) in badges(row, c.field)" :key="`w-${bi}`"
                :is="b.link ? 'router-link' : 'span'"
                :to="b.link ? { name: 'campaigns', query: b.link } : null"
                :class="b.kind === 'legacy' ? 'tag is-light is-small bu-legacy ml-1' : 'bu-warn has-text-warning-dark ml-1'"
                :title="b.tooltip" :aria-label="b.tooltip"
                :data-cy="b.kind === 'legacy' ? `usage-legacy-${row.brand}` : `usage-npwarn-${row.brand}`">
                <span>{{ b.glyph }}</span>
              </component>
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
    <!-- V2 D5: the stoplight key, then the as-of line (Mountain time). The ZMA line was removed
         2026-10-30; a ZMA row still carries its ZMA tag and the dimmed cells' hover. -->
    <p class="is-size-7 has-text-grey mt-2 bu-legend" data-cy="usage-legend">
      <span class="bu-circle bu-green">1</span> {{ $t('dashboard.usage.legendRunning') }}
      <span class="bu-circle bu-amber">1</span> {{ $t('dashboard.usage.legendIdle') }}
      <span class="bu-circle bu-red">0</span> {{ $t('dashboard.usage.legendNone') }}
      · {{ $t('dashboard.usage.legendConfig') }}
    </p>
    <p class="is-size-7 has-text-grey" data-cy="usage-note">
      {{ runTime ? $t('dashboard.usage.noteAsOf', { date: runTime }) : $t('dashboard.usage.computed') }}
    </p>
    <!-- 2026-10-30: the journey guide -- each series' steps as a timeline (the leaf hovers' D5
         texts, brandUsage.mjs::seriesGuide), side by side with the divider at the table's
         No-purchase | Post-purchase line (measured, brandUsage.mjs::guideSplitX); the Re-engagement
         series (USAGE-REENGAGEMENT-SPEC D6) follows Post-purchase behind a second divider; stacked
         when either side lacks the room. -->
    <div ref="guide" class="bu-guide" :class="{ 'bu-guide-split': guideX !== null }"
      :style="guideX !== null ? { '--bu-guide-x': `${guideX}px`, '--bu-guide-steps': guideSteps } : null"
      data-cy="usage-guide">
      <div v-for="g in guide" :key="g.family" :class="['bu-guide-series', `bu-guide-${g.family}`]"
        :data-cy="`usage-guide-${g.family}`">
        <div class="bu-guide-head">
          <h4 class="bu-guide-title">{{ $t(g.title) }}</h4>
          <p class="bu-guide-subtitle">{{ $t(g.subtitle) }}</p>
        </div>
        <ol class="bu-guide-steps">
          <li v-for="s in g.steps" :key="s.days" class="bu-guide-step">
            <span class="bu-guide-day" :aria-label="`${$t('dashboard.usage.guideDay')} ${s.days}`">
              <small>{{ $t('dashboard.usage.guideDay') }}</small>{{ s.days }}
            </span>
            <div class="bu-guide-text">
              <p class="bu-guide-step-title">{{ s.title }}</p>
              <p v-if="s.quote" class="bu-guide-quote">“{{ s.quote }}”</p>
              <p class="bu-guide-body">{{ s.body }}</p>
            </div>
          </li>
        </ol>
      </div>
    </div>
    <!-- Fork (usage decisions, integrations USAGE-DECISIONS-SPEC D5) -- Marketing's decision record
         on the lifecycle items, under the guide: the No-purchase and Post-purchase items in the
         guide's two columns (the same measured split, stacked when the guide is), Both series full
         width. The fork STORES these (D1); every rule is the server's -- who may decide
         (can_decide), the current state and voiding. Text only through mustache interpolation, never raw HTML.
         Pure half: usageDecisions.mjs. -->
    <div v-if="decisionView" class="bu-decisions" :class="{ 'bu-decisions-split': guideX !== null }"
      :style="guideX !== null ? { '--bu-guide-x': `${guideX}px` } : null" data-cy="usage-decisions">
      <!-- 2026-10-30 polish: a centered title + the published line; the approver line at the bottom.
           The series columns carry no title (the guide above names them); Both series keeps its own. -->
      <div class="bu-decisions-head" data-cy="usage-decisions-head">
        <h4 class="bu-decisions-title">{{ $t('decisions.title') }}</h4>
        <p class="bu-decisions-meta">{{ decisionsPublished }}</p>
      </div>
      <div v-for="col in decisionColumns" :key="col.key" :class="['bu-decisions-col', `bu-decisions-${col.key}`]"
        :data-cy="`usage-decisions-${col.key}`">
        <h4 v-if="col.title" class="bu-guide-title">{{ col.title }}</h4>
        <!-- USAGE-REENGAGEMENT-SPEC D9/D10: the Curated Sunset Policy block -- the planned-policy line,
             then the stored non-decision table (usageDecisions.mjs::tableOf; never approvable) and its
             note in small italic, then the items as Both series renders them. -->
        <p v-if="col.planned" class="bu-decisions-meta bu-decisions-planned" data-cy="usage-decisions-planned">
          {{ col.planned }}
        </p>
        <div v-if="col.table" class="bu-decisions-table" data-cy="usage-decisions-table">
          <table class="table is-narrow">
            <thead>
              <tr>
                <th v-for="(h, hi) in col.table.columns" :key="hi">{{ h }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(r, ri) in col.table.rows" :key="ri">
                <td v-for="(v, vi) in r" :key="vi">{{ v }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="col.table.note" class="bu-decisions-table-note">{{ col.table.note }}</p>
        </div>
        <ul class="bu-decision-list">
          <li v-for="it in col.items" :key="it.key" class="bu-decision" :data-cy="`decision-${it.key}`">
            <!-- One block per text line; each line's lead (the first sentence, "Recommended:",
                 "NOTE:") in bold -- usageDecisions.mjs::textLines. Kept on one line: no template
                 whitespace inside a line. -->
            <p class="bu-decision-text">
              <span v-for="(l, li) in textLines(it.text)" :key="li" class="bu-decision-line"><strong
                v-if="li === 0 && itemNum(it)" class="bu-decision-n">{{ itemNum(it) }}.</strong><strong
                v-if="l.lead">{{ l.lead }}</strong>{{ l.rest }}</span>
            </p>
            <p :class="['bu-decision-state', `bu-decision-${stateOf(it).kind}`]" :data-cy="`decision-state-${it.key}`">
              {{ stateOf(it).text }}
            </p>
            <p v-if="stateOf(it).note" class="bu-decision-note">{{ stateOf(it).note }}</p>
            <div v-if="canDecide" class="bu-decision-controls">
              <template v-if="rejecting !== it.key">
                <b-button size="is-small" type="is-success" :loading="saving === it.key" :disabled="saving !== null"
                  :data-cy="`decision-accept-${it.key}`" @click="decide(it, 'accept')">
                  {{ $t('decisions.accept') }}
                </b-button>
                <b-button size="is-small" :disabled="saving !== null" :data-cy="`decision-reject-${it.key}`"
                  @click="startReject(it)">
                  {{ $t('decisions.reject') }}
                </b-button>
              </template>
              <template v-else>
                <b-input v-model="note" type="textarea" maxlength="2000" rows="2"
                  :placeholder="$t('decisions.notePlaceholder')" :data-cy="`decision-note-${it.key}`" />
                <div class="buttons mt-1">
                  <b-button size="is-small" type="is-danger" :loading="saving === it.key"
                    :disabled="!rejectReady || saving !== null" :data-cy="`decision-submit-${it.key}`"
                    @click="decide(it, 'reject')">
                    {{ $t('decisions.submit') }}
                  </b-button>
                  <b-button size="is-small" :disabled="saving !== null" @click="cancelReject">
                    {{ $t('decisions.cancel') }}
                  </b-button>
                </div>
              </template>
            </div>
            <details v-if="historyOf(it).rows.length" class="bu-decision-history" :data-cy="`decision-history-${it.key}`">
              <summary>{{ historyOf(it).summary }}</summary>
              <ol>
                <li v-for="(h, hi) in historyOf(it).rows" :key="hi" :class="{ 'bu-decision-voided': h.voided }">
                  {{ h.text }}<span v-if="h.mark" class="bu-decision-mark"> ({{ h.mark }})</span>
                  <span v-if="h.note" class="bu-decision-note">{{ h.note }}</span>
                </li>
              </ol>
            </details>
          </li>
        </ul>
      </div>
      <p class="bu-decisions-meta bu-decisions-approver" data-cy="usage-decisions-approver">{{ decisionsApprover }}</p>
    </div>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { sessionStore } from '../brandMatrix.mjs'; // eslint-disable-line import/extensions
import {
  SECTIONS, bodyColumns, cell as cellOf, cellBadges, cellLink, circleLink, circleName, clickSort, columnMeta, guideSplitX, headerLayout, latestAsOf,
  linkTip, loadState, mountainRunTime, saveState, seriesGuide, shapeRows, sortRows,
} from '../brandUsage.mjs'; // eslint-disable-line import/extensions
import {
  SECTION_SUNSET_POLICY, approverText, canSubmitReject, columnsOf, decisionBody, historyOf as decisionHistory, isStale,
  itemNumber, publishedText, replaceItem, showControls, stateLine, tableOf, textLines,
} from '../usageDecisions.mjs'; // eslint-disable-line import/extensions
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
      // The journey guide's divider x (px from the guide's left edge), or null = stacked.
      guideX: null,
      // USAGE-DECISIONS-SPEC D5 -- GET /api/usage/decisions (null hides the section), the item
      // whose Reject note is open, its note, and the item being saved.
      decisionView: null,
      rejecting: null,
      note: '',
      saving: null,
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

    // The body columns with the header's flags (band, section start, the in-section rule).
    columns() {
      return bodyColumns();
    },

    header() {
      return headerLayout();
    },

    guide() {
      return seriesGuide((k, p) => this.$t(k, p));
    },

    // The shared grid's step-row count: the longest series (4 today; Re-engagement's 2 take the
    // first two rows).
    guideSteps() {
      return Math.max(...this.guide.map((g) => g.steps.length));
    },

    // USAGE-DECISIONS-SPEC D5 -- the header, the columns (the guide's two titles, Both series full
    // width), and whether to show Accept / Reject (the server's can_decide only).
    decisionsPublished() {
      return publishedText(this.decisionView, (k, p) => this.$t(k, p));
    },

    decisionsApprover() {
      return approverText(this.decisionView, (k, p) => this.$t(k, p));
    },

    decisionColumns() {
      const c = columnsOf(this.decisionView);
      const cols = [
        { key: 'no-purchase', title: '', items: c.noPurchase },
        { key: 'post-purchase', title: '', items: c.postPurchase },
        { key: 'both', title: this.$t('decisions.both'), items: c.both },
      ];
      // USAGE-REENGAGEMENT-SPEC D9: the fourth block, full width below Both series (shown when the GET
      // carries its items or its table).
      const table = tableOf(this.decisionView, SECTION_SUNSET_POLICY);
      if (c.sunsetPolicy.length || table) {
        cols.push({
          key: SECTION_SUNSET_POLICY,
          title: this.$t('decisions.sunsetPolicy'),
          planned: this.$t('decisions.sunsetPolicyPlanned'),
          table,
          items: c.sunsetPolicy,
        });
      }
      return cols;
    },

    canDecide() {
      return showControls(this.decisionView);
    },

    rejectReady() {
      return canSubmitReject(this.note);
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

    // USAGE-DECISIONS-SPEC D4/D5 -- the decision record. A failed read hides the section.
    fetchDecisions() {
      this.$api.getUsageDecisions().then((data) => {
        this.decisionView = data && Array.isArray(data.items) ? data : null;
      }).catch(() => {
        this.decisionView = null;
      });
    },

    itemNum(it) {
      return itemNumber(it);
    },

    textLines(text) {
      return textLines(text);
    },

    stateOf(it) {
      return stateLine(it, (k, p) => this.$t(k, p));
    },

    historyOf(it) {
      return decisionHistory(it, (k, p) => this.$t(k, p));
    },

    startReject(it) {
      this.rejecting = it.key;
      this.note = '';
    },

    cancelReject() {
      this.rejecting = null;
      this.note = '';
    },

    // One decision. A 409 (the text changed since the page loaded) shows the stale message and
    // reloads the decisions; any other refusal shows the server's message.
    decide(it, decision) {
      if (this.saving !== null || (decision === 'reject' && !this.rejectReady)) {
        return;
      }
      this.saving = it.key;
      this.$api.postUsageDecision(it.key, decisionBody(it, decision, this.note)).then((item) => {
        this.decisionView = replaceItem(this.decisionView, item);
        this.cancelReject();
      }).catch((err) => {
        if (isStale(err)) {
          this.$utils.toast(this.$t('decisions.stale'), 'is-danger');
          this.cancelReject();
          this.fetchDecisions();
          return;
        }
        const msg = err && err.response && err.response.data && err.response.data.message;
        this.$utils.toast(msg || String(err), 'is-danger');
      }).finally(() => {
        this.saving = null;
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

    circleTo(row, field, color) {
      return circleLink(row, field, color);
    },

    // Each circle's own accessible name (its part, not the whole cell's tooltip). A Re-engagement
    // circle carries its own (brandUsage.mjs::reCircleName, USAGE-REENGAGEMENT-SPEC D3).
    circleAria(ci) {
      return ci.name || circleName(ci.color, ci.n, (k, p) => this.$t(k, p));
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

    // The guide's divider follows the table's No-purchase | Post-purchase line: the Post-purchase
    // group header's left edge, relative to the guide (the table container's scroll included).
    measureGuide() {
      const th = this.$el && this.$el.querySelector('[data-cy="usage-group-postPurchase"]');
      const { guide } = this.$refs;
      if (!th || !guide) {
        this.guideX = null;
        return;
      }
      const x = th.getBoundingClientRect().left - guide.getBoundingClientRect().left;
      this.guideX = guideSplitX(x, guide.clientWidth);
    },

    queueMeasure() {
      this.$nextTick(() => this.measureGuide());
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

    // The column widths follow the rows' content, so the divider is re-measured after each render.
    tableRows() {
      this.queueMeasure();
    },
  },

  created() {
    this.$root.$on('page.refresh', this.fetchRows);
  },

  destroyed() {
    this.$root.$off('page.refresh', this.fetchRows);
    window.removeEventListener('resize', this.queueMeasure);
    if (this.guideObserver) {
      this.guideObserver.disconnect();
    }
    const box = this.$el && this.$el.querySelector('.table-container');
    if (box) {
      box.removeEventListener('scroll', this.queueMeasure);
    }
  },

  mounted() {
    this.afterListsLoaded(() => this.fetchRows());
    this.fetchDecisions();
    window.addEventListener('resize', this.queueMeasure);
    const box = this.$el.querySelector('.table-container');
    if (box) {
      box.addEventListener('scroll', this.queueMeasure, { passive: true });
    }
    // A pane shown after mount (a hidden tab) or a resized table re-measures too.
    if (typeof ResizeObserver !== 'undefined') {
      this.guideObserver = new ResizeObserver(() => this.queueMeasure());
      this.guideObserver.observe(this.$el);
    }
    this.queueMeasure();
  },
});
</script>
