<template>
  <section class="campaigns">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4">
          {{ $t('globals.terms.campaigns') }}
          <span v-if="!isNaN(campaigns.total)">({{ campaigns.total }})</span>
        </h1>
      </div>
      <div class="column has-text-right">
        <b-field v-if="$can('campaigns:manage')" expanded>
          <b-button expanded :to="newCampaignRoute" tag="router-link" class="btn-new"
            type="is-primary" icon-left="plus" data-cy="btn-new">
            {{ $t('globals.buttons.new') }}
          </b-button>
        </b-field>
      </div>
    </header>

    <b-table :data="campaigns.results" :loading="loading.campaigns" :row-class="highlightedRow"
      @check-all="onTableCheck" @check="onTableCheck" :checked-rows.sync="bulk.checked" paginated backend-pagination
      pagination-position="both" @page-change="onPageChange" :current-page="queryParams.page"
      :per-page="campaigns.perPage" :total="campaigns.total" hoverable checkable backend-sorting @sort="onSort">
      <template #top-left>
        <!-- Fork (integrations JOURNEY-UX-SPEC A1.15) -- the scope toggle far left, then the list
             filter, then the search. -->
        <div class="columns">
          <div class="column is-narrow">
            <!-- Fork (evergreen) -- Broadcasts | Automations scope pill. Server-side filter;
                 switching resets the page and the bulk selection (bulk delete is scoped too).
                 Disabled while a fetch is in flight so two quick clicks cannot leave the table
                 showing one kind while the pill (and a bulk delete) is on the other. -->
            <!-- DASHBOARDS-POLISH3-SPEC D3 -- All | Broadcasts | Automations; the pill's value is the
                 scope KIND string (campaignsRouteQuery.mjs evergreenOfKind/kindOfEvergreen), All =
                 evergreen null = no evergreen param. -->
            <b-field class="segmented">
              <!-- G5: All first (the Launched/Unlaunched toggle's All-first order); a fresh session
                   still defaults to Broadcasts (campaignsRouteQuery.mjs::scopeKindPref). -->
              <b-radio-button v-model="scopeKind" native-value="all" type="is-primary"
                :disabled="loading.campaigns"
                data-cy="scope-all" @input="onScopeChange">
                {{ $t('campaigns.typeAll') }}
              </b-radio-button>
              <b-radio-button v-model="scopeKind" native-value="broadcasts" type="is-primary"
                :disabled="loading.campaigns"
                data-cy="scope-broadcasts" @input="onScopeChange">
                {{ $t('campaigns.typeBroadcasts') }}
              </b-radio-button>
              <b-radio-button v-model="scopeKind" native-value="automations" type="is-primary"
                :disabled="loading.campaigns"
                data-cy="scope-automations" @input="onScopeChange">
                {{ $t('campaigns.typeAutomations') }}
              </b-radio-button>
            </b-field>
          </div>
          <!-- DASHBOARDS-POLISH3-SPEC D2 -- the route status scope's chip (a Dashboard broadcast
               link's status=finished): dismissal, a real list pick, a brand-selector change or the
               scope pill clears it (campaignsRouteQuery.mjs::routeStatusAfter). -->
          <div v-if="queryParams.routeStatus" class="column is-narrow">
            <b-tag type="is-primary" size="is-medium" closable
              :title="$t('campaigns.statusScopeHelp')" data-cy="route-status"
              @close="onDismissStatus">
              {{ $t('campaigns.statusScope', { status: $t(`campaigns.status.${queryParams.routeStatus}`) }) }}
            </b-tag>
          </div>
          <div class="column is-narrow">
            <!-- Fork (list filter) -- server-side list_id filter on GET /api/campaigns.
                 Single-select (the API param is repeatable); "All lists" sends no param.
                 Deliberately NOT sticky, unlike the scope pill. Its options are the global brand's
                 lists (integrations GLOBAL-BRAND-SPEC D7); "All lists" then sends the scope.
                 DASHBOARDS-POLISH2-SPEC D2: a Dashboard link carrying SEVERAL list ids is the route
                 list scope, shown as the disabled "n linked lists" option; picking any real list
                 (or All lists) clears it (campaignsRouteQuery.mjs::selectList). -->
            <b-field>
              <b-select v-model="listSelect" :disabled="loading.campaigns"
                name="list_id" data-cy="filter-list">
                <option v-if="queryParams.routeListIDs" :value="routeScopeOption" disabled data-cy="filter-list-route">
                  {{ $tc('campaigns.linkedLists', queryParams.routeListIDs.length, { num: queryParams.routeListIDs.length }) }}
                </option>
                <option :value="null">{{ $t('campaigns.allLists') }}</option>
                <option v-for="l in filterLists" :key="l.id" :value="l.id">{{ l.name }}</option>
              </b-select>
            </b-field>
          </div>
          <div class="column is-6">
            <form @submit.prevent="getCampaigns">
              <div>
                <b-field>
                  <b-input v-model="queryParams.query" name="query" expanded
                    :placeholder="$t('campaigns.queryPlaceholder')" icon="magnify" ref="query" />
                  <p class="controls">
                    <b-button native-type="submit" type="is-primary" icon-left="magnify" />
                  </p>
                </b-field>
              </div>
            </form>
          </div>
        </div>

        <div class="actions" v-if="bulk.checked.length > 0">
          <a class="a" href="#" @click.prevent="deleteCampaigns" data-cy="btn-delete-campaigns">
            <b-icon icon="trash-can-outline" size="is-small" /> Delete
          </a>
          <span class="a">
            {{ $tc('globals.messages.numSelected', numSelectedCampaigns, { num: numSelectedCampaigns }) }}
            <!-- Fork (list filter) -- "select all N" selects the whole QUERY, and the
                 by-query bulk delete carries only query + evergreen, never list_id, so
                 offering it under a list filter would delete campaigns the page is not
                 showing. Page-level checkboxes stay available. The same holds under a global
                 brand (GLOBAL-BRAND-SPEC D7), whose scope is a list_id too, and under a
                 Dashboard link's route list scope (DASHBOARDS-POLISH2-SPEC D2) --
                 campaignsRouteQuery.mjs::canSelectAll. -->
            <span v-if="selectAllOffered">
              &mdash;
              <a href="#" @click.prevent="onSelectAll" data-cy="select-all-campaigns">
                {{ $tc('globals.messages.selectAll', campaigns.total, { num: campaigns.total }) }}
              </a>
            </span>
          </span>
        </div>
      </template>

      <b-table-column v-slot="props" cell-class="status" field="status" :label="$t('globals.fields.status')" width="10%"
        sortable :td-attrs="$utils.tdID" header-class="cy-status">
        <div>
          <p>
            <router-link :to="{ name: 'campaign', params: { id: props.row.id } }">
              <b-tag :class="props.row.status">
                {{ $t(`campaigns.status.${props.row.status}`) }}
              </b-tag>
              <!-- Fork (send retry, SEND-RETRY-SPEC D6) -- a finished broadcast that did not
                   reach every recipient is a warning, not a quiet "Finished". -->
              <b-tooltip v-if="isShortfall(props.row)" :label="$t('campaigns.shortfallHelp')" type="is-dark"
                multilined>
                <b-tag type="is-warning" class="shortfall" data-cy="tag-shortfall">
                  {{ $t('campaigns.shortfall', { sent: $utils.formatNumber(props.row.sent), toSend: $utils.formatNumber(props.row.toSend) }) }}
                </b-tag>
              </b-tooltip>
              <span class="spinner is-tiny" v-if="isRunning(props.row.id)">
                <b-loading :is-full-page="false" active />
              </span>
            </router-link>
          </p>
          <p v-if="isSheduled(props.row)">
            <span class="is-size-7 has-text-grey scheduled">
              <b-icon icon="alarm" size="is-small" />
              <span v-if="!isDone(props.row) && !isRunning(props.row)">
                {{ $utils.duration(new Date(), props.row.sendAt, true) }}
                <br />
              </span>
              {{ $utils.niceDate(props.row.sendAt, true) }}
            </span>
          </p>
        </div>
      </b-table-column>
      <b-table-column v-slot="props" field="name" :label="$t('globals.fields.name')" width="25%" sortable
        header-class="cy-name">
        <div>
          <p>
            <b-tag v-if="props.row.type === 'optin'" class="is-small">
              {{ $t('lists.optin') }}
            </b-tag>
            <!-- Fork (multi-language campaigns) -- absent lang is "All", one word everywhere. -->
            <b-tag v-if="serverConfig.lang_enabled || (props.row.attribs && props.row.attribs.lang)" class="is-small lang"
              data-cy="lang-chip" :title="langChipTitle(props.row)">
              {{ props.row.attribs && props.row.attribs.lang ? langChip(props.row.attribs.lang) : $t('campaigns.langAll') }}
            </b-tag>
            <router-link :to="{ name: 'campaign', params: { id: props.row.id } }">
              {{ props.row.name }}
              <copy-text :text="props.row.name" hide-text />
            </router-link>
          </p>
          <!-- Fork: the id leads the subject line (`#12 · <subject>`), the Lists page's sub-line
          treatment; the subject keeps its copy control. -->
          <p class="is-size-7 has-text-grey">
            <span data-cy="campaign-id">#{{ props.row.id }}</span> ·
            <copy-text :text="props.row.subject" />
          </p>
          <b-taglist>
            <b-tag class="is-small" v-for="t in props.row.tags" :key="t">
              {{ t }}
            </b-tag>
          </b-taglist>
        </div>
      </b-table-column>
      <b-table-column v-slot="props" cell-class="lists" field="lists" :label="$t('globals.terms.lists')" width="15%">
        <ul>
          <li v-for="l in props.row.lists" :key="l.id">
            <router-link :to="{ name: 'subscribers_list', params: { listID: l.id } }">
              {{ l.name }}
            </router-link>
          </li>
        </ul>
      </b-table-column>
      <b-table-column v-slot="props" field="created_at" :label="$t('campaigns.timestamps')" width="19%" sortable
        header-class="cy-timestamp">
        <div class="fields timestamps" :set="stats = getCampaignStats(props.row)">
          <p>
            <label for="#">{{ $t('globals.fields.createdAt') }}</label>
            <span>{{ $utils.niceDate(props.row.createdAt, true) }}</span>
          </p>
          <p v-if="stats.startedAt">
            <label for="#">{{ $t('campaigns.startedAt') }}</label>
            <span>{{ $utils.niceDate(stats.startedAt, true) }}</span>
          </p>
          <p v-if="isDone(props.row)">
            <label for="#">{{ $t('campaigns.ended') }}</label>
            <span>{{ $utils.niceDate(stats.updatedAt, true) }}</span>
          </p>
          <p v-if="stats.startedAt && stats.updatedAt" class="is-capitalized">
            <label for="#"><b-icon icon="alarm" size="is-small" /></label>
            <span>{{ $utils.duration(stats.startedAt, stats.updatedAt) }}</span>
          </p>
        </div>
      </b-table-column>

      <b-table-column v-slot="props" field="stats" :label="$t('campaigns.stats')" width="15%">
        <div class="fields stats" :set="stats = getCampaignStats(props.row)">
          <!-- Fork (list-page audience) -- a broadcast that has not started shows the LIVE
               count it would send to now (its lists, opt-in rules and language), not the
               stored to_send, which stays 0 until the send-time claim. Zero is a warning. -->
          <p v-if="hasAudience(props.row)" data-cy="stat-to-send">
            <label for="#">{{ $t('campaigns.toSend') }}</label>
            <b-tooltip :label="toSendHelp(props.row)" type="is-dark" multilined>
              <span :class="{ 'has-text-danger has-text-weight-bold': props.row.audience === 0 }">
                {{ $utils.formatNumber(props.row.audience) }}
              </span>
            </b-tooltip>
          </p>
          <p v-else>
            <label for="#">{{ $t('campaigns.sent') }}</label>
            <span v-if="props.row.evergreen">
              {{ $utils.formatNumber(stats.sent) }}
            </span>
            <span v-else>
              {{ $utils.formatNumber(stats.sent) }} /
              {{ $utils.formatNumber(stats.toSend) }}
            </span>
          </p>
          <!-- Fork (campaign list rates, CAMPAIGN-RATES-SPEC D5-D7) -- Views, Clicks and Bounces
               show the rate over Sent (props.row.sent, the same fetch as the counts) with the
               count in grey; with nothing sent the count stands alone and there is no hover. -->
          <p v-for="f in rateFields" :key="f.field" :set="cell = rateOf(props.row, f)">
            <label for="#">{{ $t(f.label) }}</label>
            <span :title="rateTip(props.row)">
              <router-link v-if="f.field === 'bounces'"
                :to="{ name: 'bounces', query: { campaign_id: props.row.id } }">
                <template v-if="cell.pct">
                  {{ cell.pct }}
                  <span class="is-size-7 has-text-grey">({{ cell.count }})</span>
                </template>
                <template v-else>{{ cell.count }}</template>
              </router-link>
              <template v-else-if="cell.pct">
                {{ cell.pct }}
                <span class="is-size-7 has-text-grey">({{ cell.count }})</span>
              </template>
              <template v-else>{{ cell.count }}</template>
            </span>
          </p>
          <p v-if="stats.rate">
            <label for="#"><b-icon icon="speedometer" size="is-small" /></label>
            <span class="send-rate">
              <b-tooltip
                :label="`${stats.netRate} / ${$t('campaigns.rateMinuteShort')} @ ${$utils.duration(stats.startedAt, stats.updatedAt)}`"
                type="is-dark">
                {{ stats.rate.toFixed(0) }} / {{ $t('campaigns.rateMinuteShort') }}
              </b-tooltip>
            </span>
          </p>
          <p v-if="isRunning(props.row.id) && !props.row.evergreen">
            <label for="#">
              {{ $t('campaigns.progress') }}
              <span class="spinner is-tiny">
                <b-loading :is-full-page="false" active />
              </span>
            </label>
            <span>
              <b-progress :value="stats.sent / stats.toSend * 100" size="is-small" />
            </span>
          </p>
        </div>
      </b-table-column>

      <b-table-column v-slot="props" cell-class="actions" width="15%" align="right">
        <div>
          <!-- Fork (campaign review, CAMPAIGN-INSPECT-SPEC §3.2) -- with the gate on, Start/Resume/
               Schedule are replaced by one "Open to inspect" glyph, OUTSIDE the campaigns:send block
               so Marketing sees it; the status endpoint would refuse an uninspected start anyway.
               A row the server marks review_passed (and a user who can send) gets its ordinary
               Start/Resume/Schedule glyph instead (LIST-START-GLYPH-SPEC D3, campaignGlyph.mjs). -->
          <router-link v-if="$can('campaigns:manage_all', 'campaigns:manage')
            && glyph(props.row) === 'inspect'" :to="{ name: 'campaign', params: { id: props.row.id } }"
            data-cy="btn-open-inspect" :aria-label="$t('campaigns.review.openToInspect')">
            <b-tooltip :label="$t('campaigns.review.openToInspect')" type="is-dark">
              <b-icon icon="creation" size="is-small" />
            </b-tooltip>
          </router-link>

          <!-- start / pause / resume / scheduled -->
          <template v-if="$can('campaigns:send')">
            <a v-if="glyph(props.row) === 'start'" href="#"
              @click.prevent="$utils.confirm(null, () => changeCampaignStatus(props.row, 'running'))"
              data-cy="btn-start" :aria-label="$t('campaigns.start')">
              <b-tooltip :label="$t('campaigns.start')" type="is-dark">
                <b-icon icon="rocket-launch-outline" size="is-small" />
              </b-tooltip>
            </a>

            <a v-if="canPause(props.row)" href="#"
              @click.prevent="$utils.confirm(null, () => changeCampaignStatus(props.row, 'paused'))" data-cy="btn-pause"
              :aria-label="$t('campaigns.pause')">
              <b-tooltip :label="$t('campaigns.pause')" type="is-dark">
                <b-icon icon="pause-circle-outline" size="is-small" />
              </b-tooltip>
            </a>

            <a v-if="glyph(props.row) === 'resume'" href="#"
              @click.prevent="$utils.confirm(null, () => changeCampaignStatus(props.row, 'running'))"
              data-cy="btn-resume" :aria-label="$t('campaigns.send')">
              <b-tooltip :label="$t('campaigns.send')" type="is-dark">
                <b-icon icon="rocket-launch-outline" size="is-small" />
              </b-tooltip>
            </a>

            <a v-if="glyph(props.row) === 'schedule'" href="#"
              @click.prevent="$utils.confirm($t('campaigns.confirmSchedule'), () => changeCampaignStatus(props.row, 'scheduled'))"
              data-cy="btn-schedule" :aria-label="$t('campaigns.schedule')">
              <b-tooltip :label="$t('campaigns.schedule')" type="is-dark">
                <b-icon icon="clock-start" size="is-small" />
              </b-tooltip>
            </a>

            <!-- placeholder for finished campaigns -->
            <a v-if="!canCancel(props.row) && !canSchedule(props.row) && !canStart(props.row)" href="#" data-disabled
              aria-label=" ">
              <b-icon icon="rocket-launch-outline" size="is-small" />
            </a>

            <a v-if="canCancel(props.row)" href="#"
              @click.prevent="$utils.confirm(null, () => changeCampaignStatus(props.row, 'cancelled'))"
              data-cy="btn-cancel" :aria-label="$t('globals.buttons.cancel')">
              <b-tooltip :label="$t('globals.buttons.cancel')" type="is-dark">
                <b-icon icon="cancel" size="is-small" />
              </b-tooltip>
            </a>
            <a v-else href="#" data-disabled aria-label=" ">
              <b-icon icon="cancel" size="is-small" />
            </a>
          </template>

          <a href="#" @click.prevent="previewCampaign(props.row)" data-cy="btn-preview"
            :aria-label="$t('campaigns.preview')">
            <b-tooltip :label="$t('campaigns.preview')" type="is-dark">
              <b-icon icon="file-find-outline" size="is-small" />
            </b-tooltip>
          </a>
          <a v-if="$can('campaigns:manage')" href="#" @click.prevent="$utils.prompt($t('globals.buttons.clone'),
            {
              placeholder: $t('globals.fields.name'),
              value: $t('campaigns.copyOf', { name: props.row.name }),
            },
            (name) => cloneCampaign(name, props.row))" data-cy="btn-clone" :aria-label="$t('globals.buttons.clone')">
            <b-tooltip :label="$t('globals.buttons.clone')" type="is-dark">
              <b-icon icon="file-multiple-outline" size="is-small" />
            </b-tooltip>
          </a>
          <router-link v-if="$can('campaigns:get_analytics')"
            :to="{ name: 'campaignAnalytics', query: { id: props.row.id } }">
            <b-tooltip :label="$t('globals.terms.analytics')" type="is-dark">
              <b-icon icon="chart-bar" size="is-small" />
            </b-tooltip>
          </router-link>
          <a v-if="$can('campaigns:manage')" href="#"
            @click.prevent="$utils.confirm($t('campaigns.confirmDelete', { name: props.row.name }), () => deleteCampaign(props.row))"
            data-cy="btn-delete" :aria-label="$t('globals.buttons.delete')">
            <b-icon icon="trash-can-outline" size="is-small" />
          </a>
        </div>
      </b-table-column>

      <template #empty v-if="!loading.campaigns">
        <empty-placeholder />
      </template>
    </b-table>

    <campaign-preview v-if="previewItem" type="campaign" :id="previewItem.id" :title="previewItem.name"
      @close="closePreview" />
  </section>
