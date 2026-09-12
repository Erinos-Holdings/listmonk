<template>
  <section class="media-files">
    <h1 class="title is-4">
      {{ $t('media.title') }}
      <span v-if="media.total > 0">({{ media.total }})</span>
      <span class="has-text-grey-light"> / {{ serverConfig.media_provider }}</span>
    </h1>

    <b-loading :active="isProcessing || loading.media" />

    <section class="wrap gallery mt-6">
      <div class="columns mb-4">
        <div class="column">
          <form @submit.prevent="onQueryMedia" class="search">
            <div>
              <b-field>
                <b-input v-model="queryParams.query" name="query" expanded icon="magnify" ref="query" data-cy="query" />
                <p class="controls">
                  <b-button native-type="submit" type="is-primary" icon-left="magnify" data-cy="btn-query" />
                </p>
              </b-field>
            </div>
          </form>
        </div>
        <div v-if="$can('media:manage')" class="column is-narrow">
          <b-button @click="onToggleForm" icon-left="file-upload-outline" data-cy="btn-toggle-upload">
            {{ $t('media.upload') }}
          </b-button>
        </div>
      </div>

      <!-- Fork (media tags) -- MEDIA-TAGS-SPEC 3.6. Tag filter row: one toggle per context tag
           (ON when the picker opens) and per tag added below (removable), Untagged, All. OR
           semantics server-side. No persistence: the modal starts from context, the page on All. -->
      <div class="media-tag-filter mb-4" data-cy="media-tag-filter">
        <b-taglist>
          <b-tag v-for="c in chips" :key="c.tag" size="is-medium" class="is-clickable"
            :type="c.on && !allOn ? 'is-primary' : ''" :class="{ 'is-disabled': allOn }"
            :closable="!c.context" @click.native="onToggleChip(c)" @close="onRemoveChip(c)">
            <b-icon v-if="c.on && !allOn" icon="check" size="is-small" />
            {{ c.tag }}
          </b-tag>
          <b-tag size="is-medium" class="is-clickable" :type="untaggedOn && !allOn ? 'is-primary' : ''"
            :class="{ 'is-disabled': allOn }" @click.native="onToggleUntagged">
            {{ $t('media.untagged') }}
          </b-tag>
          <b-tag size="is-medium" class="is-clickable" :type="isAll ? 'is-dark' : ''" @click.native="onToggleAll">
            {{ $t('media.all') }}
          </b-tag>
        </b-taglist>
        <b-autocomplete v-model="filterQuery" :data="filterSuggestions" :placeholder="$t('media.filterByTag')"
          icon="tag-outline" size="is-small" open-on-focus clear-on-select keep-first class="media-tag-add"
          @select="onAddFilterTag" />
      </div>

      <b-collapse v-if="$can('media:manage')" v-model="showUploadForm" animation="">
        <form @submit.prevent="onSubmit" class="mb-6" data-cy="upload">
          <div>
            <b-field :label="$t('media.upload')">
              <b-upload v-model="form.files" drag-drop multiple xaccept=".png,.jpg,.jpeg,.gif,.svg" expanded>
                <div class="has-text-centered section">
                  <p>
                    <b-icon icon="file-upload-outline" size="is-large" />
                  </p>
                  <p>{{ $t('media.uploadHelp') }}</p>
                </div>
              </b-upload>
            </b-field>
            <div class="tags" v-if="form.files.length > 0">
              <b-tag v-for="(f, i) in form.files" :key="i" size="is-medium" closable @close="removeUploadFile(i)">
                {{ f.name }}
              </b-tag>
            </div>
            <!-- D6: pre-filled from the active filter's real tags, editable before submit. -->
            <b-field :label="$t('media.tags')"
              :message="uploadTags.length > 0
                ? $t('media.uploadTaggedAs', { tags: uploadTags.join(', ') }) : $t('media.uploadUntagged')">
              <b-taginput v-model="uploadTags" :data="tagSuggestions(uploadQuery, uploadTags)" autocomplete allow-new
                open-on-focus icon="tag-outline" :before-adding="beforeAddingTag" @typing="(q) => { uploadQuery = q; }"
                @input="(t) => { uploadTags = normalizeTags(t); }" data-cy="upload-tags" />
            </b-field>
            <div class="buttons">
              <b-button native-type="submit" type="is-primary" icon-left="file-upload-outline"
                :disabled="form.files.length === 0" :loading="isProcessing">
                {{ $tc('media.upload') }}
              </b-button>
            </div>
          </div>
        </form>
      </b-collapse>

      <!-- Pagination -->
      <div v-if="media.total > media.perPage" class="pagination-wrapper mt-5">
        <b-pagination :total="media.total" :current.sync="media.page" :per-page="media.perPage"
          @change="onPageChange" />
      </div>

      <div v-if="loading.media" class="has-text-centered py-6">
        <b-loading :active="loading.media" />
      </div>
      <div v-else-if="items.length > 0" class="grid">
        <div v-for="item in items" :key="item.id" class="item">
          <div class="thumb">
            <a @click="(e) => onMediaSelect(item, e)" :href="item.url" target="_blank" rel="noopener noreferer"
              class="thumb-link">
              <!-- Fork (dark-mode readiness) -- DARK-MODE-SPEC D3. The same <img> is drawn
                   over a ground that is light on the left and dark on the right. Mail clients
                   never recolour large images, so what the dark half shows IS the dark-mode
                   outcome for them; small dark-on-transparent glyphs are the exception (Gmail
                   Android recolours those), which is what the tile's title says. -->
              <div class="thumb-container" :class="{ 'thumb-split': item.thumbUrl }"
                :title="thumbTitle(item)">
                <img v-if="item.thumbUrl" :src="item.thumbUrl" :alt="item.filename" />
                <div v-else class="thumb-placeholder">
                  <span class="file-ext">
                    {{ item.filename.split(".").pop().toUpperCase() }}
                  </span>
                </div>
              </div>
              <span v-if="darkVerdict(item)" class="darkmode-badge" :class="darkVerdict(item).kind"
                :title="darkVerdict(item).title">{{ darkVerdict(item).label }}</span>
            </a>
            <div class="actions">
              <a href="#" @click.prevent="$utils.confirm(null, () => onDeleteMedia(item.id))" data-cy="btn-delete"
                :aria-label="$t('globals.buttons.delete')" class="delete-btn">
                <b-icon icon="trash-can-outline" size="is-small" />
              </a>
            </div>
          </div>
          <div class="info">
            <p class="filename" :title="item.filename">{{ item.filename }}</p>
            <p class="date">{{ $utils.niceDate(item.createdAt, false) }}</p>

            <!-- D7: tags are edited here, never written by selecting the image. Saves on change
                 and patches the row in place, so an untagged row does not vanish mid-edit. -->
            <div class="media-tags">
              <template v-if="editingId !== item.id">
                <b-tag v-for="t in (item.tags || [])" :key="t" size="is-small">{{ t }}</b-tag>
                <a v-if="$can('media:manage')" href="#" class="media-tags-edit" :title="$t('media.editTags')"
                  :aria-label="$t('media.editTags')" @click.prevent="onEditTags(item)" data-cy="btn-edit-tags">
                  <b-icon icon="tag-outline" size="is-small" />
                </a>
              </template>
              <b-taginput v-else v-model="editTags" :data="tagSuggestions(editQuery, editTags)" autocomplete allow-new
                open-on-focus size="is-small" icon="tag-outline" :before-adding="beforeAddingTag"
                @typing="(q) => { editQuery = q; }" @input="(t) => onSaveTags(item, t)" @blur="onEditBlur" />
            </div>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div v-else-if="!loading.media">
        <empty-placeholder />
      </div>

      <!-- Pagination -->
      <div v-if="media.total > media.perPage" class="pagination-wrapper mt-5">
        <b-pagination :total="media.total" :current.sync="media.page" :per-page="media.perPage"
          @change="onPageChange" />
      </div>
    </section>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import { BRAND_TAG_PREFIX } from '../brand';
