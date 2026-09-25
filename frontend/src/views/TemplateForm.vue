<template>
  <section>
    <form @submit.prevent="onSubmit">
      <div class="modal-card content template-modal-content" style="width: auto">
        <header class="modal-card-head">
          <b-button @click="onTogglePreview" class="is-pulled-right" type="is-primary" icon-left="file-find-outline">
            {{ $t('templates.preview') }} (F9)
          </b-button>

          <template v-if="isEditing">
            <h4>{{ data.name }}</h4>
            <p class="has-text-grey is-size-7">
              {{ $t('globals.fields.id') }}: <span data-cy="id"><copy-text :text="`${data.id}`" /></span>
            </p>
          </template>
          <h4 v-else>
            {{ $t('templates.newTemplate') }}
          </h4>
        </header>
        <section expanded class="modal-card-body mb-0 pb-0">
          <!-- Fork (official footer) -- OFFICIAL-FOOTER-SPEC D10. -->
          <b-message v-if="isOfficial" type="is-warning" class="official-banner" data-cy="official-banner">
            <b-icon icon="lock-outline" size="is-small" /> {{ $t('templates.officialHint') }}
          </b-message>
          <div class="columns">
            <div class="column is-9">
              <b-field :label="$t('globals.fields.name')" label-position="on-border">
                <b-input :maxlength="200" :ref="'focus'" v-model="form.name" name="name"
                  :placeholder="$t('globals.fields.name')" required />
              </b-field>
            </div>
            <div class="column is-3">
              <b-field :label="$t('globals.fields.type')" label-position="on-border">
                <b-select v-model="form.type" :disabled="isEditing" expanded>
                  <option value="campaign">
                    {{ $tc('templates.typeCampaignHTML') }}
                  </option>
                  <option value="campaign_visual">
                    {{ $tc('templates.typeCampaignVisual') }}
                  </option>
                  <option value="tx">
                    {{ $tc('templates.typeTransactional') }}
                  </option>
                </b-select>
              </b-field>
            </div>
          </div>
          <div class="columns" v-if="form.type === 'tx'">
            <div class="column is-12">
              <b-field :label="$t('templates.subject')" label-position="on-border">
                <b-input :maxlength="200" :ref="'focus'" v-model="form.subject" name="name"
                  :placeholder="$t('templates.subject')" required />
              </b-field>
            </div>
          </div>

          <!-- Fork (LIST-GRID-SPEC D12). The language the body is written in. Importing this
               template into a campaign sets the campaign's language to it. -->
          <div class="columns" v-if="form.type !== 'tx'">
            <div class="column is-4">
              <b-field :label="$t('campaigns.lang')" label-position="on-border" :message="$t('templates.langHelp')">
                <b-select v-model="form.lang" name="lang" expanded required data-cy="template-lang">
                  <option v-for="l in langOptions" :key="l.code" :value="l.code">{{ l.label }}</option>
                </b-select>
              </b-field>
            </div>
          </div>

          <template v-if="form.body !== null">
            <!-- Brand swatch picker: no list selection exists here to derive a brand from,
                 so the brand is picked explicitly. No selection -> no brand row. -->
            <div class="columns" v-if="form.type === 'campaign_visual'">
              <div class="column is-4">
                <b-field :label="$t('templates.brandSwatches')" label-position="on-border"
                  :message="brandNote ? $t(brandNote, { brand: brandSlug }) : ''">
                  <b-select v-model="brandSlug" name="brand-swatches" expanded>
                    <option value="">{{ $t('templates.brandSwatchesNone') }}</option>
                    <option v-for="b in brandRoster" :key="b" :value="b">{{ b }}</option>
                  </b-select>
                </b-field>
              </div>
            </div>

            <b-field v-if="form.type === 'campaign_visual'" label-position="on-border" class="mb-1">
              <visual-editor v-if="form.type === 'campaign_visual'" ref="visualEditor" name="body"
                :source="form.bodySource" @change="onChangeVisualEditor" height="70vh"
                :brand-palettes="brandPalettes" :media-context="mediaContext"
                :official-context="officialContext" :official-footers="officialFooters" />
            </b-field>

            <b-field v-else :label="$t('templates.rawHTML')" label-position="on-border">
              <code-editor lang="html" v-model="form.body" name="body" />
            </b-field>
          </template>

          <p class="is-size-7">
            <template v-if="form.type === 'campaign'">
              {{ $t('templates.placeholderHelp', { placeholder: egPlaceholder }) }}
            </template>
            <a target="_blank" rel="noopener noreferer" href="https://listmonk.app/docs/templating">
              {{ $t('globals.buttons.learnMore') }}
            </a>
          </p>
        </section>
        <footer class="modal-card-foot has-text-right">
          <b-button @click="$parent.close()">
            {{ $t('globals.buttons.close') }}
          </b-button>
          <b-button v-if="$can('templates:manage')" native-type="submit" type="is-primary" :loading="loading.templates">
            {{ $t('globals.buttons.saveChanges') }}
          </b-button>
        </footer>
      </div>
    </form>
    <campaign-preview v-if="previewItem" is-post type="template" :title="previewItem.name"
      :template-type="previewItem.type" :body="form.body" @close="onTogglePreview" />
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import CampaignPreview from '../components/CampaignPreview.vue';
import CodeEditor from '../components/CodeEditor.vue';
import VisualEditor from '../components/VisualEditor.vue';
import CopyText from '../components/CopyText.vue';
import { BRAND_TAG_PREFIX, brandThemePalette, reBrandSlug } from '../brand';
import { CAMPAIGN_LANGS } from '../langs';
import { normalizeMediaTagsLenient } from '../mediaTags';
import { isOfficialName, runSweep, selectSweepItems } from '../officialSweep.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  components: {
    CampaignPreview,
    CopyText,
    'code-editor': CodeEditor,
    'visual-editor': VisualEditor,
  },

  props: {
    data: { type: Object, default: () => { } },
    isEditing: { type: Boolean, default: false },
  },

  data() {
    return {
      // Binds form input values.
      form: {
        name: '',
        subject: '',
        // Dead in practice — mounted() replaces the form with the data prop
        // (Templates.vue showNewForm() holds the live default). Kept aligned
        // with it so a refactor of that spread cannot silently revert the
        // visual-first default.
        type: 'campaign_visual',
        optin: '',
        body: null,
        bodySource: null,
      },
      previewItem: null,
      egPlaceholder: '{{ template "content" . }}',

      // Brand swatch picker state. brandSlug is the dropdown selection (roster entries are
      // lowercase-folded, so it is always canonical); it doubles as the stale-response guard:
      // rapid dropdown flips leave several theme fetches in flight and an older response can
      // resolve last, so a response is discarded unless it answers the current selection.
      brandSlug: '',
      brandPalettes: [],
      // The i18n key of the inline note under the dropdown, or null for none. A deliberate
      // gesture that visibly does nothing reads as broken — unlike the campaign editor, where
      // passive absence is the designed behavior — so a pick resolving to no palette says so
      // (brandNoPage), and a failed fetch says that instead (brandFetchFailed) rather than
      // misdiagnosing a transport error as a missing catalog page.
      brandNote: null,

      // DOCUMENT COLOR PROVENANCE, exactly as in Campaign.vue: the rebrand sweep's mapping
      // source — it follows every found:true pick while the template has no content, then is
      // seeded by the first found:true pick, moved only by an Apply that rewrote
      // colors, pinned on Keep. A fresh (never-saved) template open has none (nothing is
      // persisted yet), so the rebrand gesture is still two-step there: pick the template's
      // ORIGINAL brand first (seeds provenance, and the swatches visibly matching the design
      // confirms it), then the target brand — which prompts. A pageless or None pick in
      // between neither clears provenance nor suppresses the later prompt.
      // D4 (templates.brand persisted): reopening a SAVED template seeds provenance straight
      // from the stored brand instead (maybeOfferBrandSweep's bodySource-present /
      // heldBrandPalette-null silent-seed branch), so the rebrand gesture becomes one-step for
      // it — pick the new brand, get the prompt. Templates saved before this change carry ''
      // and keep the two-step gesture until their first save with a brand.
      heldBrandPalette: null,
    };
  },

  methods: {
    onTogglePreview() {
      this.previewItem = !this.previewItem ? this.form : null;
    },

    onPreviewShortcut(e) {
      if (e.key === 'F9') {
        this.onTogglePreview();
        e.preventDefault();
      }
    },

    onSubmit() {
      // OFFICIAL-FOOTER-SPEC D10/D11: an official save is a considered act -- confirm with the
      // counts the re-save sweep will touch, then save, then sweep.
      if (this.isOfficialSave) {
        this.confirmOfficialSave();
        return;
      }

      if (this.isEditing) {
        this.updateTemplate();
        return;
      }

      this.createTemplate();
    },

    // The sweep plan for a save of `saved` ({id, name}): raw campaigns + templates, selected by
    // resolution (officialSweep.mjs). Also returns the fresh `Official_` references.
    async planOfficialSweep(saved) {
      const [camps, tpls] = await Promise.all([this.$api.getCampaignsRaw(), this.$api.getTemplatesRaw()]);
      const templates = Array.isArray(tpls) ? tpls : [];
      const plan = selectSweepItems((camps && camps.results) || [], templates, saved, (this.lists && this.lists.results) || []);
      const refs = templates
        .filter((t) => t.type === 'campaign_visual' && isOfficialName(t.name))
        .map((t) => ({ id: t.id, name: t.name, body_source: t.body_source }));
      return { plan, refs };
    },

    confirmOfficialSave() {
      const saved = { id: this.data.id, name: this.form.name };
      this.planOfficialSweep(saved).then(({ plan }) => this.$t('templates.officialSaveConfirm', {
        campaigns: plan.campaigns.length,
        evergreens: plan.evergreens.length,
        templates: plan.templates.length,
      }), () => this.$t('templates.officialSaveConfirmUnknown')).then((msg) => {
        this.$utils.confirm(msg, () => {
          const save = this.isEditing ? this.updateTemplate() : this.createTemplate();
          save.then((d) => {
            if (d) {
              this.runOfficialSweep({ id: d.id, name: d.name });
            }
          });
        });
      });
    },

    // D11: re-save every carrier of the saved template. A sweep failure never un-saves the
    // template; the recovery is integrations' scripts/repair-official-footers.ts.
    async runOfficialSweep(saved) {
      const ve = this.$refs.visualEditor;
      const em = ve && ve.builder ? ve.builder() : null;
      if (!em || !em.compileDocument) {
        this.$utils.toast(this.$t('templates.officialSweepUnavailable'), 'is-danger', 15000);
        return;
      }

      let plan;
      let refs;
      try {
        ({ plan, refs } = await this.planOfficialSweep(saved));
      } catch (e) {
        this.$utils.toast(this.$t('templates.officialSweepUnavailable'), 'is-danger', 15000);
        return;
      }
      // The saved row is fresh in `refs`; the editor's own references follow via the store.
      this.$api.getTemplates();

      const canSend = this.$can('campaigns:send');
      const total = plan.campaigns.length + plan.templates.length + (canSend ? plan.evergreens.length : 0);
      if (total === 0 && plan.evergreens.length === 0 && plan.unresolved.length === 0) {
        return;
      }
      if (total > 0) {
        this.$utils.toast(this.$t('templates.officialSweepProgress', { n: total }), 'is-info', 5000);
      }

      const summary = await runSweep(plan, {
        api: {
          getCampaign: (id) => this.$api.getCampaignRaw(id),
          getTemplate: (id) => this.$api.getTemplateRaw(id),
          updateCampaign: (id, payload) => this.$api.updateCampaign(id, payload),
          updateTemplate: (payload) => this.$api.updateTemplate(payload),
          changeStatus: (id, status) => this.$api.changeCampaignStatus(id, status),
        },
        compile: (doc, context) => ve.compileDocument(doc, context, refs),
        canSend,
        lists: (this.lists && this.lists.results) || [],
      });

      // Left-paused first: it is the one that stops a live automation.
      const lines = [];
      if (summary.leftPaused.length > 0) {
        lines.push(this.$t('templates.officialSweepLeftPaused', { refs: summary.leftPaused.map((l) => l.ref).join(', ') }));
      }
      lines.push(this.$t('templates.officialSweepDone', { saved: summary.saved.length, failed: summary.failed.length }));
      if (summary.failed.length > 0) {
        lines.push(this.$t('templates.officialSweepFailed', { refs: summary.failed.map((f) => `${f.ref} (${f.error})`).join('; ') }));
      }
      if (summary.notResaved.length > 0) {
        lines.push(this.$t('templates.officialSweepNotResaved', { n: summary.notResaved.length, refs: summary.notResaved.map((id) => `c${id}`).join(', ') }));
      }
      if (summary.unresolved.length > 0) {
        lines.push(this.$t('templates.officialSweepUnresolved', { refs: summary.unresolved.map((u) => `c${u.id}`).join(', ') }));
      }
      const clean = summary.failed.length === 0 && summary.leftPaused.length === 0
        && summary.notResaved.length === 0 && summary.unresolved.length === 0;
      this.$utils.toast(lines.join(' '), clean ? 'is-success' : 'is-warning', clean ? 5000 : 30000);
    },

    createTemplate() {
      const data = {
        id: this.data.id,
        name: this.form.name,
        type: this.form.type,
        subject: this.form.subject,
        body: this.form.body,
        body_source: this.form.bodySource,
        brand: this.brandSlug,
        lang: this.form.lang || 'en',
      };

      return this.$api.createTemplate(data).then((d) => {
        this.$emit('finished');
        // No $parent.close() here (D2): a save stays open, matching Campaigns' "Save
        // changes" behavior. The create-vs-update branch is the isEditing PROP, owned by
        // Templates.vue, not this.data.id (data is itself a prop) — emit the created
        // record so the parent can flip curItem/isEditing.
        this.$emit('created', d);
        this.$utils.toast(this.$t('globals.messages.created', { name: d.name }));
        return d;
      });
    },

    updateTemplate() {
      const data = {
        id: this.data.id,
        name: this.form.name,
        type: this.form.type,
        subject: this.form.subject,
        body: this.form.body,
        body_source: this.form.bodySource,
        brand: this.brandSlug,
        lang: this.form.lang || 'en',
      };

      return this.$api.updateTemplate(data).then((d) => {
        this.$emit('finished');
        this.$utils.toast(`'${d.name}' updated`);
        return d;
      });
    },

    onChangeVisualEditor({ source, body }) {
      this.form.body = body;
      this.form.bodySource = source;
    },

    // Fetch the picked brand's theme and drive the swatch row. found:false and fetch failure
    // each land on their own inline note (never a toast — disableToast on the endpoint);
    // either way there is no row (no palette overrides, by design).
    onBrandPick(slug) {
      this.brandNote = null;
      if (!slug) {
        this.brandPalettes = [];
        return;
      }

      this.$api.getBrandTheme(slug).then((data) => {
        // Stale-response guard: only the response for the current selection may act.
        if (slug !== this.brandSlug) {
          return;
        }

        const p = data.found ? brandThemePalette(slug, data.theme) : null;
        this.brandPalettes = p ? [p] : [];
        this.brandNote = p ? null : 'templates.brandNoPage';
        this.maybeOfferBrandSweep(p);
      }).catch(() => {
        if (slug === this.brandSlug) {
          this.brandPalettes = [];
          this.brandNote = 'templates.brandFetchFailed';
        }
      });
    },

    // Rebrand sweep, same held-provenance rules as Campaign.vue with a simpler trigger: an
    // explicit dropdown pick, no error transit, no re-offer latch (one pick, one evaluation).
    maybeOfferBrandSweep(palette) {
      if (!palette) {
        return;
      }

      // No document yet (a brand-new template with no edits): nothing to sweep, so held
      // provenance silently follows each pick until content exists. A freshly opened CLONE
      // has a document, so the two-step rebrand gesture is unaffected.
      if (!this.form.bodySource) {
        this.heldBrandPalette = palette;
        return;
      }

      if (!this.heldBrandPalette) {
        this.heldBrandPalette = palette;
        return;
      }
      if (this.heldBrandPalette.label === palette.label) {
        return;
      }

      this.$buefy.dialog.confirm({
        scroll: 'keep',
        message: this.$utils.escapeHTML(this.$t('templates.brandSweepPrompt', {
          old: this.heldBrandPalette.label, new: palette.label,
        })),
        confirmText: this.$t('campaigns.brandSweepApply'),
        cancelText: this.$t('campaigns.brandSweepKeep'),
        onConfirm: () => this.applyBrandSweep(palette),
        // Keep (cancel/dismiss) is deliberately a no-op: provenance stays with the palette
        // actually in the document.
      });
    },

    applyBrandSweep(newPalette) {
      const ve = this.$refs.visualEditor;
      const replaced = ve ? ve.remapColors(this.form.bodySource, this.heldBrandPalette, newPalette) : null;

      if (replaced === null) {
        this.$utils.toast(this.$t('campaigns.brandSweepUnavailable'), 'is-warning');
        return;
      }
      if (replaced === 0) {
        // Identical toast to the campaign editor, by decision — and provenance stays put:
        // nothing was rewritten, so nothing changed hands.
        this.$utils.toast(this.$t('campaigns.brandSweepNoMatches'), 'is-warning');
        return;
      }
      this.heldBrandPalette = newPalette;
    },
  },

  computed: {
    ...mapState(['loading', 'lists', 'templates']),

    // Fork (official footer) -- OFFICIAL-FOOTER-SPEC D10: an `Official_` template (the stored
    // name, or the one being typed).
    isOfficial() {
      return isOfficialName(this.form.name) || (this.isEditing && isOfficialName(this.data && this.data.name));
    },

    // A save that must confirm + sweep: the saved row will carry an official name.
    isOfficialSave() {
      return this.form.type === 'campaign_visual' && isOfficialName(this.form.name);
    },

    // D9: the template's own columns; `official` hides the builder's Insert action (a reference
    // can never hold an OfficialFooter block).
    officialContext() {
      return { lang: this.form.lang || 'en', brand: this.brandSlug || 'curated', official: this.isOfficial };
    },

    officialFooters() {
      return (this.templates || [])
        .filter((t) => t.type === 'campaign_visual' && isOfficialName(t.name))
        .map((t) => ({ id: t.id, name: t.name, body_source: t.bodySource }));
    },

    langOptions() {
      return CAMPAIGN_LANGS;
    },

    // Fork (media tags) -- MEDIA-TAGS-SPEC D3. The media picker's context is the template's
    // brand; no brand = no context.
    mediaContext() {
      return this.brandSlug ? normalizeMediaTagsLenient([this.brandSlug]) : [];
    },

    // Distinct brand slugs from the vuex lists store's `brand:` tags, lowercase-folded before
    // dedupe — mixed-case tags are valid on the send path and the theme proxy folds, so
    // `brand:Liyora` and `brand:liyora` are one brand, not two dropdown entries. The store is
    // the roster's source of truth (App.vue loads it once, globally — the same store campaign
    // derivation reads, so the two editors agree by construction); there is no catalog brand
    // index to consult. Slugs failing reBrandSlug are omitted: a bare `brand:` tag would
    // otherwise render an empty option colliding with the None sentinel, and a slug the proxy
    // rejects (e.g. embedded space) would 400 and misdiagnose the tag as a missing catalog
    // page. The campaign editor is where that misconfiguration errors loudly.
    brandRoster() {
      const out = new Set();
      ((this.lists && this.lists.results) || []).forEach((l) => {
        const t = (l.tags || []).find((x) => x.startsWith(BRAND_TAG_PREFIX));
        if (t) {
          const slug = t.slice(BRAND_TAG_PREFIX.length).toLowerCase();
          if (reBrandSlug.test(slug)) {
            out.add(slug);
          }
        }
      });
      return [...out].sort();
    },
  },

  watch: {
    brandSlug(slug) {
      this.onBrandPick(slug);
    },
  },

  mounted() {
    // Fork (LIST-GRID-SPEC D12) -- English preselected on a new template.
    this.form = { ...this.$props.data, lang: this.$props.data.lang || 'en' };
    // D4: restore the persisted brand (the existing brandSlug watcher runs onBrandPick ->
    // theme fetch -> swatch row, with no new code path).
    this.brandSlug = this.$props.data.brand || '';

    this.$nextTick(() => {
      this.$refs.focus.focus();
    });

    window.addEventListener('keydown', this.onPreviewShortcut);
  },

  beforeDestroy() {
    window.removeEventListener('keydown', this.onPreviewShortcut);
  },
});
</script>
