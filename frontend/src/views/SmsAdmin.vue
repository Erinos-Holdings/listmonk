<template>
  <!-- Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC D1/D7/D9/D10). Two tabs: the yq.link
  short links (through the sms:manage proxy to the ShortlinksProd admin API) and the SMS promo code
  report over the purchase feed. The page reads counts, never computes them; its pure logic is
  smsAdmin.mjs. Disposable at upstream v7 (the API half in cmd/sms.go survives). -->
  <section class="sms-admin">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4">{{ $t('sms.title') }}</h1>
      </div>
    </header>

    <b-tabs type="is-boxed" :animated="false" v-model="tab">
      <!-- Short links -->
      <b-tab-item :label="$t('sms.tabLinks')" value="links" icon="link-variant">
        <b-notification v-if="linksError" :type="linksErrorStatus === 503 ? 'is-warning' : 'is-danger'" :closable="false"
          data-cy="links-error">
          {{ linksError }}
        </b-notification>

        <div class="columns is-vcentered mb-2">
          <div class="column">
            <b-field>
              <b-radio-button v-for="f in filters" :key="f" v-model="filter" :native-value="f" size="is-small"
                type="is-light is-outlined">
                {{ $t(filterLabels[f]) }}
              </b-radio-button>
            </b-field>
          </div>
          <div class="column has-text-right">
            <b-button type="is-primary" icon-left="plus" :disabled="linksErrorStatus === 503" @click="showNewLink"
              data-cy="btn-new-link">
              {{ $t('sms.newLink') }}
            </b-button>
          </div>
        </div>

        <b-table :data="shownLinks" :loading="loadingLinks" hoverable default-sort="slug">
          <b-table-column v-slot="props" field="slug" :label="$t('sms.shortUrl')" sortable>
            <span :class="{ 'has-text-grey': props.row.active === false }">{{ shortUrlOf(props.row.slug) }}</span>
            <copy-text :text="shortUrlOf(props.row.slug)" hide-text />
          </b-table-column>
          <b-table-column v-slot="props" field="target" :label="$t('sms.target')">
            <a v-if="isLink(props.row.target)" :href="props.row.target" target="_blank" rel="noopener noreferrer"
              class="sms-target">{{ props.row.target }}</a>
            <span v-else class="sms-target">{{ props.row.target }}</span>
          </b-table-column>
          <b-table-column v-slot="props" field="brand" :label="$t('sms.brand')" sortable>
            {{ props.row.brand || '—' }}
          </b-table-column>
          <b-table-column v-slot="props" field="campaign" :label="$t('sms.campaign')" sortable>
            {{ props.row.campaign || '—' }}
          </b-table-column>
          <b-table-column v-slot="props" field="clicks" :label="$t('sms.clicks')" numeric sortable>
            {{ props.row.clicks || 0 }}
          </b-table-column>
          <b-table-column v-slot="props" field="previews" :label="$t('sms.previews')" numeric sortable>
            {{ props.row.previews || 0 }}
          </b-table-column>
          <b-table-column v-slot="props" field="last_click_at" :label="$t('sms.lastClick')" sortable>
            {{ time(props.row.last_click_at) }}
          </b-table-column>
          <b-table-column v-slot="props" field="active" :label="$t('sms.active')" sortable>
            <b-tag :class="props.row.active === false ? '' : 'enabled'">
              {{ props.row.active === false ? $t('sms.filterInactive') : $t('sms.filterActive') }}
            </b-tag>
          </b-table-column>
          <b-table-column v-slot="props" field="updated_at" :label="$t('sms.updated')" sortable>
            {{ time(props.row.updated_at) }}
          </b-table-column>
          <b-table-column v-slot="props" cell-class="actions" align="right">
            <div>
              <a href="#" @click.prevent="showEditLink(props.row)" data-cy="btn-edit-link" :aria-label="$t('sms.edit')">
                <b-tooltip :label="$t('sms.edit')" type="is-dark">
                  <b-icon icon="pencil-outline" size="is-small" />
                </b-tooltip>
              </a>
              <a v-if="props.row.active !== false" href="#" @click.prevent="deactivate(props.row)"
                data-cy="btn-deactivate-link" :aria-label="$t('sms.deactivate')">
                <b-tooltip :label="$t('sms.deactivate')" type="is-dark">
                  <b-icon icon="cancel" size="is-small" />
                </b-tooltip>
              </a>
              <a v-else href="#" @click.prevent="reactivate(props.row)" data-cy="btn-reactivate-link"
                :aria-label="$t('sms.reactivate')">
                <b-tooltip :label="$t('sms.reactivate')" type="is-dark">
                  <b-icon icon="check-circle-outline" size="is-small" />
                </b-tooltip>
              </a>
            </div>
          </b-table-column>
          <template #empty v-if="!loadingLinks">
            <empty-placeholder />
          </template>
        </b-table>
      </b-tab-item>

      <!-- Promo codes -->
      <b-tab-item :label="$t('sms.tabPromo')" value="promo" icon="tag-outline">
        <b-notification v-if="report.feed.stale" type="is-warning" :closable="false" data-cy="feed-stale">
          {{ $t('sms.feedStale', { date: time(report.feed.last_received_at) }) }}
        </b-notification>
        <p class="is-size-7 has-text-grey mb-4" data-cy="report-note">
          {{ $t('sms.reportNote') }}
          <template v-if="report.feed.oldest_ordered_at">
            {{ $t('sms.feedBegins', { date: time(report.feed.oldest_ordered_at) }) }}
          </template>
          <template v-else>{{ $t('sms.feedEmpty') }}</template>
        </p>

        <form class="box" @submit.prevent="addCode" data-cy="add-code">
          <div class="columns">
            <div class="column is-6">
              <b-field :label="$t('sms.code')" label-position="on-border" :message="$t('sms.codeHelp')">
                <b-input v-model="codeForm.code" :maxlength="128" required list="sms-known-codes" autocomplete="off"
                  name="code" data-cy="code" />
              </b-field>
              <datalist id="sms-known-codes">
                <option v-for="k in report.known" :key="k.value" :value="k.value">
                  {{ k.value }} ({{ k.orders }})
                </option>
              </datalist>
            </div>
            <div class="column is-4">
              <b-field :label="$t('sms.brand')" label-position="on-border">
                <b-select v-model="codeForm.brand" expanded name="brand">
                  <option value="">{{ $t('sms.allBrands') }}</option>
                  <option v-for="b in brands" :key="b.slug" :value="b.slug">{{ brandLabel(b) }}</option>
                </b-select>
              </b-field>
            </div>
            <div class="column is-2">
              <b-button native-type="submit" type="is-primary" expanded :loading="savingCode" data-cy="btn-add-code">
                {{ $t('sms.addCode') }}
              </b-button>
            </div>
          </div>
        </form>

        <b-table :data="report.rows" :loading="loadingReport" hoverable>
          <b-table-column v-slot="props" field="code" :label="$t('sms.code')">
            <strong>{{ props.row.code }}</strong>
          </b-table-column>
          <b-table-column v-slot="props" field="brand" :label="$t('sms.brand')">
            {{ props.row.brand || $t('sms.allBrands') }}
          </b-table-column>
          <b-table-column v-slot="props" field="orders" :label="$t('sms.orders')" numeric>
            <span v-if="props.row.orders > 0">{{ $utils.formatNumber(props.row.orders) }}</span>
            <span v-else class="has-text-grey">{{ $t('sms.noOrders') }}</span>
          </b-table-column>
          <b-table-column v-slot="props" field="buyers" :label="$t('sms.buyers')" numeric>
            {{ props.row.orders > 0 ? $utils.formatNumber(props.row.buyers) : '—' }}
          </b-table-column>
          <b-table-column v-slot="props" field="first_order_at" :label="$t('sms.firstOrder')">
            {{ time(props.row.first_order_at) }}
          </b-table-column>
          <b-table-column v-slot="props" field="last_order_at" :label="$t('sms.lastOrder')">
            {{ time(props.row.last_order_at) }}
          </b-table-column>
          <b-table-column v-slot="props" field="created_by" :label="$t('sms.markedBy')">
            {{ props.row.created_by }}
            <div class="is-size-7 has-text-grey">{{ time(props.row.created_at) }}</div>
          </b-table-column>
          <b-table-column v-slot="props" cell-class="actions" align="right">
            <a href="#" @click.prevent="unmark(props.row)" data-cy="btn-unmark" :aria-label="$t('sms.unmark')">
              <b-tooltip :label="$t('sms.unmark')" type="is-dark">
                <b-icon icon="trash-can-outline" size="is-small" />
              </b-tooltip>
            </a>
          </b-table-column>
          <template #empty v-if="!loadingReport">
            <p class="has-text-grey has-text-centered p-4">{{ $t('sms.emptyCodes') }}</p>
          </template>
        </b-table>
      </b-tab-item>
    </b-tabs>

    <!-- New / edit link modal -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isLinkFormVisible" :width="640">
      <form @submit.prevent="saveLink">
        <div class="modal-card content" style="width: auto">
          <header class="modal-card-head">
            <h4>{{ editingSlug ? $t('sms.editLink') : $t('sms.newLink') }}</h4>
          </header>
          <section expanded class="modal-card-body">
            <b-notification v-if="linkFormError" type="is-danger" :closable="false" data-cy="link-form-error">
              {{ linkFormError }}
            </b-notification>
            <b-field :label="$t('sms.slug')" label-position="on-border"
              :message="editingSlug ? shortUrlOf(editingSlug) : $t('sms.slugHelp')">
              <b-input v-model="linkForm.slug" :disabled="!!editingSlug" :maxlength="40" name="slug" autocomplete="off"
                data-cy="slug" />
            </b-field>
            <b-field :label="$t('sms.target')" label-position="on-border" :message="$t('sms.targetHelp')">
              <b-input v-model="linkForm.target" required :maxlength="2048" name="target" autocomplete="off"
                placeholder="https://" data-cy="target" />
            </b-field>
            <div class="columns is-multiline">
              <div v-for="k in utmKeys" :key="k" class="column is-6">
                <b-field :label="k" label-position="on-border">
                  <b-input v-model="linkForm[k]" :maxlength="200" :name="k" autocomplete="off" />
                </b-field>
              </div>
              <div class="column is-6">
                <b-field :label="$t('sms.brand')" label-position="on-border">
                  <b-select v-model="linkForm.brand" expanded name="brand">
                    <option value="">{{ $t('sms.noBrand') }}</option>
                    <option v-for="b in brands" :key="b.slug" :value="b.slug">{{ brandLabel(b) }}</option>
                    <option v-if="linkForm.brand && !brands.some((b) => b.slug === linkForm.brand)" :value="linkForm.brand">
                      {{ linkForm.brand }}
                    </option>
                  </b-select>
                </b-field>
              </div>
            </div>
            <b-field :label="$t('sms.campaign')" label-position="on-border">
              <b-input v-model="linkForm.campaign" :maxlength="200" name="campaign" autocomplete="off" />
            </b-field>
            <b-field :label="$t('sms.notes')" label-position="on-border">
              <b-input v-model="linkForm.notes" :maxlength="200" name="notes" type="textarea" rows="2" />
            </b-field>
            <b-field v-if="editingSlug">
              <b-checkbox v-model="linkForm.active" data-cy="active">{{ $t('sms.active') }}</b-checkbox>
            </b-field>
          </section>
          <footer class="modal-card-foot has-text-right">
            <b-button @click="isLinkFormVisible = false">{{ $t('globals.buttons.close') }}</b-button>
            <b-button native-type="submit" type="is-primary" :loading="savingLink" data-cy="btn-save-link">
              {{ $t('globals.buttons.save') }}
            </b-button>
          </footer>
        </div>
      </form>
    </b-modal>
  </section>
