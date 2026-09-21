<template>
  <section class="subscribers">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4">
          {{ $t('globals.terms.subscribers') }}
          <span v-if="!isNaN(subscribers.total)">
            (<span data-cy="count">{{ subscribers.total }}</span>)
          </span>
          <span v-if="currentList">
            &raquo; {{ currentList.name }}
            <span v-if="queryParams.subStatus" class="has-text-grey has-text-weight-normal is-capitalized">({{
              queryParams.subStatus }})</span>
          </span>
          <!-- Fork (list grid, LIST-GRID-SPEC D8). The Lists-page grid's segment filter. It rides
          every query, page, sort, export and select-all bulk request until removed. The language
          half of that filter is the picker beside this title (LIST-COLLAPSE-SPEC C8) -- one
          control per fact. -->
          <b-taglist v-if="queryParams.segment" class="grid-filter is-inline-flex ml-2">
            <b-tag closable attached type="is-light" data-cy="chip-segment" @close="clearGridFilter('segment')">
              {{ $t(`lists.grid.${queryParams.segment}`) }}
            </b-tag>
          </b-taglist>
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

    <section class="subscribers-controls">
      <div class="columns">
        <div class="column is-8">
          <form @submit.prevent="onSubmit">
            <div>
              <b-field addons>
                <!-- Fork (LIST-COLLAPSE-SPEC C8). The language filter, inline to the left of the
                search, and the only path to the no-language residue now that the Lists grid's
                suffix link is gone. It WRITES THE ROUTE AND NOTHING ELSE: :value + @input,
                never v-model / .sync on queryParams.lang, which keeps its single writer (route
                hydration) and single request reader (gridFilter). "English" is STORED en
                (lang=en_only), disjoint from "No language"; the Lists grid's EN cells link to
                lang=en -- the send language, en plus none -- which arrives here as the extra
                option below. -->
                <b-select v-if="showLangPicker" class="lang-picker" data-cy="select-lang"
                  :aria-label="$t('subscribers.langFilter')" :value="queryParams.lang || ''" @input="onLangSelect">
                  <option value="">{{ $t('subscribers.langAll') }}</option>
                  <option value="en_only">{{ langLabel('en') }}</option>
                  <option value="none">{{ $t('lists.grid.noLangChip') }}</option>
                  <option v-for="l in langOptions" :key="l" :value="l">{{ langLabel(l) }}</option>
                  <!-- A route value outside the set (a grid EN cell's lang=en; the grid's Other
                  line; a hand-typed ?lang=pt, which the server 400s) is appended as one extra
                  option, so the select is never blank and "All languages" always recovers. -->
                  <option v-if="extraLang" :value="extraLang">{{ extraLangLabel }}</option>
                </b-select>
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
    <b-table :data="subscribers.results ?? []" :loading="loading.subscribers" @check-all="onTableCheck"
      @check="onTableCheck" :checked-rows.sync="bulk.checked" paginated backend-pagination pagination-position="both"
      @page-change="onPageChange" :current-page="queryParams.page" :per-page="subscribers.perPage"
      :total="subscribers.total" hoverable checkable backend-sorting @sort="onSort">
      <template #top-left>
        <div class="actions">
          <a class="a" href="#" @click.prevent="exportSubscribers" data-cy="btn-export-subscribers">
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
              {{ $t('globals.messages.numSelected', { num: numSelectedSubscribers }) }}
              <span v-if="!bulk.all && subscribers.total > subscribers.perPage">
                &mdash;
                <a href="#" @click.prevent="selectAllSubscribers">
                  {{ $t('globals.messages.selectAll', { num: subscribers.total }) }}
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
      <subscriber-bulk-list :num-subscribers="listFormTarget ? 1 : numSelectedSubscribers" @finished="bulkChangeLists" />
    </b-modal>

    <!-- Add / edit form modal -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isFormVisible" :width="850" @close="onFormClose">
      <subscriber-form :data="curItem" :is-editing="isEditing" @finished="querySubscribers" />
    </b-modal>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import { uris, MANAGE_PREFS_URL } from '../constants';
import SubscriberBulkList from './SubscriberBulkList.vue';
import SubscriberForm from './SubscriberForm.vue';
import CopyText from '../components/CopyText.vue';
import { CAMPAIGN_LANGS, campaignLangLabel } from '../langs';

