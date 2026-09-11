<template>
  <div>
    <b-modal scroll="keep" @close="close" :aria-modal="true" :active="isVisible">
      <div>
        <div class="modal-card" style="width: auto">
          <header class="modal-card-head">
            <h4>{{ title }}</h4>
            <!-- Fork (dark-mode readiness) -- DARK-MODE-SPEC D2. -->
            <div v-if="canSimulate" class="preview-scheme">
              <b-field>
                <b-radio-button v-model="scheme" native-value="light" size="is-small" data-cy="scheme-light">
                  {{ $t('campaigns.previewSchemeLight') }}
                </b-radio-button>
                <b-radio-button v-model="scheme" native-value="partial" size="is-small" data-cy="scheme-partial">
                  {{ $t('campaigns.previewSchemePartial') }}
                </b-radio-button>
                <b-radio-button v-model="scheme" native-value="full" size="is-small" data-cy="scheme-full">
                  {{ $t('campaigns.previewSchemeFull') }}
                </b-radio-button>
              </b-field>
              <p class="is-size-7 has-text-grey">{{ $t('campaigns.previewSchemeHelp') }}</p>
            </div>
          </header>
        </div>
        <section expanded class="modal-card-body preview">
          <b-loading :active="isLoading" :is-full-page="false" />
          <!-- The form is the fallback path only: it is submitted when the credentialed
               fetch below fails, so a preview never goes blank on a fetch error. -->
          <form v-if="isPost" method="post" :action="previewURL" target="iframe" ref="form">
            <input v-if="templateId" type="hidden" name="template_id" :value="templateId" />
            <input v-if="contentType" type="hidden" name="content_type" :value="contentType" />
            <input v-if="templateType" type="hidden" name="template_type" :value="templateType" />
            <input v-if="archiveMeta" type="hidden" name="archive_meta" :value="archiveMeta" />
            <input v-if="body" type="hidden" name="body" :value="body" />
            <input v-if="attribs" type="hidden" name="attribs" :value="attribs" />
          </form>

          <iframe id="iframe" name="iframe" ref="iframe" :title="title" @load="onLoaded"
            sandbox="allow-scripts" />
        </section>
        <footer class="modal-card-foot has-text-right">
          <b-button @click="close">
            {{ $t('globals.buttons.close') }}
          </b-button>
        </footer>
      </div>
    </b-modal>
  </div>
</template>

<script>
import { uris } from '../constants';
// Fork (dark-mode readiness) -- DARK-MODE-SPEC D2. The simulation is a pure function kept
// in the email-builder source tree so its unit tests run in CI (the SPA has no test
// harness); see frontend/email-builder/src/darkSim.js. The relative path is deliberate --
// email-builder is a sibling workspace built separately, not an installed dependency of the
// SPA, so there is no package specifier to import it by.
// eslint-disable-next-line import/no-relative-packages
import { applyScheme } from '../../email-builder/src/darkSim';

const SCHEMES = ['light', 'partial', 'full'];

export default {
  name: 'CampaignPreview',

  props: {
    isPost: { type: Boolean, default: false },

    // Template or campaign ID.
    id: { type: Number, default: 0 },
    title: { type: String, default: '' },

    // campaign | template.
    type: { type: String, default: '' },

    // campaign | tx.
    templateType: { type: String, default: '' },

    archiveMeta: { type: String, default: null },

    body: { type: String, default: '' },
    // Fork (multi-language campaigns) -- JSON attribs to render with (lang, preheader).
    attribs: { type: String, default: '' },
    contentType: { type: String, default: '' },
    templateId: { type: [Number, null], default: null },
    isArchive: { type: Boolean, default: false },
  },

  data() {
    const pref = this.$utils.getPref('previewScheme');
    return {
      isVisible: true,
      isLoading: true,
      formSubmitted: false,

      // Fork (dark-mode readiness). The ORIGINAL preview HTML, fetched once and cached.
      // Every scheme (Light included) re-renders srcdoc from this string, so the toggle
      // can never drift the document, and nothing is ever written back to the server.
      originalHTML: null,
      // Set when the fetch fails and the browser's own form/src path took over: the
      // simulation has no document to work on, so the control is hidden.
      usedFallback: false,
      scheme: SCHEMES.includes(pref) ? pref : 'light',
    };
  },

  watch: {
    scheme(s) {
      this.$utils.setPref('previewScheme', s);
      this.renderScheme();
    },
  },

  methods: {
    close() {
      this.$emit('close');
      this.isVisible = false;
    },

    // On iframe load, kill the spinner. The iframe's initial about:blank load fires before
    // the fetch lands, so ignore it until there is a document to show.
    onLoaded() {
      if (this.usedFallback && this.isPost && !this.formSubmitted) {
        return;
      }
      if (!this.usedFallback && this.originalHTML === null) {
        return;
      }
      this.isLoading = false;
    },

    // Fetch the preview HTML ourselves instead of pointing the iframe at the URL. The
    // iframe is sandboxed WITHOUT allow-same-origin (upstream 74dc5a01, GHSA-jmr4-p576-v565),
    // so the parent cannot reach contentDocument to post-process it -- and granting
    // allow-same-origin would let framed script reach the admin, which is not an option.
    // Fetching and loading through srcdoc keeps the sandbox exactly as it is.
    loadPreview() {
      const opts = { credentials: 'same-origin' };
      if (this.isPost) {
        const form = new FormData();
        const fields = {
          template_id: this.templateId,
          content_type: this.contentType,
          template_type: this.templateType,
          archive_meta: this.archiveMeta,
          body: this.body,
          attribs: this.attribs,
        };
        Object.keys(fields).forEach((k) => {
          if (fields[k]) {
            form.set(k, fields[k]);
          }
        });
        opts.method = 'POST';
        opts.body = form;
      }

      fetch(this.previewURL, opts).then((resp) => {
        if (!resp.ok) {
          throw new Error(`${resp.status}`);
        }
        return resp.text();
      }).then((html) => {
        this.originalHTML = html;
        this.renderScheme();
      }).catch(() => {
        this.fallback();
      });
    },

    // The browser's own path, exactly as it worked before the toggle existed. The scheme
    // is left alone: the control is hidden, nothing renders through it, and resetting it
    // here would overwrite the user's persisted preference through the watcher.
    fallback() {
      this.usedFallback = true;
      if (this.isPost) {
        this.$refs.form.submit();
        this.formSubmitted = true;
        return;
      }
      this.$refs.iframe.src = this.previewURL;
    },

    renderScheme() {
      if (this.originalHTML === null) {
        return;
      }
      this.$refs.iframe.srcdoc = applyScheme(this.originalHTML, this.scheme);
    },
  },

  computed: {
    canSimulate() {
      return !this.usedFallback && this.originalHTML !== null;
    },

    previewURL() {
      let uri = 'about:blank';

      if (this.type === 'campaign') {
        uri = this.isArchive ? uris.previewCampaignArchive : uris.previewCampaign;
      } else if (this.type === 'template') {
        if (this.id) {
          uri = uris.previewTemplate;
        } else {
          uri = uris.previewRawTemplate;
        }
      }

      return uri.replace(':id', this.id);
    },
  },

  mounted() {
    this.loadPreview();
  },
};
</script>