</template>

<script>
import Vue from 'vue';
import CopyText from '../components/CopyText.vue';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import {
  createBody, filterLinks, formFromLink, isLinkTarget, linkErrorMessage, shortUrl, smsTime, updateBody,
  DEFAULT_LINK_FILTER, LINK_FILTERS, UTM_KEYS,
} from '../smsAdmin.mjs'; // eslint-disable-line import/extensions

const errStatus = (err) => (err && err.response ? err.response.status : 0);
const errMessage = (err) => (err && err.response && err.response.data && err.response.data.message) || '';

export default Vue.extend({
  components: { CopyText, EmptyPlaceholder },

  data() {
    return {
      tab: 'links',
      filters: LINK_FILTERS,
      filterLabels: { active: 'sms.filterActive', inactive: 'sms.filterInactive', all: 'sms.filterAll' },
      filter: DEFAULT_LINK_FILTER,
      utmKeys: UTM_KEYS,

      links: [],
      loadingLinks: false,
      linksError: '',
      linksErrorStatus: 0,

      isLinkFormVisible: false,
      editingSlug: '',
      linkForm: formFromLink(null),
      linkFormError: '',
      savingLink: false,

      brands: [],

      report: { rows: [], feed: {}, known: [] },
      loadingReport: false,
      codeForm: { code: '', brand: '' },
      savingCode: false,
    };
  },

  computed: {
    shownLinks() {
      return filterLinks(this.links, this.filter);
    },
  },

  methods: {
    shortUrlOf: shortUrl,
    isLink: isLinkTarget,
    time: smsTime,

    brandLabel(b) {
      return b.display_name ? `${b.display_name} (${b.slug})` : b.slug;
    },

    getLinks() {
      this.loadingLinks = true;
      this.$api.getSmsLinks().then((data) => {
        this.links = Array.isArray(data) ? data : [];
        this.linksError = '';
        this.linksErrorStatus = 0;
      }).catch((err) => {
        this.links = [];
        this.linksErrorStatus = errStatus(err);
        this.linksError = linkErrorMessage(this.linksErrorStatus, errMessage(err), (k) => this.$t(k));
      }).finally(() => {
        this.loadingLinks = false;
      });
    },

    getReport() {
      this.loadingReport = true;
      this.$api.getSmsPromoCodes().then((data) => {
        this.report = { rows: data.rows || [], feed: data.feed || {}, known: data.known || [] };
      }).finally(() => {
        this.loadingReport = false;
      });
    },

    getBrands() {
      this.$api.getBrands(true).then((rows) => {
        this.brands = Array.isArray(rows) ? rows : [];
      }).catch(() => {
        this.brands = [];
      });
    },

    showNewLink() {
      this.editingSlug = '';
      this.linkForm = formFromLink(null);
      this.linkFormError = '';
      this.isLinkFormVisible = true;
    },

    showEditLink(link) {
      this.editingSlug = link.slug;
      this.linkForm = formFromLink(link);
      this.linkFormError = '';
      this.isLinkFormVisible = true;
    },

    saveLink() {
      this.savingLink = true;
      this.linkFormError = '';
      const req = this.editingSlug
        ? this.$api.updateSmsLink(this.editingSlug, updateBody(this.linkForm))
        : this.$api.createSmsLink(createBody(this.linkForm));
      req.then((link) => {
        this.isLinkFormVisible = false;
        this.$utils.toast(this.$t('sms.saved', { slug: link.slug }));
        this.getLinks();
      }).catch((err) => {
        // Keyed on the status alone: a 409 is "That slug is taken" and nothing changed.
        this.linkFormError = linkErrorMessage(errStatus(err), errMessage(err), (k) => this.$t(k));
      }).finally(() => {
        this.savingLink = false;
      });
    },

    deactivate(link) {
      this.$utils.confirm(this.$t('sms.confirmDeactivate', { slug: link.slug }), () => {
        this.$api.deleteSmsLink(link.slug).then(() => {
          this.$utils.toast(this.$t('sms.deactivated', { slug: link.slug }));
          this.getLinks();
        }).catch((err) => {
          this.$utils.toast(linkErrorMessage(errStatus(err), errMessage(err), (k) => this.$t(k)), 'is-danger');
        });
      });
    },

    // Reactivation is the whole record with active true (an omitted field would be cleared).
    reactivate(link) {
      this.$api.updateSmsLink(link.slug, updateBody({ ...formFromLink(link), active: true })).then(() => {
        this.$utils.toast(this.$t('sms.reactivated', { slug: link.slug }));
        this.getLinks();
      }).catch((err) => {
        this.$utils.toast(linkErrorMessage(errStatus(err), errMessage(err), (k) => this.$t(k)), 'is-danger');
      });
    },

    addCode() {
      const code = this.codeForm.code.trim();
      if (!code) {
        return;
      }
      this.savingCode = true;
      this.$api.createSmsPromoCode({ code, brand: this.codeForm.brand }).then((row) => {
        this.$utils.toast(this.$t('sms.marked', { code: row.code }));
        this.codeForm = { code: '', brand: '' };
        this.getReport();
      }).catch(() => {
        // The interceptor toasted the server's message (409 duplicate, 400 invalid code or brand).
      }).finally(() => {
        this.savingCode = false;
      });
    },

    unmark(row) {
      this.$utils.confirm(this.$t('sms.confirmUnmark', { code: row.code }), () => {
        this.$api.deleteSmsPromoCode(row.id).then(() => {
          this.$utils.toast(this.$t('sms.unmarked', { code: row.code }));
          this.getReport();
        });
      });
    },
  },

  mounted() {
    if (!this.$can('sms:manage')) {
      this.$router.replace({ name: 'dashboard' });
      return;
    }
    this.getBrands();
    this.getLinks();
    this.getReport();
  },
});
</script>

<style scoped>
.sms-target {
  word-break: break-all;
}
</style>