import { foldMediaTag, isValidMediaTag, normalizeMediaTagsLenient } from '../mediaTags';

export default Vue.extend({
  components: {
    EmptyPlaceholder,
  },

  name: 'Media',

  props: {
    isModal: Boolean,
    type: { type: String, default: '' },

    // Fork (media tags) -- MEDIA-TAGS-SPEC 3.5. The editing context's tags (a campaign's derived
    // brand + its Tags field, a template's brand), already lenient-normalized by the parent.
    // Every context tag starts as an ON chip. Empty (the /media page) = open on All.
    context: { type: Array, default: () => [] },
  },

  data() {
    return {
      form: {
        files: [],
      },
      toUpload: 0,
      uploaded: 0,
      showUploadForm: false,

      queryParams: {
        page: 1,
        query: '',
      },

      // Fork (media tags). Filter state -- never persisted (D5).
      chips: [],
      untaggedOn: false,
      allOn: false,
      filterQuery: '',

      // Tag autocomplete source (D8): GET /api/media/tags, unioned with the brand roster.
      tagCounts: [],

      // Upload tags (D6), re-seeded from the active filter whenever it changes.
      uploadTags: [],
      uploadQuery: '',

      // Tile editor. `items` is a component-local copy of media.results (the store's only
      // mutation is a whole-model replace), so a saved row is patched in place.
      items: [],
      editingId: null,
      editTags: [],
      editQuery: '',
    };
  },

  methods: {
    removeUploadFile(i) {
      this.form.files.splice(i, 1);
    },

    getMedia() {
      const params = {
        page: this.queryParams.page,
        query: this.queryParams.query,
      };
      if (!this.isAll) {
        if (this.activeTags.length > 0) {
          params.tag = this.activeTags;
        }
        if (this.untaggedOn) {
          params.untagged = true;
        }
      }
      this.$api.getMedia(params);
    },

    loadTagCounts() {
      this.$api.getMediaTags().then((data) => {
        this.tagCounts = Array.isArray(data) ? data : [];
      }, () => {
        this.tagCounts = [];
      });
    },

    // Start from context: every context chip ON; no context = All.
    initFilter() {
      this.chips = normalizeMediaTagsLenient(this.context).map((tag) => ({ tag, on: true, context: true }));
      this.untaggedOn = false;
      this.allOn = this.chips.length === 0;
    },

    onFilterChanged() {
      this.queryParams.page = 1;
      this.uploadTags = [...this.activeTags];
      this.getMedia();
    },

    onToggleChip(c) {
      if (this.allOn) {
        // Picking a chip while All is on leaves All and filters on that chip alone.
        this.allOn = false;
        this.chips = this.chips.map((x) => ({ ...x, on: x.tag === c.tag }));
        this.untaggedOn = false;
      } else {
        this.chips = this.chips.map((x) => (x.tag === c.tag ? { ...x, on: !x.on } : x));
      }
      this.onFilterChanged();
    },

    onRemoveChip(c) {
      this.chips = this.chips.filter((x) => x !== c);
      this.onFilterChanged();
    },

    onToggleUntagged() {
      if (this.allOn) {
        this.allOn = false;
        this.chips = this.chips.map((x) => ({ ...x, on: false }));
        this.untaggedOn = true;
      } else {
        this.untaggedOn = !this.untaggedOn;
      }
      this.onFilterChanged();
    },

    onToggleAll() {
      this.allOn = !this.isAll;
      this.onFilterChanged();
    },

    onAddFilterTag(t) {
      const tag = foldMediaTag(t);
      if (!tag || !isValidMediaTag(tag)) {
        return;
      }
      this.allOn = false;
      if (this.chips.some((c) => c.tag === tag)) {
        this.chips = this.chips.map((c) => (c.tag === tag ? { ...c, on: true } : c));
      } else {
        this.chips = [...this.chips, { tag, on: true, context: false }];
      }
      this.$nextTick(() => { this.filterQuery = ''; });
      this.onFilterChanged();
    },

    // before-adding for user-typed tags: the server's rule, applied to the folded form.
    beforeAddingTag(t) {
      const tag = foldMediaTag(t);
      if (!isValidMediaTag(tag)) {
        this.$utils.toast(this.$t('media.tagInvalid', { tag }), 'is-danger');
        return false;
      }
      return true;
    },

    normalizeTags(tags) {
      return normalizeMediaTagsLenient(tags);
    },

    // Autocomplete candidates for a tag input: the known vocabulary minus what is already
    // chosen, filtered by what has been typed.
    tagSuggestions(query, chosen) {
      const q = foldMediaTag(query);
      const have = new Set(chosen || []);
      return this.tagVocabulary.filter((t) => !have.has(t) && (!q || t.includes(q)));
    },

    onEditTags(item) {
      this.editingId = item.id;
      this.editTags = [...(item.tags || [])];
      this.editQuery = '';
    },

    onEditBlur() {
      // Leave the editor only once the input is empty, so a half-typed tag is not lost.
      if (!this.editQuery) {
        this.editingId = null;
      }
    },

    onSaveTags(item, tags) {
      const next = normalizeMediaTagsLenient(tags);
      this.editTags = next;
      this.$api.updateMediaTags(item.id, next).then((m) => {
        const i = this.items.findIndex((x) => x.id === item.id);
        if (i > -1) {
          this.items.splice(i, 1, { ...this.items[i], ...m });
        }
        this.loadTagCounts();
      });
    },

    onToggleForm() {
      this.showUploadForm = !this.showUploadForm;
      this.$utils.setPref('media.upload', this.showUploadForm);
    },

    onQueryMedia() {
      this.queryParams.page = 1;
      this.getMedia();
    },

    // Fork (dark-mode readiness) -- DARK-MODE-SPEC D3/D4. The verdict the uploader stamped
    // into meta.darkmode, rendered as a tile badge. Rows uploaded before the classifier
    // existed carry nothing and get no badge -- the sweep (D5) is what fills those in.
    darkVerdict(item) {
      const d = item.meta && item.meta.darkmode;
      if (!d || !d.class) {
        return null;
      }

      const warnings = d.warnings || [];
      if (warnings.length > 0) {
        return {
          kind: 'is-warning',
          label: `warn: ${warnings.join(', ')}`,
          title: this.$t('media.darkmodeWarn', { codes: warnings.join(', '), class: d.class }),
        };
      }
      if (d.fixed) {
        return { kind: 'is-success', label: 'fixed', title: this.$t('media.darkmodeFixed', { class: d.class }) };
      }
      return { kind: 'is-light', label: 'ok', title: this.$t('media.darkmodeOk', { class: d.class }) };
    },

    thumbTitle(item) {
      return `${item.filename}\n${this.$t('media.darkmodeTileHelp')}`;
    },

    onMediaSelect(m, e) {
      // If the component is open in the modal mode, close the modal and
      // fire the selection event.
      // Otherwise, do nothing and let the image open like a normal link.
      // Fork (media tags) -- D7: selecting never writes a tag.
      if (this.isModal) {
        e.preventDefault();
        this.$emit('selected', m);
        this.$parent.close();
      }
    },

    onSubmit() {
      this.toUpload = this.form.files.length;
      const tags = normalizeMediaTagsLenient(this.uploadTags);

      // Upload N files with N requests.
      for (let i = 0; i < this.toUpload; i += 1) {
        const params = new FormData();
        params.set('file', this.form.files[i]);
        this.$api.uploadMedia(params, tags).then((m) => {
          // Fork (dark-mode readiness) -- DARK-MODE-SPEC D4. The uploader repairs what it
          // safely can; the two classes it deliberately does NOT touch surface here, once,
          // non-blocking. The tile badge (D3) is the durable record.
          const v = this.darkVerdict(m);
          if (v && v.kind === 'is-warning') {
            this.$utils.toast(`${m.filename}: ${v.title}`, 'is-warning', 5000);
          }
          this.onUploaded();
        }, () => {
          this.onUploaded();
        });
      }
    },

    onDeleteMedia(id) {
      this.$api.deleteMedia(id).then(() => {
        this.getMedia();
        this.loadTagCounts();
      });
    },

    onUploaded() {
      this.uploaded += 1;
      if (this.uploaded >= this.toUpload) {
        this.toUpload = 0;
        this.uploaded = 0;
        this.form.files = [];

        this.getMedia();
        this.loadTagCounts();
      }
    },

    onPageChange(p) {
      this.queryParams.page = p;
      this.getMedia();
    },
  },

  computed: {
    ...mapState(['loading', 'media', 'serverConfig', 'lists']),

    isProcessing() {
      if (this.toUpload > 0 && this.uploaded < this.toUpload) {
        return true;
      }
      return false;
    },

    // No tag filter is sent when All is on, or when nothing is selected.
    isAll() {
      return this.allOn || (!this.untaggedOn && !this.chips.some((c) => c.on));
    },

    // The active filter's REAL tags (Untagged/All excluded) -- what an upload is tagged with.
    activeTags() {
      if (this.isAll) {
        return [];
      }
      return this.chips.filter((c) => c.on).map((c) => c.tag);
    },

    // D8: media tags in use ∪ brand slugs from the lists store (a brand with no media yet
    // still autocompletes).
    tagVocabulary() {
      const out = new Set(this.tagCounts.map((t) => t.tag));
      ((this.lists && this.lists.results) || []).forEach((l) => {
        const b = (l.tags || []).find((x) => x.startsWith(BRAND_TAG_PREFIX));
        if (b) {
          out.add(b.slice(BRAND_TAG_PREFIX.length));
        }
      });
      return normalizeMediaTagsLenient([...out]);
    },

    filterSuggestions() {
      return this.tagSuggestions(this.filterQuery, this.chips.map((c) => c.tag));
    },
  },

  watch: {
    // Re-seed the local grid copy whenever the store's media model is replaced.
    media: {
      handler(m) {
        this.items = (m && m.results) ? [...m.results] : [];
      },
      immediate: true,
    },

    context(next, prev) {
      if (JSON.stringify(next) !== JSON.stringify(prev)) {
        this.initFilter();
        this.onFilterChanged();
      }
    },
  },

  created() {
    this.$root.$on('page.refresh', this.getMedia);
  },

  destroyed() {
    this.$root.$off('page.refresh', this.getMedia);
  },

  mounted() {
    this.initFilter();
    this.uploadTags = [...this.activeTags];
    this.getMedia();
    this.loadTagCounts();

    if (this.$utils.getPref('media.upload')) {
      this.showUploadForm = true;
    }
  },
});
</script>

<style scoped>
.media-tag-filter {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
}
.media-tag-filter .tags {
  margin-bottom: 0;
}
.media-tag-filter .tag.is-disabled {
  opacity: 0.5;
}
.media-tag-add {
  min-width: 12rem;
}
.media-tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.25rem;
  margin-top: 0.25rem;
}
.media-tags-edit {
  line-height: 1;
}
</style>
