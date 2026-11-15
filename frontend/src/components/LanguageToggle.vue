<template>
  <!-- Fork (naming standard, integrations NAMING-STANDARD-SPEC D2; global language, GLOBAL-LANG-SPEC
       D1/D2; SUBSCRIBERS-FILTERS-SPEC D1/D2; SUBSCRIBERS-FILTERS-2-SPEC D1/D2). The navbar language
       toggle All | EN+ | ES | FR | DE | IT, plus Other only while some list in the brand scope has an
       unrecognised language (or Other is selected) -- langScope.mjs::toggleValues; beside the brand
       selector, on every route with the navbar (hidden only while app.lang_enable is off); Other
       filters the Subscribers page only and reads grey (All) everywhere else. A presentation filter,
       never a scope or a permission. The selected button wears the view's applicability
       (langScope.mjs::LANG_APPLICABILITY): blue applies, orange partly applies, grey not
       applicable; off All a grey line beside it names what is filtered (the "Filtered" marker), and
       the title carries the same text -- colour is never the only signal. Never disabled: on a grey
       view it still changes the global selection. -->
  <div v-if="shown" class="language-toggle" :class="`lang-app-${applicability.state}`" data-cy="language-toggle"
    role="group" :aria-label="$t('naming.langToggle')" :title="noteText || descText">
    <b-field class="segmented is-global">
      <b-radio-button v-for="v in values" :key="v" v-model="value" :native-value="v" size="is-small" :type="buttonType"
        :data-cy="`lang-${v}`">
        {{ labelOf(v) }}
      </b-radio-button>
    </b-field>
    <span v-if="noteText" class="lang-note is-size-7 has-text-grey" data-cy="lang-note">{{ noteText }}</span>
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters, mapState } from 'vuex';
import {
  SUBSCRIBER_ROUTES, langApplicability, langLabel, langNote, langNoteLabel, langToggleVisible, otherLangPresent,
  toggleValues,
} from '../langScope.mjs'; // eslint-disable-line import/extensions

// The Buefy button type per applicability state (the classes are style.scss's).
const BUTTON_TYPES = Object.freeze({ all: 'is-primary', partial: 'is-lang-partial', none: 'is-lang-none' });

export default Vue.extend({
  name: 'LanguageToggle',

  computed: {
    ...mapGetters(['langSelection', 'viewPane', 'brandSelection']),
    ...mapState(['serverConfig', 'lists']),

    // SUBSCRIBERS-FILTERS-2-SPEC D2 (I3/I4) -- Other only while present in the brand scope (the
    // store lists' has_other_lang) or selected; the one caller of toggleValues.
    values() {
      return toggleValues(otherLangPresent(this.lists, this.brandSelection), this.langSelection);
    },

    // D1 (S2-14): hidden everywhere while app.lang_enable is off.
    shown() {
      return langToggleVisible(this.serverConfig);
    },

    applicability() {
      const r = this.$route || {};
      return langApplicability(r.name, { pane: this.viewPane, selection: this.langSelection });
    },

    onSubscribers() {
      return SUBSCRIBER_ROUTES.includes((this.$route || {}).name);
    },

    buttonType() {
      return BUTTON_TYPES[this.applicability.state] || BUTTON_TYPES.all;
    },

    // D2/I3: the grey line, only off All.
    noteText() {
      const n = langNote(this.langSelection, this.applicability);
      return n ? this.translate(n.key, n.lang) : '';
    },

    // The title under All: the view's description, with the language left out.
    descText() {
      return this.translate(this.applicability.desc, this.langSelection);
    },

    // D2: the control commits selectLang (the router guard is its only other caller,
    // SUBSCRIBERS-FILTERS-SPEC D3.3).
    value: {
      get() {
        return this.langSelection;
      },
      set(v) {
        this.$store.commit('selectLang', v);
      },
    },
  },

  methods: {
    labels() {
      return { all: this.$t('naming.langAll'), other: this.$t('naming.langOther') };
    },

    // SUBSCRIBERS-FILTERS D1 (I14) -- the one label mapper, langScope.mjs::langLabel.
    labelOf(v) {
      return langLabel(v, this.labels());
    },

    // D1 -- the note's {lang}: bare EN for `en` off the Subscribers page.
    translate(key, lang) {
      return this.$t(key, { lang: langNoteLabel(lang, this.onSubscribers, this.labels()) });
    },
  },
});
</script>

<style lang="scss">
.language-toggle {
  display: inline-flex;
  align-items: center;
  // Polish 2026-11-14: Bulma's `.field:not(:last-child)` margin applied once the note followed the
  // field (a language selected) and lifted the control off the row's centre line. Pinned at 0 with
  // a selector that outranks it.
  .field,
  > .field.segmented:not(:last-child) {
    margin-bottom: 0;
  }
  .lang-note {
    margin-left: 0.5rem;
    max-width: 22rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
</style>