</template>

<script>
import dayjs from 'dayjs';
import Vue from 'vue';
import { mapState } from 'vuex';
import CampaignPreview from '../components/CampaignPreview.vue';
import CopyText from '../components/CopyText.vue';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import { isSendPlus, sendLangCode } from '../langs';
import { hasEnSplit, enSplit } from '../audience-box.mjs'; // eslint-disable-line import/extensions
import { rateCell, rateTipKey } from '../campaignRates.mjs'; // eslint-disable-line import/extensions
import { filterByIds } from '../brandScope.mjs'; // eslint-disable-line import/extensions
import { listGlyph } from '../campaignGlyph.mjs'; // eslint-disable-line import/extensions
import {
  LEGACY_SCOPE_PREF, ROUTE_SCOPE_OPTION, SCOPE_KIND_PREF, afterBrandChange, campaignsRouteQuery, canSelectAll,
  deleteEvergreenParam, displayedRunning, evergreenOfKind, kindOfEvergreen, kindStatusParams, listParam,
  listSelectValue, pollsStats, preEnablesEvergreen, routeStatusAfter, routeStatusOf, scopeKindPref, selectList,
} from '../campaignsRouteQuery.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';

// Fork (campaign list rates, CAMPAIGN-RATES-SPEC D5/D6) -- the rate cells, in column order after
// Sent / To send. Bounces keep two decimals (the Dashboard's bounce digits).
const RATE_FIELDS = [
  { field: 'views', label: 'campaigns.views', digits: 1 },
  { field: 'clicks', label: 'campaigns.clicks', digits: 1 },
  { field: 'bounces', label: 'globals.terms.bounces', digits: 2 },
];

