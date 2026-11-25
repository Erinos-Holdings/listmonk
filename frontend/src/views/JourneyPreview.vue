<template>
  <!-- Fork (journey preview, integrations listmonk/JOURNEY-PREVIEW-SPEC D3; source and language rule
       USAGE-PREVIEW-LANG-SPEC D3/D4). Each brand's journey step previews (every brand of the global
       brand selection, stacked -- the page has no brand picker of its own) in the journey guide's
       layout (the Usage pane's guide: the three series in GUIDE_SERIES order, their titles/subtitles,
       each step's Day bubble and title), each step's campaign rendered at the email's 600 px canvas --
       scaled down to the column when narrower -- across the full width of the page. A step's
       previews are exactly the automations the Brands > Usage circles count: the stored `counted`
       entries of the brand's usage doc (GET /api/dashboard/brand-matrix), their campaigns read once
       by id; the grouping, the status filter and the language pick are journeyPreview.mjs.
       Read-only: no server change; the reads apply the user's list permissions as everywhere else. -->
  <section class="journey-preview" data-cy="journey-preview">
    <header class="columns page-header">
      <div class="column">
        <h1 class="title is-4 mb-2">
          <router-link :to="{ name: 'brands' }">{{ $t('brands.title') }}</router-link>
          / {{ $t('journeyPreview.title') }}
          <span v-if="sections.length === 1"> / {{ sections[0].label }}</span>
        </h1>
      </div>
      <div v-if="sections.length" class="column is-narrow">
        <b-field :label="$t('journeyPreview.status')" label-position="on-border">
          <b-select v-model="status" data-cy="journey-status">
            <option v-for="f in statusFilters" :key="f.value" :value="f.value">{{ $t(f.label) }}</option>
          </b-select>
        </b-field>
      </div>
    </header>

    <b-loading v-if="loading" active :is-full-page="false" />

    <p v-if="!loading && !sections.length" class="has-text-grey" data-cy="journey-empty">{{ $t('journeyPreview.empty') }}</p>

    <!-- Every brand of the global brand selection (and the navbar launch filter), stacked; the
         Usage glyph links to a brand's section by its anchor. -->
    <section v-for="b in sections" :id="`journey-brand-${b.slug}`" :key="b.slug" class="jp-brand"
      :data-cy="`journey-brand-${b.slug}`">
    <h2 v-if="sections.length > 1" class="title is-5 jp-brand-title">{{ b.label }}</h2>
    <div class="jp-columns" :class="{ 'jp-split': split }" data-cy="journey-columns">
      <div v-for="g in b.columns" :key="g.family" :class="['jp-series', `jp-${g.family}`]"
        :data-cy="`journey-${g.family}`">
        <div class="bu-guide-head">
          <h4 class="bu-guide-title">{{ $t(g.title) }}</h4>
          <p class="bu-guide-subtitle">{{ $t(g.subtitle) }}</p>
        </div>
        <ol class="jp-steps">
          <li v-for="s in g.steps" :key="s.days" class="jp-step" :data-cy="`journey-${g.family}-${s.days}`">
            <div class="jp-step-head">
              <span class="bu-guide-day" :aria-label="`${$t('dashboard.usage.guideDay')} ${s.label}`" :title="s.hover || null">
                <small>{{ $t('dashboard.usage.guideDay') }}</small>{{ s.label }}
              </span>
              <p class="bu-guide-step-title">{{ s.primary }}<span v-if="s.secondary" class="bu-guide-step-sub">{{ s.secondary }}</span></p>
            </div>
            <template v-if="s.preview && s.preview.campaign">
              <p class="jp-subject">
                <strong>{{ s.preview.campaign.subject }}</strong>
                <template v-if="preheaderOf(s.preview.campaign)"> — {{ preheaderOf(s.preview.campaign) }}</template>
              </p>
              <!-- Integrations JOURNEY-DRAFTS-SPEC D9: the copy's suggested visual direction. -->
              <p v-if="visualOf(s.preview.campaign)" class="jp-meta" data-cy="journey-visual">
                {{ $t('journeyPreview.visual', { visual: visualOf(s.preview.campaign) }) }}
              </p>
              <p class="jp-meta">
                <router-link :to="{ name: 'campaign', params: { id: s.preview.campaign.id } }"
                  :title="$t('journeyPreview.open')">
                  {{ s.preview.campaign.name }}
                </router-link>
              </p>
              <!-- USAGE-PREVIEW-LANG-SPEC D4: under All, the step's other languages. -->
              <p v-if="s.preview.otherLangs" class="jp-meta" data-cy="journey-other-langs">
                {{ $t('journeyPreview.otherLangs', { n: s.preview.otherLangs }) }}
              </p>
              <p v-if="s.preview.others" class="jp-meta has-text-warning-dark">
                {{ $t('journeyPreview.others', { n: s.preview.others }) }}
              </p>
              <div class="jp-render-box">
                <div class="jp-render" :style="renderStyle">
                  <iframe :src="previewSrc(s.preview.campaign.id)" :title="s.preview.campaign.name"
                    sandbox="allow-scripts" loading="lazy" :width="frameWidth" :height="frameHeight"
                    :style="frameStyle" />
                </div>
              </div>
            </template>
            <!-- No campaign: the guide's own description, so the page still reads as the guide.
                 D4: under a language with entries but none covering it, "None in this language"
                 above it -- never an English fallback. -->
            <div v-else class="jp-placeholder" :title="$t('journeyPreview.noCampaign')">
              <p v-if="s.preview && s.preview.noneInLang" class="jp-meta has-text-weight-semibold"
                data-cy="journey-none-in-lang">
                {{ $t('journeyPreview.noneInLang') }}
              </p>
              <p v-if="s.quote" class="bu-guide-quote">“{{ s.quote }}”</p>
              <p class="bu-guide-body">{{ s.body }}</p>
            </div>
          </li>
        </ol>
      </div>
    </div>
    </section>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { brandLabelOf, filterBrandRows, filterRowsByLaunch } from '../brandScope.mjs'; // eslint-disable-line import/extensions
import { guideColumnsFit } from '../brandUsage.mjs'; // eslint-disable-line import/extensions
import {
  FRAME_HEIGHT, FRAME_WIDTH, STATUS_FILTERS, frameScale, journeyColumns, previewIds, usageBrands,
} from '../journeyPreview.mjs'; // eslint-disable-line import/extensions
import { uris } from '../constants';

export default Vue.extend({
  data() {
    return {
      loading: false,
      // GET /api/dashboard/brand-matrix rows (the user's permitted brands) and every brand's
      // counted campaigns by id (read once, so a brand-selector change needs no re-read).
      rows: [],
      campaignsById: new Map(),
      // USAGE-PREVIEW-LANG D3: the status dropdown -- local state, default All, not in the query.
      status: 'all',
      statusFilters: STATUS_FILTERS,
      // The three series side by side (guideColumnsFit) or stacked; the measured column width.
      split: true,
      columnWidth: 0,
      frameWidth: FRAME_WIDTH,
      frameHeight: FRAME_HEIGHT,
    };
  },

  computed: {
    ...mapGetters(['langEffective', 'brandSelection', 'launch', 'brandHealth']),

    // The brands shown: those with a usage doc, narrowed by the global brand selection and the
    // launch filter exactly as the Brands page's tables are; sorted by label.
    sections() {
      const labels = this.$store.state.brandRows;
      const withUsage = new Set(usageBrands(this.rows));
      const scoped = filterBrandRows(this.rows.filter((r) => r && withUsage.has(r.brand)), this.brandSelection);
      return filterRowsByLaunch(scoped, this.launch, (r) => r.brand, this.brandHealth)
        .map((r) => ({ slug: r.brand, label: brandLabelOf(r.brand, labels), usage: r.usage }))
        .sort((x, y) => x.label.localeCompare(y.label, 'en', { sensitivity: 'base' }) || x.slug.localeCompare(y.slug))
        .map((x) => ({
          ...x,
          columns: journeyColumns(x.usage, this.campaignsById, { lang: this.langEffective, status: this.status }, (k, p) => this.$t(k, p)),
        }));
    },

    scale() {
      return frameScale(this.columnWidth);
    },

    // The wrapper takes the scaled box, so the layout follows the visible size.
    renderStyle() {
      return { width: `${FRAME_WIDTH * this.scale}px`, height: `${FRAME_HEIGHT * this.scale}px` };
    },

    frameStyle() {
      return { transform: `scale(${this.scale})`, transformOrigin: 'top left' };
    },
  },

  methods: {
    preheaderOf(c) {
      return (c && c.attribs && typeof c.attribs.preheader === 'string') ? c.attribs.preheader : '';
    },

    visualOf(c) {
      const v = c && c.attribs && c.attribs.visual_direction;
      return (typeof v === 'string' && v.trim()) ? v.trim() : '';
    },

    previewSrc(id) {
      return uris.previewCampaign.replace(':id', id);
    },

    // D3: the usage docs, then every brand's counted campaigns in one read (subject, attribs,
    // tags, updated_at, name). An id the read does not return is dropped by stepCell.
    fetchAll() {
      this.loading = true;
      this.$api.getDashboardBrandMatrix({}).then((data) => {
        this.rows = data && Array.isArray(data.rows) ? data.rows : [];
        const ids = [...new Set(this.rows.flatMap((r) => (r && r.usage ? previewIds(r.usage) : [])))];
        if (!ids.length) {
          return null;
        }
        return this.$api.getCampaigns({ id: ids, per_page: 'all', no_body: true }).then((camps) => {
          const results = camps && Array.isArray(camps.results) ? camps.results : [];
          this.campaignsById = new Map(results.filter((c) => c && c.id != null).map((c) => [c.id, c]));
        });
      }).finally(() => {
        this.loading = false;
        this.queueMeasure();
        // The Usage glyph's anchor: the section exists only now.
        this.$nextTick(() => {
          const el = this.$route.hash && document.getElementById(this.$route.hash.slice(1));
          if (el) {
            el.scrollIntoView();
          }
        });
      });
    },

    // The layout from the page's width (stacked under GUIDE_MIN_COLUMN x 3), then the column's
    // content width from the first rendering box (every column has the same width).
    measure() {
      if (!this.$el || !this.$el.clientWidth) {
        return;
      }
      this.split = guideColumnsFit(this.$el.clientWidth);
      this.$nextTick(() => {
        const box = this.$el && this.$el.querySelector('.jp-render-box');
        this.columnWidth = box ? box.clientWidth : 0;
      });
    },

    queueMeasure() {
      this.$nextTick(() => this.measure());
    },
  },

  watch: {
    // A filter, language or brand change shows other campaigns: re-measure their boxes.
    sections() {
      this.queueMeasure();
    },
  },

  mounted() {
    this.fetchAll();
    if (typeof ResizeObserver !== 'undefined') {
      this.observer = new ResizeObserver(() => this.queueMeasure());
      this.observer.observe(this.$el);
    } else {
      window.addEventListener('resize', this.queueMeasure);
    }
  },

  beforeDestroy() {
    if (this.observer) {
      this.observer.disconnect();
    }
    window.removeEventListener('resize', this.queueMeasure);
  },
});
</script>
