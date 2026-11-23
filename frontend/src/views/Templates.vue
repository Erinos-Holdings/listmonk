<template>
  <section class="templates">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4">
          {{ $t('globals.terms.templates') }}
          <span v-if="shownTemplates.length > 0">({{ shownTemplates.length }})</span>
        </h1>
      </div>
      <div class="column has-text-right">
        <b-field v-if="$can('templates:manage')" expanded>
          <b-button expanded type="is-primary" icon-left="plus" class="btn-new" @click="showNewForm">
            {{ $t('globals.buttons.new') }}
          </b-button>
        </b-field>
      </div>
    </header>

    <!-- Polish 2026-11-14 (the user): the Footer | Other toggle (the official footer templates, or
         everything else; templatesPage.mjs), the Campaigns page's page-local .segmented style. -->
    <b-field class="segmented mb-3" data-cy="template-kind">
      <b-radio-button :value="kind" native-value="footer" type="is-primary" data-cy="template-kind-footer" @input="onKind">
        {{ $t('templates.kindFooter') }}
      </b-radio-button>
      <b-radio-button :value="kind" native-value="other" type="is-primary" data-cy="template-kind-other" @input="onKind">
        {{ $t('templates.kindOther') }}
      </b-radio-button>
    </b-field>

    <!-- Fork (global brand, integrations GLOBAL-BRAND-SPEC D12) -- filtered client-side; the
         store stays every row (the campaign form's template picker is never filtered, S3).
         Polish 2026-11-14: the filter caption -- the selected brand and language (filterBar.mjs). -->
    <b-table :data="shownTemplates" :hoverable="true" :loading="loading.templates" default-sort="createdAt"
      :caption="$filterCaption()">
      <!-- Polish 2026-11-16 (the user): ID is the first column. -->
      <b-table-column v-slot="props" field="id" :label="$t('globals.fields.id')" sortable>
        {{ props.row.id }}
      </b-table-column>

      <b-table-column v-slot="props" field="name" :label="$t('globals.fields.name')" :td-attrs="$utils.tdID" sortable>
        <!-- Fork (naming standard, NAMING-STANDARD-SPEC D10) -- the former name on hover. Polish
             2026-11-16 (the user): no brand chip on this page. -->
        <a href="#" @click.prevent="showEditForm(props.row)" :title="nameTitle(props.row)">
          {{ props.row.name }}
        </a>
        <!-- Fork (official footer) -- OFFICIAL-FOOTER-SPEC D10. -->
        <b-tooltip v-if="isOfficial(props.row)" :label="$t('templates.officialHint')" type="is-dark" multilined
          data-cy="official-lock">
          <b-icon icon="lock-outline" size="is-small" :aria-label="$t('templates.officialHint')" />
        </b-tooltip>
        <b-tag v-if="props.row.isDefault">
          {{ $t('templates.default') }}
        </b-tag>

        <p class="is-size-7 has-text-grey" v-if="props.row.type === 'tx'">
          {{ props.row.subject }}
        </p>
      </b-table-column>

      <!-- Polish 2026-11-14 (the user): the template's language as its own column, right of Name, in
           plain text -- shown only under All languages (otherwise the filter bar names the language);
           it replaces the language chip. -->
      <b-table-column v-if="showLang" v-slot="props" field="lang" :label="$t('analytics.matrix.lang')" sortable centered>
        <span data-cy="template-lang">{{ langText(props.row.lang) }}</span>
      </b-table-column>

      <b-table-column v-slot="props" field="type" :label="$t('globals.fields.type')" sortable>
        <b-tag v-if="props.row.type === 'campaign'" :class="props.row.type" :data-cy="`type-${props.row.type}`">
          {{ $tc('templates.typeCampaignHTML') }}
        </b-tag>
        <b-tag v-else-if="props.row.type === 'campaign_visual'" :class="props.row.type"
          :data-cy="`type-${props.row.type}`">
          {{ $tc('templates.typeCampaignVisual') }}
        </b-tag>
        <b-tag v-else :class="props.row.type" :data-cy="`type-${props.row.type}`">
          {{ $tc('templates.typeTransactional') }}
        </b-tag>
      </b-table-column>

      <b-table-column v-slot="props" field="createdAt" :label="$t('globals.fields.createdAt')" sortable>
        {{ $utils.niceDate(props.row.createdAt) }}
      </b-table-column>

      <b-table-column v-slot="props" field="updatedAt" :label="$t('globals.fields.updatedAt')" sortable>
        {{ $utils.niceDate(props.row.updatedAt) }}
      </b-table-column>

      <b-table-column v-slot="props" cell-class="actions" align="right">
        <div>
          <a href="#" @click.prevent="previewTemplate(props.row)" data-cy="btn-preview"
            :aria-label="$t('templates.preview')">
            <b-tooltip :label="$t('templates.preview')" type="is-dark">
              <b-icon icon="file-find-outline" size="is-small" />
            </b-tooltip>
          </a>
          <a href="#" @click.prevent="showEditForm(props.row)" data-cy="btn-edit"
            :aria-label="$t('globals.buttons.edit')">
            <b-tooltip :label="$t('globals.buttons.edit')" type="is-dark">
              <b-icon icon="pencil-outline" size="is-small" />
            </b-tooltip>
          </a>
          <!-- Fork (naming standard, NAMING-STANDARD-SPEC D7/D3) -- `Clone<N> <name>`, 30 with the hint. -->
          <a href="#" @click.prevent="promptClone(props.row)" data-cy="btn-clone" :aria-label="$t('globals.buttons.clone')">
            <b-tooltip :label="$t('globals.buttons.clone')" type="is-dark">
              <b-icon icon="file-multiple-outline" size="is-small" />
            </b-tooltip>
          </a>
          <a v-if="!props.row.isDefault && props.row.type === 'campaign'" href="#"
            @click.prevent="$utils.confirm(null, () => makeTemplateDefault(props.row))" data-cy="btn-set-default"
            :aria-label="$t('templates.makeDefault')">
            <b-tooltip :label="$t('templates.makeDefault')" type="is-dark">
              <b-icon icon="check-circle-outline" size="is-small" />
            </b-tooltip>
          </a>
          <span v-else class="a has-text-grey-light">
            <b-icon icon="check-circle-outline" size="is-small" />
          </span>

          <a v-if="!props.row.isDefault && !isOfficial(props.row)" href="#"
            @click.prevent="$utils.confirm(null, () => deleteTemplate(props.row))"
            data-cy="btn-delete" :aria-label="$t('globals.buttons.delete')">
            <b-tooltip :label="$t('globals.buttons.delete')" type="is-dark">
              <b-icon icon="trash-can-outline" size="is-small" />
            </b-tooltip>
          </a>
          <span v-else class="a has-text-grey-light">
            <b-icon icon="trash-can-outline" size="is-small" />
          </span>
        </div>
      </b-table-column>

      <template #empty v-if="!loading.templates">
        <empty-placeholder />
      </template>
    </b-table>

    <!-- Add / edit form modal -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isFormVisible" :width="1440" :can-cancel="false"
      class="template-modal">
      <template-form :data="curItem" :is-editing="isEditing" @finished="formFinished" @created="onTemplateCreated" />
    </b-modal>

    <campaign-preview v-if="previewItem" type="template" :id="previewItem.id" :template-type="previewItem.type"
      :title="previewItem.name" @close="closePreview" />
  </section>
</template>

<script>
import Vue from 'vue';
import { mapGetters, mapState } from 'vuex';
import CampaignPreview from '../components/CampaignPreview.vue';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';

import TemplateForm from './TemplateForm.vue';
import { isOfficialName } from '../officialSweep.mjs'; // eslint-disable-line import/extensions
import {
  TEMPLATE_KIND_PREF, normalizeTemplateKind, templateRows,
} from '../templatesPage.mjs'; // eslint-disable-line import/extensions
import {
  cloneName, formerNameOf, isNameBlocked, langColumnText, nameCounter, nameWarnings, showLangColumn,
} from '../naming.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';

export default Vue.extend({
  components: {
    CampaignPreview,
    TemplateForm,
    EmptyPlaceholder,
  },

  // Fork (global brand, integrations GLOBAL-BRAND-SPEC D12) -- re-filters on a selection change
  // (shownTemplates is a computed over the selection).
  mixins: [brandScopeMixin],

  data() {
    return {
      curItem: null,
      isEditing: false,
      isFormVisible: false,
      previewItem: null,
      // Polish 2026-11-14: the Footer | Other toggle, remembered per viewer (pref templates.kind).
      kind: normalizeTemplateKind(this.$utils.getPref(TEMPLATE_KIND_PREF)),
    };
  },

  methods: {
    // Fork (official footer) -- D10: an official template has no Delete action (the server
    // refuses it too: remove it from the seed file first).
    isOfficial(t) {
      return isOfficialName(t && t.name);
    },

    fetchTemplates() {
      this.$api.getTemplates();
    },

    // Show the edit form.
    showEditForm(data) {
      this.curItem = data;
      this.isFormVisible = true;
      this.isEditing = true;
    },

    // Show the new form. We only build visual templates, so default the type
    // (TemplateForm's own data() default is dead — mounted() replaces the form
    // with this object).
    showNewForm() {
      this.curItem = { type: 'campaign_visual' };
      this.isFormVisible = true;
      this.isEditing = false;
    },

    formFinished() {
      this.$api.getTemplates();
    },

    // D2: the child no longer closes the modal on create, and the create-vs-update branch
    // is the isEditing prop (owned here), not this.data.id (data is a prop; writing to it in
    // the child would mutate curItem without flipping isEditing, and a second Save would
    // create a duplicate). getTemplates() is already triggered by the sibling 'finished' emit.
    onTemplateCreated(d) {
      this.curItem = d;
      this.isEditing = true;
    },

    previewTemplate(c) {
      this.previewItem = c;
    },

    closePreview() {
      this.previewItem = null;
    },

    // Polish 2026-11-14: the Footer | Other toggle's change.
    onKind(v) {
      this.kind = normalizeTemplateKind(v);
      this.$utils.setPref(TEMPLATE_KIND_PREF, this.kind);
    },

    // Polish 2026-11-14: the Lang column's cell.
    langText(lang) {
      return langColumnText(lang);
    },

    nameTitle(row) {
      const f = formerNameOf(row);
      return f ? this.$t('naming.formerName', { name: f }) : null;
    },

    // D7 -- the clone prompt over every template name.
    promptClone(row) {
      const names = (Array.isArray(this.templates) ? this.templates : []).map((t) => t.name);
      const value = cloneName(row.name, names);
      const count = nameCounter('template', value);
      this.$utils.prompt(
        `${this.$t('globals.buttons.clone')} — ${this.$t('naming.hint')} (${this.$t('naming.counter', { n: count.n, max: count.max })})`,
        { placeholder: this.$t('globals.fields.name'), value, maxlength: count.maxlength || 200 },
        (name) => {
          // Polish 2026-11-14 (review R7): an exact check failing refuses the clone.
          // NAME-BLOCKS-SPEC D7: with the brand rows, so a leading brand (now blocking) refuses too.
          if (isNameBlocked(nameWarnings('template', name, { brands: this.brandRows }))) {
            this.$utils.toast(this.$t('naming.cloneBlocked'), 'is-danger');
            return;
          }
          this.cloneTemplate(name, row);
        },
      );
    },

    cloneTemplate(name, t) {
      const data = {
        name,
        type: t.type,
        subject: t.subject,
        body: t.body,
        body_source: t.bodySource,
        brand: t.brand,
        // Fork (LIST-GRID-SPEC D12) -- this is a fixed field list. A cloned FR template must
        // not come back as the server default (en).
        lang: t.lang,
      };
      this.$api.createTemplate(data).then((d) => {
        this.$api.getTemplates();
        this.$emit('finished');
        this.$utils.toast(`'${d.name}' created`);
      });
    },

    makeTemplateDefault(tpl) {
      this.$api.makeTemplateDefault(tpl.id).then(() => {
        this.$api.getTemplates();
        this.$utils.toast(this.$t('globals.messages.created', { name: tpl.name }));
      });
    },

    deleteTemplate(tpl) {
      this.$api.deleteTemplate(tpl.id).then(() => {
        this.$api.getTemplates();
        this.$utils.toast(this.$t('globals.messages.deleted', { name: tpl.name }));
      });
    },
  },

  computed: {
    ...mapState(['templates', 'loading']),
    // GLOBAL-LANG-SPEC D1 -- All while app.lang_enable is off (the toggle is hidden then).
    ...mapGetters({ langSelection: 'langEffective' }),
    ...mapGetters(['launch', 'brandHealth']),

    // Fork (global brand, D12) -- a brand's rows plus the brandless rows; No brand the brandless
    // rows only; All brands every row. Fork (naming standard, NAMING-STANDARD-SPEC D2) -- then the
    // language toggle, client-side (naming.mjs::langMatches; a template has a language column).
    // Polish 2026-11-14: the Lang column only under All languages.
    showLang() {
      return showLangColumn(this.langSelection);
    },

    // Polish 2026-11-14 (the user): every global filter -- brand, launch (newly honoured) and
    // language -- and the Footer | Other toggle (templatesPage.mjs::templateRows).
    shownTemplates() {
      return templateRows(this.templates, {
        selection: this.brandSelection,
        launch: this.launch,
        docs: this.brandHealth,
        lang: this.langSelection,
        kind: this.kind,
      });
    },
  },

  created() {
    this.$root.$on('page.refresh', this.fetchTemplates);
  },

  destroyed() {
    this.$root.$off('page.refresh', this.fetchTemplates);
  },

  mounted() {
    this.$api.getTemplates();
  },
});
</script>