export default Vue.extend({
  components: {
    CampaignPreview,
    EmptyPlaceholder,
    CopyText,
  },

  // Fork (global brand, integrations GLOBAL-BRAND-SPEC D4/D7).
  mixins: [brandScopeMixin],

  data() {
    // Fork (brand usage, integrations BRAND-USAGE-V2-SPEC D4/I8; DASHBOARDS-POLISH-SPEC D2/I3;
    // DASHBOARDS-POLISH2-SPEC D2/I2) -- the Dashboard's Brand Usage links land here with
    // ?list_id=<id>&evergreen=true (welcome / trigger cells), or with the brand's active list ids
    // as a repeated list_id and evergreen=true (Journeys) / evergreen=false (broadcast cells).
    // One id preselects the list; several are the route list scope (routeListIDs). Exactly "true"
    // forces Automations and exactly "false" forces Broadcasts for this visit; anything else keeps
    // the pref. Applied to the query state only: never written to the sticky
    // campaigns.scopeAutomations pref (a link must not flip it), nor to any other pref.
    // DASHBOARDS-POLISH3-SPEC D3 -- the standing kind is the string pref campaigns.scopeKind (the
    // legacy boolean read only while it is unset, never written again); a route evergreen=all
    // forces All (evergreen null) for this visit. D2 -- a route status (enum-validated) is the
    // route status scope, in-page state only.
    const fromRoute = campaignsRouteQuery(
      this.$route && this.$route.query,
      evergreenOfKind(scopeKindPref(this.$utils.getPref(SCOPE_KIND_PREF), this.$utils.getPref(LEGACY_SCOPE_PREF))),
    );
    return {
      previewItem: null,
      rateFields: RATE_FIELDS,
      queryParams: {
        page: 1,
        query: '',
        orderBy: 'created_at',
        order: 'desc',
        // Fork (evergreen) -- false = Broadcasts (default), true = Automations, null = All
        // (POLISH3 D3). Sticky across refresh/navigation via the shared localStorage pref (a route
        // query's evergreen=true / false / all selects Automations / Broadcasts / All for this
        // visit only).
        evergreen: fromRoute.evergreen,
        // DASHBOARDS-POLISH3-SPEC D2 -- the route status scope (one campaign status, or null).
        routeStatus: routeStatusOf(this.$route && this.$route.query),
        // Fork (list filter) -- null = all lists (no param sent). In-page state only; a route
        // query's single list_id preselects it.
        listID: fromRoute.listID,
        // DASHBOARDS-POLISH2-SPEC D2 -- the route list scope (a link's several list ids), or
        // null. In-page state only, never a pref; cleared by any real list selection and by any
        // brand-selector change (campaignsRouteQuery.mjs).
        routeListIDs: fromRoute.routeListIDs,
      },
      routeScopeOption: ROUTE_SCOPE_OPTION,
      pollID: null,
      campaignStatsData: {},

      // Table bulk row selection states.
      bulk: {
        checked: [],
        all: false,
      },
    };
  },

  methods: {
    // Fork (langs.js, the "+" convention). The chip names a send audience: EN+.
    langChip(lang) {
      return sendLangCode(lang);
    },

    langChipTitle(row) {
      return row.attribs && isSendPlus(row.attribs.lang) ? this.$t('langs.sendPlusHelp') : null;
    },

    // Campaign statuses.
    canStart(c) {
      return c.status === 'draft' && !c.sendAt;
    },
    // Fork (campaign review, LIST-START-GLYPH-SPEC D3): the row's start-side glyph -- 'inspect',
    // 'start', 'resume', 'schedule' or 'none'. The row is camelCased by the API client.
    glyph(c) {
      return listGlyph(
        {
          status: c.status, type: c.type, send_at: c.sendAt, review_passed: c.reviewPassed,
        },
        this.serverConfig.review_enabled,
        this.$can('campaigns:send'),
      );
    },
    canSchedule(c) {
      return c.status === 'draft' && c.sendAt;
    },
    canPause(c) {
      return c.status === 'running';
    },
    canCancel(c) {
      return c.status === 'running' || c.status === 'paused';
    },
    isSheduled(c) {
      return c.status === 'scheduled' || c.sendAt !== null;
    },
    isDone(c) {
      return c.status === 'finished' || c.status === 'cancelled';
    },
    // Fork (list-page audience). The server decides which rows carry `audience`
    // (core.wantsAudience -- not-yet-started broadcasts); null means "no estimate", never zero.
    hasAudience(c) {
      return c.audience !== null && c.audience !== undefined;
    },

    // Fork (campaign list rates, CAMPAIGN-RATES-SPEC D5) -- a Views/Clicks/Bounces cell's
    // { pct, count }, the rate over the row's own sent (null with nothing sent).
    rateOf(c, f) {
      return rateCell(c[f.field], c.sent, f.digits, (n) => this.$utils.formatNumber(n));
    },

    // D7 -- the hover names the denominator and the counting mode; none when there is no rate.
    rateTip(c) {
      const unique = !!(this.serverConfig.privacy && this.serverConfig.privacy.individual_tracking);
      const key = rateTipKey(unique, c.sent);
      return key ? this.$t(key, { sent: this.$utils.formatNumber(c.sent) }) : null;
    },

    // An English audience with no-language rows hovers as the Lists grid's EN+ split
    // ("n en + m no language"); every other campaign keeps the plain To send text.
    toSendHelp(c) {
      const lang = (c.attribs && c.attribs.lang) || '';
      if (hasEnSplit(lang, 'audience', c.audienceNoLang)) {
        const parts = enSplit(c.audience, c.audienceNoLang);
        return this.$t('lists.grid.enSplit', {
          en: this.$utils.formatNumber(parts.en),
          none: this.$utils.formatNumber(parts.none),
        });
      }
      return this.$t('campaigns.toSendHelp');
    },

    // Fork (send retry, SEND-RETRY-SPEC D6). to_send is recomputed from live list
    // membership per scan tick, so this also flags a mid-send list edit; the tooltip says so.
    isShortfall(c) {
      return c.status === 'finished' && !c.evergreen && c.toSend > 0 && c.sent < c.toSend;
    },

    isRunning(id) {
      if (id in this.campaignStatsData) {
        return true;
      }
      return false;
    },

    highlightedRow(data) {
      if (data.status === 'running') {
        return ['running'];
      }
      return '';
    },

    onPageChange(p) {
      this.queryParams.page = p;
      this.refreshCampaigns();
    },

    onSort(field, direction) {
      this.queryParams.orderBy = field;
      this.queryParams.order = direction;
      this.refreshCampaigns();
    },

    // Campaign actions.
    previewCampaign(c) {
      this.previewItem = c;
    },

    closePreview() {
      this.previewItem = null;
    },

    // Fork (evergreen) -- the scope pill. Resets the page and clears any bulk
    // selection (a stale selection would carry rows of the other kind), then refetches.
    // DASHBOARDS-POLISH3-SPEC D3 -- writes the string pref campaigns.scopeKind (the legacy
    // boolean is never written again); D2 -- a scope change clears the route status scope.
    onScopeChange() {
      this.$utils.setPref(SCOPE_KIND_PREF, kindOfEvergreen(this.queryParams.evergreen));
      this.queryParams.routeStatus = routeStatusAfter(this.queryParams.routeStatus, 'scope');
      this.queryParams.page = 1;
      this.bulk.checked = [];
      this.bulk.all = false;
      this.refreshCampaigns();
    },

    // DASHBOARDS-POLISH3-SPEC D2 -- the status chip's dismissal: the scope clears, the list-change
    // hygiene and refetch run. Never a pref.
    onDismissStatus() {
      this.queryParams.routeStatus = routeStatusAfter(this.queryParams.routeStatus, 'dismiss');
      this.onListChange();
    },

    // Fork (list filter) -- same selection hygiene as the scope pill: a stale selection
    // would carry rows that are no longer shown.
    onListChange() {
      this.queryParams.page = 1;
      this.bulk.checked = [];
      this.bulk.all = false;
      this.refreshCampaigns();
    },

    // Anything that changes WHICH rows are displayed, or that the user invokes to get
    // current data, must go through here. getCampaigns() alone leaves the poll in
    // whatever state the previous view left it: stopped (a running campaign then sits
    // frozen) or running against rows that are gone. Keeping the pair in one method is
    // what stops the next handler from forgetting half of it.
    refreshCampaigns() {
      this.getCampaigns();
      this.pollStats();
    },

    getCampaigns() {
      const params = {
        page: this.queryParams.page,
        query: this.queryParams.query.replace(/[^\p{L}\p{N}\s]/gu, ' '),
        order_by: this.queryParams.orderBy,
        order: this.queryParams.order,
        // POLISH3 D2/D3 -- evergreen omitted under All; status only under the route status scope.
        ...kindStatusParams(this.queryParams),
        no_body: true,
      };

      // Fork (list filter) -- "All lists" sends no param at all. Fork (global brand,
      // GLOBAL-BRAND-SPEC D7) -- under a brand "All lists" sends the brand's list ids; a chosen
      // list (inside the scope by construction) sends that one id. DASHBOARDS-POLISH2-SPEC D2 --
      // a route list scope takes the chosen list's slot (it beats the brand scope).
      const listID = listParam({
        listID: this.queryParams.listID,
        routeListIDs: this.queryParams.routeListIDs,
        scopeListIds: this.scopeListIds,
      });
      if (listID !== undefined) {
        params.list_id = listID;
      }

      this.$api.getCampaigns(params);
    },

    // Stats returns the campaign object with stats (sent, toSend etc.)
    // if there's live stats available for running campaigns. Otherwise,
    // it returns the incoming campaign object that has the static stats
    // values.
    getCampaignStats(c) {
      if (c.id in this.campaignStatsData) {
        return this.campaignStatsData[c.id];
      }
      return c;
    },

    pollStats() {
      // Clear any running status polls.
      clearInterval(this.pollID);

      // Fork (evergreen) -- Automations are never polled. An evergreen campaign runs
      // indefinitely by design, so polling it is an unbounded 1/sec request stream for
      // a counter nobody watches live. The Automations tab shows the state as of the
      // last fetch: switching to the tab, refreshing, or acting on a campaign refetches.
      // POLISH3 D3 (stage 2 F2) -- the poll runs whenever broadcasts are displayed: Broadcasts
      // and All (campaignsRouteQuery.mjs::pollsStats).
      if (!pollsStats(this.queryParams.evergreen)) {
        return;
      }

      // Poll for the status as long as the import is running.
      this.pollID = setInterval(() => {
        this.$api.getCampaignStats().then((data) => {
          // A row rendered as "running" that the server no longer reports as running
          // is stale -- the campaign finished, paused or was cancelled since the last
          // list fetch -- so refetch to pick up its final status and counts.
          //
          // Keyed on the DISPLAYED rows rather than on the response becoming empty,
          // for two reasons. An evergreen campaign runs indefinitely, so with one
          // active the response is never empty and the old empty-response branch
          // never fired, leaving every finished broadcast stuck on "running" until a
          // manual reload. And a short campaign can finish between two polls without
          // ever appearing in one, so keying on ids leaving the response would miss
          // it. Both sides read campaigns.status from the DB
          // (GetRunningCampaignStats -> get-campaign-status), so this is never a race
          // against a still-registering campaign: it is true only when the list is
          // genuinely behind, and it clears after one refetch. Ids are compared as
          // strings because the response carries numbers.
          const runningIDs = new Set(data.map((c) => String(c.id)));
          const stale = (this.campaigns.results || []).some(
            (c) => c.status === 'running' && !runningIDs.has(String(c.id)),
          );

          // Turn the list of campaigns [{id: 1, ...}, {id: 2, ...}] into
          // a map indexed by the id: {1: {}, 2: {}}. Empty when nothing is running.
          this.campaignStatsData = data.reduce((obj, cur) => ({ ...obj, [cur.id]: cur }), {});

          // Guarded on the in-flight flag: `stale` is computed from campaigns.results,
          // which only updates when the fetch resolves, so an unguarded call would
          // stack another refetch on every poll until it did.
          // getCampaigns(), never refreshCampaigns(): this runs INSIDE the poll, and
          // the wrapper would restart the interval on every tick.
          if (stale && !this.loading.campaigns) {
            this.getCampaigns();
          }

          // Stop polling once nothing ON SCREEN is running. Checking the displayed rows
          // rather than `data.length === 0` is what makes this terminate at all: the
          // response carries EVERY running campaign, including evergreens that never
          // finish, so an empty-response condition is never met while an automation is
          // live. The Broadcasts tab filters evergreens out server-side, so once the
          // refetch above lands the finished status, no displayed row is running and the
          // poll ends -- on an idle tab, after a single request. POLISH3 D3: under All the
          // displayed running EVERGREENS are not followed (they never finish), so the same rule
          // ends the poll there too (campaignsRouteQuery.mjs::displayedRunning).
          if (!displayedRunning(this.campaigns.results)) {
            clearInterval(this.pollID);
          }
        }, () => {
          clearInterval(this.pollID);
        });
      }, 1000);
    },

    changeCampaignStatus(c, status) {
      this.$api.changeCampaignStatus(c.id, status).then((d) => {
        this.$utils.toast(this.$t('campaigns.statusChanged', { name: c.name, status }));
        this.$utils.showWarnings(d.warnings);
        this.refreshCampaigns();
      });
    },

    async cloneCampaign(name, c) {
      // Fetch the template body from the server.
      let body = '';
      let bodySource = null;
      await this.$api.getCampaign(c.id).then((data) => {
        body = data.body;
        bodySource = data.bodySource;
      });

      const now = this.$utils.getDate();
      const sendLater = !!c.sendAt;
      let sendAt = null;
      if (sendLater) {
        sendAt = dayjs(c.sendAt).isAfter(now) ? c.sendAt : now.add(7, 'day');
      }

      const data = {
        name,
        subject: c.subject,
        lists: c.lists.map((l) => l.id),
        type: c.type,
        from_email: c.fromEmail,
        content_type: c.contentType,
        messenger: c.messenger,
        tags: c.tags,
        template_id: c.templateId,
        body,
        body_source: bodySource,
        altbody: c.altbody,
        headers: c.headers,
        // attribs carry the preheader; without them a clone silently loses it.
        attribs: c.attribs,
        send_later: sendLater,
        send_at: sendAt,
        archive: c.archive,
        archive_template_id: c.archiveTemplateId,
        archive_meta: c.archiveMeta,
        media: c.media.map((m) => m.id),
        // Fork (evergreen) -- a clone keeps the flag; starting it while the original runs
        // is refused server-side (same list + same delay).
        evergreen: !!c.evergreen,
        send_delay_secs: c.sendDelaySecs || 0,
      };

      if (c.archive) {
        data.archive_slug = `${name.toLowerCase().replace(/[^a-z0-9]/g, '-')}-${Date.now().toString().slice(-4)}`;
      }

      this.$api.createCampaign(data).then((d) => {
        this.$router.push({ name: 'campaign', params: { id: d.id } });
      });
    },

    deleteCampaign(c) {
      this.$api.deleteCampaign(c.id).then(() => {
        this.refreshCampaigns();
        this.$utils.toast(this.$t('globals.messages.deleted', { name: c.name }));
      });
    },

    // Mark all campaigns in the query as selected.
    onSelectAll() {
      this.bulk.all = true;
    },

    onTableCheck() {
      // Disable bulk.all selection if there are no rows checked in the table.
      if (this.bulk.checked.length !== this.campaigns.total) {
        this.bulk.all = false;
      }
    },

    deleteCampaigns() {
      const name = this.$tc('globals.terms.campaign', this.numSelectedCampaigns);

      const fn = () => {
        const params = {};
        if (!this.bulk.all && this.bulk.checked.length > 0) {
          // If 'all' is not selected, delete campaigns by IDs.
          params.id = this.bulk.checked.map((c) => c.id);
        } else {
          // 'All' is selected, delete by query.
          params.query = this.queryParams.query.replace(/[^\p{L}\p{N}\s]/gu, ' ');
          params.all = this.bulk.all;
          // Fork (evergreen) -- scope the by-query delete to the kind shown on the page; POLISH3
          // D3: All omits the param, matching exactly the both-kinds view on screen.
          Object.assign(params, deleteEvergreenParam(this.queryParams.evergreen));
        }

        this.$api.deleteCampaigns(params)
          .then(() => {
            this.refreshCampaigns();
            this.$utils.toast(this.$tc(
              'globals.messages.deletedCount',
              this.numSelectedCampaigns,
              { num: this.numSelectedCampaigns, name },
            ));
          });
      };

      this.$utils.confirm(this.$tc(
        'globals.messages.confirmDelete',
        this.numSelectedCampaigns,
        { num: this.numSelectedCampaigns, name: name.toLowerCase() },
      ), fn);
    },
  },

  computed: {
    ...mapState(['campaigns', 'lists', 'loading', 'serverConfig']),

    // Fork (evergreen) -- New from the Automations scope pre-enables the evergreen
    // toggle on the create form (Campaign.vue reads the query param).
    newCampaignRoute() {
      const r = { name: 'campaign', params: { id: 'new' } };
      // POLISH3 D3 -- only from Automations (never from All).
      if (preEnablesEvergreen(this.queryParams.evergreen)) {
        r.query = { evergreen: '1' };
      }
      return r;
    },

    // DASHBOARDS-POLISH3-SPEC D3 -- the pill's value: the kind string over queryParams.evergreen
    // (false / true / null). The setter runs before onScopeChange (v-model's callback first).
    scopeKind: {
      get() {
        return kindOfEvergreen(this.queryParams.evergreen);
      },
      set(v) {
        this.queryParams.evergreen = evergreenOfKind(v);
      },
    },

    numSelectedCampaigns() {
      return this.bulk.all ? this.campaigns.total : this.bulk.checked.length;
    },

    // Fork (global brand, D7) -- the in-page list filter's options: the scope's lists.
    filterLists() {
      return filterByIds(this.lists, this.scopeListIds);
    },

    // DASHBOARDS-POLISH2-SPEC D2 -- the Lists select's value: the route-scope placeholder while a
    // link's list set is active, else the single filter. Picking any real list (or All lists)
    // clears the route scope, then the list-change hygiene and refetch run.
    listSelect: {
      get() {
        return listSelectValue(this.queryParams);
      },
      set(v) {
        const next = selectList(this.queryParams, v);
        this.queryParams.listID = next.listID;
        this.queryParams.routeListIDs = next.routeListIDs;
        // POLISH3 D2 -- a real list pick (or All lists) clears the route status scope; the
        // disabled placeholder, were it ever emitted, changes nothing.
        if (v !== ROUTE_SCOPE_OPTION) {
          this.queryParams.routeStatus = routeStatusAfter(this.queryParams.routeStatus, 'list');
        }
        this.onListChange();
      },
    },

    // Stage 2 F1 -- "select all N" (the by-query bulk delete) only on an unnarrowed query.
    selectAllOffered() {
      return canSelectAll({
        bulkAll: this.bulk.all,
        total: this.campaigns.total,
        perPage: this.campaigns.perPage,
        listID: this.queryParams.listID,
        routeListIDs: this.queryParams.routeListIDs,
        scopeListIds: this.scopeListIds,
        routeStatus: this.queryParams.routeStatus,
      });
    },
  },

  watch: {
    // Fork (global brand, D4) -- reset the in-page list filter to "All lists" when the chosen
    // list is outside the new scope, then fetch through refreshCampaigns (the poll's path).
    // DASHBOARDS-POLISH2-SPEC D2 (stage 2 F2) -- ANY selector change also clears the route list
    // scope (campaignsRouteQuery.mjs::afterBrandChange). The pending transitions are the initial
    // resolution of the stored selection, not a user change, and leave it in place.
    brandKey(now, before) {
      if (before === 'pending' || now === 'pending') {
        return;
      }
      const next = afterBrandChange(this.queryParams, this.scopeListIds);
      this.queryParams.listID = next.listID;
      this.queryParams.routeListIDs = next.routeListIDs;
      // POLISH3 D2 -- any selector change also clears the route status scope.
      this.queryParams.routeStatus = routeStatusAfter(this.queryParams.routeStatus, 'brand');
      this.queryParams.page = 1;
      this.bulk.checked = [];
      this.bulk.all = false;
      this.refreshCampaigns();
    },
  },

  created() {
    this.$root.$on('page.refresh', this.refreshCampaigns);
  },

  mounted() {
    // Fork (global brand, D3) -- the first scoped fetch waits for the lists (I13).
    this.afterListsLoaded(() => this.refreshCampaigns());
  },

  destroyed() {
    this.$root.$off('page.refresh', this.refreshCampaigns);
    clearInterval(this.pollID);
  },
});
</script>
