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
      <table class="table is-hoverable is-narrow bu-table" data-cy="usage-table"
        :style="{ '--bu-line-pad': `${linePadPx}px` }">
        <thead>
          <!-- 2026-11-02 (user): the parent brand's band, above the header and out of the sort --
               the Loyalty Rewards block (this month and next, per language: EN over SKU, FR..IT over
               their Skipped columns; brandUsage.mjs::loyaltyLayout), the parent's row (the same
               BrandUsageRow as the body), then a gap. Shown on every brand selection the user's
               lists permit (fetchParent). Placeholder red 0s until the Loyalty journeys exist. -->
          <template v-if="parentRow">
            <template v-if="loyalty">
              <tr class="bu-lr" data-cy="usage-loyalty">
                <td :colspan="loyalty.lead" />
                <!-- Right-justified to the IT circle's right edge (alignLoyalty measures it). -->
                <td :colspan="loyalty.cells.length" class="bu-lr-title" :class="lrTitlePad">
                  <span class="bu-lr-title-text" :style="titleShift">{{ $t('dashboard.usage.loyaltyRewards') }}</span>
                </td>
                <td v-if="loyalty.tail" :colspan="loyalty.tail" />
              </tr>
              <tr class="bu-lr">
                <td :colspan="loyalty.lead" />
                <td v-for="(lc, li) in loyalty.cells" :key="lc.lang" class="has-text-centered bu-lr-lang"
                  :class="padClass(lc.col)">
                  <span :style="li === 0 ? enShift : null">{{ lc.lang.toUpperCase() }}</span>
                </td>
                <td v-if="loyalty.tail" :colspan="loyalty.tail" />
              </tr>
              <tr v-for="(m, mi) in loyaltyMonths" :key="m.key" class="bu-lr bu-lr-month"
                :class="{ 'bu-zma': parentRow.zma, 'bu-lr-last': mi === loyaltyMonths.length - 1 }"
                :data-cy="`usage-loyalty-${m.key}`">
                <!-- The label sits the EN label's vertical gap left of the EN circle (alignLoyalty). -->
                <td :colspan="loyalty.lead" class="bu-lr-label"><span class="bu-lr-label-text" :style="labelShift">{{ m.label }}</span></td>
                <td v-for="(lc, li) in loyalty.cells" :key="lc.lang" class="has-text-centered" :class="padClass(lc.col)">
                  <span v-for="ci in loyaltyCircles" :key="ci.color" :class="['bu-circle', 'bu-lr-circle', `bu-${ci.color}`]"
                    :style="li === 0 ? enShift : null" :title="`${lc.lang.toUpperCase()} · ${m.label}`"
                    :aria-label="`${lc.lang.toUpperCase()} ${m.label}: ${circleAria(ci)}`">{{ ci.n }}</span>
                </td>
                <td v-if="loyalty.tail" :colspan="loyalty.tail" />
              </tr>
            </template>
            <brand-usage-row :row="parentRow" :columns="columns" :sort-field="usage.sort.field" class="bu-parent" />
            <tr class="bu-parent-gap"><td :colspan="2 + columns.length" /></tr>
          </template>
          <!-- 2026-10-31 polish 2: every header cell carries the same --bu-line-pad sides as the
               body cells of its column(s) (brandUsage.mjs::headerLayout padLeft/padRight, the
               bu-pad-l / bu-pad-r classes), so each header stays centred over its content. -->
          <tr class="bu-groups">
            <th colspan="2" class="bu-pad-r" />
            <th v-for="s in header.row1" :key="s.key" :colspan="s.span"
              :class="['bu-group', 'bu-start', `bu-band-${s.band}`, padClass(s)]">
              {{ $t(s.label) }}
            </th>
          </tr>
          <!-- DASHBOARDS-POLISH3-SPEC D4/D5 -- brandUsage.mjs::headerLayout: plain leaves span the
               two lower header rows; a sub-group (No-purchase series, Post-purchase series, Skipped)
               puts its label over its leaves (Day 0 | 2d | 5d; Day 0 | 5d | 14d | 30d --
               LIFECYCLE-SERIES-SPEC S9, each leaf's hover its D5 description; FR ES DE IT). -->
          <tr>
            <th rowspan="2" class="bu-head" :class="buTh('score')" :title="$t('dashboard.usage.scoreHelp')"
              :aria-sort="buAria('score')" @click="onSort('score')">
              {{ $t('dashboard.usage.score') }}<span class="bm-arrow">{{ buArrow('score') }}</span>
            </th>
            <th rowspan="2" class="bu-head bu-pad-r" :class="buTh('brand')" :aria-sort="buAria('brand')" @click="onSort('brand')">
              {{ $t('dashboard.matrix.brand') }}<span class="bm-arrow">{{ buArrow('brand') }}</span>
            </th>
            <template v-for="c in header.row2">
              <th v-if="c.kind === 'group'" :key="`g-${c.key}`" :colspan="c.span"
                class="has-text-centered bu-head bu-subgroup"
                :class="[`bu-band-${c.band}`, padClass(c), { 'bu-start': c.first, 'bu-small': c.small, 'bu-substart': c.divide }]"
                :title="$t(c.help)" :data-cy="`usage-group-${c.key}`">
                {{ $t(c.label) }}
              </th>
              <th v-else :key="c.field" rowspan="2" class="has-text-centered bu-head bu-leafhead"
                :class="[buTh(c.field), `bu-band-${c.band}`, padClass(c), { 'bu-start': c.first, 'bu-substart': c.divide }]" :title="hover(c.field)"
                :aria-sort="buAria(c.field)" @click="onSort(c.field)">
                {{ label(c.field) }}<span class="bm-arrow">{{ buArrow(c.field) }}</span>
              </th>
            </template>
          </tr>
          <tr>
            <th v-for="c in header.row3" :key="c.field" class="bu-small has-text-centered"
              :class="[buTh(c.field), `bu-band-${c.band}`, padClass(c), { 'bu-start': c.first, 'bu-substart': c.groupStart }]"
              :title="hover(c.field)" :aria-sort="buAria(c.field)" @click="onSort(c.field)"
              :data-cy="`usage-head-${c.field}`">
              {{ label(c.field) }}<span class="bm-arrow">{{ buArrow(c.field) }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <brand-usage-row v-for="row in tableRows" :key="row.brand" :row="row" :columns="columns"
            :sort-field="usage.sort.field" />
          <tr v-if="!loading && !tableRows.length && !parentSelected">
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
      <span class="bu-circle bu-grey">1</span> {{ $t('dashboard.usage.legendDraft') }}
      <span class="bu-circle bu-red">0</span> {{ $t('dashboard.usage.legendNone') }}
      · {{ $t('dashboard.usage.legendConfig') }}
    </p>
    <p class="is-size-7 has-text-grey" data-cy="usage-note">
      {{ runTime ? $t('dashboard.usage.noteAsOf', { date: runTime }) : $t('dashboard.usage.computed') }}
    </p>
    <!-- 2026-10-30: the journey guide -- each series' steps as a timeline (the leaf hovers' D5
         texts, brandUsage.mjs::seriesGuide). 2026-10-31 polish: the three series side by side in
         left-justified list-box columns (brandUsage.mjs::guideColumnsFit), no longer aligned to the
         table's line; stacked when the width lacks the room. -->
    <!-- Fork (journey preview, integrations JOURNEY-PREVIEW-SPEC D2) -- the campaign editor's own
         Preview button (Editor.vue), right-aligned above the guide: click or F9 (bound while this pane
         is mounted) opens the journey preview page, carrying the global selector's brand when it is
         one brand (journeyPreview.mjs::usageTarget). -->
    <div class="bu-preview has-text-right" :style="guideSplit && fitWidthPx ? { width: `${fitWidthPx}px` } : null">
      <b-button @click="onPreview" type="is-primary" icon-left="file-find-outline" data-cy="usage-preview"
        aria-keyshortcuts="F9">
        <span class="has-kbd">{{ $t('campaigns.preview') }} <span class="kbd">F9</span></span>
      </b-button>
    </div>
    <div ref="guide" class="bu-guide" :class="{ 'bu-guide-split': guideSplit }"
      :style="guideSplit ? { '--bu-guide-steps': guideSteps, width: fitWidthPx ? `${fitWidthPx}px` : null } : null"
      data-cy="usage-guide">
      <div v-for="g in guide" :key="g.family" :class="['bu-guide-series', `bu-guide-${g.family}`]"
        :data-cy="`usage-guide-${g.family}`">
        <div class="bu-guide-head">
          <h4 class="bu-guide-title">{{ $t(g.title) }}</h4>
          <p class="bu-guide-subtitle">{{ $t(g.subtitle) }}</p>
        </div>
        <ol class="bu-guide-steps">
          <li v-for="s in g.steps" :key="s.days" class="bu-guide-step">
            <span class="bu-guide-day" :aria-label="`${$t('dashboard.usage.guideDay')} ${s.label}`" :title="s.hover || null">
              <small>{{ $t('dashboard.usage.guideDay') }}</small>{{ s.label }}
            </span>
            <div class="bu-guide-text">
              <p class="bu-guide-step-title">{{ s.primary }}<span v-if="s.secondary" class="bu-guide-step-sub">{{ s.secondary }}</span></p>
              <p v-if="s.quote" class="bu-guide-quote">“{{ s.quote }}”</p>
              <p class="bu-guide-body">{{ s.body }}</p>
            </div>
          </li>
        </ol>
      </div>
    </div>
    <!-- 2026-11-05 (user): the guide's evidence brief; 2026-11-08 (user): two more internal
         documentation pages beside it -- Kaylinn's AI copy direction for the journey drafts and the
         shop-link attribution standard. Static pages in frontend/public/docs, served to a logged-in
         user only by /admin/docs/:page (cmd/handlers.go AdminDoc), opened in a new tab. Figures on
         them are point-in-time; refreshing them is a fork release. -->
    <p class="bu-guide-sources" :style="guideSplit && fitWidthPx ? { width: `${fitWidthPx}px` } : null">
      <a href="/admin/docs/lifecycle-sources" target="_blank" rel="noopener" data-cy="usage-guide-sources">
        {{ $t('dashboard.usage.guideSources') }}
      </a>
      <span class="bu-guide-sep" aria-hidden="true">·</span>
      <a href="/admin/docs/ai-journey-campaigns" target="_blank" rel="noopener" data-cy="usage-guide-ai-journeys">
        {{ $t('dashboard.usage.guideAiJourneys') }}
      </a>
      <span class="bu-guide-sep" aria-hidden="true">·</span>
      <a href="/admin/docs/shop-link-attribution" target="_blank" rel="noopener" data-cy="usage-guide-shop-links">
        {{ $t('dashboard.usage.guideShopLinks') }}
      </a>
    </p>
    <!-- Fork (usage decisions, integrations USAGE-DECISIONS-SPEC D5) -- Marketing's decision record
         on the lifecycle items, under the guide's three columns: No-purchase and Post-purchase items
         under their series, the Curated Sunset Policy under the Re-engagement series, Both series
         across the first two (stacked when the guide is). The fork STORES these (D1); every rule is the server's -- who may decide
         (can_decide), the current state and voiding. Text only through mustache interpolation, never raw HTML.
         Pure half: usageDecisions.mjs. -->
    <div v-if="decisionView" class="bu-decisions" :class="{ 'bu-decisions-split': guideSplit }"
      :style="guideSplit && fitWidthPx ? { width: `${fitWidthPx}px` } : null"
      data-cy="usage-decisions">
      <!-- 2026-10-30 polish: a centered title + the published line; the approver line at the bottom.
           The series columns carry no title (the guide above names them); Both series keeps its own. -->
      <div class="bu-decisions-head" data-cy="usage-decisions-head">
        <h4 class="bu-decisions-title">{{ $t('decisions.title') }}</h4>
        <p class="bu-decisions-meta">{{ decisionsPublished }}</p>
      </div>
      <div v-for="col in decisionColumns" :key="col.key" :class="['bu-decisions-col', `bu-decisions-${col.key}`]"
        :data-cy="`usage-decisions-${col.key}`">
        <!-- 2026-10-31 polish 2 (user): the guide's series heading repeated at the top of its decision
             column -- the record sits far enough below the guide that the columns need their own. -->
        <div v-if="col.guide" class="bu-guide-head" :data-cy="`usage-decisions-guide-${col.key}`">
          <h4 class="bu-guide-title">{{ $t(col.guide.title) }}</h4>
          <p class="bu-guide-subtitle">{{ $t(col.guide.subtitle) }}</p>
        </div>
        <h4 v-if="col.title" class="bu-guide-title">{{ col.title }}</h4>
        <!-- USAGE-REENGAGEMENT-SPEC D9: the Curated Sunset Policy block -- the stored non-decision
             table (usageDecisions.mjs::tableOf; never approvable) and its note in small italic, then
             the items as Both series renders them. (D10's planned-policy line was struck by the user
             2026-10-31: the texts read as the policy being released.) -->
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
                 "NOTE:") in bold -- usageDecisions.mjs::textLines; from the "Recommended:" line on,
                 every line in red (2026-10-31 polish 2, user). Kept on one line: no template
                 whitespace inside a line. -->
            <p class="bu-decision-text">
              <span v-for="(l, li) in textLines(it.text)" :key="li" :class="['bu-decision-line', { 'bu-decision-recommended': l.recommended, 'bu-decision-para': paraOf(l, li) }]"><strong
                v-if="li === 0 && itemNum(it)" class="bu-decision-n">{{ itemNum(it) }}.</strong><strong
                v-if="l.lead">{{ l.lead }}</strong>{{ l.rest }}</span>
            </p>
            <p :class="['bu-decision-state', `bu-decision-${stateOf(it).kind}`]" :data-cy="`decision-state-${it.key}`">
              {{ stateOf(it).text }}
            </p>
            <p v-if="stateOf(it).note" class="bu-decision-note">{{ stateOf(it).note }}</p>
            <!-- Integrations SMART-SENDING-SPEC D11: the LAST changelog entry's callout, one small red
                 line (usageDecisions.mjs::calloutOf), plain text through interpolation. -->
            <p v-if="calloutOf(it)" class="bu-decision-callout" :data-cy="`decision-callout-${it.key}`">
              {{ $t('decisions.calloutLead') }}{{ calloutOf(it) }}
            </p>
            <div v-if="canDecide" class="bu-decision-controls">
              <!-- 2026-10-31 polish: one small neutral segmented toggle -- Current | Recommended |
                   Reject on an item with a "Recommended:" line, else Accept | Reject (the server's
                   `choices`); nothing selected until pressed, the pressed (or stored) choice blue.
                   Each button's hover says what it records (usageDecisions.mjs::choicesOf). -->
              <b-field :key="`${it.key}-${selectedOf(it)}-${toggleRev}`" class="segmented bu-decision-toggle"
                :data-cy="`decision-choices-${it.key}`">
                <!-- A radio fires no input when pressed while already selected, so a stored Reject
                     reopens its note through the native click (Buefy delivers a label click twice;
                     the second is a no-op: the note is already open). -->
                <b-radio-button v-for="ch in choicesOf(it)" :key="ch.value" :value="selectedOf(it)"
                  :native-value="ch.value" :name="`decision-${it.key}`" size="is-small" type="is-primary"
                  :title="ch.help" :disabled="saving !== null" :data-cy="`decision-${ch.value}-${it.key}`"
                  @input="pick(it, ch.value)" @click.native="repress(it, ch.value)">
                  {{ ch.label }}
                </b-radio-button>
              </b-field>
              <template v-if="rejecting === it.key">
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
            <!-- LIFECYCLE-DOCS-SPEC D7: every item has at least its publication entry, so the
                 expander always shows. -->
            <details class="bu-decision-history" :data-cy="`decision-history-${it.key}`">
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
    </div>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { sessionStore } from '../brandMatrix.mjs'; // eslint-disable-line import/extensions
