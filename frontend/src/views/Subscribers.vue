<template>
  <section class="subscribers">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4">
          {{ $t('globals.terms.subscribers') }}
          <!-- SUBSCRIBERS-FILTERS-SPEC D7 -- every displayed count with thousands separators. -->
          <span v-if="!isNaN(subscribers.total)">
            (<span data-cy="count">{{ $utils.formatNumber(subscribers.total) }}</span>)
          </span>
          <span v-if="currentList">
            &raquo; {{ currentList.name }}
            <span v-if="queryParams.subStatus" class="has-text-grey has-text-weight-normal is-capitalized">({{
              queryParams.subStatus }})</span>
          </span>
        </h1>
      </div>
      <div class="column has-text-right">
        <b-field v-if="$can('subscribers:manage')" expanded>
          <b-button expanded type="is-primary" icon-left="plus" @click="showNewForm" data-cy="btn-new" class="btn-new">
            {{ $t('globals.buttons.new') }}
          </b-button>
        </b-field>
      </div>
    </header>

    <!-- Fork (global brand, integrations GLOBAL-BRAND-SPEC D8/D11, S4) -- a list route outside the
         selected brand keeps its one list, with the notice. -->
    <brand-context-notice v-if="queryParams.listID" :record-brand="currentList ? currentList.brand : undefined"
      :kind="$tc('globals.terms.list', 1).toLowerCase()" />

    <section class="subscribers-controls">
      <!-- Fork (Subscribers filters, integrations SUBSCRIBERS-FILTERS-SPEC D4-D6) -- Lists ▾ ·
           Status ▾ · search, the Campaigns page's controls layout. Both dropdowns WRITE THE ROUTE
           AND NOTHING ELSE (:value + @input, never v-model on queryParams): the route stays the
           page's single source of the filter, the router guard corrects it before the page mounts
           (subscriberFilters.mjs::subscribersRouteFix), and the language is the navbar toggle's. -->
      <div class="columns">
        <div class="column is-narrow">
          <b-field grouped>
            <!-- D4 -- the lists of the brand scope, system lists grouped last; disabled under All
                 brands unless the route already names a list (then All lists + that list). -->
            <b-select data-cy="select-list" :aria-label="$t('subscribers.listFilter')"
              :value="queryParams.listID || ''" :disabled="!listOpts.enabled" @input="onListSelect">
              <option value="">{{ $t('subscribers.allLists') }}</option>
              <option v-for="l in listOpts.ordinary" :key="l.id" :value="l.id">{{ l.name }}</option>
              <optgroup v-if="listOpts.system.length" :label="$t('subscribers.systemLists')">
                <option v-for="l in listOpts.system" :key="l.id" :value="l.id">{{ l.name }}</option>
              </optgroup>
              <option v-if="listOpts.extra" :value="listOpts.extra.id">{{ listOpts.extra.name }}</option>
            </b-select>
            <!-- D5 -- a status is a status OF a list: enabled only when the route names one. -->
            <b-select data-cy="select-status" :aria-label="$t('subscribers.statusFilter')"
              :title="statusOn ? null : $t('subscribers.statusFilter')"
              :value="statusOn ? (queryParams.segment || '') : ''" :disabled="!statusOn" @input="onStatusSelect">
              <option value="">{{ $t('subscribers.allStatuses') }}</option>
              <option v-for="s in segments" :key="s" :value="s">{{ $t(`lists.grid.${s}`) }}</option>
            </b-select>
          </b-field>
        </div>
        <div class="column is-6">
          <form @submit.prevent="onSubmit">
            <div>
              <b-field addons>
                <b-input @input="onSimpleQueryInput" v-model="queryInput" expanded
                  :placeholder="$t('subscribers.queryPlaceholder')" icon="magnify" ref="query"
                  :disabled="isSearchAdvanced" data-cy="search" />
                <p class="controls">
                  <b-button native-type="submit" type="is-primary" icon-left="magnify" :disabled="isSearchAdvanced"
                    data-cy="btn-search" />
                </p>
              </b-field>

              <div v-if="isSearchAdvanced">
                <b-input v-model="queryParams.queryExp" @keydown.native.enter="onAdvancedQueryEnter" type="textarea"
                  ref="queryExp" placeholder="subscribers.name LIKE '%user%' or subscribers.status='blocklisted'"
                  data-cy="query" />
                <span class="is-size-6 has-text-grey">
                  {{ $t('subscribers.advancedQueryHelp') }}.{{ ' ' }}
                  <a href="https://listmonk.app/docs/querying-and-segmentation" target="_blank"
                    rel="noopener noreferrer">
                    {{ $t('globals.buttons.learnMore') }}.
                  </a>
                </span>
                <div class="buttons">
                  <b-button native-type="submit" type="is-primary" icon-left="magnify" data-cy="btn-query">
                    {{
                      $t('subscribers.query') }}
                  </b-button>
                  <b-button @click.prevent="toggleAdvancedSearch" icon-left="cancel" data-cy="btn-query-reset">
                    {{ $t('subscribers.reset') }}
                  </b-button>
                </div>
              </div><!-- advanced query -->
            </div>
          </form>
          <div v-if="!isSearchAdvanced" class="toggle-advanced">
            <a href="#" @click.prevent="toggleAdvancedSearch" data-cy="btn-advanced-search">
              <b-icon icon="cog-outline" size="is-small" />
              {{ $t('subscribers.advancedQuery') }}
            </a>
          </div>
        </div><!-- search -->
      </div>
    </section><!-- control -->

    <br />
    <!-- Polish 2026-11-14: the filter caption -- the brand and the ROUTE's language (review R4: the
         route's lang is this page's filter, so the global selection could misstate it). -->
    <b-table :data="subscribers.results ?? []" :caption="$filterCaption({ langText: captionLang })" :loading="loading.subscribers" @check-all="onTableCheck"
      @check="onTableCheck" :checked-rows.sync="bulk.checked" paginated backend-pagination pagination-position="both"
      @page-change="onPageChange" :current-page="queryParams.page" :per-page="subscribers.perPage"
      :total="subscribers.total" hoverable checkable backend-sorting @sort="onSort">
      <template #top-left>
        <!-- Fork (global language, GLOBAL-LANG-SPEC D4, S2-1) -- the export and every by-query
             action are disabled while the rows are loading (byQueryBusy), so the rows, the total in
             a confirmation and the by-query request always describe the same filter. -->
        <div class="actions" :class="{ 'is-busy': byQueryBusy }">
          <a class="a" href="#" @click.prevent="exportSubscribers" data-cy="btn-export-subscribers"
            :aria-disabled="byQueryBusy ? 'true' : null">
            <b-icon icon="cloud-download-outline" size="is-small" />
            {{ $t('subscribers.export') }}
          </a>
          <template v-if="bulk.checked.length > 0">
            <a class="a" href="#" @click.prevent="showBulkListForm" data-cy="btn-manage-lists">
              <b-icon icon="format-list-bulleted-square" size="is-small" /> Manage lists
            </a>
            <a class="a" href="#" @click.prevent="deleteSubscribers" data-cy="btn-delete-subscribers">
              <b-icon icon="trash-can-outline" size="is-small" /> Delete
            </a>
            <a class="a" href="#" @click.prevent="blocklistSubscribers" data-cy="btn-manage-blocklist">
              <b-icon icon="account-off-outline" size="is-small" /> Blocklist
            </a>
            <span class="a">
              {{ $t('globals.messages.numSelected', { num: $utils.formatNumber(numSelectedSubscribers) }) }}
              <span v-if="!bulk.all && subscribers.total > subscribers.perPage && !byQueryBusy">
                &mdash;
                <a href="#" @click.prevent="selectAllSubscribers">
                  {{ $t('globals.messages.selectAll', { num: $utils.formatNumber(subscribers.total) }) }}
                </a>
              </span>
            </span>
          </template>
        </div>
      </template>

      <b-table-column v-slot="props" field="email" :label="$t('subscribers.email')" header-class="cy-email" sortable
        :td-attrs="$utils.tdID">
        <a :href="`/subscribers/${props.row.id}`" @click.prevent="showEditForm(props.row)"
          :class="{ 'blocklisted': props.row.status === 'blocklisted' }">
          {{ props.row.email }}
          <copy-text :text="`${props.row.email}`" hide-text />
        </a>
        <b-tag v-if="props.row.status !== 'enabled'" :class="props.row.status" data-cy="blocklisted">
          {{ $t(`subscribers.status.${props.row.status}`) }}
        </b-tag>
        <b-taglist>
          <template v-for="l in props.row.lists">
            <router-link :to="`/subscribers/lists/${l.id}`" :key="l.id" style="padding-right:0.5em;">
              <b-tag :class="l.subscriptionStatus" size="is-small" :key="l.id">
                {{ l.name }}
                <sup v-if="l.optin === 'double' || l.subscriptionStatus == 'unsubscribed'">
                  {{ $t(`subscribers.status.${l.subscriptionStatus}`) }}
                </sup>
              </b-tag>
            </router-link>
          </template>
        </b-taglist>
      </b-table-column>

      <b-table-column v-slot="props" field="name" :label="$t('globals.fields.name')" header-class="cy-name" sortable>
        <a :href="`/subscribers/${props.row.id}`" @click.prevent="showEditForm(props.row)"
          :class="{ 'blocklisted': props.row.status === 'blocklisted' }">
          {{ props.row.name }}
          <copy-text :text="`${props.row.name}`" hide-text />
        </a>
      </b-table-column>

      <!-- SUBSCRIBERS-FILTERS-2-SPEC D3 -- the stored language, under All, EN+ and Other (the route's
           lang); sorted server-side by the bucket, no language first (order_by=lang). -->
      <b-table-column v-if="langColumnOn" v-slot="props" field="lang" :label="$t('subscribers.langColumn')"
        header-class="cy-lang" sortable>
        <span data-cy="lang-cell">{{ langCellOf(props.row) }}</span>
      </b-table-column>

      <b-table-column v-slot="props" field="lists" :label="$t('globals.terms.lists')" header-class="cy-lists" centered>
        {{ listCount(props.row.lists) }}
      </b-table-column>

      <b-table-column v-slot="props" field="created_at" :label="$t('globals.fields.createdAt')"
        header-class="cy-created_at" sortable>
        {{ $utils.niceDate(props.row.createdAt) }}
      </b-table-column>

      <b-table-column v-slot="props" field="updated_at" :label="$t('globals.fields.updatedAt')"
        header-class="cy-updated_at" sortable>
        {{ $utils.niceDate(props.row.updatedAt) }}
      </b-table-column>

      <b-table-column v-slot="props" cell-class="actions" align="right">
        <div>
          <a v-if="$can('subscribers:manage')" :href="`/subscribers/${props.row.id}`"
            @click.prevent="showEditForm(props.row)" data-cy="btn-edit" :aria-label="$t('globals.buttons.edit')">
            <b-tooltip :label="$t('globals.buttons.edit')" type="is-dark">
              <b-icon icon="pencil-outline" size="is-small" />
            </b-tooltip>
          </a>
          <!-- Fork: per-row Manage lists — the same modal as the bulk toolbar's, scoped to this
               one subscriber (ids path, so Preconfirm can re-subscribe an unsubscribed row). -->
          <a v-if="$can('subscribers:manage')" href="#" @click.prevent="showRowListForm(props.row)"
            data-cy="btn-row-manage-lists" :aria-label="$t('subscribers.manageLists')">
            <b-tooltip :label="$t('subscribers.manageLists')" type="is-dark">
              <b-icon icon="format-list-bulleted-square" size="is-small" />
            </b-tooltip>
          </a>
          <!-- Fork: opens the customer's manage-preferences page as that subscriber. The UUID is
               the page's authorization (listmonk-footer link form), so a save from here is a
               consent write attributed to the customer. -->
          <a v-if="$can('subscribers:manage')" :href="managePrefsUrl(props.row)" target="_blank" rel="noopener" data-cy="btn-manage-prefs"
            :aria-label="$t('subscribers.managePrefs')">
            <b-tooltip :label="$t('subscribers.managePrefs')" type="is-dark">
              <b-icon icon="arrow-top-right" size="is-small" />
            </b-tooltip>
          </a>
          <a :href="`/api/subscribers/${props.row.id}/export`" data-cy="btn-download"
            :aria-label="$t('subscribers.downloadData')">
            <b-tooltip :label="$t('subscribers.downloadData')" type="is-dark">
              <b-icon icon="cloud-download-outline" size="is-small" />
            </b-tooltip>
          </a>
          <!-- Fork: per-row Blocklist — the bulk toolbar's action for this one subscriber. -->
          <a v-if="$can('subscribers:manage') && props.row.status !== 'blocklisted'" href="#"
            @click.prevent="blocklistSubscriber(props.row)" data-cy="btn-row-blocklist"
            :aria-label="$t('subscribers.blocklist')">
            <b-tooltip :label="$t('subscribers.blocklist')" type="is-dark">
              <b-icon icon="account-off-outline" size="is-small" />
            </b-tooltip>
          </a>
          <a v-if="$can('subscribers:manage')" href="#" @click.prevent="deleteSubscriber(props.row)"
            data-cy="btn-delete" :aria-label="$t('globals.buttons.delete')">
            <b-tooltip :label="$t('globals.buttons.delete')" type="is-dark">
              <b-icon icon="trash-can-outline" size="is-small" />
            </b-tooltip>
          </a>
        </div>
      </b-table-column>

      <template #empty v-if="!loading.subscribers">
        <empty-placeholder />
      </template>
    </b-table>

    <!-- Manage list modal -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isBulkListFormVisible" :width="500" class="has-overflow">
      <subscriber-bulk-list :num-subscribers="listFormTarget ? 1 : numSelectedSubscribers" :options="bulkListOptions"
        :lang-note="bulkListLangNote" @finished="bulkChangeLists" />
    </b-modal>

    <!-- Add / edit form modal -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isFormVisible" :width="850" @close="onFormClose">
      <subscriber-form :data="curItem" :is-editing="isEditing" @finished="querySubscribers" />
    </b-modal>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapGetters, mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import { uris, MANAGE_PREFS_URL } from '../constants';