// Fork (LIST-COLLAPSE-SPEC C8). The send languages offered by the picker after English
// (which is en union no-language, its own option) and before No language. Derived, so a new
// campaign language is one edit (langs.js), not two.
const PICKER_LANGS = CAMPAIGN_LANGS.map((l) => l.code).filter((c) => c !== 'en');

export default Vue.extend({
  components: {
    SubscriberForm,
    SubscriberBulkList,
    CopyText,
    EmptyPlaceholder,
  },

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
        this.$router.push({ name: 'subscribers' });
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

    // Fork (list grid). Removing a chip goes through the route: the router-view is keyed by
    // fullPath, so the page reloads with the remaining filter and nothing can go on carrying
    // the removed one (a bulk selection made under it is dropped with the page).
    // The picker sits inside the search field, so the two read as one filter: every filter
    // route push carries the CURRENT simple search text across its reload (mounted() reads it
    // back) -- set from the box each time, so a stale search= in the route can never come back.
    // An advanced SQL query is not carried; it does not belong in a URL.
    withRouteSearch(query) {
      const { search: _stale, ...rest } = query;
      const search = this.isSearchAdvanced ? '' : this.queryInput.trim();
      return search ? { ...rest, search } : rest;
    },

    clearGridFilter(which) {
      const query = { ...this.$route.query };
      delete query[which];
      this.$router.push({ path: this.$route.path, query: this.withRouteSearch(query) });
    },

    // Fork (LIST-COLLAPSE-SPEC C8). Same reload path as clearGridFilter: the page remounts with
    // the new language, so the search, advanced query, sort and page number are discarded (they
    // are not route state) along with any bulk selection made under the old filter.
    onLangSelect(lang) {
      const query = { ...this.$route.query };
      if (lang) {
        query.lang = lang;
      } else {
        delete query.lang;
      }
      this.$router.push({ path: this.$route.path, query: this.withRouteSearch(query) });
    },

    langLabel(code) {
      return campaignLangLabel(code);
    },

    // Search / query subscribers.
    querySubscribers(params) {
      this.queryParams = { ...this.queryParams, ...params };

      const qp = {
        list_id: this.queryParams.listID,
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

      this.$nextTick(() => {
        this.$api.getSubscribers(qp).then(() => {
          this.bulk.checked = [];
        });
      });
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
      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, blocklist subscribers by IDs.
        fn = () => {
          const ids = this.bulk.checked.map((s) => s.id);
          this.$api.blocklistSubscribers({ ids })
            .then(() => this.querySubscribers());
        };
      } else {
        // 'All' is selected, blocklist by query.
        fn = () => {
          this.$api.blocklistSubscribersByQuery({
            search: this.queryParams.search,
            query: this.queryParams.queryExp,
            list_ids: this.queryParams.listID ? [this.queryParams.listID] : null,
            subscription_status: this.queryParams.subStatus,
            ...this.gridFilter,
          }).then(() => this.querySubscribers());
        };
      }

      this.$utils.confirm(this.$t('subscribers.confirmBlocklist', { num: this.numSelectedSubscribers }), fn);
    },

    exportSubscribers() {
      const num = !this.bulk.all && this.bulk.checked.length > 0
        ? this.bulk.checked.length : this.subscribers.total;

      this.$utils.confirm(this.$t('subscribers.confirmExport', { num }), () => {
        const q = new URLSearchParams();

        if (this.queryParams.search) {
          q.append('search', this.queryParams.search);
        } else if (this.queryParams.queryExp) {
          q.append('query', this.queryParams.queryExp);
        }

        if (this.queryParams.listID) {
          q.append('list_id', this.queryParams.listID);
        }

        if (this.queryParams.subStatus) {
          q.append('subscription_status', this.queryParams.subStatus);
        }

        // Fork (list grid).
        Object.entries(this.gridFilter).forEach(([k, v]) => q.append(k, v));

        // Export selected subscribers.
        if (!this.bulk.all && this.bulk.checked.length > 0) {
          this.bulk.checked.map((s) => q.append('id', s.id));
        }

        document.location.href = `${uris.exportSubscribers}?${q.toString()}`;
      });
    },

    deleteSubscribers() {
      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, delete subscribers by IDs.
        fn = () => {
          const ids = this.bulk.checked.map((s) => s.id);
          this.$api.deleteSubscribers({ id: ids })
            .then(() => {
              this.querySubscribers();

              this.$utils.toast(this.$t('subscribers.subscribersDeleted', { num: this.numSelectedSubscribers }));
            });
        };
      } else {
        // 'All' is selected, delete by query.
        fn = () => {
          this.$api.deleteSubscribersByQuery({
            // If the query expression is empty, explicitly pass `all=true`
            // so that the backend deletes all records in the DB with an empty query string.
            all: this.queryParams.queryExp.trim() === '' && this.queryParams.search.trim() === '',
            search: this.queryParams.search,
            query: this.queryParams.queryExp,
            list_ids: this.queryParams.listID ? [this.queryParams.listID] : null,
            subscription_status: this.queryParams.subStatus,
            // Fork (list grid, D7). all=true above is sent exactly when a grid-linked page has
            // no search -- the server keeps the segment through it (it never voids a segment).
            ...this.gridFilter,
          }).then(() => {
            this.querySubscribers();

            this.$utils.toast(this.$t(
              'subscribers.subscribersDeleted',
              { num: this.numSelectedSubscribers },
            ));
          });
        };
      }

      this.$utils.confirm(this.$t('subscribers.confirmDelete', { num: this.numSelectedSubscribers }), fn);
    },

    bulkChangeLists(action, preconfirm, lists, backfill = false) {
      const data = {
        action,
        query: this.fullQueryExp,
        search: this.queryParams.search,
        list_ids: this.queryParams.listID ? [this.queryParams.listID] : null,
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
      } else if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, perform by IDs.
        fn = this.$api.addSubscribersToLists;
        data.ids = this.bulk.checked.map((s) => s.id);
      } else {
        // 'All' is selected, perform by query.
        data.query = this.queryParams.queryExp;
        data.subscription_status = this.queryParams.subStatus;
        Object.assign(data, this.gridFilter); // Fork (list grid, D7).
        fn = this.$api.addSubscribersToListsByQuery;
      }

      fn(data).then(() => {
        this.querySubscribers();
        this.$utils.toast(this.$t('subscribers.listChangeApplied'));
      });
    },
  },

  watch: {
    // Fork: a row-opened Manage lists modal dismissed without saving must not leave its target behind.
    isBulkListFormVisible(visible) {
      if (!visible) {
        this.listFormTarget = null;
      }
    },
  },

  computed: {
    ...mapState(['subscribers', 'lists', 'loading', 'serverConfig']),

    // Fork (list grid, LIST-GRID-SPEC D7). THE one place the segment/lang filter is read for a
    // request. Every read and every select-all write spreads it, so a by-query delete,
    // blocklist or list change can never reach beyond the rows the page is showing.
    gridFilter() {
      const out = {};
      if (this.queryParams.segment) {
        out.segment = this.queryParams.segment;
      }
      if (this.queryParams.lang) {
        out.lang = this.queryParams.lang;
      }
      return out;
    },

    // Fork (LIST-COLLAPSE-SPEC C8). Shown when languages are on, or when the route carries one
    // (so a lang= link stays visible and clearable even with app.lang_enable off).
    showLangPicker() {
      return this.serverConfig.lang_enabled || !!this.queryParams.lang;
    },

    langOptions() {
      return PICKER_LANGS;
    },

    // A route lang that is not one of the picker's own options.
    extraLang() {
      const l = this.queryParams.lang;
      if (!l || l === 'en_only' || l === 'none' || PICKER_LANGS.includes(l)) {
        return null;
      }
      return l;
    },

    extraLangLabel() {
      if (this.extraLang === 'en') {
        return this.$t('subscribers.langEnSend');
      }
      return this.extraLang === 'other' ? this.$t('lists.grid.other') : this.extraLang;
    },

    numSelectedSubscribers() {
      if (this.bulk.all) {
        return this.subscribers.total;
      }
      return this.bulk.checked.length;
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

  destroyed() {
    this.$root.$off('page.refresh', this.querySubscribers);
  },

  mounted() {
    if (this.$route.params.listID) {
      this.queryParams.listID = parseInt(this.$route.params.listID, 10);
    }
    if (this.$route.query.subscription_status) {
      this.queryParams.subStatus = this.$route.query.subscription_status;
    }
    // Fork (list grid). Unknown values are passed through and refused by the server (400).
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
    } else {
      // Get subscribers on load.
      this.querySubscribers();
    }
  },
});
</script>