import {
  GUIDE_COLUMN_REM, GUIDE_SERIES, bodyColumns, circleName, clickSort, columnMeta, computedAtText, guideColumnsFit, headerLayout, hoverText, linePad, pageWidth,
  loadState, saveState, seriesGuide, shapeRows, sortRows,
  LOYALTY_PLACEHOLDER, brandListIds, loyaltyLayout, loyaltyMonths, loyaltyShift, splitParent,
} from '../brandUsage.mjs'; // eslint-disable-line import/extensions
import { PARENT_BRAND } from '../brandScope.mjs'; // eslint-disable-line import/extensions
import BrandUsageRow from './BrandUsageRow.vue';
import {
  SECTION_SUNSET_POLICY, calloutOf, canSubmitReject, choicesOf as decisionChoices, columnsOf, decisionBody,
  historyOf as decisionHistory, isStale, itemNumber, publishedText, replaceItem, selectedChoice, showControls, stateLine,
  tableOf, textLines, opensParagraph,
} from '../usageDecisions.mjs'; // eslint-disable-line import/extensions
import { usageTarget } from '../journeyPreview.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';

export default Vue.extend({
  // Integrations CAMPAIGN-MATRIX-SPEC D1 -- the pane carries the scope mixin itself (see BrandMatrix.vue)
  // and renders off its own brand-matrix fetch.
  mixins: [brandScopeMixin],

  components: { BrandUsageRow },

  data() {
    return {
      apiRows: [],
      // 2026-11-02 (user): the parent brand's API row when the selection scopes it out of apiRows
      // (fetchParent); null otherwise.
      parentApiRow: null,
      // The Loyalty block's measured shifts (alignLoyalty): EN one step left of FR, the title's
      // right edge on the IT circle's.
      enShiftPx: 0,
      titleShiftPx: 0,
      labelShiftPx: 0,
      loading: false,
      // BRAND-USAGE-SPEC D7 -- the sort, per browser session (sessionStorage key dashboard.usage, kept;
      // V2 D2: a fresh session is Score descending).
      usage: loadState(sessionStore(typeof window !== 'undefined' ? window : null)),
      // The journey guide side by side (true) or stacked (false); the decision record follows it.
      guideSplit: false,
      // The px padded on each side of each of the table's vertical lines (fitTable).
      linePadPx: 0,
      // 2026-11-02 (user): the ONE page width — the table's natural width (or the guide's minimum
      // when the table is narrower). The guide and the decision record are set to it, so whatever
      // the table grows to, the documentation follows (fitTable); 0 = not yet measured.
      fitWidthPx: 0,
      // USAGE-DECISIONS-SPEC D5 -- GET /api/usage/decisions (null hides the section), the item
      // whose Reject note is open, its note, and the item being saved.
      decisionView: null,
      rejecting: null,
      // Bumped after every save attempt so the toggles redraw from the stored state.
      toggleRev: 0,
      note: '',
      saving: null,
    };
  },

  computed: {
    ...mapGetters(['launch']),

    // D2/D7 -- the matrix rows (same launch filter and display names), sorted. DASHBOARDS-POLISH2-SPEC
    // D2 -- the lists store (active, permitted lists) gives each row its brand list-id set.
    // 2026-11-02 (user): the parent brand's row is out of the sort (it renders in the band above
    // the header, parentRow).
    tableRows() {
      const rows = shapeRows(this.apiRows, {
        brands: this.brandRows, launch: this.launch, lists: this.$store.state.lists,
      });
      return sortRows(splitParent(rows).rest, this.usage.sort, (this.$i18n && this.$i18n.locale) || 'en');
    },

    // The parent brand's row on every selection, whatever the launch filter: from apiRows when the
    // selection includes it, else the separate fetch. null when the user's lists hold none of it.
    parentRow() {
      const source = this.apiRows.find((r) => r && r.brand === PARENT_BRAND) || this.parentApiRow;
      if (!source) {
        return null;
      }
      return shapeRows([source], { brands: this.brandRows, launch: 'all', lists: this.$store.state.lists })[0] || null;
    },

    parentSelected() {
      return this.brandSelection.kind === 'brand' && this.brandSelection.slug === PARENT_BRAND;
    },

    loyalty() {
      return loyaltyLayout(this.columns);
    },

    loyaltyMonths() {
      return loyaltyMonths(new Date(), (this.$i18n && this.$i18n.locale) || 'en');
    },

    loyaltyCircles() {
      return LOYALTY_PLACEHOLDER;
    },

    // The title cell pads like EN's left and IT's right, so its text box ends on IT's content box.
    lrTitlePad() {
      const c = this.loyalty.cells;
      return { 'bu-pad-l': !!c[0].col.padLeft, 'bu-pad-r': !!c[c.length - 1].col.padRight };
    },

    enShift() {
      return this.enShiftPx ? { transform: `translateX(${this.enShiftPx}px)` } : null;
    },

    titleShift() {
      return this.titleShiftPx ? { transform: `translateX(${this.titleShiftPx}px)` } : null;
    },

    labelShift() {
      return this.labelShiftPx ? { transform: `translateX(${this.labelShiftPx}px)` } : null;
    },

    // HOURLY-HEALTH-SPEC D5 -- the newest stored computedAt in Mountain time, null without one.
    runTime() {
      return computedAtText(this.tableRows);
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

    decisionColumns() {
      const c = columnsOf(this.decisionView);
      // 2026-10-31 polish 2: each series column repeats its guide heading (GUIDE_SERIES order:
      // No-purchase, Post-purchase, Re-engagement over the Curated Sunset Policy).
      const [npGuide, ppGuide, reGuide] = GUIDE_SERIES;
      const cols = [
        {
          key: 'no-purchase', title: '', guide: npGuide, items: c.noPurchase,
        },
        {
          key: 'post-purchase', title: '', guide: ppGuide, items: c.postPurchase,
        },
        { key: 'both', title: this.$t('decisions.both'), items: c.both },
      ];
      // USAGE-REENGAGEMENT-SPEC D9: the fourth block, under the Re-engagement series (shown when the
      // GET carries its items or its table).
      const table = tableOf(this.decisionView, SECTION_SUNSET_POLICY);
      if (c.sunsetPolicy.length || table) {
        cols.push({
          key: SECTION_SUNSET_POLICY,
          title: this.$t('decisions.sunsetPolicy'),
          guide: reGuide,
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
      this.fetchParent();
    },

    // 2026-11-02 (user): the parent brand's row when the selection scopes it out -- the same
    // endpoint narrowed to the parent's lists in the user's (permission-filtered) lists store; none
    // there, no band. Under All brands, or with the parent selected, apiRows carries it.
    fetchParent() {
      const ids = brandListIds(this.$store.state.lists, PARENT_BRAND);
      if (!this.scopeListIds || this.parentSelected || !ids.length) {
        this.parentApiRow = null;
        return;
      }
      this.$api.getDashboardBrandMatrix({ list_id: ids }).then((data) => {
        const rows = (data && Array.isArray(data.rows)) ? data.rows : [];
        this.parentApiRow = rows.find((r) => r && r.brand === PARENT_BRAND) || null;
      }).catch(() => {
        this.parentApiRow = null;
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

    // A later line renders as a paragraph unless it is a numbered sub-point (usageDecisions.mjs).
    paraOf(line, index) {
      return opensParagraph(line, index);
    },

    stateOf(it) {
      return stateLine(it, (k, p) => this.$t(k, p));
    },

    calloutOf(it) {
      return calloutOf(it);
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
        // Redraw the toggles from the stored state: a refused press never stays blue.
        this.toggleRev += 1;
      });
    },

    // 2026-10-31 polish: a toggle press. Reject opens the note (Reject shows selected while it is
    // written); every other choice (Accept / Current / Recommended) is recorded at once.
    pick(it, value) {
      if (value === 'reject') {
        this.startReject(it);
        return;
      }
      this.cancelReject();
      this.decide(it, value);
    },

    // A press on the STORED Reject (the radio emits no input then) reopens its note, so a new reject
    // row with a new note can still be recorded; any other re-press does nothing.
    repress(it, value) {
      if (value === 'reject' && this.saving === null && this.rejecting !== it.key && selectedChoice(it, null) === 'reject') {
        this.startReject(it);
      }
    },

    choicesOf(it) {
      return decisionChoices(it, (k, p) => this.$t(k, p));
    },

    selectedOf(it) {
      return selectedChoice(it, this.rejecting === it.key ? 'reject' : null, (k, p) => this.$t(k, p));
    },

    onSort(field) {
      this.usage.sort = clickSort(this.usage.sort, field);
      saveState(sessionStore(window), { sort: this.usage.sort });
    },

    meta(field) {
      return columnMeta(field);
    },

    label(field) {
      const m = columnMeta(field);
      return m.text !== undefined ? m.text : this.$t(m.label, m.params);
    },

    // LIFECYCLE-DOCS-SPEC D21: a leaf header's hover -- the translated help, then (an approximate
    // Post-purchase step) a newline and the real clock (brandUsage.mjs::hoverText).
    hover(field) {
      return hoverText(columnMeta(field), (k, p) => this.$t(k, p));
    },

    // The Loyalty placeholder circles' accessible name (the rows' own live in BrandUsageRow).
    circleAria(ci) {
      return ci.name || circleName(ci.color, ci.n, (k, p) => this.$t(k, p));
    },

    // 2026-10-31 polish 2: the line-pad sides, header and body alike
    // (brandUsage.mjs::headerLayout / bodyColumns).
    padClass(c) {
      return { 'bu-pad-l': !!c.padLeft, 'bu-pad-r': !!c.padRight };
    },

    buSorted(field) {
      return this.usage.sort.field === field;
    },

    buTh(field) {
      return ['bm-sort', { 'bu-sorted': this.buSorted(field) }];
    },

    buArrow(field) {
      if (!this.buSorted(field)) {
        return '';
      }
      return this.usage.sort.order === 'asc' ? '↑' : '↓';
    },

    // 2026-11-02 (user): ONE page width, owned here -- brandUsage.mjs::pageWidth: the WIDER of the
    // table's natural width (its measured width minus the current line pads) and the
    // documentation's standard width (three GUIDE_COLUMN_REM columns, capped by the container).
    // The table is padded UP to it along its group dividers (brandUsage.mjs::linePad -- the
    // padding at the secondary headers), and the guide and the decision record are set to exactly
    // that width (three equal columns of it, style.scss). So a table wider than the standard -- a
    // new leaf, a wider header -- widens the documentation by itself; nothing elsewhere has to be
    // kept in step. Stacked (narrow) layouts carry neither the width nor the pad.
    measureGuide() {
      this.$nextTick(() => this.fitTable(0));
    },

    fitTable(pass) {
      const table = this.$el && this.$el.querySelector('table.bu-table');
      // 2026-11-02: any brand row -- the parent band's row carries the same cells as the body's.
      const row = table && table.querySelector('tr.bu-row');
      if (!table || !row) {
        this.guideSplit = false;
        this.linePadPx = 0;
        this.fitWidthPx = 0;
        this.$nextTick(() => this.alignLoyalty());
        return;
      }
      const lines = row.querySelectorAll('td.bu-start, td.bu-substart').length;
      const measured = table.getBoundingClientRect().width;
      const natural = Math.max(0, measured - 2 * lines * this.linePadPx);
      const rem = parseFloat(window.getComputedStyle(document.documentElement).fontSize) || 16;
      const target = pageWidth(natural, GUIDE_SERIES.length * GUIDE_COLUMN_REM * rem, this.$el.clientWidth);
      this.guideSplit = guideColumnsFit(target);
      this.fitWidthPx = this.guideSplit ? Math.round(target) : 0;
      const next = this.guideSplit ? linePad(target, measured, this.linePadPx, lines) : 0;
      if (next !== this.linePadPx && pass < 3) {
        this.linePadPx = next;
        this.$nextTick(() => this.fitTable(pass + 1));
        return;
      }
      this.$nextTick(() => this.alignLoyalty());
    },

    // 2026-11-02 (user): the Loyalty block's two measured placements, after the table's widths
    // settle -- EN one even step left of FR (brandUsage.mjs::loyaltyShift; the SKU column it sits
    // over is wider and padded differently from the Skipped columns), and the title's right edge on
    // the IT circle's. Transforms only: no column width changes.
    alignLoyalty() {
      const first = this.$el && this.$el.querySelector('tr.bu-lr-month');
      const cs = first ? Array.from(first.querySelectorAll('.bu-lr-circle')) : [];
      if (cs.length < 3) {
        return;
      }
      const centers = cs.map((el) => {
        const r = el.getBoundingClientRect();
        return r.left + r.width / 2;
      });
      const prevEn = this.enShiftPx;
      const en = loyaltyShift(centers, prevEn);
      if (en !== this.enShiftPx) {
        this.enShiftPx = en;
      }
      // The month labels: right edge one gap left of the EN circle, the gap being the EN label's
      // vertical distance to its circle (2026-11-02, user). The EN circle is measured as rendered
      // (prevEn) and moved to its new shift; the label's own shift is taken out before measuring.
      const enLabel = this.$el.querySelector('td.bu-lr-lang span');
      const label = first.querySelector('.bu-lr-label-text');
      if (enLabel && label) {
        const enBox = cs[0].getBoundingClientRect();
        const gap = Math.max(0, enBox.top - enLabel.getBoundingClientRect().bottom);
        const enLeft = enBox.left + (en - prevEn);
        const shift = Math.round((enLeft - gap - (label.getBoundingClientRect().right - this.labelShiftPx)) * 10) / 10;
        if (shift !== this.labelShiftPx) {
          this.labelShiftPx = shift;
        }
      }
      const text = this.$el.querySelector('.bu-lr-title-text');
      if (text) {
        const itRight = cs[cs.length - 1].getBoundingClientRect().right;
        const shift = Math.round((itRight - (text.getBoundingClientRect().right - this.titleShiftPx)) * 10) / 10;
        if (shift !== this.titleShiftPx) {
          this.titleShiftPx = shift;
        }
      }
    },

    queueMeasure() {
      this.$nextTick(() => this.measureGuide());
    },

    // JOURNEY-PREVIEW-SPEC D2 -- the Preview button and its F9 shortcut (Editor.vue's pattern).
    onPreview() {
      this.$router.push(usageTarget(this.brandSelection));
    },

    onKeyboardShortcut(e) {
      if (e.key === 'F9') {
        this.onPreview();
        e.preventDefault();
      }
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

    parentRow() {
      this.queueMeasure();
    },
  },

  created() {
    this.$root.$on('page.refresh', this.fetchRows);
  },

  destroyed() {
    this.$root.$off('page.refresh', this.fetchRows);
    window.removeEventListener('resize', this.queueMeasure);
    window.removeEventListener('keydown', this.onKeyboardShortcut);
    if (this.guideObserver) {
      this.guideObserver.disconnect();
    }
  },

  mounted() {
    this.afterListsLoaded(() => this.fetchRows());
    this.fetchDecisions();
    window.addEventListener('resize', this.queueMeasure);
    window.addEventListener('keydown', this.onKeyboardShortcut);
    // A pane shown after mount (a hidden tab) or a resized table re-measures too.
    if (typeof ResizeObserver !== 'undefined') {
      this.guideObserver = new ResizeObserver(() => this.queueMeasure());
      this.guideObserver.observe(this.$el);
    }
    this.queueMeasure();
  },
});
</script>