import SubscriberBulkList from './SubscriberBulkList.vue';
import SubscriberForm from './SubscriberForm.vue';
import CopyText from '../components/CopyText.vue';
import { campaignLangLabel, sendLangLabel } from '../langs';
import {
  effectiveListIds, filterByIds, gridFilterOf, scopeUnion, subscriberExportParams, subscriberQueryBody,
} from '../brandScope.mjs'; // eslint-disable-line import/extensions
import {
  answeredFilter, bulkTarget, byQueryLangNote, langParam, routeLangLabel, subscriberResponseCurrent,
} from '../langScope.mjs'; // eslint-disable-line import/extensions
import { subscriberCaptionLang } from '../filterBar.mjs'; // eslint-disable-line import/extensions
import {
  SEGMENTS, langCell, langColumnShown, listOptions, listRoute, routeListIdOf, statusEnabled, statusQuery,
} from '../subscriberFilters.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';
import BrandContextNotice from '../components/BrandContextNotice.vue';

export default Vue.extend({
  components: {
    SubscriberForm,
    SubscriberBulkList,
    CopyText,
    EmptyPlaceholder,
    BrandContextNotice,
  },

  // Fork (global brand, integrations GLOBAL-BRAND-SPEC D4/D8).
  mixins: [brandScopeMixin],

  data() {
    return {
      // Current subscriber item being edited.
      curItem: null,
      isSearchAdvanced: false,
      isEditing: false,
      isFormVisible: false,
      isBulkListFormVisible: false,
      // Fork: set when the Manage lists modal was opened from a row action; the submit then
      // targets only this subscriber instead of the bulk selection.
      listFormTarget: null,

      // Table bulk row selection states.
      bulk: {
        checked: [],
        all: false,
      },

      queryInput: '',

      // SUBSCRIBERS-FILTERS D5 -- the Status dropdown's segments.
      segments: SEGMENTS,

      // Fork (global language, GLOBAL-LANG-SPEC D4, S2-1) -- the latest subscribers query this
      // page issued and the latest one answered; a response is applied only when it answers the
      // latest query of the still-mounted page (subscriberResponseCurrent).
      // S4-1: answeredSeq starts below any sequence, so a page that has not yet had a query of
      // its own answered is busy (the store may still hold another mount's rows).
      reqSeq: 0,
      answeredSeq: -1,
      // Stage 4b S4b-2 -- the queryParams the accepted answer answered; every by-query body, the
      // export and their language line are built from it (langScope.mjs::answeredFilter), never
      // from the live queryParams the search box edits before any query runs.
      answeredQuery: null,

      // Query params to filter the getSubscribers() API call.
      queryParams: {
        // Search query expression.
        queryExp: '',
        search: '',

        // ID of the list the current subscriber view is filtered by.
        listID: null,
        page: 1,
        orderBy: 'id',
        order: 'desc',
        subStatus: null,

        // Fork (list grid). Server-authored filters from the Lists-page grid (?segment=&lang=).
        segment: null,
        lang: null,
      },
    };
  },

  methods: {
    // Count the lists from which a subscriber has not unsubscribed.
    listCount(lists) {
      return lists.reduce((defVal, item) => (defVal + (item.subscriptionStatus !== 'unsubscribed' ? 1 : 0)), 0);
    },

    toggleAdvancedSearch() {
      this.isSearchAdvanced = !this.isSearchAdvanced;
      this.queryParams.search = '';

      // Toggling to simple search.
      if (!this.isSearchAdvanced) {
        this.queryInput = '';
        this.queryParams.queryExp = '';
        this.queryParams.page = 1;
        this.querySubscribers();
        this.$refs.query.focus();
        return;
      }

      // Toggling to advanced search.
      const q = this.queryInput.replace(/'/, "''").trim();
      if (q) {
        if (this.$utils.validateEmail(q)) {
          this.queryParams.queryExp = `email = '${q.toLowerCase()}'`;
        } else {
          this.queryParams.queryExp = `(name ~* '${q}' OR email ~* '${q.toLowerCase()}')`;
        }
      }

      // Toggling to advanced search.
      this.$nextTick(() => {
        this.$refs.queryExp.focus();
      });
    },

    // Mark all subscribers in the query as selected.
    selectAllSubscribers() {
      this.bulk.all = true;
    },

    onTableCheck() {
      // Disable bulk.all selection if there are no rows checked in the table.
      if (this.bulk.checked.length !== this.subscribers.total) {
        this.bulk.all = false;
      }
    },

    // Show the edit list form.
    showEditForm(sub) {
      this.curItem = sub;
      this.isFormVisible = true;
      this.isEditing = true;
    },

    // Show the new list form.
    showNewForm() {
      this.curItem = {};
      this.isFormVisible = true;
      this.isEditing = false;
    },

    showBulkListForm() {
      if (this.bulk.all && this.byQueryBusy) {
        return;
      }
      this.listFormTarget = null;
      this.isBulkListFormVisible = true;
    },

    showRowListForm(sub) {
      this.listFormTarget = sub;
      this.isBulkListFormVisible = true;
    },

    blocklistSubscriber(sub) {
      this.$utils.confirm(this.$t('subscribers.confirmBlocklist', { num: 1 }), () => {
        this.$api.blocklistSubscribers({ ids: [sub.id] })
          .then(() => this.querySubscribers());
      });
    },

    onFormClose() {
      if (this.$route.params.id) {
        // GLOBAL-LANG stage 4 S4-3 -- the language the page was entered under (and only that)
        // rides the push (the guard, SUBSCRIBERS-FILTERS D3, would add the selection anyway).
        const query = typeof this.$route.query.lang === 'string' ? { lang: this.$route.query.lang } : {};
        this.$router.push({ name: 'subscribers', query });
      }
    },

    onPageChange(p) {
      this.querySubscribers({ page: p });
    },

    onSort(field, direction) {
      this.querySubscribers({ orderBy: field, order: direction });
    },

    // Prepares an SQL expression for simple name search inputs and saves it
    // in this.queryExp.
    onSimpleQueryInput(v) {
      const q = v.replace(/'/, "''").trim();
      this.queryParams.queryExp = '';
      this.queryParams.page = 1;
      this.queryParams.search = q.toLowerCase();
    },

    // Ctrl + Enter on the advanced query searches.
    onAdvancedQueryEnter(e) {
      if (e.ctrlKey) {
        this.onSubmit();
      }
    },

    onSubmit() {
      this.querySubscribers({ page: 1 });
    },

    // Fork (list grid; SUBSCRIBERS-FILTERS-SPEC D4/D5). Every filter change goes through the route:
    // the router-view is keyed by fullPath, so the page reloads with the new filter and nothing can
    // go on carrying the old one (a bulk selection made under it is dropped with the page).
    // The dropdowns sit beside the search field, so they read as one filter: every filter
    // route push carries the CURRENT simple search text across its reload (mounted() reads it
    // back) -- set from the box each time, so a filter push never resurrects an older search=.
    // Submitting or clearing the box does NOT rewrite the route, so between pushes the URL's
    // search= can lag the box (a reload re-applies it). That is cosmetic, never unsafe:
    // mounted() sets the box and queryParams.search together, so the box, the request and any
    // by-query bulk action always agree. An advanced SQL query is not carried; it does not
    // belong in a URL.
    withRouteSearch(query) {
      const { search: _stale, ...rest } = query;
      const search = this.isSearchAdvanced ? '' : this.queryInput.trim();
      return search ? { ...rest, search } : rest;
    },

    // SUBSCRIBERS-FILTERS D4 -- a list pick (null = All lists) through the one route builder, which
    // drops the segment and the legacy subscription_status and keeps lang and the search.
    onListSelect(id) {
      const listId = id === null || id === '' ? null : Number(id);
      if (listId === (this.queryParams.listID || null)) {
        return;
      }
      this.$router.push(listRoute(this.withRouteSearch({ ...this.$route.query }), listId));
    },

    // SUBSCRIBERS-FILTERS D5 -- a status pick ('' = All statuses) on the current list route.
    onStatusSelect(segment) {
      if ((segment || '') === (this.$route.query.segment || '')) {
        return;
      }
      this.$router.push({ path: this.$route.path, query: this.withRouteSearch(statusQuery(this.$route.query, segment)) });
    },

    // Fork (LIST-COLLAPSE-SPEC C8/C11; since SUBSCRIBERS-FILTERS D3 the navbar toggle's): the page
    // remounts with the new language. The simple search text is carried (withRouteSearch); an
    // advanced query, the sort and the page number are discarded (they are not route state) along
    // with any bulk selection made under the old filter.
    onLangSelect(lang) {
      const query = { ...this.$route.query };
      if (lang) {
        query.lang = lang;
      } else {
        delete query.lang;
      }
      this.$router.push({ path: this.$route.path, query: this.withRouteSearch(query) });
    },

    // Search / query subscribers.
    querySubscribers(params) {
      this.queryParams = { ...this.queryParams, ...params };

      const qp = {
        // Fork (global brand, D8) -- the chosen list, else the global brand's list ids, else none.
        list_id: effectiveListIds(this.queryParams.listID, this.scopeListIds),
        search: this.queryParams.search,
        query: this.queryParams.queryExp,
        page: this.queryParams.page,
        subscription_status: this.queryParams.subStatus,
        order_by: this.queryParams.orderBy,
        order: this.queryParams.order,
        ...this.gridFilter,
      };

      if (this.queryParams.queryExp) {
        delete qp.search;
      } else {
        delete qp.queryExp;
      }

      // Fork (global language, D4, S2-1, I4c) -- numbered; the api layer commits the rows only
      // when this is still the mounted page's latest query.
      this.reqSeq += 1;
      const seq = this.reqSeq;
      const current = () => subscriberResponseCurrent(seq, this.reqSeq, this.pageMounted === true);
      // Stage 4b S4b-2 -- the filter this query asks, kept as answeredQuery once its answer is
      // accepted: the rows on screen answer exactly it.
      const asked = { ...this.queryParams };
      this.$nextTick(() => {
        // S4-2: only a SUCCESSFUL answer un-busies the page -- a failed query leaves the previous
        // filter's rows on screen, so the page stays busy until a query of it succeeds.
        this.$api.getSubscribers(qp, { accept: current }).then(() => {
          if (!current()) {
            return;
          }
          this.answeredSeq = seq;
          this.answeredQuery = asked;
          this.bulk.checked = [];
        });
      });
    },

    // Fork (global language, D4, S2-3, I6) -- the confirmation text with the by-query language
    // line appended when the request carries a lang.
    withLangNote(msg, params) {
      const note = byQueryLangNote(params, (l) => this.$t('naming.byQueryLang', { lang: this.routeLangLabel(l) }));
      return note ? `${msg} ${note}` : msg;
    },

    // A route lang value named for the by-query note (langScope.mjs::routeLangLabel, the one
    // mapper; S4-7).
    routeLangLabel(l) {
      return routeLangLabel(l, {
        all: this.$t('subscribers.langAll'),
        other: this.$t('lists.grid.other'),
        name: campaignLangLabel,
        sendName: sendLangLabel,
      });
    },

    // SUBSCRIBERS-FILTERS-2-SPEC D3 -- a row's Lang cell (subscriberFilters.mjs::langCell).
    langCellOf(row) {
      return langCell(row ? row.attribs : undefined);
    },

    managePrefsUrl(sub) {
      return `${MANAGE_PREFS_URL}?uuid=${encodeURIComponent(sub.uuid)}&email=${encodeURIComponent(sub.email)}`;
    },

    deleteSubscriber(sub) {
      this.$utils.confirm(
        null,
        () => {
          this.$api.deleteSubscriber(sub.id).then(() => {
            this.querySubscribers();

            this.$utils.toast(this.$t('globals.messages.deleted', { name: sub.name }));
          });
        },
      );
    },

    blocklistSubscribers() {
      if (this.byQueryBusy) {
        return;
      }
      let fn = null;
      let body = null;
      // Stage 4b S4b-1 -- by query ONLY under "select all"; nothing checked and no select-all
      // sends nothing (langScope.mjs::bulkTarget).
      const target = bulkTarget({ all: this.bulk.all, checkedCount: this.bulk.checked.length });
      const filter = answeredFilter(this.answeredQuery);
      if (target === 'none' || (target === 'query' && !filter)) {
        return;
      }
      if (target === 'ids') {
        // If 'all' is not selected, blocklist subscribers by IDs.
        fn = () => {
          const ids = this.bulk.checked.map((s) => s.id);
          this.$api.blocklistSubscribers({ ids })
            .then(() => this.querySubscribers());
        };
      } else {
        // 'All' is selected, blocklist by query.
        // Fork (global brand, D8/I12) -- the body carries the effective list ids.
        // GLOBAL-LANG D4 (S2-3) -- the body is built once: the confirmation names its language.
        // Stage 4b S4b-2 -- from the answered filter, never the live (possibly edited) one.
        body = subscriberQueryBody(filter, this.scopeListIds);
        fn = () => {
          this.$api.blocklistSubscribersByQuery(body)
            .then(() => this.querySubscribers());
        };
      }

      this.$utils.confirm(this.withLangNote(this.$t('subscribers.confirmBlocklist', { num: this.$utils.formatNumber(this.numSelectedSubscribers) }), body), fn);
    },

    exportSubscribers() {
      if (this.byQueryBusy) {
        return;
      }
      const num = !this.bulk.all && this.bulk.checked.length > 0
        ? this.bulk.checked.length : this.subscribers.total;

      // Fork (global brand, D8/I12) -- search/query, the effective list ids, status, the grid
      // filter (list grid) and the selected ids, built by brandScope.mjs. GLOBAL-LANG D4 (S2-3)
      // -- built before the confirmation, which names its language.
      const ids = !this.bulk.all && this.bulk.checked.length > 0 ? this.bulk.checked.map((s) => s.id) : [];
      // Stage 4b S4b-2 -- from the answered filter, never the live (possibly edited) one.
      const filter = answeredFilter(this.answeredQuery);
      if (!filter) {
        return;
      }
      const params = subscriberExportParams(filter, this.scopeListIds, ids);
      this.$utils.confirm(this.withLangNote(this.$t('subscribers.confirmExport', { num: this.$utils.formatNumber(num) }), params), () => {
        const q = new URLSearchParams(params);

        document.location.href = `${uris.exportSubscribers}?${q.toString()}`;
      });
    },

    deleteSubscribers() {
      if (this.byQueryBusy) {
        return;
      }
      let fn = null;
      let body = null;
      // Stage 4b S4b-1 -- by query ONLY under "select all" (langScope.mjs::bulkTarget).
      const target = bulkTarget({ all: this.bulk.all, checkedCount: this.bulk.checked.length });
      const filter = answeredFilter(this.answeredQuery);
      if (target === 'none' || (target === 'query' && !filter)) {
        return;
      }
      if (target === 'ids') {
        // If 'all' is not selected, delete subscribers by IDs.
        fn = () => {
          const ids = this.bulk.checked.map((s) => s.id);
          this.$api.deleteSubscribers({ id: ids })
            .then(() => {
              this.querySubscribers();

              this.$utils.toast(this.$t('subscribers.subscribersDeleted', { num: this.$utils.formatNumber(this.numSelectedSubscribers) }));
            });
        };
      } else {
        // 'All' is selected, delete by query. GLOBAL-LANG D4 (S2-3) -- the body is built once:
        // the confirmation names its language.
        body = {
          // If the query expression is empty, explicitly pass `all=true`
          // so that the backend deletes all records in the DB with an empty query string.
          // Stage 4b S4b-2 -- the answered filter's texts, never the live box's.
          all: filter.queryExp.trim() === '' && filter.search.trim() === '',
          // Fork (list grid, D7). all=true above is sent exactly when a grid-linked page has
          // no search -- the server keeps the segment through it (it never voids a segment).
          // Fork (global brand, D8/I12) -- and the effective list ids, which it keeps too.
          ...subscriberQueryBody(filter, this.scopeListIds),
        };
        fn = () => {
          this.$api.deleteSubscribersByQuery(body).then(() => {
            this.querySubscribers();

            this.$utils.toast(this.$t(
              'subscribers.subscribersDeleted',
              { num: this.$utils.formatNumber(this.numSelectedSubscribers) },
            ));
          });
        };
      }

      this.$utils.confirm(this.withLangNote(this.$t('subscribers.confirmDelete', { num: this.$utils.formatNumber(this.numSelectedSubscribers) }), body), fn);
    },

    bulkChangeLists(action, preconfirm, lists, backfill = false) {
      const data = {
        action,
        query: this.fullQueryExp,
        search: this.queryParams.search,
        // Fork (global brand, D8/I12) -- the effective list ids (read by the by-query path only).
        list_ids: effectiveListIds(this.queryParams.listID, this.scopeListIds),
        target_list_ids: lists.map((l) => l.id),
        // Fork (evergreen) -- see the bulk modal's checkbox.
        backfill: action === 'add' && !!backfill,
      };

      if (preconfirm) {
        data.status = 'confirmed';
      }

      let fn = null;
      if (this.listFormTarget) {
        // Fork: opened from a row action -- this one subscriber, by ID.
        fn = this.$api.addSubscribersToLists;
        data.ids = [this.listFormTarget.id];
        this.listFormTarget = null;
      } else {
        // Stage 4b S4b-1 -- by query ONLY under "select all"; a selection emptied while the modal
        // was open (a query answered) sends nothing -- never a fallthrough to the whole filter.
        const target = bulkTarget({ all: this.bulk.all, checkedCount: this.bulk.checked.length });
        if (target === 'ids') {
          // If 'all' is not selected, perform by IDs.
          fn = this.$api.addSubscribersToLists;
          data.ids = this.bulk.checked.map((s) => s.id);
        } else {
          const filter = answeredFilter(this.answeredQuery);
          // GLOBAL-LANG D4 (S2-1) -- never by query while the rows are loading.
          if (target !== 'query' || this.byQueryBusy || !filter) {
            this.isBulkListFormVisible = false;
            // Stage 4c S4c-3 -- never a silent close: nothing was written, say so.
            this.$utils.toast(this.$t('subscribers.listChangeNotApplied'), 'is-warning');
            return;
          }
          // 'All' is selected, perform by query. Fork (list grid, D7; global brand, D8/I12) -- the
          // shared by-query body: query, search, status, grid filter and the effective list ids,
          // from the answered filter (stage 4b S4b-2).
          Object.assign(data, subscriberQueryBody(filter, this.scopeListIds));
          fn = this.$api.addSubscribersToListsByQuery;
        }
      }

      fn(data).then(() => {
        this.querySubscribers();
        this.$utils.toast(this.$t('subscribers.listChangeApplied'));
      });
    },
  },

  watch: {
    // Fork (global language, GLOBAL-LANG-SPEC D4; SUBSCRIBERS-FILTERS-SPEC D2/D3) -- the navbar
    // toggle changed while on the page: rewrite the route's lang to the RAW selection (All removes
    // it), so the page remounts under it; the router guard then sees agreeing values. The route
    // stays the page's single source of the filter.
    langSubscribers(now) {
      const want = langParam(now);
      if ((this.$route.query.lang || '') === want) {
        return;
      }
      this.onLangSelect(want);
    },

    // Fork: a row-opened Manage lists modal dismissed without saving must not leave its target behind.
    isBulkListFormVisible(visible) {
      if (!visible) {
        this.listFormTarget = null;
      }
    },

    // Fork (global brand, D4) -- a selection change goes through the route remount (the keyed
    // <router-view>), which drops the bulk selection and the chosen list with the page, so a
    // by-query action can never act on a selection made under another scope. The simple search is
    // carried (withRouteSearch). On the plain subscribers route the push would be a duplicate
    // (same fullPath, no remount), so the page drops the same state itself and re-reads.
    // SUBSCRIBERS-FILTERS D4 (I8) -- through the one route builder, which drops the list's segment.
    brandKey(now) {
      if (!this.brandChanged(now)) {
        return;
      }
      if (this.$route.name === 'subscribers_list') {
        this.$router.push(listRoute(this.withRouteSearch({ ...this.$route.query }), null));
        return;
      }
      this.isBulkListFormVisible = false;
      this.listFormTarget = null;
      this.bulk = { checked: [], all: false };
      this.querySubscribers({ page: 1 });
    },
  },

  computed: {
    ...mapState(['subscribers', 'lists', 'loading']),

    // Fork (list grid, LIST-GRID-SPEC D7). THE one place the segment/lang filter is read for a
    // request. Every read and every select-all write spreads it, so a by-query delete,
    // blocklist or list change can never reach beyond the rows the page is showing.
    // GLOBAL-LANG D4 (S2-11) -- through the same builder the by-query body and the export use.
    gridFilter() {
      return gridFilterOf(this.queryParams);
    },

    // SUBSCRIBERS-FILTERS D2 -- the raw selection (subscriber-only languages included); every other
    // view reads langEffective.
    ...mapGetters(['langSubscribers']),

    // SUBSCRIBERS-FILTERS D4 -- the Lists dropdown's options and enablement.
    listOpts() {
      return listOptions(this.lists, this.brandSelection, this.queryParams.listID);
    },

    // D5 -- the Status dropdown is enabled only on a list route.
    statusOn() {
      return statusEnabled(this.queryParams.listID);
    },

    // GLOBAL-LANG D4 (S2-1) -- true while the rows on screen may not answer the latest query: the
    // store's loading flag, or a query of this page still unanswered. The export and every
    // by-query action wait for it.
    byQueryBusy() {
      return !!this.loading.subscribers || this.answeredSeq !== this.reqSeq;
    },

    // GLOBAL-LANG D4 (S2-3, I6) -- the Manage-lists modal's language line, by-query mode only.
    bulkListLangNote() {
      const filter = answeredFilter(this.answeredQuery);
      if (this.listFormTarget || !filter
        || bulkTarget({ all: this.bulk.all, checkedCount: this.bulk.checked.length }) !== 'query') {
        return '';
      }
      // Stage 4b S4b-2 -- the answered filter's language, the one the by-query body will carry.
      return byQueryLangNote(gridFilterOf(filter), (l) => this.$t('naming.byQueryLang', { lang: this.routeLangLabel(l) }));
    },

    // Polish 2026-11-14 -- the filter caption's language: the route's lang (the page's own filter).
    // SUBSCRIBERS-FILTERS D8 (I13), SUBSCRIBERS-FILTERS-2-SPEC D4 (I8): under EN+ `English n + No
    // language m` once the answer to the CURRENT query (the page not busy, its answered filter
    // under en) carries lang_none_total, else English+; Other; the English name
    // (filterBar.mjs::subscriberCaptionLang).
    captionLang() {
      const answered = answeredFilter(this.answeredQuery);
      const split = {
        current: !this.byQueryBusy && !!answered && answered.lang === 'en',
        total: this.subscribers.total,
        langNoneTotal: this.subscribers.langNoneTotal,
      };
      return subscriberCaptionLang(this.queryParams.lang, {
        none: this.$t('lists.grid.noLangChip'),
        other: this.$t('lists.grid.other'),
      }, split, (n) => this.$utils.formatNumber(n));
    },

    // SUBSCRIBERS-FILTERS-2-SPEC D3 (I9) -- the Lang column, under All, EN+ and Other (the route's
    // lang, the caption's source).
    langColumnOn() {
      return langColumnShown(this.queryParams.lang);
    },

    numSelectedSubscribers() {
      if (this.bulk.all) {
        return this.subscribers.total;
      }
      return this.bulk.checked.length;
    },

    // Fork (global brand, D8/I6) -- the Manage-lists modal's option set, by mode: a row action is
    // the scope ∪ that row's lists; a bulk selection by ids the scope ∪ the union of the checked
    // rows' lists (a cross-brand removal stays possible); a bulk selection by query the scope
    // only. Every list under All brands.
    bulkListOptions() {
      let current = [];
      if (this.listFormTarget) {
        current = [this.listFormTarget.lists];
      } else if (!this.bulk.all && this.bulk.checked.length > 0) {
        current = this.bulk.checked.map((s) => s.lists);
      }
      return filterByIds(this.lists, scopeUnion(this.scopeListIds, current));
    },

    // Returns the list that the subscribers are being filtered by in.
    currentList() {
      if (!this.queryParams.listID || !this.lists.results) {
        return null;
      }

      return this.lists.results.find((l) => l.id === this.queryParams.listID);
    },
  },

  created() {
    this.$root.$on('page.refresh', this.querySubscribers);
  },

  // GLOBAL-LANG D4 (S2-1) -- a response landing after the page is gone is never applied.
  beforeDestroy() {
    this.pageMounted = false;
  },

  destroyed() {
    this.$root.$off('page.refresh', this.querySubscribers);
  },

  mounted() {
    // Non-reactive on purpose: read only by the stale-response check.
    this.pageMounted = true;
    // SUBSCRIBERS-FILTERS Stage 4 S4-1 -- the guard's own parser: a malformed id is no list on both
    // sides (Status disabled, list_id from the scope), never list 5 here and "no list" there.
    if (this.$route.params.listID) {
      this.queryParams.listID = routeListIdOf(this.$route.params);
    }
    if (this.$route.query.subscription_status) {
      this.queryParams.subStatus = this.$route.query.subscription_status;
    }
    // Fork (list grid). SUBSCRIBERS-FILTERS D3 -- the router guard has already dropped a segment
    // without a list or outside the five, and a lang the toggle cannot hold, so both are read as is.
    if (this.$route.query.segment) {
      this.queryParams.segment = this.$route.query.segment;
    }
    if (this.$route.query.lang) {
      this.queryParams.lang = this.$route.query.lang;
    }
    // The simple search a language pick carried across its reload (onLangSelect).
    if (typeof this.$route.query.search === 'string' && this.$route.query.search) {
      this.queryInput = this.$route.query.search;
      this.onSimpleQueryInput(this.queryInput);
    }

    if (this.$route.params.id) {
      this.$api.getSubscriber(parseInt(this.$route.params.id, 10)).then((data) => {
        this.showEditForm(data);
      });
    }
    // Get subscribers on load. Fork (global brand, D3) -- once the lists have loaded (I13).
    // GLOBAL-LANG stage 4 S4-1 -- the `subscriber` route too, so the table behind the form shows
    // this page's own rows (never another mount's) and by-query actions un-busy on its answer.
    this.afterListsLoaded(() => this.querySubscribers());
  },
});
</script>
