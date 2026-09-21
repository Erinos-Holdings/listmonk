<template>
  <section class="lists">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4 mb-2">
          {{ $t('globals.terms.lists') }}
          <span v-if="queryParams.status === 'archived'" class="has-text-grey-light">/ {{ queryParams.status }} </span>
          <span v-if="!isNaN(lists.total)">({{ lists.total }})</span>
        </h1>

        <div class="is-size-7">
          <router-link v-if="queryParams.status !== 'archived'" :to="{ name: 'lists', query: { status: 'archived' } }">
            {{ $t('globals.buttons.view') }} {{ $t('lists.archived').toLowerCase() }} &rarr;
          </router-link>
          <router-link v-else :to="{ name: 'lists' }">
            {{ $t('globals.buttons.view') }} {{ $t('menu.allLists').toLowerCase() }} &rarr;
          </router-link>
        </div>
      </div>
      <div class="column has-text-right">
        <b-field v-if="$can('lists:manage_all')" expanded>
          <b-button expanded type="is-primary" icon-left="plus" class="btn-new" @click="showNewForm" data-cy="btn-new">
            {{ $t('globals.buttons.new') }}
          </b-button>
        </b-field>
      </div>
    </header>

    <b-table :data="lists.results" :loading="loading.listsFull" @check-all="onTableCheck" @check="onTableCheck"
      :checked-rows.sync="bulk.checked" hoverable :default-sort="['created_at', 'desc']" paginated backend-pagination
      pagination-position="both" @page-change="onPageChange" :current-page="queryParams.page" :per-page="lists.perPage"
      :total="lists.total" checkable backend-sorting @sort="onSort" ref="table">
      <template #top-left>
        <div class="columns">
          <div class="column is-6">
            <form @submit.prevent="getLists">
              <b-field>
                <b-input v-model="queryParams.query" name="query" expanded icon="magnify" ref="query" data-cy="query" />
                <p class="controls">
                  <b-button native-type="submit" type="is-primary" icon-left="magnify" data-cy="btn-query" />
                </p>
              </b-field>
            </form>
          </div>
        </div>
        <div class="actions" v-if="bulk.checked.length > 0">
          <a class="a" href="#" @click.prevent="deleteLists" data-cy="btn-delete-lists">
            <b-icon icon="trash-can-outline" size="is-small" /> {{ $t('globals.buttons.delete') }}
          </a>
          <span class="a">
            {{ $tc('globals.messages.numSelected', numSelectedLists, { num: numSelectedLists }) }}
            <span v-if="!bulk.all && lists.total > lists.perPage">
              &mdash;
              <a href="#" @click.prevent="onSelectAll" data-cy="select-all-lists">
                {{ $tc('globals.messages.selectAll', lists.total, { num: lists.total }) }}
              </a>
            </span>
          </span>
        </div>
      </template>

      <!-- Fork (list collapse, LIST-COLLAPSE-SPEC C1/C2). Rows are collapsed by default; the
      chevron expands THIS row in place (never b-table's `detailed` slot, whose full-width detail
      row would take the language lines out of their columns). The column is deliberately its own
      NON-SORTABLE column so the expand-all control can have a #header slot: a slot on a sortable
      column's header drops Buefy's sort arrow, and the <th> click would re-sort. ICONS: the
      fork ships a fontello SUBSET (assets/icons/fontello.css) -- a name that is not in it renders
      nothing at all, silently (chevron-down and alert-outline both did). Check the file. -->
      <b-table-column custom-key="expand" header-class="cy-expand" cell-class="expand-cell" width="40">
        <template #header>
          <button type="button" class="expand-btn" data-cy="btn-expand-all"
            :aria-label="$t('lists.grid.expandAll')" :aria-expanded="allExpanded ? 'true' : 'false'"
            @click.stop="toggleExpandAll">
            <b-icon :icon="allExpanded ? 'minus' : 'plus'" size="is-small" />
          </button>
        </template>
        <template #default="props">
          <button type="button" class="expand-btn" data-cy="btn-expand"
            :aria-label="$t('lists.grid.expandRow', { name: props.row.name })"
            :aria-expanded="expanded[props.row.id] ? 'true' : 'false'" @click.stop="toggleExpand(props.row.id)">
            <b-icon :icon="expanded[props.row.id] ? 'minus' : 'plus'" size="is-small" />
          </button>
        </template>
      </b-table-column>

      <b-table-column v-slot="props" field="name" :label="$t('globals.fields.name')" header-class="cy-name" sortable
        paginated backend-pagination pagination-position="both" :td-attrs="$utils.tdID" @page-change="onPageChange">
        <div>
          <a :href="`/lists/${props.row.id}`" @click.prevent="showEditForm(props.row)">
            {{ props.row.name }}
          </a>
          <!-- Fork (LIST-COLLAPSE-SPEC C6). `Other` is an alarm, so it survives the collapse: the
          marker shows on the collapsed row and expands it. -->
          <b-tooltip v-if="hasOther(props.row) && !expanded[props.row.id]" :label="$t('lists.grid.otherHelp')" type="is-dark" multilined
            :triggers="['hover', 'focus']">
            <button type="button" class="grid-other-marker" data-cy="grid-other-marker"
              :aria-label="$t('lists.grid.otherHelp')" @click.stop="expandRow(props.row.id)">
              <b-icon icon="warning-empty" size="is-small" />
            </button>
          </b-tooltip>
          <!-- C3/C4: tags live behind the expand; one per line (C5) so the column can shrink. -->
          <b-taglist v-if="expanded[props.row.id]" class="stacked-tags">
            <b-tag class="is-small" v-for="t in props.row.tags" :key="t" :title="t">
              {{ t }}
            </b-tag>
          </b-taglist>
        </div>
      </b-table-column>

      <b-table-column v-slot="props" field="type" :label="$t('globals.fields.type')" header-class="cy-type" sortable>
        <!-- C3: the type tag alone when collapsed; C4 adds the opt-in tag and the opt-in campaign
        link on expand. C5: one per line. -->
        <div class="tags stacked-tags">
          <b-tag :class="props.row.type" :data-cy="`type-${props.row.type}`">
            {{ $t(`lists.types.${props.row.type}`) }}
          </b-tag>

          <template v-if="expanded[props.row.id]">
            <b-tag :class="props.row.optin" :data-cy="`optin-${props.row.optin}`">
              <b-icon :icon="props.row.optin === 'double' ? 'account-check-outline' : 'account-off-outline'"
                size="is-small" />
              {{ ' ' }}
              {{ $t(`lists.optins.${props.row.optin}`) }}
            </b-tag>

            <a v-if="props.row.optin === 'double'" class="is-size-7 send-optin" href="#"
              @click="$utils.confirm(null, () => createOptinCampaign(props.row))" data-cy="btn-send-optin-campaign">
              <b-tooltip :label="$t('lists.sendOptinCampaign')" type="is-dark">
                <b-icon icon="rocket-launch-outline" size="is-small" />
                {{ $t('lists.sendOptinCampaign') }}
              </b-tooltip>
            </a>
          </template>
        </div>
      </b-table-column>

      <!-- Fork (list grid, integrations LIST-GRID-SPEC D8/D9). Six columns that partition the
      list -- Subscribers (total) and the five segments -- by one "all" line and, where the list
      has more than one send language (or a no-language residue, or an unrecognised value), one
      line per SEND language. Every number links to exactly the subscribers it counts
      (?segment=&lang=, filters the server writes). The numbers are the API's subscriber_grid as
      is. Nothing is added up or folded here -- en already includes the no-language rows. -->
      <b-table-column v-for="col in visibleGridCols" :key="col.key" v-slot="props" :field="col.field"
        :label="$t(col.label)" :th-attrs="() => gridThAttrs(col)" :header-class="`cy-${col.key}`"
        cell-class="grid-cell" numeric sortable>
        <!-- Pending cannot occur on a single opt-in list. -->
        <div v-if="!(col.key === 'pending' && props.row.optin === 'single')" class="grid-lines">
          <p v-for="line in visibleLines(props.row)" :key="line.key" :class="['grid-line', `lang-${line.key}`]"
            :data-cy="`grid-${col.key}-${line.key}`">
            <!-- The line's label sits in the first (Subscribers) column. -->
            <template v-if="col.key === 'total' && line.key !== 'all'">
              <b-tooltip v-if="line.key === 'other'" :label="$t('lists.grid.otherHelp')" type="is-dark" multilined
                :triggers="['hover', 'focus']">
                <component :is="canViewSubs ? 'router-link' : 'span'" :to="gridLink(props.row, '', line.key)"
                  class="grid-label">
                  {{ $t('lists.grid.other') }}
                </component>
              </b-tooltip>
              <component v-else :is="canViewSubs ? 'router-link' : 'span'" :to="gridLink(props.row, '', line.key)"
                class="grid-label">
                {{ line.key.toUpperCase() }}
              </component>
            </template>

            <span v-if="!line.row[col.key]" class="grid-zero">0</span>
            <b-tooltip v-else-if="line.key === 'en' && line.none && line.none[col.key] > 0" type="is-dark"
              :triggers="['hover', 'focus']" :label="$t('lists.grid.enSplit', {
                en: $utils.formatNumber(line.row[col.key] - line.none[col.key]),
                none: $utils.formatNumber(line.none[col.key]),
              })">
              <component :is="canViewSubs ? 'router-link' : 'span'"
                :to="gridLink(props.row, col.key === 'total' ? '' : col.key, line.key)" :class="col.key">
                {{ $utils.formatNumber(line.row[col.key]) }}
              </component>
            </b-tooltip>
            <component v-else :is="canViewSubs ? 'router-link' : 'span'"
              :to="gridLink(props.row, col.key === 'total' ? '' : col.key, line.key)" :class="col.key">
              {{ $utils.formatNumber(line.row[col.key]) }}
            </component>
          </p>
        </div>
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
          <router-link v-if="$can('campaigns:manage')" :to="`/campaigns/new?list_id=${props.row.id}`"
            data-cy="btn-campaign">
            <b-tooltip :label="$t('lists.sendCampaign')" type="is-dark">
              <b-icon icon="rocket-launch-outline" size="is-small" />
            </b-tooltip>
          </router-link>

          <a v-if="$can('lists:manage') || $canList(props.row.id, 'list:manage')" href="#"
            @click.prevent="showEditForm(props.row)" data-cy="btn-edit" :aria-label="$t('globals.buttons.edit')">
            <b-tooltip :label="$t('globals.buttons.edit')" type="is-dark">
              <b-icon icon="pencil-outline" size="is-small" />
            </b-tooltip>
          </a>

          <router-link v-if="$can('subscribers:import')" :to="{ name: 'import', query: { list_id: props.row.id } }"
            data-cy="btn-import">
            <b-tooltip :label="$t('import.title')" type="is-dark">
              <b-icon icon="file-upload-outline" size="is-small" />
            </b-tooltip>
          </router-link>

          <a v-if="$can('lists:manage') || $canList(props.row.id, 'list:manage')" href="#"
            @click.prevent="deleteList(props.row)" data-cy="btn-delete" :aria-label="$t('globals.buttons.delete')">
            <b-tooltip :label="$t('globals.buttons.delete')" type="is-dark">
              <b-icon icon="trash-can-outline" size="is-small" />
            </b-tooltip>
          </a>
        </div>
      </b-table-column>

      <template #empty v-if="!loading.listsFull">
        <empty-placeholder />
      </template>
    </b-table>

    <!-- Add / edit form modal -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isFormVisible" :width="600" @close="onFormClose">
      <list-form :data="curItem" :is-editing="isEditing" @finished="formFinished" />
    </b-modal>

    <p v-if="settings['app.cache_slow_queries']" class="has-text-grey">
      *{{ $t('globals.messages.slowQueriesCached') }}
      <a href="https://listmonk.app/docs/maintenance/performance/" target="_blank" rel="noopener noreferer"
        class="has-text-grey">
        <b-icon icon="link-variant" /> {{ $t('globals.buttons.learnMore') }}
      </a>
    </p>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import ListForm from './ListForm.vue';

// Fork (list grid). Send-language lines in display order. The API's en already includes none.
const GRID_LANGS = ['en', 'fr', 'es', 'de', 'it', 'other'];

// Fork. Columns whose FIRST click sorts ascending (see onSort). Text columns go here; every other
// sortable column -- counts and dates -- sorts biggest/newest first.
const ASC_FIRST_FIELDS = ['name', 'type'];

export default Vue.extend({
  components: {
    ListForm,
    EmptyPlaceholder,
  },

  data() {
    return {
      // Current list item being edited.
      curItem: null,
      isEditing: false,
      isFormVisible: false,
      lists: [],

      // Fork (list grid). field = a column in core.go listQuerySortFields (the grid's all row).
      // title (LIST-COLLAPSE-SPEC C9) = the full word behind an abbreviated header, rendered as
      // the <th>'s title through :th-attrs -- NEVER a #header slot, which would drop the sort
      // arrow. globals.terms.subscribers and lists.grid.unsubscribed are shared keys and are not
      // edited: the short forms are their own keys (V6).
      gridCols: [
        {
          key: 'total',
          field: 'subscriber_count',
          label: 'lists.grid.totalShort',
          title: 'globals.terms.subscribers',
        },
        { key: 'active', field: 'active_count', label: 'lists.grid.active' },
        { key: 'held', field: 'held_count', label: 'lists.grid.held' },
        {
          key: 'unsubscribed',
          field: 'unsubscribed_count',
          label: 'lists.grid.unsubscribedShort',
          title: 'lists.grid.unsubscribed',
        },
        { key: 'pending', field: 'pending_count', label: 'lists.grid.pending' },
        { key: 'blocked', field: 'blocked_count', label: 'lists.grid.blocked' },
      ],

      // Fork (list collapse, C1). Expanded row ids, { [listId]: true }. A plain object because
      // Vue 2.7 does not make a Set reactive. Not persisted: reset on every mount, and it
      // survives paging, sorting and getLists() refreshes within the visit (ids that are no
      // longer on the page are simply inert).
      expanded: {},

      // Fork (list collapse, C10). Does ANY list (under the page's status, ignoring search and
      // pagination) have Pending? Starts hidden; a rejected probe shows the column -- a hidden
      // non-zero Pending is worse than a column of zeros.
      hasPending: false,

      queryParams: {
        page: 1,
        query: '',
        // Fork -- must name a column in core.go listQuerySortFields; anything else is
        // silently replaced by created_at DESC server-side, and must match default-sort.
        orderBy: 'created_at',
        order: 'desc',
        status: this.$route.query.status || 'active',
      },

      // Table bulk row selection states.
      bulk: {
        checked: [],
        all: false,
      },
    };
  },

  methods: {
    onPageChange(p) {
      this.queryParams.page = p;
      this.getLists();
    },

    // Fork. Counts and dates sort biggest/newest first on the FIRST click of a column; Name and
    // Type stay A-Z. Buefy's default-sort-direction is table-wide and read inside sort() before
    // it emits, so the per-column rule lives here: on a click that CHANGES the column Buefy has
    // just set isAsc = true and emits 'asc' -- flip its isAsc (the arrow reads it; Buefy does not
    // touch it again after the emit) and ask the server for desc. A second click on the same
    // column is Buefy's own toggle and passes through untouched. isAsc is Buefy-internal state
    // (verified against 0.9.29): if an upgrade ever removes it, skip the whole override and fall
    // back to stock asc-first, so the arrow and the data can never disagree.
    onSort(field, direction) {
      let order = direction;
      const { table } = this.$refs;
      const descFirst = !ASC_FIRST_FIELDS.includes(field);
      if (descFirst && field !== this.queryParams.orderBy && direction === 'asc'
        && table && typeof table.isAsc === 'boolean') {
        table.isAsc = false;
        order = 'desc';
      }
      this.queryParams.orderBy = field;
      this.queryParams.order = order;
      this.getLists();
    },

    // Show the edit list form.
    showEditForm(list) {
      this.curItem = list;
      this.isFormVisible = true;
      this.isEditing = true;
    },

    // Show the new list form.
    showNewForm() {
      this.curItem = {};
      this.isFormVisible = true;
      this.isEditing = false;
    },

    formFinished() {
      this.refresh();
    },

    // A refresh that may have changed the data re-asks the Pending question too (C10).
    refresh() {
      this.getLists();
      this.probePending();
    },

    // Fork (list collapse, C1/C2).
    toggleExpand(id) {
      this.$set(this.expanded, id, !this.expanded[id]);
    },

    expandRow(id) {
      this.$set(this.expanded, id, true);
    },

    // If any row on the page is collapsed, expand all; otherwise collapse all.
    toggleExpandAll() {
      const expand = !this.allExpanded;
      this.pageRows.forEach((l) => this.$set(this.expanded, l.id, expand));
    },

    // C6. The unrecognised-language alarm, which must be visible without expanding.
    hasOther(list) {
      // With app.lang_enable off gridLines shows no Other line, so there is nothing to expand to.
      if (!this.serverConfig.lang_enabled) {
        return false;
      }
      const grid = list.subscriberGrid || {};
      return !!(grid.other && grid.other.total > 0);
    },

    // C3/C4. Collapsed = the "all" line alone; expanded = today's gridLines rule, unchanged.
    visibleLines(list) {
      const lines = this.gridLines(list);
      return this.expanded[list.id] ? lines : lines.slice(0, 1);
    },

    // C9. The full word behind an abbreviated header.
    gridThAttrs(col) {
      return col.title ? { title: this.$t(col.title) } : {};
    },

    onFormClose() {
      if (this.$route.params.id) {
        this.$router.push({ name: 'lists' });
      }
    },

    // Fork (list grid). The lines of one list's grid: "all", then one per send language with
    // subscribers, in a fixed order. Language lines show only with app.lang_enable on AND when
    // they say something the "all" line does not: more than one send language, a no-language
    // residue (none, a subset of en), or an unrecognised value (other -- an alarm, so it is
    // never hidden behind the one-language rule).
    gridLines(list) {
      const grid = list.subscriberGrid || {};
      const lines = [{ key: 'all', row: grid.all || {} }];
      if (!this.serverConfig.lang_enabled) {
        return lines;
      }

      const langs = GRID_LANGS.filter((k) => grid[k] && grid[k].total > 0);
      const hasNone = grid.none && grid.none.total > 0;
      if (langs.length > 1 || hasNone || langs.includes('other')) {
        langs.forEach((k) => lines.push({ key: k, row: grid[k], none: k === 'en' ? grid.none : null }));
      }
      return lines;
    },

    // The subscribers page filtered to exactly what a cell counts. segment '' = the whole
    // line, lang 'all' = the whole column.
    gridLink(list, segment, lang) {
      const query = {};
      if (segment) {
        query.segment = segment;
      }
      if (lang && lang !== 'all') {
        query.lang = lang;
      }
      return { path: `/subscribers/lists/${list.id}`, query };
    },

    getLists() {
      this.$api.queryLists({
        page: this.queryParams.page,
        query: this.queryParams.query.replace(/[^\p{L}\p{N}\s]/gu, ' '),
        order_by: this.queryParams.orderBy,
        order: this.queryParams.order,
        status: this.queryParams.status,
      }).then((resp) => {
        this.lists = resp;
      });

      // Also fetch the minimal lists for the global store that appears
      // in dropdown menus on other pages like import and campaigns.
      this.$api.getLists({ minimal: true, per_page: 'all', status: 'active' });
    },

    // Fork (list collapse, C10). Is the Pending column worth a column at all? One extra
    // query-lists execution ordered by pending_count -- read as subscriberGrid.all.pending,
    // because pending_count itself is `json:"-"` (a sort key only) and would be undefined.
    // Issued with the page's first load and after a write, never on paging, sorting or search,
    // so the column set is stable across pages. An empty result set means hidden; a rejected
    // request shows the column (fail open, V8).
    probePending() {
      this.$api.probeListsPending({ status: this.queryParams.status }).then((resp) => {
        const results = (resp && resp.results) || [];
        this.hasPending = ((results[0] && results[0].subscriberGrid
          && results[0].subscriberGrid.all && results[0].subscriberGrid.all.pending) || 0) > 0;
      }).catch(() => {
        this.hasPending = true;
      });
    },

    deleteList(list) {
      this.$utils.confirm(
        this.$t('lists.confirmDelete'),
        () => {
          this.$api.deleteList(list.id).then(() => {
            this.getLists();
            this.probePending();

            this.$utils.toast(this.$t('globals.messages.deleted', { name: list.name }));
          });
        },
      );
    },

    // Mark all lists in the query as selected.
    onSelectAll() {
      this.bulk.all = true;
    },

    onTableCheck() {
      // Disable bulk.all selection if there are no rows checked in the table.
      if (this.bulk.checked.length !== this.lists.total) {
        this.bulk.all = false;
      }
    },

    deleteLists() {
      const name = this.$tc('globals.terms.list', this.numSelectedCampaigns);

      const fn = () => {
        const params = {};
        if (!this.bulk.all && this.bulk.checked.length > 0) {
          // If 'all' is not selected, delete lists by IDs.
          params.id = this.bulk.checked.map((l) => l.id);
        } else {
          // 'All' is selected, delete by query.
          params.query = this.queryParams.query.replace(/[^\p{L}\p{N}\s]/gu, ' ');
          params.all = this.bulk.all;
        }

        this.$api.deleteLists(params)
          .then(() => {
            this.getLists();
            this.probePending();
            this.$utils.toast(this.$tc(
              'globals.messages.deletedCount',
              this.numSelectedLists,
              { num: this.numSelectedLists, name },
            ));
          });
      };

      this.$utils.confirm(this.$tc(
        'globals.messages.confirmDelete',
        this.numSelectedLists,
        { num: this.numSelectedLists, name: name.toLowerCase() },
      ), fn);
    },

    createOptinCampaign(list) {
      const data = {
        name: this.$t('lists.optinTo', { name: list.name }),
        subject: this.$t('lists.confirmSub', { name: list.name }),
        lists: [list.id],
        from_email: this.settings['app.from_email'],
        content_type: 'richtext',
        messenger: 'email',
        type: 'optin',
      };

      this.$api.createCampaign(data).then((d) => {
        this.$router.push({ name: 'campaign', hash: '#content', params: { id: d.id } });
      });
      return false;
    },
  },

  computed: {
    ...mapState(['loading', 'settings', 'serverConfig']),

    // Without a subscribers permission the grid is plain numbers.
    canViewSubs() {
      return this.$can('subscribers:get_all', 'subscribers:get');
    },

    numSelectedLists() {
      return this.bulk.all ? this.lists.total : this.bulk.checked.length;
    },

    // Fork (list collapse, C2). The rows of the current page.
    pageRows() {
      return this.lists.results || [];
    },

    allExpanded() {
      return this.pageRows.length > 0 && this.pageRows.every((l) => this.expanded[l.id]);
    },

    // Fork (list collapse, C10). Pending renders only when some list has Pending -- or while it
    // is the active sort key: Buefy reads default-sort once, so destroying the sorted column
    // would strand the sort with no way to reset it.
    visibleGridCols() {
      return this.gridCols.filter((c) => c.key !== 'pending'
        || this.hasPending || this.queryParams.orderBy === 'pending_count');
    },
  },

  created() {
    this.$root.$on('page.refresh', this.refresh);
  },

  destroyed() {
    this.$root.$off('page.refresh', this.refresh);
  },

  mounted() {
    if (this.$route.params.id) {
      this.$api.getList(parseInt(this.$route.params.id, 10)).then((data) => {
        this.showEditForm(data);
      });
    } else {
      this.getLists();
      this.probePending();
    }
  },
});
</script>
